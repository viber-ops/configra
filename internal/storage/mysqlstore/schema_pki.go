package mysqlstore

import (
	"context"
	"database/sql"
	"fmt"
)

func applySchemaV2(ctx context.Context, connection *sql.Conn) error {
	if _, err := connection.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS certificate_authorities (
		id BINARY(16) NOT NULL PRIMARY KEY,
		display_name VARCHAR(128) NOT NULL,
		certificate_der BLOB NOT NULL,
		algorithm VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		key_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		nonce VARBINARY(32) NOT NULL,
		ciphertext BLOB NOT NULL,
		encrypted_dek VARBINARY(512) NOT NULL,
		not_before DATETIME(6) NOT NULL,
		not_after DATETIME(6) NOT NULL,
		revoked_at DATETIME(6) NULL,
		created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		KEY certificate_authorities_active (revoked_at, not_after)
	) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`); err != nil {
		return fmt.Errorf("create certificate authorities: %w", err)
	}
	var exists int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'client_certificates' AND column_name = 'authority_id'`).Scan(&exists); err != nil {
		return fmt.Errorf("inspect certificate schema: %w", err)
	}
	if exists == 0 {
		if _, err := connection.ExecContext(ctx, `ALTER TABLE client_certificates
			ADD COLUMN authority_id BINARY(16) NULL,
			ADD KEY client_certificates_authority (authority_id),
			ADD CONSTRAINT client_certificates_authority_fk FOREIGN KEY (authority_id) REFERENCES certificate_authorities (id)`); err != nil {
			return fmt.Errorf("migrate client certificate schema: %w", err)
		}
	}
	return nil
}
