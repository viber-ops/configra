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
	page, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50}})
	tokens := page.Items
	if err != nil || page.Total != 1 || len(tokens) != 1 || tokens[0].PublicID != created.PublicID || tokens[0].DisplayName != "Datacenter A" ||
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
	if active, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50}}); err != nil || active.Total != 0 || len(active.Items) != 0 {
		t.Fatalf("active Tokens = %#v, %v", active, err)
	}
	if all, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50, IncludeInactive: true}}); err != nil || all.Total != 1 || len(all.Items) != 1 || !all.Items[0].Revoked {
		t.Fatalf("all Tokens = %#v, %v", all, err)
	}

	certificate := selfSignedClientCertificate(t, 91, "datacenter-a")
	registered, err := store.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{
		OperationID: "credential-list-certificate", Actor: actor, DisplayName: "Datacenter A", Certificate: certificate,
	})
	if err != nil {
		t.Fatalf("RegisterVerifiedClientCertificate: %v", err)
	}
	certificates, err := store.ListClientCertificates(ctx, InventoryQuery{Limit: 50})
	if err != nil || certificates.Total != 1 || len(certificates.Items) != 1 || certificates.Items[0].FingerprintSHA256 != registered.FingerprintSHA256 ||
		certificates.Items[0].Subject == "" || certificates.Items[0].Revoked {
		t.Fatalf("ListClientCertificates = %#v, %v", certificates, err)
	}
	if _, err := store.RevokeClientCertificate(ctx, ClientCertificateRevoke{
		OperationID: "credential-list-certificate-revoke", Actor: actor, FingerprintSHA256: registered.FingerprintSHA256,
	}); err != nil {
		t.Fatalf("RevokeClientCertificate: %v", err)
	}
	if active, err := store.ListClientCertificates(ctx, InventoryQuery{Limit: 50}); err != nil || active.Total != 0 || len(active.Items) != 0 {
		t.Fatalf("active Client Certificates = %#v, %v", active, err)
	}
	if all, err := store.ListClientCertificates(ctx, InventoryQuery{Limit: 50, IncludeInactive: true}); err != nil || all.Total != 1 || len(all.Items) != 1 || !all.Items[0].Revoked {
		t.Fatalf("all Client Certificates = %#v, %v", all, err)
	}
}

func TestTokenListNarrowsToAnExpiryWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_token_expiry_window")
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
		OperationID: "expiry-window-environment", Actor: actor, Action: EnvironmentCreate, Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	create := func(name string, expiresAt time.Time, never bool) string {
		created, err := store.CreateToken(ctx, TokenCreate{
			OperationID: "expiry-window-" + name, Actor: actor, DisplayName: name, EnvironmentKeys: []string{"a"},
			ExpiresAt: expiresAt, NeverExpires: never,
		})
		if err != nil {
			t.Fatalf("CreateToken %s: %v", name, err)
		}
		return created.PublicID
	}
	soon := create("soon", time.Now().Add(5*24*time.Hour), false)
	later := create("later", time.Now().Add(30*24*time.Hour), false)
	create("never", time.Time{}, true)
	expired := create("expired", time.Now().Add(24*time.Hour), false)
	if _, err := store.db.ExecContext(ctx, `UPDATE api_tokens SET expires_at = UTC_TIMESTAMP(6) - INTERVAL 2 DAY WHERE public_id = ?`, expired); err != nil {
		t.Fatalf("expire Token: %v", err)
	}
	revokedSoon := create("revoked-soon", time.Now().Add(3*24*time.Hour), false)
	if _, err := store.RevokeToken(ctx, TokenRevoke{OperationID: "expiry-window-revoke", Actor: actor, PublicID: revokedSoon}); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	page, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50}, ExpiringWithinDays: 14})
	found := map[string]bool{}
	for _, token := range page.Items {
		found[token.PublicID] = true
	}
	if err != nil || page.Total != 2 || len(page.Items) != 2 || !found[soon] || !found[expired] {
		t.Fatalf("expiring Tokens = %#v, %v", page, err)
	}
	if wide, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50}, ExpiringWithinDays: 60}); err != nil || wide.Total != 3 {
		t.Fatalf("60-day window = %#v, %v (want soon, later and expired)", wide, err)
	} else if !strings.Contains(fmtTokenIDs(wide.Items), later) {
		t.Fatalf("60-day window misses the 30-day Token: %#v", wide.Items)
	}
	for _, days := range []int{-1, MaxTokenExpiryWindowDays + 1} {
		if _, err := store.ListTokens(ctx, TokenQuery{InventoryQuery: InventoryQuery{Limit: 50}, ExpiringWithinDays: days}); err == nil {
			t.Fatalf("ExpiringWithinDays %d was accepted", days)
		}
	}
}

func fmtTokenIDs(tokens []TokenSummary) string {
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.PublicID)
	}
	return strings.Join(ids, ",")
}
