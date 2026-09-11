//go:build integration

package mysqlstore

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func pkiTestDatabase(t *testing.T) (string, *sql.DB, *vaultcrypto.LocalKeyProvider) {
	t.Helper()
	root := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if root == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	config, err := mysql.ParseDSN(root)
	if err != nil {
		t.Fatal("invalid test MySQL DSN")
	}
	admin, err := sql.Open("mysql", root)
	if err != nil {
		t.Fatal(err)
	}
	var version string
	if err := admin.QueryRow("SELECT VERSION()").Scan(&version); err != nil || version != "8.0.22" {
		t.Fatal("test requires MySQL 8.0.22")
	}
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	name := "configra_pki_" + hex.EncodeToString(id[:8])
	if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP DATABASE " + name); admin.Close() })
	config.DBName = name
	database, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return config.FormatDSN(), database, provider
}

func unpackCredential(t *testing.T, encoded string) map[string][]byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal("invalid credential export")
	}
	t.Cleanup(func() { clear(data) })
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal("invalid credential archive")
	}
	result := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		result[file.Name], err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, data := range result {
			clear(data)
		}
	})
	return result
}

func TestManagedAuthoritiesExportOnceAcrossReplicasAndRevokeExistingConnections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dsn, database, provider := pkiTestDatabase(t)
	first, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	actor := Actor{Type: "user", ID: "admin"}
	request := AuthorityCreate{OperationID: "ca-create", Actor: actor, DisplayName: "Workloads", ValidDays: 30}
	type creation struct {
		result AuthorityResult
		err    error
	}
	results := make(chan creation, 2)
	for _, replica := range []*Store{first, second} {
		go func(store *Store) {
			result, err := store.CreateCertificateAuthority(ctx, request)
			results <- creation{result, err}
		}(replica)
	}
	var authority CertificateAuthority
	var export string
	exports := 0
	for range 2 {
		created := <-results
		if created.err != nil {
			t.Fatal(created.err)
		}
		if authority.ID != "" && created.result.Authority.ID != authority.ID {
			t.Fatal("replicas created different authorities for one operation")
		}
		authority = created.result.Authority
		if created.result.ExportBundle != "" {
			exports++
			export = created.result.ExportBundle
		}
	}
	if exports != 1 {
		t.Fatalf("private exports = %d, want one", exports)
	}
	var persisted string
	if err := database.QueryRowContext(ctx, "SELECT CAST(response_json AS CHAR) FROM operations WHERE operation_id = 'ca-create'").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, "export_bundle") || strings.Contains(persisted, "PRIVATE KEY") || strings.Contains(persisted, export) {
		t.Fatal("private export was persisted in replay data")
	}
	caFiles := unpackCredential(t, export)
	private, _ := pem.Decode(caFiles["ca.key"])
	if private == nil {
		t.Fatal("missing CA key")
	}
	var encrypted []byte
	if err := database.QueryRowContext(ctx, "SELECT ciphertext FROM certificate_authorities").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, private.Bytes) {
		t.Fatal("unencrypted Authority key in database")
	}
	issueRequest := ClientCertificateIssue{OperationID: "issue-client", Actor: actor, AuthorityID: authority.ID, DisplayName: "payments", ValidDays: 90}
	issued, err := first.IssueClientCertificate(ctx, issueRequest)
	if err != nil || issued.ExportBundle == "" {
		t.Fatalf("issue credential failed: %v", err)
	}
	replayed, err := second.IssueClientCertificate(ctx, issueRequest)
	if err != nil || replayed.ExportBundle != "" || replayed.Certificate.FingerprintSHA256 != issued.Certificate.FingerprintSHA256 {
		t.Fatal("issuance replay returned private material or a different credential")
	}
	clientFiles := unpackCredential(t, issued.ExportBundle)
	caBlock, _ := pem.Decode([]byte(authority.CertificatePEM))
	caCertificate, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := clientcert.GenerateClient(caCertificate, private.Bytes, "manually-signed", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(manual.PrivateKeyDER)
	imported, err := first.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{OperationID: "import-offline-client", Actor: actor, DisplayName: "Offline signer", Certificate: manual.Certificate})
	if err != nil {
		t.Fatal(err)
	}
	var importedIssuer string
	if err := database.QueryRowContext(ctx, "SELECT LOWER(HEX(authority_id)) FROM client_certificates WHERE fingerprint_sha256 = UNHEX(?)", imported.FingerprintSHA256).Scan(&importedIssuer); err != nil || importedIssuer != authority.ID {
		t.Fatal("offline-signed client lost its managed issuer association")
	}
	pair, err := tls.X509KeyPair(clientFiles["client.crt"], clientFiles["client.key"])
	if err != nil {
		t.Fatal("invalid issued credential pair")
	}
	if _, err := first.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "create-env", Actor: actor, Action: EnvironmentCreate, Key: "production", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CommitConfig(ctx, ConfigCommit{OperationID: "create-config", Actor: actor, EnvironmentKey: "production", ConfigKey: "app", ConfigName: "Application", Format: configdoc.YAML, Content: []byte("enabled: true\n")}); err != nil {
		t.Fatal(err)
	}
	token, err := first.CreateToken(ctx, TokenCreate{OperationID: "create-token", Actor: actor, DisplayName: "Workload", EnvironmentKeys: []string{"production"}})
	if err != nil {
		t.Fatal(err)
	}
	trust, err := clientcert.NewTrustManager(ctx, nil, first.ActiveCertificateAuthorities)
	if err != nil {
		t.Fatal(err)
	}
	template := httptest.NewTLSServer(http.NotFoundHandler())
	serverCertificate := template.TLS.Certificates[0]
	template.Close()
	server := httptest.NewUnstartedServer(machine.NewHandler(first, nil))
	server.TLS = trust.TLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCertificate}})
	var connections atomic.Int32
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair}}}}
	defer client.CloseIdleConnections()
	read := func() int {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/environments/production/configs/app", nil)
		request.Header.Set("Authorization", "Bearer "+token.Token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return response.StatusCode
	}
	if status := read(); status != http.StatusOK {
		t.Fatalf("initial mTLS read = %d", status)
	}
	if _, err := second.RevokeCertificateAuthority(ctx, AuthorityRevoke{OperationID: "revoke-ca", Actor: actor, AuthorityID: authority.ID}); err != nil {
		t.Fatal(err)
	}
	if status := read(); status != http.StatusUnauthorized {
		t.Fatalf("read after Authority revocation = %d", status)
	}
	manualFingerprint := sha256.Sum256(manual.Certificate.Raw)
	if active, err := first.IsCertificateActive(ctx, manualFingerprint); err != nil || active {
		t.Fatal("CA revocation did not invalidate an imported offline-signed client")
	}
	if connections.Load() != 1 {
		t.Fatal("revocation test did not reuse the existing TLS connection")
	}
	issueRequest.OperationID = "issue-after-revoke"
	if _, err := first.IssueClientCertificate(ctx, issueRequest); !errors.Is(err, ErrValidation) {
		t.Fatal("revoked Authority issued a new credential")
	}
	fingerprintBytes, _ := hex.DecodeString(issued.Certificate.FingerprintSHA256)
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], fingerprintBytes)
	if active, err := first.IsCertificateActive(ctx, fingerprint); err != nil || active {
		t.Fatal("issued certificate remains active after Authority revocation")
	}
}

func TestSchemaV2PreservesExistingDataAndChecksMasterKeyBeforeMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	dsn, database, provider := pkiTestDatabase(t)
	for _, statement := range append([]string{schemaMigrationsDDL, cryptoSentinelDDL}, schemaV1DDL...) {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	sentinel, err := provider.CreateSentinel()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO crypto_sentinel (id, algorithm, key_version, nonce, ciphertext) VALUES (1, ?, ?, ?, ?)`, sentinel.Algorithm, sentinel.KeyVersion, sentinel.Nonce, sentinel.Ciphertext); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	id, _ := randomID()
	if _, err := database.ExecContext(ctx, "INSERT INTO environments (id, resource_key, display_name) VALUES (?, 'legacy', 'Legacy')", id); err != nil {
		t.Fatal(err)
	}
	wrong, _ := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if _, err := OpenManagement(ctx, dsn, wrong); !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatal("wrong Master Key was accepted")
	}
	var version int
	if err := database.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 1 {
		t.Fatal("wrong Master Key changed the schema")
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if version, err := store.SchemaVersion(ctx); err != nil || version != 2 {
		t.Fatal("schema v2 was not recorded")
	}
	items, err := store.ListEnvironments(ctx, false)
	if err != nil || len(items) != 1 || items[0].Key != "legacy" {
		t.Fatal("migration lost existing data")
	}
}
