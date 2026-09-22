//go:build integration

package e2e_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	configrago "github.com/viber-ops/configra-go"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestSDKReadsRealMachineAPIAndRetainsLastKnownGoodAfterTokenRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := newStore(t, ctx)
	actor := mysqlstore.Actor{Type: "user", ID: "e2e-admin"}
	if _, err := store.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{
		OperationID: "e2e-environment-create", Actor: actor, Action: mysqlstore.EnvironmentCreate,
		Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	createdVault, err := store.CommitVault(ctx, mysqlstore.VaultCommit{
		OperationID: "e2e-vault-create", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis",
		Snapshot: vaultSnapshot("", "resolved-secret-one", "file-one"),
	})
	if err != nil {
		t.Fatalf("create Vault Item: %v", err)
	}
	if _, err := store.CommitConfig(ctx, mysqlstore.ConfigCommit{
		OperationID: "e2e-config-create", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		Format: configdoc.YAML, Content: []byte("database:\n  username: '{vault.platform.redis.username}'\n  password: '{vault.platform.redis.password}'\n"),
	}); err != nil {
		t.Fatalf("create Config: %v", err)
	}
	managementAPI := management.NewHandler(store, nil, nil)
	createTokenRequest := httptest.NewRequest(http.MethodPost, "https://management.test/v1/api-tokens", strings.NewReader(
		`{"display_name":"SDK E2E","environment_keys":["a"],"allow_without_mtls":true,"never_expires":true}`,
	))
	createTokenRequest.Header.Set("Content-Type", "application/json")
	createTokenRequest.Header.Set("Idempotency-Key", "e2e-token-create")
	createTokenRequest = createTokenRequest.WithContext(humanauth.WithPrincipal(createTokenRequest.Context(), humanauth.Principal{
		Subject: "e2e-admin", Role: humanauth.RoleAdmin,
	}))
	createTokenResponse := httptest.NewRecorder()
	managementAPI.ServeHTTP(createTokenResponse, createTokenRequest)
	var token mysqlstore.TokenCreateResult
	if createTokenResponse.Code != http.StatusOK || json.NewDecoder(createTokenResponse.Body).Decode(&token) != nil || token.Token == "" {
		t.Fatalf("create API Token = %d %q", createTokenResponse.Code, createTokenResponse.Body.String())
	}

	server := httptest.NewTLSServer(machine.NewHandler(store, nil))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := configrago.NewClient(configrago.ClientOptions{
		BaseURL: server.URL, Token: token.Token, TLSConfig: &tls.Config{RootCAs: roots},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	file, err := client.ReadFile(ctx, "a", "platform", "redis", "tls_cert", "")
	if err != nil || string(file.Bytes) != "file-one" || file.Filename != "server.pem" {
		t.Fatalf("ReadFile = %#v, %v", file, err)
	}

	changes := make(chan *configrago.Snapshot, 1)
	handler, err := configrago.NewViperHandler(configrago.ViperHandlerOptions{
		Client: client, Environment: "a", Config: "payment",
		OnChange: func(_ context.Context, _, current *configrago.Snapshot) error {
			changes <- current
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewViperHandler: %v", err)
	}
	initial, err := handler.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if password := snapshotPassword(t, initial); password != "resolved-secret-one" {
		t.Fatalf("initial resolved password = %q", password)
	}
	if changed, err := handler.Reload(ctx); err != nil || changed {
		t.Fatalf("unchanged Reload = %v, %v", changed, err)
	}

	if _, err := store.CommitVault(ctx, mysqlstore.VaultCommit{
		OperationID: "e2e-vault-update", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis", ExpectedRevision: 1,
		Snapshot: vaultSnapshot(createdVault.VariantIDs[0], "resolved-secret-two", "file-two"),
	}); err != nil {
		t.Fatalf("update Vault Item: %v", err)
	}
	changed, err := handler.Reload(ctx)
	if err != nil || !changed {
		t.Fatalf("changed Reload = %v, %v", changed, err)
	}
	updated := <-changes
	if updated.ConfigRevision() != 1 || updated.VaultRevisions()["platform.redis"] != 2 || snapshotPassword(t, updated) != "resolved-secret-two" {
		t.Fatalf("updated Snapshot revision = %d/%v", updated.ConfigRevision(), updated.VaultRevisions())
	}
	file, err = client.ReadFile(ctx, "a", "platform", "redis", "tls_cert", "")
	if err != nil || string(file.Bytes) != "file-two" {
		t.Fatal("updated File read failed")
	}
	checkReadStatus := func(want int) {
		t.Helper()
		changed, configErr := handler.Reload(ctx)
		_, fileErr := client.ReadFile(ctx, "a", "platform", "redis", "tls_cert", file.ETag)
		if changed || handler.Current() != updated || snapshotPassword(t, handler.Current()) != "resolved-secret-two" {
			t.Fatal("permission/lifecycle change replaced the last-known-good Snapshot")
		}
		if want == 0 {
			if configErr != nil || !errors.Is(fileErr, configrago.ErrNotModified) {
				t.Fatal("restored access did not retain unchanged Config/File ETags")
			}
			return
		}
		for _, readErr := range []error{configErr, fileErr} {
			var apiError *configrago.APIError
			if !errors.As(readErr, &apiError) || apiError.StatusCode != want {
				t.Fatalf("conditional Config/File read did not return HTTP %d", want)
			}
			for _, sentinel := range []string{token.Token, "resolved-secret-two", "file-two"} {
				if strings.Contains(readErr.Error(), sentinel) {
					t.Fatal("authorization error leaked credentials or content")
				}
			}
		}
	}
	// Keep the same SDK client, handler and ETags: neither a warm connection nor
	// an unchanged resource may bypass a newly removed grant or archived Env.
	grantPath := "/v1/api-tokens/" + token.PublicID + "/environments"
	for _, step := range []struct {
		name, method, path, body string
		readStatus               int
	}{
		{"remove-grant", http.MethodPatch, grantPath, `{"remove":["a"]}`, http.StatusForbidden},
		{"restore-grant", http.MethodPatch, grantPath, `{"add":["a"]}`, 0},
		{"archive", http.MethodPost, "/v1/environments/a/archive", "", http.StatusNotFound},
		{"unarchive", http.MethodPost, "/v1/environments/a/unarchive", "", 0},
	} {
		response := auditRequest(managementAPI, humanauth.RoleAdmin, step.method, step.path, "sdk-auth-"+step.name, step.body)
		if response.Code != http.StatusOK {
			t.Fatalf("%s Management mutation: HTTP %d", step.name, response.Code)
		}
		checkReadStatus(step.readStatus)
	}

	revokeTokenRequest := httptest.NewRequest(http.MethodPost, "https://management.test/v1/api-tokens/"+token.PublicID+"/revoke", nil)
	revokeTokenRequest.Header.Set("Idempotency-Key", "e2e-token-revoke")
	revokeTokenRequest = revokeTokenRequest.WithContext(humanauth.WithPrincipal(revokeTokenRequest.Context(), humanauth.Principal{
		Subject: "e2e-admin", Role: humanauth.RoleAdmin,
	}))
	revokeTokenResponse := httptest.NewRecorder()
	managementAPI.ServeHTTP(revokeTokenResponse, revokeTokenRequest)
	if revokeTokenResponse.Code != http.StatusOK {
		t.Fatalf("revoke API Token = %d %q", revokeTokenResponse.Code, revokeTokenResponse.Body.String())
	}
	checkReadStatus(http.StatusUnauthorized)
}

func newStore(t *testing.T, ctx context.Context) *mysqlstore.Store {
	t.Helper()
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	configuration, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse MySQL Root DSN: %v", err)
	}
	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open MySQL Root connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var version string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil || version != "8.0.22" {
		t.Fatalf("MySQL version = %q, %v; want 8.0.22", version, err)
	}
	database := fmt.Sprintf("configra_sdk_e2e_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+database+" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create E2E database: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(cleanupContext, "DROP DATABASE "+database)
	})
	configuration.DBName = database
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := mysqlstore.OpenManagement(ctx, configuration.FormatDSN(), provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func vaultSnapshot(variantID, password, file string) vaultdoc.Snapshot {
	username := "redis-user"
	return vaultdoc.Snapshot{
		Fields: []vaultdoc.Field{
			{Key: "username", Name: "Username", Type: vaultdoc.Text},
			{Key: "password", Name: "Password", Type: vaultdoc.Secret},
			{Key: "tls_cert", Name: "TLS Certificate", Type: vaultdoc.File},
		},
		Variants: []vaultdoc.Variant{{
			ID: variantID, Environments: []string{"a"},
			Values: map[string]vaultdoc.Value{
				"username": {Text: &username},
				"password": {Text: &password},
				"tls_cert": {File: &vaultdoc.FileValue{Filename: "server.pem", ContentType: "application/x-pem-file", Bytes: []byte(file)}},
			},
		}},
	}
}

func snapshotPassword(t *testing.T, snapshot *configrago.Snapshot) string {
	t.Helper()
	var configuration struct {
		Database struct{ Password string }
	}
	if err := snapshot.Unmarshal(&configuration); err != nil {
		t.Fatalf("Unmarshal Snapshot: %v", err)
	}
	return configuration.Database.Password
}
