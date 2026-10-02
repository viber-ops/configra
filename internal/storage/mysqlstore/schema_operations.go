package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
)

func applySchemaV4(ctx context.Context, connection *sql.Conn) error {
	_, err := connection.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS management_session_policies (
		identity_id BINARY(32) NOT NULL PRIMARY KEY,
		issuer VARCHAR(512) NOT NULL,
		target_kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		target VARCHAR(1024) NOT NULL,
		generation BIGINT UNSIGNED NOT NULL DEFAULT 0,
		blocked BOOLEAN NOT NULL DEFAULT FALSE,
		revoked_at DATETIME(6) NOT NULL,
		CONSTRAINT management_session_policies_kind CHECK (target_kind IN ('subject','sid'))
	) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`)
	if err != nil {
		return errors.New("migrate session policy schema")
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS config_release_sets (
			environment_id BINARY(16) NOT NULL,
			config_id BINARY(16) NOT NULL,
			resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			manifest JSON NOT NULL,
			digest BINARY(32) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			PRIMARY KEY(environment_id,config_id,resource_key),
			FOREIGN KEY(environment_id) REFERENCES environments(id),
			FOREIGN KEY(config_id) REFERENCES configs(id)
		) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS config_release_states (
			environment_id BINARY(16) NOT NULL,
			config_id BINARY(16) NOT NULL,
			active_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NULL,
			generation BIGINT UNSIGNED NOT NULL DEFAULT 0,
			PRIMARY KEY(environment_id,config_id),
			FOREIGN KEY(environment_id) REFERENCES environments(id),
			FOREIGN KEY(config_id) REFERENCES configs(id),
			FOREIGN KEY(environment_id,config_id,active_key) REFERENCES config_release_sets(environment_id,config_id,resource_key)
		) ENGINE=InnoDB`,
	} {
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			return errors.New("migrate release set schema")
		}
	}
	var exists int
	if connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='outbox_events' AND index_name='outbox_events_retention'`).Scan(&exists) != nil {
		return errors.New("inspect outbox retention index")
	}
	if exists == 0 {
		if _, err := connection.ExecContext(ctx, `ALTER TABLE outbox_events ADD KEY outbox_events_retention(status,completed_at,id)`); err != nil {
			return errors.New("create outbox retention index")
		}
	}
	return nil
}
