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
	Key          string              `json:"key"`
	DisplayName  string              `json:"display_name"`
	Archived     bool                `json:"archived"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Environments []ConfigEnvironment `json:"environments"`
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

func (store *Store) ListConfigs(ctx context.Context, includeArchived bool) ([]ConfigSummary, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT config.resource_key, config.display_name, config.archived_at IS NOT NULL,
		       config.created_at, config.updated_at,
		       environment.resource_key, state.current_revision,
		       COALESCE(environment.archived_at IS NOT NULL, FALSE)
		FROM configs AS config
		LEFT JOIN config_env_states AS state ON state.config_id = config.id
		LEFT JOIN environments AS environment ON environment.id = state.environment_id
		WHERE config.archived_at IS NULL OR ?
		ORDER BY config.resource_key, environment.resource_key
	`, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("list Configs: %w", err)
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
			&environment, &revision, &envArchived,
		); err != nil {
			return nil, fmt.Errorf("scan Config: %w", err)
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
		return nil, fmt.Errorf("iterate Configs: %w", err)
	}
	return result, nil
}

func (store *Store) ListConfigRevisions(ctx context.Context, environmentKey, configKey string) ([]ConfigRevision, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT revision.revision, revision.format, revision.operation_id,
		       revision.actor_type, revision.actor_id, revision.created_at,
		       source_environment.resource_key, source_config.resource_key, revision.source_revision
		FROM config_revisions AS revision
		JOIN configs AS config ON config.id = revision.config_id
		JOIN environments AS environment ON environment.id = revision.environment_id
		LEFT JOIN configs AS source_config ON source_config.id = revision.source_config_id
		LEFT JOIN environments AS source_environment ON source_environment.id = revision.source_environment_id
		WHERE environment.resource_key = ? AND config.resource_key = ?
		ORDER BY revision.revision DESC
	`, environmentKey, configKey)
	if err != nil {
		return nil, fmt.Errorf("list Config Revisions: %w", err)
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
			return nil, fmt.Errorf("scan Config Revision: %w", err)
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
		return nil, fmt.Errorf("iterate Config Revisions: %w", err)
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	return result, nil
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
