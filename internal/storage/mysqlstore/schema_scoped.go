package mysqlstore

import (
	"context"
	"database/sql"
	"fmt"
)

func applySchemaV3(ctx context.Context, connection *sql.Conn) error {
	var exists int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'api_tokens' AND column_name = 'kind'`).Scan(&exists); err != nil {
		return fmt.Errorf("inspect scoped Token schema: %w", err)
	}
	if exists == 0 {
		_, err := connection.ExecContext(ctx, `ALTER TABLE api_tokens
			ADD COLUMN kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'read-only',
			ADD COLUMN config_keys JSON NULL,
			ADD COLUMN namespace_keys JSON NULL,
			ADD COLUMN parent_public_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
			ADD COLUMN deployment_certificate_fingerprint BINARY(32) NULL,
			ADD KEY api_tokens_parent (parent_public_id),
			ADD CONSTRAINT api_tokens_parent_fk FOREIGN KEY (parent_public_id) REFERENCES api_tokens (public_id),
			ADD CONSTRAINT api_tokens_kind CHECK (kind IN ('read-only', 'write-scoped')),
			ADD CONSTRAINT api_tokens_write_auth CHECK (kind <> 'write-scoped' OR (allow_without_mtls = FALSE AND expires_at IS NOT NULL))`)
		if err != nil {
			return fmt.Errorf("migrate scoped Token schema: %w", err)
		}
	}
	return nil
}
