//go:build integration

package mysqlstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestVerifyEncryptedStateIncludesInactiveHistoryAndRejectsCorruption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dsn, database, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if report, err := store.VerifyEncryptedState(ctx); err != nil || report != (EncryptionVerification{}) {
		t.Fatalf("initialized empty database: %#v, %v", report, err)
	}
	actor := Actor{Type: "user", ID: "recovery-test@example.test"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "verify-env", Actor: actor,
		Action: EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	variant := ""
	for revision := uint64(0); revision < 2; revision++ {
		result, err := store.CommitVault(ctx, VaultCommit{OperationID: fmt.Sprintf("verify-vault-%d", revision), Actor: actor,
			NamespaceKey: "ops", ItemKey: "secret", ItemName: "Secret", ExpectedRevision: revision,
			Snapshot: testVaultSnapshot(variant, []string{"prod"}, "user", fmt.Sprintf("secret-sentinel-%d", revision))})
		if err != nil {
			t.Fatal(err)
		}
		variant = result.VariantIDs[0]
	}
	if _, err := store.ApplyVaultLifecycleChange(ctx, VaultLifecycleChange{OperationID: "archive-vault", Actor: actor,
		Action: VaultArchive, NamespaceKey: "ops", Key: "secret"}); err != nil {
		t.Fatal(err)
	}
	ca, err := store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "verify-ca", Actor: actor, DisplayName: "CA", ValidDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeCertificateAuthority(ctx, AuthorityRevoke{OperationID: "revoke-ca", Actor: actor, AuthorityID: ca.Authority.ID}); err != nil {
		t.Fatal(err)
	}
	// An expired but intact CA is still recoverable data. Do not advance the host
	// clock or pretend its certificate is currently usable for issuing clients.
	expired, err := clientcert.GenerateAuthority("Expired", 1, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(expired.PrivateKeyDER)
	expiredID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := provider.EncryptSecret(authorityKeyIdentity(hex.EncodeToString(expiredID), expired.Certificate.Raw), expired.PrivateKeyDER)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO certificate_authorities
		(id, display_name, certificate_der, algorithm, key_version, nonce, ciphertext, encrypted_dek, not_before, not_after)
		VALUES (?, 'Expired', ?, ?, ?, ?, ?, ?, ?, ?)`, expiredID, expired.Certificate.Raw, encrypted.Algorithm,
		encrypted.KeyVersion, encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, expired.Certificate.NotBefore, expired.Certificate.NotAfter); err != nil {
		t.Fatal(err)
	}
	url, secret := "https://hooks.example.test/credential-sentinel", "notification-secret-sentinel"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "verify-notification", Actor: actor,
		Key: "ops", DisplayName: "Ops", Provider: NotificationGenericWebhook, URL: &url, Secret: &secret, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyNotificationDestinationLifecycle(ctx, NotificationDestinationLifecycle{OperationID: "archive-notification", Actor: actor,
		Key: "ops", Action: NotificationDestinationArchive}); err != nil {
		t.Fatal(err)
	}
	want := EncryptionVerification{VaultRevisions: 2, CertificateAuthorities: 2, NotificationDestinations: 1}
	assertGood := func() {
		t.Helper()
		if got, err := store.VerifyEncryptedState(ctx); err != nil || got != want {
			t.Fatalf("verification = %#v, %v; want %#v", got, err, want)
		}
	}
	assertBad := func(table string) {
		t.Helper()
		got, err := store.VerifyEncryptedState(ctx)
		if !errors.Is(err, vaultcrypto.ErrIntegrity) || !strings.Contains(err.Error(), table) ||
			strings.Contains(err.Error(), "secret-sentinel") || got != (EncryptionVerification{}) {
			t.Fatalf("corruption check for %s did not fail closed: %v", table, err)
		}
	}
	assertGood()
	t.Run("blocked SQL honors deadline", func(t *testing.T) {
		locker, err := database.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer locker.Close()
		if _, err := locker.ExecContext(ctx, "LOCK TABLES vault_item_revisions WRITE"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := locker.ExecContext(cleanup, "UNLOCK TABLES"); err != nil {
				t.Error(err)
			}
		}()
		deadline, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if got, err := store.VerifyEncryptedState(deadline); !errors.Is(err, context.DeadlineExceeded) || got != (EncryptionVerification{}) {
			t.Fatalf("blocked verification did not time out: %#v, %v", got, err)
		}
	})
	assertGood()
	for _, target := range []struct{ table, where string }{
		{"vault_item_revisions", "revision = 1"},
		{"certificate_authorities", "revoked_at IS NOT NULL"},
		{"certificate_authorities", "not_after < UTC_TIMESTAMP(6)"},
		{"notification_destinations", "archived_at IS NOT NULL"},
		{"crypto_sentinel", "id = 1"},
	} {
		columns := []string{"ciphertext", "nonce"}
		if target.table != "crypto_sentinel" {
			columns = append(columns, "encrypted_dek")
		}
		for _, column := range columns {
			t.Run(target.table+"/"+target.where+"/"+column, func(t *testing.T) {
				var original []byte
				if err := database.QueryRowContext(ctx, "SELECT "+column+" FROM "+target.table+" WHERE "+target.where).Scan(&original); err != nil {
					t.Fatal(err)
				}
				bad := append([]byte(nil), original...)
				bad[0] ^= 0xff
				update := "UPDATE " + target.table + " SET " + column + " = ? WHERE " + target.where
				if _, err := database.ExecContext(ctx, update, bad); err != nil {
					t.Fatal(err)
				}
				if target.table == "crypto_sentinel" {
					assertBad("Sentinel")
				} else {
					assertBad(target.table)
				}
				if _, err := database.ExecContext(ctx, update, original); err != nil {
					t.Fatal(err)
				}
				assertGood()
			})
		}
	}
	var caDER, caNonce, caCiphertext, caDEK []byte
	if err := database.QueryRowContext(ctx, `SELECT certificate_der, nonce, ciphertext, encrypted_dek
		FROM certificate_authorities WHERE id = UNHEX(?)`, ca.Authority.ID).Scan(&caDER, &caNonce, &caCiphertext, &caDEK); err != nil {
		t.Fatal(err)
	}
	mismatched, err := provider.EncryptSecret(authorityKeyIdentity(ca.Authority.ID, caDER), expired.PrivateKeyDER)
	if err != nil {
		t.Fatal(err)
	}
	const updateCA = `UPDATE certificate_authorities SET nonce = ?, ciphertext = ?, encrypted_dek = ? WHERE id = UNHEX(?)`
	if _, err := database.ExecContext(ctx, updateCA, mismatched.Nonce, mismatched.Ciphertext, mismatched.EncryptedDEK, ca.Authority.ID); err != nil {
		t.Fatal(err)
	}
	assertBad("certificate_authorities")
	if _, err := database.ExecContext(ctx, updateCA, caNonce, caCiphertext, caDEK, ca.Authority.ID); err != nil {
		t.Fatal(err)
	}
	assertGood()
	// Correct encryption cannot make an invalid plaintext payload a valid backup.
	var itemID []byte
	if err := database.QueryRowContext(ctx, "SELECT item_id FROM vault_item_revisions WHERE revision = 1").Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	invalid, err := provider.EncryptSnapshot(vaultcrypto.SnapshotIdentity{ItemID: hex.EncodeToString(itemID), Revision: 1}, []byte(`{"secret-sentinel":"unknown-field"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE vault_item_revisions SET nonce = ?, ciphertext = ?, encrypted_dek = ?, ciphertext_size = ?
		WHERE item_id = ? AND revision = 1`, invalid.Nonce, invalid.Ciphertext, invalid.EncryptedDEK, len(invalid.Ciphertext), itemID); err != nil {
		t.Fatal(err)
	}
	assertBad("vault_item_revisions")
	canceled, stop := context.WithCancel(ctx)
	stop()
	if got, err := store.VerifyEncryptedState(canceled); !errors.Is(err, context.Canceled) || got != (EncryptionVerification{}) {
		t.Fatalf("canceled check: %#v, %v", got, err)
	}
}
