package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

type VaultItemSummary struct {
	NamespaceKey     string    `json:"namespace_key"`
	Key              string    `json:"key"`
	DisplayName      string    `json:"display_name"`
	Revision         uint64    `json:"revision"`
	EnvironmentKeys  []string  `json:"environment_keys"`
	EnvironmentCount uint64    `json:"environment_count"`
	Archived         bool      `json:"archived"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type VaultUsage struct {
	FieldKey       string `json:"field_key"`
	EnvironmentKey string `json:"environment_key"`
	ConfigKey      string `json:"config_key"`
	ConfigName     string `json:"config_name"`
	ConfigRevision uint64 `json:"config_revision"`
}

type VaultItem struct {
	NamespaceKey string            `json:"namespace_key"`
	Key          string            `json:"key"`
	DisplayName  string            `json:"display_name"`
	Revision     uint64            `json:"revision"`
	Archived     bool              `json:"archived"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Snapshot     vaultdoc.Snapshot `json:"snapshot"`
}

type VaultRevision struct {
	Revision             uint64    `json:"revision"`
	DisplayName          string    `json:"display_name"`
	OperationID          string    `json:"operation_id"`
	ActorType            string    `json:"actor_type"`
	ActorID              string    `json:"actor_id"`
	RestoredFromRevision *uint64   `json:"restored_from_revision,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type VaultQuery struct {
	InventoryQuery
	Namespace   string
	Environment string
}

func (store *Store) ListVaultItems(ctx context.Context, query VaultQuery) (InventoryPage[VaultItemSummary], error) {
	page := InventoryPage[VaultItemSummary]{Items: make([]VaultItemSummary, 0)}
	if !query.valid() || (query.Namespace != "" && !validResourceKey(query.Namespace)) || (query.Environment != "" && !validResourceKey(query.Environment)) {
		return page, ErrValidation
	}
	where, arguments := query.filter("item.resource_key", "item.archived_at", "CONCAT(item.namespace_key, '.', item.resource_key)", "item.display_name")
	if query.Namespace != "" {
		where += ` AND item.namespace_key = ?`
		arguments = append(arguments, query.Namespace)
	}
	if query.Environment != "" {
		where += ` AND EXISTS (SELECT 1 FROM vault_revision_variant_environments AS binding
			JOIN environments AS environment ON environment.id = binding.environment_id
			WHERE binding.item_id = item.id AND binding.revision = item.current_revision AND environment.resource_key = ?)`
		arguments = append(arguments, query.Environment)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_items AS item WHERE `+where, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count Vault Items: %w", err)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT item.namespace_key, item.resource_key, item.display_name, item.current_revision,
		       item.archived_at IS NOT NULL, item.created_at, item.updated_at,
		       (SELECT COUNT(*) FROM vault_revision_variant_environments AS binding WHERE binding.item_id = item.id AND binding.revision = item.current_revision),
		       preview.resource_key
		FROM (SELECT item.id, item.namespace_key, item.resource_key, item.display_name, item.current_revision, item.archived_at, item.created_at, item.updated_at
		      FROM vault_items AS item WHERE `+where+` ORDER BY item.namespace_key, item.resource_key LIMIT ? OFFSET ?) AS item
		LEFT JOIN LATERAL (
			SELECT environment.resource_key
			FROM vault_revision_variant_environments AS binding JOIN environments AS environment ON environment.id = binding.environment_id
			WHERE binding.item_id = item.id AND binding.revision = item.current_revision
			ORDER BY environment.resource_key LIMIT ?
		) AS preview ON TRUE
		ORDER BY item.namespace_key, item.resource_key, preview.resource_key
	`, append(arguments, query.Limit, query.Offset, SummaryEnvironmentLimit)...)
	if err != nil {
		return page, fmt.Errorf("list Vault Items: %w", err)
	}
	defer rows.Close()
	result := make([]VaultItemSummary, 0)
	for rows.Next() {
		var (
			item        VaultItemSummary
			environment sql.NullString
		)
		if err := rows.Scan(&item.NamespaceKey, &item.Key, &item.DisplayName, &item.Revision, &item.Archived, &item.CreatedAt, &item.UpdatedAt, &item.EnvironmentCount, &environment); err != nil {
			return page, fmt.Errorf("scan Vault Item: %w", err)
		}
		if len(result) == 0 || result[len(result)-1].NamespaceKey != item.NamespaceKey || result[len(result)-1].Key != item.Key {
			item.CreatedAt = item.CreatedAt.UTC()
			item.UpdatedAt = item.UpdatedAt.UTC()
			item.EnvironmentKeys = make([]string, 0)
			result = append(result, item)
		}
		if environment.Valid {
			result[len(result)-1].EnvironmentKeys = append(result[len(result)-1].EnvironmentKeys, environment.String)
		}
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Vault Items: %w", err)
	}
	page.Items = result
	return page, nil
}

type VaultUsageQuery struct {
	InventoryQuery
	// Impact checks the entire current reference set against a proposed schema,
	// independently of which usage page a browser has visited. Values are never needed.
	Impact       bool
	Fields       []string
	Environments []string
}

func (store *Store) ListVaultUsages(ctx context.Context, namespaceKey, itemKey string, query VaultUsageQuery) (InventoryPage[VaultUsage], error) {
	page := InventoryPage[VaultUsage]{Items: make([]VaultUsage, 0)}
	if !query.valid() || query.Key != "" || query.IncludeInactive || query.OnlyInactive || !validResourceKey(namespaceKey) || !validResourceKey(itemKey) ||
		(query.Impact && (query.Fields == nil || query.Environments == nil || query.Search != "")) || (!query.Impact && (query.Fields != nil || query.Environments != nil)) {
		return page, ErrValidation
	}
	for _, keys := range [][]string{query.Fields, query.Environments} {
		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			if !validResourceKey(key) || seen[key] {
				return page, ErrValidation
			}
			seen[key] = true
		}
	}
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return page, fmt.Errorf("begin Vault usage read: %w", err)
	}
	defer transaction.Rollback()
	var exists bool
	if err := transaction.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM vault_items WHERE namespace_key = ? AND resource_key = ?)
	`, namespaceKey, itemKey).Scan(&exists); err != nil {
		return page, fmt.Errorf("check Vault Item usage identity: %w", err)
	}
	if !exists {
		return page, ErrNotFound
	}
	from := `
		FROM config_revision_vault_refs AS reference
		JOIN config_env_states AS state
		  ON state.config_id = reference.config_id
		 AND state.environment_id = reference.environment_id
		 AND state.current_revision = reference.revision
		JOIN configs AS config ON config.id = reference.config_id AND config.archived_at IS NULL
		JOIN environments AS environment ON environment.id = reference.environment_id AND environment.archived_at IS NULL
		WHERE reference.namespace_key = ? AND reference.item_key = ?`
	arguments := []any{namespaceKey, itemKey}
	if query.Search != "" {
		from += ` AND LOCATE(LOWER(?), LOWER(CONCAT_WS(' ', reference.field_key, environment.resource_key, config.resource_key, config.display_name))) > 0`
		arguments = append(arguments, query.Search)
	}
	if query.Impact {
		fields, _ := json.Marshal(query.Fields)
		environments, _ := json.Marshal(query.Environments)
		// JSON_TABLE binds two documents, avoiding an unbounded IN placeholder list.
		from += ` AND (NOT EXISTS (SELECT 1 FROM JSON_TABLE(?, '$[*]' COLUMNS(resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin PATH '$')) AS kept WHERE kept.resource_key = reference.field_key)
			OR NOT EXISTS (SELECT 1 FROM JSON_TABLE(?, '$[*]' COLUMNS(resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin PATH '$')) AS kept WHERE kept.resource_key = environment.resource_key))`
		arguments = append(arguments, string(fields), string(environments))
	}
	if err := transaction.QueryRowContext(ctx, `SELECT COUNT(*)`+from, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count Vault usages: %w", err)
	}
	rows, err := transaction.QueryContext(ctx, `SELECT reference.field_key, environment.resource_key, config.resource_key, config.display_name, state.current_revision`+from+`
		ORDER BY reference.field_key, environment.resource_key, config.resource_key LIMIT ? OFFSET ?
	`, append(arguments, query.Limit, query.Offset)...)
	if err != nil {
		return page, fmt.Errorf("list Vault usages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var usage VaultUsage
		if err := rows.Scan(&usage.FieldKey, &usage.EnvironmentKey, &usage.ConfigKey, &usage.ConfigName, &usage.ConfigRevision); err != nil {
			return page, fmt.Errorf("scan Vault usage: %w", err)
		}
		page.Items = append(page.Items, usage)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Vault usages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return page, err
	}
	return page, transaction.Commit()
}

func (store *Store) ListVaultRevisions(ctx context.Context, namespaceKey, itemKey string, query RevisionQuery) (RevisionPage[VaultRevision], error) {
	var page RevisionPage[VaultRevision]
	if query.Limit < 1 || query.Limit > MaxRevisionPageSize {
		return page, ErrValidation
	}
	var itemID []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT id FROM vault_items WHERE namespace_key = ? AND resource_key = ?
	`, namespaceKey, itemKey).Scan(&itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return page, ErrNotFound
		}
		return page, fmt.Errorf("read Vault history identity: %w", err)
	}
	statement := `
		SELECT revision.revision, revision.item_display_name, revision.operation_id,
		       revision.actor_type, revision.actor_id, revision.restored_from_revision, revision.created_at
		FROM vault_item_revisions AS revision
		WHERE revision.item_id = ?`
	arguments := []any{itemID}
	if query.Before != 0 {
		statement += ` AND revision.revision < ?`
		arguments = append(arguments, query.Before)
	}
	statement += ` ORDER BY revision.item_id DESC, revision.revision DESC LIMIT ?`
	arguments = append(arguments, query.Limit+1)
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return page, fmt.Errorf("list Vault Revisions: %w", err)
	}
	defer rows.Close()
	result := make([]VaultRevision, 0)
	for rows.Next() {
		var (
			revision VaultRevision
			restored sql.NullInt64
		)
		if err := rows.Scan(
			&revision.Revision, &revision.DisplayName, &revision.OperationID,
			&revision.ActorType, &revision.ActorID, &restored, &revision.CreatedAt,
		); err != nil {
			return page, fmt.Errorf("scan Vault Revision: %w", err)
		}
		revision.CreatedAt = revision.CreatedAt.UTC()
		if restored.Valid && restored.Int64 > 0 {
			value := uint64(restored.Int64)
			revision.RestoredFromRevision = &value
		}
		result = append(result, revision)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Vault Revisions: %w", err)
	}
	if len(result) > query.Limit {
		result = result[:query.Limit]
		page.NextBefore = result[len(result)-1].Revision
	}
	page.Items = result
	return page, nil
}

func (store *Store) ReadVaultItem(ctx context.Context, namespaceKey, itemKey string, revision uint64, includeValues bool) (VaultItem, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return VaultItem{}, fmt.Errorf("begin Vault Item snapshot: %w", err)
	}
	defer transaction.Rollback()
	var (
		itemID          []byte
		currentRevision uint64
		item            VaultItem
	)
	if err := transaction.QueryRowContext(ctx, `
			SELECT id, namespace_key, resource_key, current_revision, archived_at IS NOT NULL, created_at, updated_at
			FROM vault_items WHERE namespace_key = ? AND resource_key = ?
		`, namespaceKey, itemKey).Scan(&itemID, &item.NamespaceKey, &item.Key, &currentRevision, &item.Archived, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VaultItem{}, ErrNotFound
		}
		return VaultItem{}, fmt.Errorf("read Vault Item: %w", err)
	}
	if item.Archived && includeValues {
		return VaultItem{}, ErrNotFound
	}
	if revision == 0 {
		revision = currentRevision
	}
	item.Revision = revision
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	if err := transaction.QueryRowContext(ctx, `
		SELECT item_display_name FROM vault_item_revisions WHERE item_id = ? AND revision = ?
	`, itemID, revision).Scan(&item.DisplayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VaultItem{}, ErrNotFound
		}
		return VaultItem{}, fmt.Errorf("read Vault Revision metadata: %w", err)
	}

	fieldRows, err := transaction.QueryContext(ctx, `
		SELECT resource_key, display_name, field_type, archived
		FROM vault_revision_fields
		WHERE item_id = ? AND revision = ?
		ORDER BY resource_key
	`, itemID, revision)
	if err != nil {
		return VaultItem{}, fmt.Errorf("read Vault Revision Fields: %w", err)
	}
	for fieldRows.Next() {
		var field vaultdoc.Field
		var archived bool
		if err := fieldRows.Scan(&field.Key, &field.Name, &field.Type, &archived); err != nil {
			fieldRows.Close()
			return VaultItem{}, fmt.Errorf("scan Vault Revision Field: %w", err)
		}
		if !archived {
			item.Snapshot.Fields = append(item.Snapshot.Fields, field)
		}
	}
	if err := fieldRows.Err(); err != nil {
		fieldRows.Close()
		return VaultItem{}, fmt.Errorf("iterate Vault Revision Fields: %w", err)
	}
	if err := fieldRows.Close(); err != nil {
		return VaultItem{}, fmt.Errorf("close Vault Revision Fields: %w", err)
	}
	if len(item.Snapshot.Fields) == 0 {
		return VaultItem{}, errors.New("Vault Revision Field metadata failed integrity validation")
	}

	variantRows, err := transaction.QueryContext(ctx, `
		SELECT HEX(variant.variant_id), environment.resource_key
		FROM vault_revision_variants AS variant
		LEFT JOIN vault_revision_variant_environments AS binding
		  ON binding.item_id = variant.item_id AND binding.revision = variant.revision AND binding.variant_id = variant.variant_id
		LEFT JOIN environments AS environment ON environment.id = binding.environment_id
		WHERE variant.item_id = ? AND variant.revision = ?
		ORDER BY variant.ordinal, environment.resource_key
	`, itemID, revision)
	if err != nil {
		return VaultItem{}, fmt.Errorf("read Vault Revision Variants: %w", err)
	}
	for variantRows.Next() {
		var id string
		var environment sql.NullString
		if err := variantRows.Scan(&id, &environment); err != nil {
			variantRows.Close()
			return VaultItem{}, fmt.Errorf("scan Vault Revision Variant: %w", err)
		}
		id = strings.ToLower(id)
		if len(item.Snapshot.Variants) == 0 || item.Snapshot.Variants[len(item.Snapshot.Variants)-1].ID != id {
			item.Snapshot.Variants = append(item.Snapshot.Variants, vaultdoc.Variant{ID: id})
		}
		if environment.Valid {
			index := len(item.Snapshot.Variants) - 1
			item.Snapshot.Variants[index].Environments = append(item.Snapshot.Variants[index].Environments, environment.String)
		}
	}
	if err := variantRows.Err(); err != nil {
		variantRows.Close()
		return VaultItem{}, fmt.Errorf("iterate Vault Revision Variants: %w", err)
	}
	if err := variantRows.Close(); err != nil {
		return VaultItem{}, fmt.Errorf("close Vault Revision Variants: %w", err)
	}
	if len(item.Snapshot.Variants) == 0 {
		return VaultItem{}, errors.New("Vault Revision Variant metadata failed integrity validation")
	}

	if includeValues {
		plaintext, _, err := store.currentVaultSnapshot(ctx, transaction, itemID, revision)
		if err != nil {
			return VaultItem{}, err
		}
		payload, err := vaultdoc.Decode(plaintext)
		clear(plaintext)
		if err != nil || len(payload.Variants) != len(item.Snapshot.Variants) {
			return VaultItem{}, fmt.Errorf("read Vault values: %w", vaultcrypto.ErrIntegrity)
		}
		for index := range item.Snapshot.Variants {
			values, ok := payload.Variants[item.Snapshot.Variants[index].ID]
			if !ok {
				return VaultItem{}, fmt.Errorf("read Vault values: %w", vaultcrypto.ErrIntegrity)
			}
			item.Snapshot.Variants[index].Values = values.Values
		}
	}
	if err := transaction.Commit(); err != nil {
		return VaultItem{}, fmt.Errorf("commit Vault Item snapshot: %w", err)
	}
	return item, nil
}
