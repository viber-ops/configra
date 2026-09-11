//go:build integration

package mysqlstore

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestTokenManagementScopesEnvironmentsSupportsTokenOnlyAndRevokes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_token_mutation")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "admin@example.com"}
	for _, key := range []string{"a", "b"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
			OperationID: "operation-token-environment-" + key, Actor: actor, Action: EnvironmentCreate,
			Key: key, DisplayName: "Environment " + strings.ToUpper(key),
		}); err != nil {
			t.Fatalf("create Environment %s: %v", key, err)
		}
		if _, err := store.CommitConfig(ctx, ConfigCommit{
			OperationID: "operation-token-config-" + key, Actor: actor, EnvironmentKey: key,
			ConfigKey: "payment", ConfigName: "Payment", ExpectedRevision: 0,
			Format: configdoc.YAML, Content: []byte("environment: " + key + "\n"),
		}); err != nil {
			t.Fatalf("create Config in Environment %s: %v", key, err)
		}
	}
	certificateDER := []byte("registered-client-certificate")
	certificateFingerprint := sha256.Sum256(certificateDER)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO client_certificates
			(id, fingerprint_sha256, display_name, certificate_der, subject, serial_hex, not_before, not_after)
		VALUES (?, ?, 'Test Certificate', ?, 'CN=test', '01', '2020-01-01', '2100-01-01')
	`, repeatedID(9), certificateFingerprint[:], certificateDER); err != nil {
		t.Fatalf("seed Client Certificate: %v", err)
	}

	created, err := store.CreateToken(ctx, TokenCreate{
		OperationID: "operation-token-create", Actor: actor, DisplayName: "Datacenter Client",
		EnvironmentKeys: []string{"a"}, ExpiresAt: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || created.Outcome != OutcomeSuccess || !strings.HasPrefix(created.Token, "cfg_"+created.PublicID+"_") {
		t.Fatalf("CreateToken = %#v, %v", created, err)
	}
	replayed, err := store.CreateToken(ctx, TokenCreate{
		OperationID: "operation-token-create", Actor: actor, DisplayName: "Datacenter Client",
		EnvironmentKeys: []string{"a"}, ExpiresAt: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || replayed.PublicID != created.PublicID || replayed.Token != "" {
		t.Fatalf("CreateToken replay = %#v, %v; plaintext must only be shown once", replayed, err)
	}
	handler := machine.NewHandler(store, nil)
	request := func(environment, token string, tlsState *tls.ConnectionState) *httptest.ResponseRecorder {
		httpRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/"+environment+"/configs/payment", nil)
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		httpRequest.TLS = tlsState
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	registeredTLS := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: certificateDER}}}
	if response := request("a", created.Token, registeredTLS); response.Code != http.StatusOK {
		t.Fatalf("authorized Environment a = %d %s", response.Code, response.Body)
	}
	if response := request("b", created.Token, registeredTLS); response.Code != http.StatusForbidden {
		t.Fatalf("unauthorized Environment b = %d %s", response.Code, response.Body)
	}

	updated, err := store.SetTokenEnvironments(ctx, TokenEnvironmentChange{
		OperationID: "operation-token-environments", Actor: actor, PublicID: created.PublicID, EnvironmentKeys: []string{"b"},
	})
	if err != nil || updated.Outcome != OutcomeSuccess || len(updated.EnvironmentKeys) != 1 || updated.EnvironmentKeys[0] != "b" {
		t.Fatalf("SetTokenEnvironments = %#v, %v", updated, err)
	}
	if response := request("a", created.Token, registeredTLS); response.Code != http.StatusForbidden {
		t.Fatalf("removed Environment a grant = %d %s", response.Code, response.Body)
	}
	if response := request("b", created.Token, registeredTLS); response.Code != http.StatusOK {
		t.Fatalf("new Environment b grant = %d %s", response.Code, response.Body)
	}

	tokenOnly, err := store.CreateToken(ctx, TokenCreate{
		OperationID: "operation-token-only", Actor: actor, DisplayName: "Token-only Client",
		EnvironmentKeys: []string{"a"}, AllowWithoutMTLS: true, NeverExpires: true,
	})
	if err != nil {
		t.Fatalf("CreateToken token-only: %v", err)
	}
	if response := request("a", tokenOnly.Token, &tls.ConnectionState{}); response.Code != http.StatusOK {
		t.Fatalf("Token-only request = %d %s", response.Code, response.Body)
	}

	revoked, err := store.RevokeToken(ctx, TokenRevoke{OperationID: "operation-token-revoke", Actor: actor, PublicID: created.PublicID})
	if err != nil || revoked.Outcome != OutcomeSuccess || !revoked.Revoked {
		t.Fatalf("RevokeToken = %#v, %v", revoked, err)
	}
	if response := request("b", created.Token, registeredTLS); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked Token request = %d %s", response.Code, response.Body)
	}
}
