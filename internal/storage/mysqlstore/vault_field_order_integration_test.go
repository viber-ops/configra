//go:build integration

package mysqlstore

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// Fields are shown in the order they were first added to an Item, not in key order (#54).
func TestVaultFieldsKeepTheOrderTheyWereAdded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_vault_field_order")
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
		OperationID: "field-order-environment", Actor: actor, Action: EnvironmentCreate, Key: "hk47", DisplayName: "HK 47",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}

	// Twenty Fields in an order that is neither alphabetical nor reverse alphabetical.
	order := []string{"host", "port", "username", "password"}
	for index := 0; len(order) < 20; index++ {
		order = append(order, fmt.Sprintf("service-%02d", (index*7)%16))
	}
	snapshot := func(keys []string, names map[string]string) vaultdoc.Snapshot {
		fields := make([]vaultdoc.Field, 0, len(keys))
		values := make(map[string]vaultdoc.Value, len(keys))
		for _, key := range keys {
			name := names[key]
			if name == "" {
				name = key
			}
			fields = append(fields, vaultdoc.Field{Key: key, Name: name, Type: vaultdoc.Text})
			value := "value-" + key
			values[key] = vaultdoc.Value{Text: &value}
		}
		return vaultdoc.Snapshot{Fields: fields, Variants: []vaultdoc.Variant{{ID: "0123456789abcdef0123456789abcdef", Environments: []string{"hk47"}, Values: values}}}
	}
	keysOf := func(revision uint64) []string {
		item, err := store.ReadVaultItem(ctx, "order", "clients", revision, false)
		if err != nil {
			t.Fatalf("ReadVaultItem(%d): %v", revision, err)
		}
		keys := make([]string, 0, len(item.Snapshot.Fields))
		for _, field := range item.Snapshot.Fields {
			keys = append(keys, field.Key)
		}
		return keys
	}

	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "field-order-create", Actor: actor, NamespaceKey: "order", ItemKey: "clients", ItemName: "Clients",
		Snapshot: snapshot(order, nil),
	}); err != nil {
		t.Fatalf("create Vault Item: %v", err)
	}
	if got := keysOf(0); !slices.Equal(got, order) {
		t.Fatalf("created order = %v, want %v", got, order)
	}

	// Renaming a Field keeps every position; a new Field goes last.
	next := append(slices.Clone(order), "database")
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "field-order-append", Actor: actor, NamespaceKey: "order", ItemKey: "clients", ItemName: "Clients",
		ExpectedRevision: 1, Snapshot: snapshot(next, map[string]string{"port": "Service port"}),
	}); err != nil {
		t.Fatalf("append Vault Field: %v", err)
	}
	if got := keysOf(0); !slices.Equal(got, next) {
		t.Fatalf("order after rename and append = %v, want %v", got, next)
	}
	if got := keysOf(1); !slices.Equal(got, order) {
		t.Fatalf("revision 1 order = %v, want %v", got, order)
	}

	// Removing a Field leaves the others where they were.
	without := slices.DeleteFunc(slices.Clone(next), func(key string) bool { return key == "username" })
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "field-order-remove", Actor: actor, NamespaceKey: "order", ItemKey: "clients", ItemName: "Clients",
		ExpectedRevision: 2, Snapshot: snapshot(without, nil),
	}); err != nil {
		t.Fatalf("remove Vault Field: %v", err)
	}
	if got := keysOf(0); !slices.Equal(got, without) {
		t.Fatalf("order after removal = %v, want %v", got, without)
	}
}
