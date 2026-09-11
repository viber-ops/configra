//go:build integration

package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestManagementServerDeliversAuditOutboxToClickHouse(t *testing.T) {
	dsn := os.Getenv("CONFIGRA_TEST_MYSQL_DSN")
	natsURL := os.Getenv("CONFIGRA_TEST_NATS_URL")
	clickHouseDSN := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	if dsn == "" || natsURL == "" || clickHouseDSN == "" {
		t.Skip("Management process dependencies are not configured")
	}
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelStartup()
	managementStore, err := mysqlstore.OpenManagement(startupContext, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	defer managementStore.Close()

	var issuer string
	oidcServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/keys",
			"userinfo_endpoint":                     issuer + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	issuer = oidcServer.URL
	defer oidcServer.Close()

	certificateFile, privateKeyFile, certificate := processTLSFiles(t)
	directory := t.TempDir()
	masterKeyFile := filepath.Join(directory, "master-key")
	if err := os.WriteFile(masterKeyFile, []byte(base64.StdEncoding.EncodeToString(masterKey)), 0o600); err != nil {
		t.Fatalf("write Master Key: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	t.Setenv("CONFIGRA_PROCESS_TEST_DSN", dsn)
	t.Setenv("CONFIGRA_PROCESS_TEST_OIDC_SECRET", "oidc-secret-sentinel")
	t.Setenv("CONFIGRA_PROCESS_TEST_CLICKHOUSE_DSN", clickHouseDSN)
	configFile := filepath.Join(directory, "management.yaml")
	configDocument := fmt.Sprintf(`
version: 1
listen: %s
tls:
  certificate_file: %s
  private_key_file: %s
  client_ca_file: %s
mysql:
  dsn_env: CONFIGRA_PROCESS_TEST_DSN
key_provider:
  master_key_file: %s
clickhouse:
  dsn_env: CONFIGRA_PROCESS_TEST_CLICKHOUSE_DSN
nats:
  urls: [%s]
oidc:
  issuer: %s
  client_id: configra
  client_secret_env: CONFIGRA_PROCESS_TEST_OIDC_SECRET
  redirect_url: https://%s/auth/callback
  role_source: id_token
  role_claim: groups
  viewer_values: [viewer]
  admin_values: [admin]
  allow_insecure_loopback: true
logging:
  level: error
`, address, certificateFile, privateKeyFile, certificateFile, masterKeyFile, natsURL, issuer, address)
	if err := os.WriteFile(configFile, []byte(configDocument), 0o600); err != nil {
		t.Fatalf("write Management Config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, bootstrap.Management, configFile) }()
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, requestErr := client.Get("https://" + address + "/health/ready")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				break
			}
		}
		select {
		case runErr := <-done:
			t.Fatalf("Management Server stopped before readiness: %v", runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("Management Server did not become ready: %v", requestErr)
		}
		time.Sleep(25 * time.Millisecond)
	}

	operationID := fmt.Sprintf("management-process-%d", time.Now().UnixNano())
	if _, err := managementStore.ApplyEnvironmentChange(context.Background(), mysqlstore.EnvironmentChange{
		OperationID: operationID,
		Actor:       mysqlstore.Actor{Type: "user", ID: "issuer|subject"},
		Action:      mysqlstore.EnvironmentCreate,
		Key:         operationID,
		DisplayName: "Management Process Test",
	}); err != nil {
		t.Fatalf("create audited Environment: %v", err)
	}
	options, _ := clickhouse.ParseDSN(clickHouseDSN)
	reader, err := clickhouse.Open(options)
	if err != nil {
		t.Fatalf("open ClickHouse reader: %v", err)
	}
	defer reader.Close()
	deadline = time.Now().Add(10 * time.Second)
	for {
		var count uint64
		err := reader.QueryRow(context.Background(), "SELECT count() FROM audit_events WHERE operation_id = ?", operationID).Scan(&count)
		if err == nil && count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivered Audit Event count = %d, error = %v", count, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Management Server shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Management Server did not stop cleanly")
	}
}

func TestAPIServerStartsOverTLSReportsReadyAndStopsCleanly(t *testing.T) {
	dsn := os.Getenv("CONFIGRA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_DSN is not set")
	}
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelStartup()
	managementStore, err := mysqlstore.OpenManagement(startupContext, dsn, provider)
	if err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	if err := managementStore.Close(); err != nil {
		t.Fatalf("close Management Store: %v", err)
	}

	certificateFile, privateKeyFile, certificate := processTLSFiles(t)
	directory := t.TempDir()
	masterKeyFile := filepath.Join(directory, "master-key")
	if err := os.WriteFile(masterKeyFile, []byte(base64.StdEncoding.EncodeToString(masterKey)), 0o600); err != nil {
		t.Fatalf("write Master Key: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	t.Setenv("CONFIGRA_PROCESS_TEST_DSN", dsn)
	configFile := filepath.Join(directory, "api.yaml")
	configDocument := fmt.Sprintf(`
version: 1
listen: %s
tls:
  certificate_file: %s
  private_key_file: %s
  client_ca_file: %s
mysql:
  dsn_env: CONFIGRA_PROCESS_TEST_DSN
key_provider:
  master_key_file: %s
nats:
  urls: [nats://127.0.0.1:1]
logging:
  level: error
`, address, certificateFile, privateKeyFile, certificateFile, masterKeyFile)
	if err := os.WriteFile(configFile, []byte(configDocument), 0o600); err != nil {
		t.Fatalf("write API Config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, bootstrap.API, configFile) }()
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}},
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, requestErr := client.Get("https://" + address + "/health/ready")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				break
			}
		}
		select {
		case runErr := <-done:
			t.Fatalf("API Server stopped before readiness: %v", runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("API Server did not become ready: %v", requestErr)
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("API Server shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("API Server did not stop cleanly")
	}
}

func processTLSFiles(t *testing.T) (string, string, *x509.Certificate) {
	t.Helper()
	testServer := httptest.NewTLSServer(http.NotFoundHandler())
	testServer.Close()
	keyPair := testServer.TLS.Certificates[0]
	certificate, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	directory := t.TempDir()
	certificateFile := filepath.Join(directory, "tls.crt")
	privateKeyFile := filepath.Join(directory, "tls.key")
	if err := os.WriteFile(certificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatalf("write Certificate: %v", err)
	}
	if err := os.WriteFile(privateKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}), 0o600); err != nil {
		t.Fatalf("write Private Key: %v", err)
	}
	return certificateFile, privateKeyFile, certificate
}
