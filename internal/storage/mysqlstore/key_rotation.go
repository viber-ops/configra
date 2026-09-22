package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

var ErrRotationOutcomeUnknown = errors.New("key rotation commit outcome is unknown; keep all replicas stopped and run doctor with each key before deciding which key to use")

type KeyRotationResult struct {
	OperationID string `json:"operation_id"`
	EncryptionVerification
}

// RotateMasterKey is an offline, atomic change of the key wrapping all stored
// DEKs. The Store retains its old provider; callers must reopen with the new key.
func (store *Store) RotateMasterKey(ctx context.Context, next *vaultcrypto.LocalKeyProvider) (result KeyRotationResult, err error) {
	defer func() {
		if err != nil {
			result = KeyRotationResult{}
			if ctx.Err() != nil && !errors.Is(err, ErrRotationOutcomeUnknown) {
				err = ctx.Err()
			}
		}
	}()
	if next == nil {
		return result, errors.New("new Master Key is required")
	}
	// ponytail: one maintenance transaction; use a resumable keyring migration
	// only if measured recovery points no longer fit the maintenance window.
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return result, errors.New("cannot begin offline key rotation")
	}
	defer tx.Rollback()
	if err := store.lockMasterKey(ctx, tx, true); err != nil {
		if errors.Is(err, vaultcrypto.ErrIntegrity) {
			return result, vaultcrypto.ErrIntegrity
		}
		return result, errors.New("cannot lock crypto Sentinel; check permissions, running writers and database availability")
	}
	if verifySentinel(ctx, tx, next) == nil {
		return result, errors.New("new Master Key must differ from the current key")
	}
	var transactionalTables int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND engine = 'InnoDB' AND table_name IN
		('crypto_sentinel', 'vault_item_revisions', 'certificate_authorities', 'notification_destinations', 'operations', 'outbox_events')`).Scan(&transactionalTables); err != nil || transactionalTables != 6 {
		return result, errors.New("rotation requires all six storage/audit tables to be visible InnoDB tables; nothing was changed")
	}
	verified, err := store.verifyEncryptedState(ctx, tx)
	if err != nil {
		return result, err
	}
	for _, table := range []struct {
		name, id string
		count    *uint64
	}{
		{"vault_item_revisions", "item_id", &result.VaultRevisions},
		{"certificate_authorities", "id", &result.CertificateAuthorities},
		{"notification_destinations", "id", &result.NotificationDestinations},
	} {
		*table.count, err = store.rewrapTable(ctx, tx, table.name, table.id, next)
		if err != nil {
			return result, err
		}
	}
	if result.EncryptionVerification != verified {
		return result, errors.New("encrypted record counts changed during offline rotation; no rotation committed")
	}
	sentinel, err := next.CreateSentinel()
	if err != nil {
		return result, errors.New("cannot create rotated crypto Sentinel")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE crypto_sentinel SET algorithm = ?, key_version = ?, nonce = ?, ciphertext = ? WHERE id = 1`,
		sentinel.Algorithm, sentinel.KeyVersion, sentinel.Nonce, sentinel.Ciphertext); err != nil {
		return result, errors.New("cannot update crypto Sentinel; no rotation committed")
	}
	id, err := randomID()
	if err != nil {
		return result, errors.New("cannot identify key rotation")
	}
	result.OperationID = "key-rotation-" + hex.EncodeToString(id)
	actor := Actor{Type: "system", ID: "offline-master-key-rotation"}
	replayed, _, err := beginOperation(ctx, tx, result.OperationID, tokenRequestDigest(result), actor)
	if err != nil || replayed {
		return result, errors.New("cannot begin key rotation audit; no rotation committed")
	}
	if err := finishOperation(ctx, tx, result.OperationID, actor, OutcomeSuccess, 0, result, mutationEvent{
		Type: "master_key.rotated", Action: "master_key.rotate", ResourceType: "deployment", ResourceKey: "master-key",
	}, false); err != nil {
		return result, errors.New("cannot record key rotation audit; no rotation committed")
	}
	if err := tx.Commit(); err != nil {
		return result, ErrRotationOutcomeUnknown
	}
	return result, nil
}

func (store *Store) rewrapTable(ctx context.Context, tx *sql.Tx, table, idColumn string, next *vaultcrypto.LocalKeyProvider) (uint64, error) {
	revisionColumn, derColumn := "0", "NULL"
	order := idColumn
	if table == "vault_item_revisions" {
		revisionColumn, order = "revision", "item_id, revision"
	} else if table == "certificate_authorities" {
		derColumn = "certificate_der"
	}
	query := "SELECT " + idColumn + ", " + revisionColumn + ", " + derColumn + ", algorithm, key_version, nonce, ciphertext, encrypted_dek FROM " + table
	where := ""
	var arguments []any
	var count uint64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var id, der []byte
		var revision uint64
		var encrypted vaultcrypto.EncryptedSnapshot
		err := tx.QueryRowContext(ctx, query+where+" ORDER BY "+order+" LIMIT 1 FOR UPDATE", arguments...).Scan(
			&id, &revision, &der, &encrypted.Algorithm, &encrypted.KeyVersion, &encrypted.Nonce, &encrypted.Ciphertext, &encrypted.EncryptedDEK)
		if errors.Is(err, sql.ErrNoRows) {
			return count, nil
		}
		if err != nil || len(id) != 16 {
			return 0, fmt.Errorf("cannot read %s during key rotation; no rotation committed", table)
		}
		identity := hex.EncodeToString(id)
		var wrapped []byte
		switch table {
		case "vault_item_revisions":
			wrapped, err = store.provider.RewrapSnapshot(vaultcrypto.SnapshotIdentity{ItemID: identity, Revision: revision}, encrypted, next)
		case "certificate_authorities":
			wrapped, err = store.provider.RewrapSecret(authorityKeyIdentity(identity, der), encrypted, next)
		case "notification_destinations":
			wrapped, err = store.provider.RewrapSecret(notificationCredentialIdentity(id), encrypted, next)
		default:
			return 0, errors.New("unsupported encrypted record kind")
		}
		if err != nil {
			return 0, fmt.Errorf("%s id=%s revision=%d: %w", table, identity, revision, vaultcrypto.ErrIntegrity)
		}
		update := "UPDATE " + table + " SET encrypted_dek = ?"
		if table == "notification_destinations" {
			// Rewrapping is not an edit to the notification configuration.
			update += ", updated_at = updated_at"
		}
		update += " WHERE " + idColumn + " = ?"
		values := []any{wrapped, id}
		where, arguments = " WHERE "+idColumn+" > ?", []any{id}
		if table == "vault_item_revisions" {
			update += " AND revision = ?"
			values = append(values, revision)
			where, arguments = " WHERE item_id > ? OR (item_id = ? AND revision > ?)", []any{id, id, revision}
		}
		changed, err := tx.ExecContext(ctx, update, values...)
		if err != nil {
			return 0, fmt.Errorf("cannot rewrap %s; no rotation committed", table)
		}
		if rows, err := changed.RowsAffected(); err != nil || rows != 1 {
			return 0, fmt.Errorf("unexpected rewrap count in %s; no rotation committed", table)
		}
		count++
	}
}
