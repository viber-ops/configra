package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type RawConfig struct {
	EnvironmentKey string `json:"environment_key"`
	ConfigKey      string `json:"config_key"`
	ConfigName     string `json:"config_name"`
	Format         string `json:"format"`
	Content        string `json:"content"`
	Revision       uint64 `json:"revision"`
}

type ConfigEnvironment struct {
	Key      string `json:"key"`
	Revision uint64 `json:"revision"`
	Archived bool   `json:"archived"`
}

type ConfigSummary struct {
	Key              string              `json:"key"`
	DisplayName      string              `json:"display_name"`
	Archived         bool                `json:"archived"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	Environments     []ConfigEnvironment `json:"environments"`
	EnvironmentCount uint64              `json:"environment_count"`
	Revision         uint64              `json:"revision,omitempty"` // Current revision when filtered by Environment.
}

type ConfigRevisionSource struct {
	EnvironmentKey string `json:"environment_key"`
	ConfigKey      string `json:"config_key"`
	Revision       uint64 `json:"revision"`
}

type ConfigRevision struct {
	Revision    uint64                `json:"revision"`
	Format      string                `json:"format"`
	OperationID string                `json:"operation_id"`
	ActorType   string                `json:"actor_type"`
	ActorID     string                `json:"actor_id"`
	Source      *ConfigRevisionSource `json:"source,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
}

type ConfigQuery struct {
	InventoryQuery
	Environment string
	Unbound     bool
}

func (store *Store) ListConfigs(ctx context.Context, query ConfigQuery) (InventoryPage[ConfigSummary], error) {
	page := InventoryPage[ConfigSummary]{Items: make([]ConfigSummary, 0)}
	if !query.valid() || (query.Environment != "" && !validResourceKey(query.Environment)) {
		return page, ErrValidation
	}
	identity := query.InventoryQuery
	identity.Search = ""
	where, arguments := identity.filter("config.resource_key", "config.archived_at")
	if query.Search != "" {
		where += ` AND (LOCATE(LOWER(?), LOWER(CONCAT_WS(' ', config.resource_key, config.display_name))) > 0 OR EXISTS (
			SELECT 1 FROM config_env_states AS state JOIN environments AS environment ON environment.id = state.environment_id
			WHERE state.config_id = config.id AND LOCATE(LOWER(?), LOWER(CONVERT(environment.resource_key USING utf8mb4))) > 0))`
		arguments = append(arguments, query.Search, query.Search)
	}
	if query.Environment != "" {
		where += ` AND EXISTS (SELECT 1 FROM config_env_states AS state JOIN environments AS environment ON environment.id = state.environment_id
			WHERE state.config_id = config.id AND environment.resource_key = ?)`
		arguments = append(arguments, query.Environment)
	}
	if query.Unbound {
		where += ` AND NOT EXISTS (SELECT 1 FROM config_env_states AS state JOIN environments AS environment ON environment.id = state.environment_id
			WHERE state.config_id = config.id AND environment.archived_at IS NULL)`
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM configs AS config WHERE `+where, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count Configs: %w", err)
	}
	// Page parent identities before their associations. The lateral preview is
	// bounded separately; counts and scoped Environment pages expose the rest.
	rows, err := store.db.QueryContext(ctx, `
		SELECT config.resource_key, config.display_name, config.archived_at IS NOT NULL,
		       config.created_at, config.updated_at,
		       (SELECT COUNT(*) FROM config_env_states AS state WHERE state.config_id = config.id),
		       COALESCE((SELECT state.current_revision FROM config_env_states AS state JOIN environments AS environment ON environment.id = state.environment_id
		           WHERE state.config_id = config.id AND environment.resource_key = ?), 0),
		       preview.resource_key, preview.current_revision, COALESCE(preview.archived, FALSE)
		FROM (SELECT config.id, config.resource_key, config.display_name, config.archived_at, config.created_at, config.updated_at
		      FROM configs AS config WHERE `+where+` ORDER BY config.resource_key LIMIT ? OFFSET ?) AS config
		LEFT JOIN LATERAL (
			SELECT environment.resource_key, state.current_revision, environment.archived_at IS NOT NULL AS archived
			FROM config_env_states AS state JOIN environments AS environment ON environment.id = state.environment_id
			WHERE state.config_id = config.id
			ORDER BY environment.archived_at IS NOT NULL, environment.resource_key LIMIT ?
		) AS preview ON TRUE
		ORDER BY config.resource_key, preview.archived, preview.resource_key
	`, append(append([]any{query.Environment}, arguments...), query.Limit, query.Offset, SummaryEnvironmentLimit)...)
	if err != nil {
		return page, fmt.Errorf("list Configs: %w", err)
	}
	defer rows.Close()
	result := make([]ConfigSummary, 0)
	for rows.Next() {
		var (
			config      ConfigSummary
			environment sql.NullString
			revision    sql.NullInt64
			envArchived bool
		)
		if err := rows.Scan(
			&config.Key, &config.DisplayName, &config.Archived, &config.CreatedAt, &config.UpdatedAt,
			&config.EnvironmentCount, &config.Revision,
			&environment, &revision, &envArchived,
		); err != nil {
			return page, fmt.Errorf("scan Config: %w", err)
		}
		if len(result) == 0 || result[len(result)-1].Key != config.Key {
			config.CreatedAt = config.CreatedAt.UTC()
			config.UpdatedAt = config.UpdatedAt.UTC()
			config.Environments = make([]ConfigEnvironment, 0)
			result = append(result, config)
		}
		if environment.Valid && revision.Valid && revision.Int64 > 0 {
			result[len(result)-1].Environments = append(result[len(result)-1].Environments, ConfigEnvironment{
				Key: environment.String, Revision: uint64(revision.Int64), Archived: envArchived,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Configs: %w", err)
	}
	page.Items = result
	return page, nil
}

func (store *Store) ListConfigRevisions(ctx context.Context, environmentKey, configKey string, query RevisionQuery) (RevisionPage[ConfigRevision], error) {
	var page RevisionPage[ConfigRevision]
	if query.Limit < 1 || query.Limit > MaxRevisionPageSize {
		return page, ErrValidation
	}
	var configID, environmentID []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT state.config_id, state.environment_id FROM config_env_states AS state
		JOIN configs AS config ON config.id = state.config_id
		JOIN environments AS environment ON environment.id = state.environment_id
		WHERE environment.resource_key = ? AND config.resource_key = ?
	`, environmentKey, configKey).Scan(&configID, &environmentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return page, ErrNotFound
		}
		return page, fmt.Errorf("read Config history identity: %w", err)
	}
	// MySQL 8.0.22 can choose an intersection with the Environment foreign-key
	// index and sort every cursor match. The primary key already covers this
	// exact resource/revision range and supports stopping at the page limit.
	statement := `
		SELECT revision.revision, revision.format, revision.operation_id,
		       revision.actor_type, revision.actor_id, revision.created_at,
		       source_environment.resource_key, source_config.resource_key, revision.source_revision
		FROM config_revisions AS revision FORCE INDEX (PRIMARY)
		LEFT JOIN configs AS source_config ON source_config.id = revision.source_config_id
		LEFT JOIN environments AS source_environment ON source_environment.id = revision.source_environment_id
		WHERE revision.config_id = ? AND revision.environment_id = ?`
	arguments := []any{configID, environmentID}
	if query.Before != 0 {
		statement += ` AND revision.revision < ?`
		arguments = append(arguments, query.Before)
	}
	statement += ` ORDER BY revision.config_id DESC, revision.environment_id DESC, revision.revision DESC LIMIT ?`
	arguments = append(arguments, query.Limit+1)
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return page, fmt.Errorf("list Config Revisions: %w", err)
	}
	defer rows.Close()
	result := make([]ConfigRevision, 0)
	for rows.Next() {
		var (
			revision          ConfigRevision
			sourceEnvironment sql.NullString
			sourceConfig      sql.NullString
			sourceRevision    sql.NullInt64
		)
		if err := rows.Scan(
			&revision.Revision, &revision.Format, &revision.OperationID,
			&revision.ActorType, &revision.ActorID, &revision.CreatedAt,
			&sourceEnvironment, &sourceConfig, &sourceRevision,
		); err != nil {
			return page, fmt.Errorf("scan Config Revision: %w", err)
		}
		revision.CreatedAt = revision.CreatedAt.UTC()
		if sourceEnvironment.Valid && sourceConfig.Valid && sourceRevision.Valid && sourceRevision.Int64 > 0 {
			revision.Source = &ConfigRevisionSource{
				EnvironmentKey: sourceEnvironment.String,
				ConfigKey:      sourceConfig.String,
				Revision:       uint64(sourceRevision.Int64),
			}
		}
		result = append(result, revision)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Config Revisions: %w", err)
	}
	if len(result) > query.Limit {
		result = result[:query.Limit]
		page.NextBefore = result[len(result)-1].Revision
	}
	page.Items = result
	return page, nil
}

func (store *Store) ReadRawConfigRevision(ctx context.Context, environmentKey, configKey string, revision uint64) (RawConfig, error) {
	var config RawConfig
	if err := store.db.QueryRowContext(ctx, `
		SELECT environment.resource_key, config.resource_key, config.display_name,
		       revision_record.format, revision_record.content, revision_record.revision
		FROM config_revisions AS revision_record
		JOIN configs AS config ON config.id = revision_record.config_id
		JOIN environments AS environment ON environment.id = revision_record.environment_id
		WHERE environment.resource_key = ? AND config.resource_key = ? AND revision_record.revision = ?
	`, environmentKey, configKey, revision).Scan(
		&config.EnvironmentKey, &config.ConfigKey, &config.ConfigName,
		&config.Format, &config.Content, &config.Revision,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RawConfig{}, ErrNotFound
		}
		return RawConfig{}, fmt.Errorf("read Raw Config Revision: %w", err)
	}
	return config, nil
}

func (store *Store) ReadRawConfig(ctx context.Context, environmentKey, configKey string) (RawConfig, error) {
	var config RawConfig
	if err := store.db.QueryRowContext(ctx, `
		SELECT e.resource_key, c.resource_key, c.display_name,
		       revision.format, revision.content, state.current_revision
		FROM config_env_states state
		JOIN configs c ON c.id = state.config_id AND c.archived_at IS NULL
		JOIN environments e ON e.id = state.environment_id AND e.archived_at IS NULL
		JOIN config_revisions revision
		  ON revision.config_id = state.config_id
		 AND revision.environment_id = state.environment_id
		 AND revision.revision = state.current_revision
		WHERE e.resource_key = ? AND c.resource_key = ?
	`, environmentKey, configKey).Scan(
		&config.EnvironmentKey, &config.ConfigKey, &config.ConfigName,
		&config.Format, &config.Content, &config.Revision,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RawConfig{}, ErrNotFound
		}
		return RawConfig{}, fmt.Errorf("read Raw Config: %w", err)
	}
	return config, nil
}
