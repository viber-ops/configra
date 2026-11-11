//go:build integration

package mysqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// Searching Vault Items also matches the current revision's active Field keys (#51).
func TestVaultSearchMatchesCurrentFieldKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_vault_field_search")
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
		OperationID: "field-search-environment", Actor: actor, Action: EnvironmentCreate, Key: "hk47", DisplayName: "HK 47",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	commit := func(operation string, revision uint64, keys ...string) {
		t.Helper()
		fields := make([]vaultdoc.Field, 0, len(keys))
		values := map[string]vaultdoc.Value{}
		for _, key := range keys {
			value := "https://example.com/" + key
			fields = append(fields, vaultdoc.Field{Key: key, Name: key, Type: vaultdoc.Text})
			values[key] = vaultdoc.Value{Text: &value}
		}
		if _, err := store.CommitVault(ctx, VaultCommit{
			OperationID: operation, Actor: actor, NamespaceKey: "order_service", ItemKey: "clients-conf", ItemName: "Clients",
			ExpectedRevision: revision,
			Snapshot:         vaultdoc.Snapshot{Fields: fields, Variants: []vaultdoc.Variant{{ID: "0123456789abcdef0123456789abcdef", Environments: []string{"hk47"}, Values: values}}},
		}); err != nil {
			t.Fatalf("commit Vault Item: %v", err)
		}
	}
	search := func(text string) uint64 {
		t.Helper()
		page, err := store.ListVaultItems(ctx, VaultQuery{InventoryQuery: InventoryQuery{Limit: 50, Search: text}})
		if err != nil {
			t.Fatalf("ListVaultItems(%q): %v", text, err)
		}
		return page.Total
	}

	commit("field-search-create", 0, "lvl-data", "lvl-download", "google-login")
	for text, want := range map[string]uint64{"DOWNLOAD": 1, "google-login": 1, "order_service.clients": 1, "Clients": 1, "payments": 0} {
		if got := search(text); got != want {
			t.Fatalf("search %q found %d Items, want %d", text, got, want)
		}
	}
	// Only the current revision's active Fields count.
	commit("field-search-remove", 1, "lvl-data", "lvl-download")
	if got := search("google-login"); got != 0 {
		t.Fatalf("a removed Field still matches: %d", got)
	}
}
