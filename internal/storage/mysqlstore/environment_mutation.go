package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type EnvironmentAction string

const (
	EnvironmentCreate    EnvironmentAction = "create"
	EnvironmentRename    EnvironmentAction = "rename"
	EnvironmentArchive   EnvironmentAction = "archive"
	EnvironmentUnarchive EnvironmentAction = "unarchive"
)

type EnvironmentChange struct {
	OperationID string
	Actor       Actor
	Action      EnvironmentAction
	Key         string
	DisplayName string
}

type EnvironmentChangeResult struct {
	Outcome     Outcome `json:"outcome"`
	Key         string  `json:"key"`
	DisplayName string  `json:"display_name"`
	Archived    bool    `json:"archived"`
}

type Environment struct {
	Key         string    `json:"key"`
	DisplayName string    `json:"display_name"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Revision    uint64    `json:"revision,omitempty"`
	Granted     bool      `json:"granted,omitempty"`
}

type EnvironmentQuery struct {
	InventoryQuery
	Config        string
	Namespace     string
	Item          string
	TokenPublicID string
}

func (store *Store) ListEnvironments(ctx context.Context, query EnvironmentQuery) (InventoryPage[Environment], error) {
	page := InventoryPage[Environment]{Items: make([]Environment, 0)}
	if !query.valid() || (query.Config != "" && !validResourceKey(query.Config)) ||
		(query.Namespace != "" && !validResourceKey(query.Namespace)) || (query.Item != "" && !validResourceKey(query.Item)) ||
		(query.Namespace == "") != (query.Item == "") || (query.TokenPublicID != "" && !validTokenPublicID(query.TokenPublicID)) {
		return page, ErrValidation
	}
	where, arguments := query.filter("environment.resource_key", "environment.archived_at", "environment.resource_key", "environment.display_name")
	if query.Config != "" {
		where += ` AND EXISTS (SELECT 1 FROM config_env_states AS state JOIN configs AS config ON config.id = state.config_id
			WHERE state.environment_id = environment.id AND config.resource_key = ?)`
		arguments = append(arguments, query.Config)
	}
	if query.Item != "" {
		where += ` AND EXISTS (SELECT 1 FROM vault_revision_variant_environments AS binding
			JOIN vault_items AS item ON item.id = binding.item_id AND item.current_revision = binding.revision
			WHERE binding.environment_id = environment.id AND item.namespace_key = ? AND item.resource_key = ?)`
		arguments = append(arguments, query.Namespace, query.Item)
	}
	var tokenID []byte
	if query.TokenPublicID != "" {
		if err := store.db.QueryRowContext(ctx, `SELECT id FROM api_tokens WHERE public_id = ?`, query.TokenPublicID).Scan(&tokenID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return page, ErrNotFound
			}
			return page, fmt.Errorf("read API Token grant identity: %w", err)
		}
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM environments AS environment WHERE `+where, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count Environments: %w", err)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT environment.resource_key, environment.display_name, environment.archived_at IS NOT NULL, environment.created_at, environment.updated_at,
		       COALESCE((SELECT state.current_revision FROM config_env_states AS state JOIN configs AS config ON config.id = state.config_id
		          WHERE state.environment_id = environment.id AND config.resource_key = ?), 0),
		       EXISTS (SELECT 1 FROM api_token_environments AS grant_record WHERE grant_record.environment_id = environment.id AND grant_record.token_id = ?)
		FROM environments AS environment WHERE `+where+` ORDER BY environment.resource_key LIMIT ? OFFSET ?`,
		append(append([]any{query.Config, tokenID}, arguments...), query.Limit, query.Offset)...)
	if err != nil {
		return page, fmt.Errorf("list Environments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var environment Environment
		if err := rows.Scan(&environment.Key, &environment.DisplayName, &environment.Archived, &environment.CreatedAt, &environment.UpdatedAt, &environment.Revision, &environment.Granted); err != nil {
			return page, fmt.Errorf("scan Environment: %w", err)
		}
		environment.CreatedAt = environment.CreatedAt.UTC()
		environment.UpdatedAt = environment.UpdatedAt.UTC()
		page.Items = append(page.Items, environment)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Environments: %w", err)
	}
	return page, nil
}

func (store *Store) ApplyEnvironmentChange(ctx context.Context, request EnvironmentChange) (EnvironmentChangeResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return EnvironmentChangeResult{}, ErrValidation
	}
	digest := environmentChangeDigest(request)
	validationErr := validateEnvironmentChange(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentChangeResult{}, fmt.Errorf("begin Environment change: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return EnvironmentChangeResult{}, err
	}
	if replayed {
		var previous EnvironmentChangeResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return EnvironmentChangeResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishEnvironmentFailure(ctx, transaction, request, OutcomeValidationFailed, "environment.validation_failed", validationErr)
	}

	var id []byte
	var displayName string
	var archivedAt sql.NullTime
	err = transaction.QueryRowContext(ctx, `
		SELECT id, display_name, archived_at FROM environments WHERE resource_key = ? FOR UPDATE
	`, request.Key).Scan(&id, &displayName, &archivedAt)
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EnvironmentChangeResult{}, fmt.Errorf("lock Environment: %w", err)
	}

	var result EnvironmentChangeResult
	eventType := "environment." + string(request.Action)
	switch request.Action {
	case EnvironmentCreate:
		if found {
			return finishEnvironmentFailure(ctx, transaction, request, OutcomeConflict, "environment.conflict", ErrConflict)
		}
		id, err = randomID()
		if err != nil {
			return EnvironmentChangeResult{}, err
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO environments (id, resource_key, display_name) VALUES (?, ?, ?)
		`, id, request.Key, request.DisplayName); err != nil {
			return EnvironmentChangeResult{}, fmt.Errorf("create Environment: %w", err)
		}
		result = EnvironmentChangeResult{Outcome: OutcomeSuccess, Key: request.Key, DisplayName: request.DisplayName}
	case EnvironmentRename:
		if !found || archivedAt.Valid {
			return finishEnvironmentFailure(ctx, transaction, request, OutcomeValidationFailed, "environment.not_active", ErrValidation)
		}
		if displayName == request.DisplayName {
			result = EnvironmentChangeResult{Outcome: OutcomeNoChange, Key: request.Key, DisplayName: displayName}
			break
		}
		if _, err := transaction.ExecContext(ctx, `UPDATE environments SET display_name = ? WHERE id = ?`, request.DisplayName, id); err != nil {
			return EnvironmentChangeResult{}, fmt.Errorf("rename Environment: %w", err)
		}
		result = EnvironmentChangeResult{Outcome: OutcomeSuccess, Key: request.Key, DisplayName: request.DisplayName}
	case EnvironmentArchive:
		if !found {
			return finishEnvironmentFailure(ctx, transaction, request, OutcomeValidationFailed, "environment.not_found", ErrValidation)
		}
		result = EnvironmentChangeResult{Outcome: OutcomeNoChange, Key: request.Key, DisplayName: displayName, Archived: true}
		if !archivedAt.Valid {
			if _, err := transaction.ExecContext(ctx, `UPDATE environments SET archived_at = UTC_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
				return EnvironmentChangeResult{}, fmt.Errorf("Archive Environment: %w", err)
			}
			result.Outcome = OutcomeSuccess
		}
	case EnvironmentUnarchive:
		if !found {
			return finishEnvironmentFailure(ctx, transaction, request, OutcomeValidationFailed, "environment.not_found", ErrValidation)
		}
		result = EnvironmentChangeResult{Outcome: OutcomeNoChange, Key: request.Key, DisplayName: displayName}
		if archivedAt.Valid {
			if _, err := transaction.ExecContext(ctx, `UPDATE environments SET archived_at = NULL WHERE id = ?`, id); err != nil {
				return EnvironmentChangeResult{}, fmt.Errorf("Unarchive Environment: %w", err)
			}
			result.Outcome = OutcomeSuccess
		}
	}
	notify := result.Outcome == OutcomeSuccess
	if err := finishEnvironmentOperation(ctx, transaction, request, result, eventType, notify); err != nil {
		return EnvironmentChangeResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return EnvironmentChangeResult{}, fmt.Errorf("commit Environment change: %w", err)
	}
	return result, nil
}

func validateEnvironmentChange(request EnvironmentChange) error {
	if !validResourceKey(request.Key) {
		return errors.New("invalid Environment Resource Key")
	}
	switch request.Action {
	case EnvironmentCreate, EnvironmentRename:
		if request.DisplayName == "" || len(request.DisplayName) > 255 {
			return errors.New("Environment display name is required and must not exceed 255 bytes")
		}
	case EnvironmentArchive, EnvironmentUnarchive:
	default:
		return errors.New("invalid Environment action")
	}
	return nil
}

func environmentChangeDigest(request EnvironmentChange) [sha256.Size]byte {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func finishEnvironmentFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request EnvironmentChange,
	outcome Outcome,
	eventType string,
	cause error,
) (EnvironmentChangeResult, error) {
	result := EnvironmentChangeResult{Outcome: outcome, Key: request.Key}
	if err := finishEnvironmentOperation(ctx, transaction, request, result, eventType, false); err != nil {
		return EnvironmentChangeResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return EnvironmentChangeResult{}, fmt.Errorf("commit Environment failure Audit: %w", err)
	}
	if outcome == OutcomeConflict {
		return result, withCommittedAudit(ErrConflict)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}

func finishEnvironmentOperation(
	ctx context.Context,
	transaction *sql.Tx,
	request EnvironmentChange,
	result EnvironmentChangeResult,
	eventType string,
	notify bool,
) error {
	return finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type:           eventType,
		Action:         "environment." + string(request.Action),
		EnvironmentKey: request.Key,
		ResourceType:   "environment",
		ResourceKey:    request.Key,
	}, notify)
}
