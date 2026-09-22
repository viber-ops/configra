package mysqlstore

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

type EncryptionVerification struct {
	VaultRevisions           uint64 `json:"vault_revisions"`
	CertificateAuthorities   uint64 `json:"certificate_authorities"`
	NotificationDestinations uint64 `json:"notification_destinations"`
}

// VerifyEncryptedState checks every encrypted record, including historical,
// archived and revoked records, in one read-only consistent snapshot. It neither
// repairs data nor proves that a backup includes records deleted before the dump.
func (store *Store) VerifyEncryptedState(ctx context.Context) (report EncryptionVerification, err error) {
	defer func() {
		if err != nil {
			report = EncryptionVerification{}
			if ctx.Err() != nil {
				err = ctx.Err()
			}
		}
	}()
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return report, errors.New("cannot begin read-only encrypted-state verification")
	}
	defer tx.Rollback()
	report, err = store.verifyEncryptedState(ctx, tx)
	if err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, errors.New("cannot finish read-only encrypted-state verification")
	}
	return report, nil
}

func (store *Store) verifyEncryptedState(ctx context.Context, tx *sql.Tx) (report EncryptionVerification, err error) {
	if err := verifyDatabase(ctx, tx, store.provider); err != nil {
		if errors.Is(err, vaultcrypto.ErrIntegrity) {
			return report, fmt.Errorf("crypto Sentinel verification failed: %w", vaultcrypto.ErrIntegrity)
		}
		return report, errors.New("cannot verify schema and crypto Sentinel; check database version and SELECT permissions")
	}
	// Stream each table separately: UNION or collecting payloads would buffer an
	// entire recovery point. Memory here scales with one record, not row count.
	for _, table := range []struct {
		name  string
		query string
		count *uint64
	}{
		{"vault_item_revisions", `SELECT item_id, revision, NULL, algorithm, key_version, nonce, ciphertext, encrypted_dek
			FROM vault_item_revisions ORDER BY item_id, revision`, &report.VaultRevisions},
		{"certificate_authorities", `SELECT id, 0, certificate_der, algorithm, key_version, nonce, ciphertext, encrypted_dek
			FROM certificate_authorities ORDER BY id`, &report.CertificateAuthorities},
		{"notification_destinations", `SELECT id, 0, NULL, algorithm, key_version, nonce, ciphertext, encrypted_dek
			FROM notification_destinations ORDER BY id`, &report.NotificationDestinations},
	} {
		*table.count, err = store.verifyEncryptedTable(ctx, tx, table.name, table.query)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (store *Store) verifyEncryptedTable(ctx context.Context, tx *sql.Tx, table, query string) (uint64, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("cannot read %s; check SELECT permissions and database availability", table)
	}
	defer rows.Close()
	var count uint64
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var id, der []byte
		var revision uint64
		var encrypted vaultcrypto.EncryptedSnapshot
		if err := rows.Scan(&id, &revision, &der, &encrypted.Algorithm, &encrypted.KeyVersion,
			&encrypted.Nonce, &encrypted.Ciphertext, &encrypted.EncryptedDEK); err != nil {
			return 0, fmt.Errorf("cannot decode encrypted row in %s", table)
		}
		if len(id) != 16 {
			return 0, fmt.Errorf("invalid identity in %s: %w", table, vaultcrypto.ErrIntegrity)
		}
		identity := hex.EncodeToString(id)
		switch table {
		case "vault_item_revisions":
			var plaintext []byte
			plaintext, err = store.provider.DecryptSnapshot(vaultcrypto.SnapshotIdentity{ItemID: identity, Revision: revision}, encrypted)
			if err == nil {
				_, err = vaultdoc.Decode(plaintext)
			}
			clear(plaintext)
		case "certificate_authorities":
			var private []byte
			private, err = store.provider.DecryptSecret(authorityKeyIdentity(identity, der), encrypted)
			if err == nil {
				var authority *x509.Certificate
				authority, err = x509.ParseCertificate(der)
				if err == nil {
					_, err = clientcert.ParseAuthorityKey(authority, private)
				}
			}
			clear(private)
		case "notification_destinations":
			_, err = store.decryptNotificationCredentials(id, encrypted)
		default:
			return 0, errors.New("unsupported encrypted record kind")
		}
		if err != nil {
			// Never return decoder errors, which may contain decrypted values.
			return 0, fmt.Errorf("%s id=%s revision=%d: %w", table, identity, revision, vaultcrypto.ErrIntegrity)
		}
		count++
	}
	if rows.Err() != nil {
		return 0, fmt.Errorf("incomplete read of %s during encrypted-state verification", table)
	}
	return count, nil
}
