//go:build integration

package mysqlstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

type rotationFixture struct {
	store            *Store
	database         *sql.DB
	dsn, authorityID string
	old, next        *vaultcrypto.LocalKeyProvider
	actor            Actor
	counts           EncryptionVerification
}

func newRotationFixture(t *testing.T, ctx context.Context) rotationFixture {
	t.Helper()
	dsn, database, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	next, err := vaultcrypto.NewLocalKeyProvider(bytes.Repeat([]byte{0x63}, 32))
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{Type: "user", ID: "rotation-test@example.test"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "rotation-env", Actor: actor, Action: EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	variant := ""
	for revision := uint64(0); revision < 3; revision++ {
		result, err := store.CommitVault(ctx, VaultCommit{OperationID: fmt.Sprintf("rotation-vault-%d", revision), Actor: actor,
			NamespaceKey: "ops", ItemKey: "db", ItemName: "Database", ExpectedRevision: revision,
			Snapshot: testVaultSnapshot(variant, []string{"prod"}, "db-user", fmt.Sprintf("rotation-secret-%d", revision))})
		if err != nil {
			t.Fatal(err)
		}
		variant = result.VariantIDs[0]
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "rotation-config", Actor: actor, EnvironmentKey: "prod", ConfigKey: "service", ConfigName: "Service",
		Format: configdoc.YAML, Content: []byte("password: '{vault.ops.db.password}'\n")}); err != nil {
		t.Fatal(err)
	}
	ca, err := store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "rotation-ca", Actor: actor, DisplayName: "CA", ValidDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	url, secret := "https://hooks.example.test/rotation-url-sentinel", "rotation-notification-secret"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "rotation-notification", Actor: actor,
		Key: "ops", DisplayName: "Ops", Provider: NotificationGenericWebhook, URL: &url, Secret: &secret, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyNotificationDestinationLifecycle(ctx, NotificationDestinationLifecycle{OperationID: "rotation-archive", Actor: actor, Key: "ops", Action: NotificationDestinationArchive}); err != nil {
		t.Fatal(err)
	}
	return rotationFixture{store: store, database: database, dsn: dsn, authorityID: ca.Authority.ID, old: provider, next: next, actor: actor,
		counts: EncryptionVerification{VaultRevisions: 3, CertificateAuthorities: 1, NotificationDestinations: 1}}
}

func TestMasterKeyRotationPreservesContentHistoryAndRejectsStaleWriters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fixture := newRotationFixture(t, ctx)
	before := rotationStateDigest(t, ctx, fixture.database, false)
	wrappedBefore := rotationStateDigest(t, ctx, fixture.database, true)
	resolved, err := fixture.store.ReadResolvedConfig(ctx, "prod", "service", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []*vaultcrypto.LocalKeyProvider{nil, fixture.old} {
		if result, err := fixture.store.RotateMasterKey(ctx, next); err == nil || result != (KeyRotationResult{}) {
			t.Fatal("accepted absent/unchanged new key")
		}
	}
	result, err := fixture.store.RotateMasterKey(ctx, fixture.next)
	if err != nil || result.EncryptionVerification != fixture.counts || result.OperationID == "" {
		t.Fatalf("rotate: %#v, %v", result, err)
	}
	if before != rotationStateDigest(t, ctx, fixture.database, false) || wrappedBefore == rotationStateDigest(t, ctx, fixture.database, true) {
		t.Fatal("rotation changed payload/metadata or did not change key wrapping")
	}
	if err := fixture.store.Ping(ctx); !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatal("stale-key replica still ready", err)
	}
	if old, err := OpenAPI(ctx, fixture.dsn, fixture.old); err == nil {
		old.Close()
		t.Fatal("old key accepted after rotation")
	}
	fresh, err := OpenAPI(ctx, fixture.dsn, fixture.next)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if report, err := fresh.VerifyEncryptedState(ctx); err != nil || report != fixture.counts {
		t.Fatalf("new-key verification: %#v, %v", report, err)
	}
	after, err := fresh.ReadResolvedConfig(ctx, "prod", "service", "")
	if err != nil || after.Content != resolved.Content || after.ETag != resolved.ETag {
		t.Fatal("resolved content or ETag changed on rotation", err)
	}
	for revision := uint64(1); revision <= 3; revision++ {
		item, err := fresh.ReadVaultItem(ctx, "ops", "db", revision, true)
		if err != nil || *item.Snapshot.Variants[0].Values["password"].Text != fmt.Sprintf("rotation-secret-%d", revision-1) {
			t.Fatal("historical revision lost", err)
		}
	}
	file, err := fresh.ReadFile(ctx, "prod", "ops", "db", "tls_cert", "")
	if err != nil || string(file.Bytes) != "sentinel-private-file-bytes" {
		t.Fatal("rotated File is unreadable", err)
	}
	if issued, err := fresh.IssueClientCertificate(ctx, ClientCertificateIssue{OperationID: "rotated-issue", Actor: fixture.actor,
		AuthorityID: fixture.authorityID, DisplayName: "New client", ValidDays: 1}); err != nil || issued.ExportBundle == "" {
		t.Fatal("rotated CA cannot issue", err)
	}
	for _, write := range staleRotationWrites(ctx, fixture) {
		if err := write(); !errors.Is(err, vaultcrypto.ErrIntegrity) {
			t.Fatal("stale provider wrote new encrypted data", err)
		}
	}
	if report, err := fresh.VerifyEncryptedState(ctx); err != nil || report != fixture.counts {
		t.Fatal("stale writes stranded old-key data", err)
	}
	events, err := fresh.ClaimOutbox(ctx, OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.OperationID == result.OperationID {
			var payload struct {
				Actor  Actor  `json:"actor"`
				Action string `json:"action"`
			}
			if event.Type != "master_key.rotated" || json.Unmarshal(event.Payload, &payload) != nil || payload.Actor.Type != "system" || payload.Action != "master_key.rotate" || strings.Contains(string(event.Payload), "secret") {
				t.Fatal("invalid rotation audit")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("rotation has no durable audit")
	}
	// A reverse rotation uses the same complete transaction; merely restoring the
	// old mounted key after success is not a rollback.
	if _, err := fresh.RotateMasterKey(ctx, fixture.old); err != nil {
		t.Fatal(err)
	}
	if report, err := fixture.store.VerifyEncryptedState(ctx); err != nil || report != fixture.counts || before != rotationStateDigest(t, ctx, fixture.database, false) {
		t.Fatal("reverse rotation lost original content", err)
	}
}

func staleRotationWrites(ctx context.Context, fixture rotationFixture) []func() error {
	return []func() error{
		func() error {
			_, err := fixture.store.CommitVault(ctx, VaultCommit{OperationID: "stale-vault", Actor: fixture.actor, NamespaceKey: "ops", ItemKey: "stale", ItemName: "Stale", Snapshot: testVaultSnapshot("", []string{"prod"}, "user", "stale-secret")})
			return err
		},
		func() error {
			_, err := fixture.store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "stale-ca", Actor: fixture.actor, DisplayName: "Stale"})
			return err
		},
		func() error {
			url := "https://hooks.example.test/stale"
			_, err := fixture.store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "stale-notify", Actor: fixture.actor, Key: "stale", DisplayName: "Stale", Provider: NotificationGenericWebhook, URL: &url})
			return err
		},
	}
}

func TestMasterKeyRotationFencesAlreadyRunningWriters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newRotationFixture(t, ctx)
	var database string
	if err := fixture.database.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		t.Fatal(err)
	}
	gate := database + "_rotate"
	blocker, err := fixture.database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	var acquired int
	if err := blocker.QueryRowContext(ctx, "SELECT GET_LOCK(?, 1)", gate).Scan(&acquired); err != nil || acquired != 1 {
		t.Fatal("cannot create rotation test gate", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = blocker.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", gate)
	}()
	// Pause an actual UPDATE while rotation holds the exclusive Sentinel lock.
	if _, err := fixture.database.ExecContext(ctx, `CREATE TRIGGER gate_rotation BEFORE UPDATE ON vault_item_revisions FOR EACH ROW
		BEGIN
			IF GET_LOCK('`+gate+`', 10) <> 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'rotation gate timed out'; END IF;
			DO RELEASE_LOCK('`+gate+`');
		END`); err != nil {
		t.Fatal(err)
	}
	rotation := make(chan error, 1)
	go func() { _, err := fixture.store.RotateMasterKey(ctx, fixture.next); rotation <- err }()
	waitForLocks := func(query string, want int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var count int
			if err := fixture.database.QueryRowContext(ctx, query, database).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count >= want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("lock count = %d, want %d", count, want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitForLocks(`SELECT COUNT(*) FROM performance_schema.data_locks WHERE OBJECT_SCHEMA = ? AND OBJECT_NAME = 'crypto_sentinel'
		AND LOCK_TYPE = 'RECORD' AND LOCK_MODE LIKE 'X%' AND LOCK_STATUS = 'GRANTED'`, 1)
	writers := make(chan error, 3)
	for _, write := range staleRotationWrites(ctx, fixture) {
		go func() { writers <- write() }()
	}
	waitForLocks(`SELECT COUNT(*) FROM performance_schema.data_lock_waits AS wait
		JOIN performance_schema.data_locks AS requested ON requested.ENGINE_LOCK_ID = wait.REQUESTING_ENGINE_LOCK_ID
		WHERE requested.OBJECT_SCHEMA = ? AND requested.OBJECT_NAME = 'crypto_sentinel'`, 3)
	if err := blocker.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", gate).Scan(&acquired); err != nil || acquired != 1 {
		t.Fatal("cannot release rotation gate", err)
	}
	if err := <-rotation; err != nil {
		t.Fatal("rotation failed", err)
	}
	for range 3 {
		if err := <-writers; !errors.Is(err, vaultcrypto.ErrIntegrity) {
			t.Fatal("in-flight stale writer was accepted", err)
		}
	}
	fresh, err := OpenAPI(ctx, fixture.dsn, fixture.next)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if report, err := fresh.VerifyEncryptedState(ctx); err != nil || report != fixture.counts {
		t.Fatal("mixed-key data after concurrency test", err)
	}
}

func TestMasterKeyRotationRollsBackPartialWritesAndHonorsLockDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fixture := newRotationFixture(t, ctx)
	before := rotationStateDigest(t, ctx, fixture.database, true)
	// Fail the final encrypted table after Vault and CA key updates have run.
	if _, err := fixture.database.ExecContext(ctx, `CREATE TRIGGER fail_rotation BEFORE UPDATE ON notification_destinations
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'database-error-secret-sentinel'`); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.RotateMasterKey(ctx, fixture.next)
	if err == nil || strings.Contains(err.Error(), "database-error-secret-sentinel") || result != (KeyRotationResult{}) || before != rotationStateDigest(t, ctx, fixture.database, true) {
		t.Fatal("failed rotation leaked details or left partial changes", err)
	}
	if _, err := fixture.database.ExecContext(ctx, "DROP TRIGGER fail_rotation"); err != nil {
		t.Fatal(err)
	}
	blocker, err := fixture.database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	var sentinelID int
	if err := blocker.QueryRowContext(ctx, "SELECT id FROM crypto_sentinel WHERE id = 1 FOR SHARE").Scan(&sentinelID); err != nil {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	result, err = fixture.store.RotateMasterKey(deadline, fixture.next)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) || result != (KeyRotationResult{}) || before != rotationStateDigest(t, ctx, fixture.database, true) {
		t.Fatal("lock deadline left partial changes", err)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if report, err := fixture.store.VerifyEncryptedState(ctx); err != nil || report != fixture.counts {
		t.Fatal("rollback stranded original data", err)
	}
	if _, err := fixture.database.ExecContext(ctx, "UPDATE vault_item_revisions SET nonce = REPEAT(0x00, 12) WHERE revision = 1"); err != nil {
		t.Fatal(err)
	}
	corrupt := rotationStateDigest(t, ctx, fixture.database, true)
	if result, err := fixture.store.RotateMasterKey(ctx, fixture.next); !errors.Is(err, vaultcrypto.ErrIntegrity) || result != (KeyRotationResult{}) || corrupt != rotationStateDigest(t, ctx, fixture.database, true) {
		t.Fatal("rotation accepted/rewrote a corrupt history", err)
	}
}

func TestMasterKeyRotationReportsUnknownWhenCommitAcknowledgmentIsLost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newRotationFixture(t, ctx)
	config, err := mysql.ParseDSN(fixture.dsn)
	if err != nil {
		t.Fatal("invalid fixture DSN")
	}
	if config.Net != "tcp" || config.TLSConfig != "" {
		t.Skip("packet fault fixture requires local unencrypted MySQL TCP")
	}
	upstream := config.Addr
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dropped := make(chan struct{})
	var dropOnce sync.Once
	var forwarding sync.WaitGroup
	acceptDone := make(chan struct{})
	t.Cleanup(func() { listener.Close(); <-acceptDone; forwarding.Wait() })
	go func() {
		defer close(acceptDone)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", upstream)
			if err != nil {
				client.Close()
				continue
			}
			var awaitingCommit atomic.Bool
			// This is a packet dropper, not a fake SQL implementation. It forwards
			// the real server's auth/queries, then drops the actual COMMIT response.
			forward := func(destination, source net.Conn, toServer bool) {
				defer forwarding.Done()
				defer client.Close()
				defer server.Close()
				for {
					var header [4]byte
					if _, err := io.ReadFull(source, header[:]); err != nil {
						return
					}
					length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
					if length > 1<<20 {
						return
					} // Fixture packets are small; never buffer arbitrary traffic.
					packet := make([]byte, 4+length)
					copy(packet, header[:])
					if _, err := io.ReadFull(source, packet[4:]); err != nil {
						return
					}
					if toServer && length == 7 && packet[4] == 3 && string(packet[5:]) == "COMMIT" {
						awaitingCommit.Store(true)
					} else if !toServer && awaitingCommit.Load() {
						dropOnce.Do(func() { close(dropped) })
						return
					}
					if count, err := destination.Write(packet); err != nil || count != len(packet) {
						return
					}
				}
			}
			forwarding.Add(2)
			go forward(server, client, true)
			go forward(client, server, false)
		}
	}()
	config.Addr = listener.Addr().String()
	proxied, err := OpenAPI(ctx, config.FormatDSN(), fixture.old)
	if err != nil {
		t.Fatal(err)
	}
	defer proxied.Close()
	result, err := proxied.RotateMasterKey(ctx, fixture.next)
	if !errors.Is(err, ErrRotationOutcomeUnknown) || result != (KeyRotationResult{}) {
		t.Fatal("lost COMMIT response was reported as a rollback or success", err)
	}
	select {
	case <-dropped:
	default:
		t.Fatal("COMMIT response fault was not exercised")
	}
	// The server did commit. Recovery must discover that fact with the new key,
	// not retry against the old key or assume that an error implies rollback.
	fresh, err := OpenAPI(ctx, fixture.dsn, fixture.next)
	if err != nil {
		t.Fatal("committed rotation not readable with new key", err)
	}
	defer fresh.Close()
	if report, err := fresh.VerifyEncryptedState(ctx); err != nil || report != fixture.counts {
		t.Fatal("uncertain commit left mixed-key data", err)
	}
	if err := fixture.store.Ping(ctx); !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatal("old key remains valid after committed rotation", err)
	}
}

func TestMasterKeyRotationRejectsNonTransactionalStorage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := newRotationFixture(t, ctx)
	before := rotationStateDigest(t, ctx, fixture.database, true)
	if _, err := fixture.database.ExecContext(ctx, "ALTER TABLE crypto_sentinel ENGINE=MyISAM"); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.RotateMasterKey(ctx, fixture.next)
	if err == nil || !strings.Contains(err.Error(), "InnoDB") || result != (KeyRotationResult{}) || before != rotationStateDigest(t, ctx, fixture.database, true) {
		t.Fatal("rotation accepted non-atomic storage", err)
	}
}

// Storage fault tests compare persisted envelopes without logging their bytes.
func rotationStateDigest(t *testing.T, ctx context.Context, database *sql.DB, wrapped bool) [32]byte {
	t.Helper()
	statements := []string{
		"SELECT item_id, revision, nonce, ciphertext, created_at FROM vault_item_revisions ORDER BY item_id, revision",
		"SELECT id, certificate_der, nonce, ciphertext, created_at FROM certificate_authorities ORDER BY id",
		"SELECT id, nonce, ciphertext, updated_at, enabled, archived_at FROM notification_destinations ORDER BY id",
	}
	if wrapped {
		statements = []string{
			"SELECT item_id, revision, encrypted_dek FROM vault_item_revisions ORDER BY item_id, revision",
			"SELECT id, encrypted_dek FROM certificate_authorities ORDER BY id",
			"SELECT id, encrypted_dek FROM notification_destinations ORDER BY id",
			"SELECT nonce, ciphertext FROM crypto_sentinel WHERE id = 1",
		}
	}
	digest := sha256.New()
	for _, statement := range statements {
		rows, err := database.QueryContext(ctx, statement)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([][]byte, len(columns))
			pointers := make([]any, len(columns))
			for index := range pointers {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			for _, value := range values {
				fmt.Fprintf(digest, "%d:", len(value))
				_, _ = digest.Write(value)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	return [32]byte(digest.Sum(nil))
}
