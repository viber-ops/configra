//go:build integration

package mysqlstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestCredentialListsExposeMetadataAndFilterRevokedRecords(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_credential_read")
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
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
		OperationID: "credential-list-environment", Actor: actor, Action: EnvironmentCreate,
		Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	created, err := store.CreateToken(ctx, TokenCreate{
		OperationID: "credential-list-token", Actor: actor, DisplayName: "Datacenter A",
		EnvironmentKeys: []string{"a"}, AllowWithoutMTLS: true, NeverExpires: true,
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	tokens, err := store.ListTokens(ctx, false)
	if err != nil || len(tokens) != 1 || tokens[0].PublicID != created.PublicID || tokens[0].DisplayName != "Datacenter A" ||
		len(tokens[0].EnvironmentKeys) != 1 || tokens[0].EnvironmentKeys[0] != "a" || !tokens[0].AllowWithoutMTLS || tokens[0].Revoked {
		t.Fatalf("ListTokens = %#v, %v", tokens, err)
	}
	encoded, err := json.Marshal(tokens)
	if err != nil || strings.Contains(string(encoded), created.Token) || strings.Contains(string(encoded), "secret_digest") {
		t.Fatalf("Token metadata leaked credential material: %s, %v", encoded, err)
	}
	if _, err := store.RevokeToken(ctx, TokenRevoke{OperationID: "credential-list-token-revoke", Actor: actor, PublicID: created.PublicID}); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if active, err := store.ListTokens(ctx, false); err != nil || len(active) != 0 {
		t.Fatalf("active Tokens = %#v, %v", active, err)
	}
	if all, err := store.ListTokens(ctx, true); err != nil || len(all) != 1 || !all[0].Revoked {
		t.Fatalf("all Tokens = %#v, %v", all, err)
	}

	certificate := selfSignedClientCertificate(t, 91, "datacenter-a")
	registered, err := store.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{
		OperationID: "credential-list-certificate", Actor: actor, DisplayName: "Datacenter A", Certificate: certificate,
	})
	if err != nil {
		t.Fatalf("RegisterVerifiedClientCertificate: %v", err)
	}
	certificates, err := store.ListClientCertificates(ctx, false)
	if err != nil || len(certificates) != 1 || certificates[0].FingerprintSHA256 != registered.FingerprintSHA256 ||
		certificates[0].Subject == "" || certificates[0].Revoked {
		t.Fatalf("ListClientCertificates = %#v, %v", certificates, err)
	}
	if _, err := store.RevokeClientCertificate(ctx, ClientCertificateRevoke{
		OperationID: "credential-list-certificate-revoke", Actor: actor, FingerprintSHA256: registered.FingerprintSHA256,
	}); err != nil {
		t.Fatalf("RevokeClientCertificate: %v", err)
	}
	if active, err := store.ListClientCertificates(ctx, false); err != nil || len(active) != 0 {
		t.Fatalf("active Client Certificates = %#v, %v", active, err)
	}
	if all, err := store.ListClientCertificates(ctx, true); err != nil || len(all) != 1 || !all[0].Revoked {
		t.Fatalf("all Client Certificates = %#v, %v", all, err)
	}
}
