//go:build integration

package mysqlstore

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestMachineHTTPReadsResolvedConfigFromOneMySQLSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_machine_read")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	secretBytes := []byte("fedcba9876543210fedcba9876543210")
	certificateDER := []byte("registered-client-certificate")
	fileBytes := []byte("sentinel-private-file-bytes")
	seedMachineReadFixture(t, ctx, store.db, provider, secretBytes, certificateDER, fileBytes)
	publisher := &integrationAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := machine.NewHandler(store, publisher)
	request := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/configs/payment",
		nil,
	)
	request.Header.Set("Authorization", "Bearer cfg_token1_"+base64.RawURLEncoding.EncodeToString(secretBytes))
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: certificateDER}}}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	result := response.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	if result.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(result.Body)
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, body)
	}
	const expected = "{\"format\":\"yaml\",\"content\":\"database:\\n  username: \\\"redis-user\\\"\\n  password: \\\"sentinel-secret\\\"\\n\",\"config_revision\":1,\"vault_revisions\":{\"platform.redis\":1}}\n"
	if response.Body.String() != expected {
		t.Fatalf("response = %q, want %q", response.Body.String(), expected)
	}
	if result.Header.Get("ETag") == "" {
		t.Fatal("response ETag is empty")
	}
	event := <-publisher.events
	if event.Resource != "payment" || event.ConfigRevision != 1 || event.VaultRevisions["platform.redis"] != 1 {
		t.Fatalf("Access Event = %#v", event)
	}

	fileRequest := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/vault-items/platform/redis/fields/tls_cert/content",
		nil,
	)
	fileRequest.Header.Set("Authorization", request.Header.Get("Authorization"))
	fileRequest.TLS = request.TLS
	fileResponse := httptest.NewRecorder()
	handler.ServeHTTP(fileResponse, fileRequest)
	if fileResponse.Code != http.StatusOK || fileResponse.Body.String() != "sentinel-private-file-bytes" {
		t.Fatalf("File response = %d %q, want exact bytes", fileResponse.Code, fileResponse.Body.String())
	}
	fileEvent := <-publisher.events
	if fileEvent.Namespace != "platform" || fileEvent.Resource != "redis.tls_cert" || fileEvent.VaultRevisions["platform.redis"] != 1 {
		t.Fatalf("File Access Event = %#v", fileEvent)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE vault_item_revisions
		SET ciphertext = CONCAT(CHAR(ASCII(SUBSTRING(ciphertext, 1, 1)) ^ 1), SUBSTRING(ciphertext, 2))
	`); err != nil {
		t.Fatalf("tamper encrypted snapshot: %v", err)
	}

	unchangedRequest := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/configs/payment",
		nil,
	)
	unchangedRequest.Header.Set("Authorization", request.Header.Get("Authorization"))
	unchangedRequest.Header.Set("If-None-Match", result.Header.Get("ETag"))
	unchangedRequest.TLS = request.TLS
	unchangedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unchangedResponse, unchangedRequest)
	if unchangedResponse.Code != http.StatusNotModified || unchangedResponse.Body.Len() != 0 {
		t.Fatalf("unchanged response = %d %q, want empty 304", unchangedResponse.Code, unchangedResponse.Body.String())
	}

	tamperedRequest := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/configs/payment",
		nil,
	)
	tamperedRequest.Header.Set("Authorization", request.Header.Get("Authorization"))
	tamperedRequest.TLS = request.TLS
	tamperedResponse := httptest.NewRecorder()
	handler.ServeHTTP(tamperedResponse, tamperedRequest)
	if tamperedResponse.Code != http.StatusInternalServerError ||
		!strings.Contains(tamperedResponse.Body.String(), `"code":"crypto_integrity_failure"`) {
		t.Fatalf("tampered response = %d %q, want crypto integrity failure", tamperedResponse.Code, tamperedResponse.Body.String())
	}
	if strings.Contains(tamperedResponse.Body.String(), "sentinel-secret") {
		t.Fatalf("tampered response leaked plaintext: %q", tamperedResponse.Body.String())
	}
	select {
	case unexpected := <-publisher.events:
		t.Fatalf("unexpected Access Event after 304 or integrity failure: %#v", unexpected)
	default:
	}
}

func TestMachineHTTPReadsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_concurrent_reads")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	secretBytes := []byte("fedcba9876543210fedcba9876543210")
	certificateDER := []byte("registered-client-certificate")
	seedMachineReadFixture(t, ctx, store.db, provider, secretBytes, certificateDER, []byte("sentinel-private-file-bytes"))
	handler := machine.NewHandler(store, nil)
	authorization := "Bearer cfg_token1_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	tlsState := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: certificateDER}}}
	const readers = 64
	errorsByReader := make(chan error, readers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/a/configs/payment", nil).WithContext(ctx)
			request.Header.Set("Authorization", authorization)
			request.TLS = tlsState
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `password: \"sentinel-secret\"`) {
				errorsByReader <- fmt.Errorf("response = %d %q", response.Code, response.Body.String())
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByReader)
	for readErr := range errorsByReader {
		t.Fatal(readErr)
	}
}

func createIntegrationDatabase(t *testing.T, ctx context.Context, database string) string {
	t.Helper()
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open root MySQL: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var version string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil || version != "8.0.22" {
		t.Fatalf("MySQL version = %q, %v; want 8.0.22", version, err)
	}
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+database); err != nil {
		t.Fatalf("drop stale test database: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+database+" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS "+database)
	})
	configuration, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse root DSN: %v", err)
	}
	configuration.DBName = database
	return configuration.FormatDSN()
}

func seedMachineReadFixture(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	provider *vaultcrypto.LocalKeyProvider,
	secretBytes []byte,
	certificateDER []byte,
	fileBytes []byte,
) {
	t.Helper()
	environmentID := repeatedID(1)
	configID := repeatedID(2)
	itemID := repeatedID(3)
	usernameFieldID := repeatedID(4)
	passwordFieldID := repeatedID(5)
	fileFieldID := repeatedID(9)
	variantID := repeatedID(6)
	tokenID := repeatedID(7)
	certificateID := repeatedID(8)
	variantKey := hex.EncodeToString(variantID)
	snapshotJSON := []byte(fmt.Sprintf(
		`{"variants":{"%s":{"values":{"password":{"text":"sentinel-secret"},"tls_cert":{"file":{"filename":"server.pem","content_type":"application/x-pem-file","bytes":"%s"}},"username":{"text":"redis-user"}}}}}`,
		variantKey,
		base64.StdEncoding.EncodeToString(fileBytes),
	))
	encrypted, err := provider.EncryptSnapshot(
		vaultcrypto.SnapshotIdentity{ItemID: hex.EncodeToString(itemID), Revision: 1},
		snapshotJSON,
	)
	if err != nil {
		t.Fatalf("EncryptSnapshot: %v", err)
	}
	configText := []byte("database:\n  username: \"{vault.platform.redis.username}\"\n  password: \"{vault.platform.redis.password}\"\n")
	configDigest := sha256.Sum256(configText)
	structureDigest := sha256.Sum256([]byte("redis-structure-v1"))
	tokenDigest := sha256.Sum256(secretBytes)
	certificateFingerprint := sha256.Sum256(certificateDER)

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin fixture: %v", err)
	}
	defer transaction.Rollback()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO environments (id, resource_key, display_name) VALUES (?, 'a', 'Environment A')", []any{environmentID}},
		{"INSERT INTO configs (id, resource_key, display_name) VALUES (?, 'payment', 'Payment')", []any{configID}},
		{"INSERT INTO config_env_states (config_id, environment_id, current_revision) VALUES (?, ?, 1)", []any{configID, environmentID}},
		{`INSERT INTO config_revisions
            (config_id, environment_id, revision, format, content, content_sha256, operation_id, actor_type, actor_id)
            VALUES (?, ?, 1, 'yaml', ?, ?, 'fixture-config', 'user', 'test-admin')`, []any{configID, environmentID, configText, configDigest[:]}},
		{"INSERT INTO config_revision_vault_refs (config_id, environment_id, revision, namespace_key, item_key, field_key) VALUES (?, ?, 1, 'platform', 'redis', 'username'), (?, ?, 1, 'platform', 'redis', 'password')", []any{configID, environmentID, configID, environmentID}},
		{"INSERT INTO vault_items (id, namespace_key, resource_key, display_name, current_revision) VALUES (?, 'platform', 'redis', 'Redis', 1)", []any{itemID}},
		{"INSERT INTO vault_fields (item_id, id, resource_key, display_name, field_type) VALUES (?, ?, 'username', 'Username', 'text'), (?, ?, 'password', 'Password', 'secret'), (?, ?, 'tls_cert', 'TLS Certificate', 'file')", []any{itemID, usernameFieldID, itemID, passwordFieldID, itemID, fileFieldID}},
		{`INSERT INTO vault_item_revisions
			(item_id, revision, item_display_name, structure_sha256, algorithm, key_version, nonce, ciphertext, encrypted_dek, ciphertext_size, operation_id, actor_type, actor_id)
			VALUES (?, 1, 'Redis', ?, ?, ?, ?, ?, ?, ?, 'fixture-vault', 'user', 'test-admin')`, []any{itemID, structureDigest[:], encrypted.Algorithm, encrypted.KeyVersion, encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, len(encrypted.Ciphertext)}},
		{"INSERT INTO vault_revision_fields (item_id, revision, field_id, resource_key, display_name, field_type) VALUES (?, 1, ?, 'username', 'Username', 'text'), (?, 1, ?, 'password', 'Password', 'secret'), (?, 1, ?, 'tls_cert', 'TLS Certificate', 'file')", []any{itemID, usernameFieldID, itemID, passwordFieldID, itemID, fileFieldID}},
		{"INSERT INTO vault_revision_variants (item_id, revision, variant_id, ordinal) VALUES (?, 1, ?, 1)", []any{itemID, variantID}},
		{"INSERT INTO vault_revision_variant_environments (item_id, revision, variant_id, environment_id) VALUES (?, 1, ?, ?)", []any{itemID, variantID, environmentID}},
		{`INSERT INTO api_tokens
            (id, public_id, display_name, display_prefix, secret_digest, allow_without_mtls, expires_at)
            VALUES (?, 'token1', 'Test Token', 'cfg_token1', ?, FALSE, '2100-01-01 00:00:00')`, []any{tokenID, tokenDigest[:]}},
		{"INSERT INTO api_token_environments (token_id, environment_id) VALUES (?, ?)", []any{tokenID, environmentID}},
		{`INSERT INTO client_certificates
            (id, fingerprint_sha256, display_name, certificate_der, subject, serial_hex, not_before, not_after)
            VALUES (?, ?, 'Test Certificate', ?, 'CN=test', '01', '2020-01-01 00:00:00', '2100-01-01 00:00:00')`, []any{certificateID, certificateFingerprint[:], certificateDER}},
	}
	for index, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("fixture statement %d: %v", index+1, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit fixture: %v", err)
	}
}

func repeatedID(value byte) []byte {
	id := make([]byte, 16)
	for index := range id {
		id[index] = value
	}
	return id
}

type integrationAccessPublisher struct {
	events chan machine.AccessEvent
}

func (publisher *integrationAccessPublisher) TryPublish(event machine.AccessEvent) bool {
	publisher.events <- event
	return true
}
