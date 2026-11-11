//go:build integration

package mysqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// v1.4.0 reads Fields in the order they were added (#54). A scoped write that changes only its own Environment's
// values must still be recognized as leaving the Fields alone, so a token granted one Environment can write to an Item
// that other Environments share, whatever order the Fields are in.
func TestScopedValueWriteIgnoresFieldOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	admin := Actor{Type: "user", ID: "order-admin"}
	for _, key := range []string{"dev", "prod"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "order-env-" + key, Actor: admin, Action: EnvironmentCreate, Key: key, DisplayName: key}); err != nil {
			t.Fatal(err)
		}
	}
	issued, err := store.CreateToken(ctx, TokenCreate{OperationID: "order-writer", Actor: admin, Kind: machine.TokenWriteScoped, DisplayName: "Writer", EnvironmentKeys: []string{"dev"}, NamespaceKeys: []string{"ops"}, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	writer := Actor{Type: "token", ID: issued.PublicID}
	text := func(value string) vaultdoc.Value { return vaultdoc.Value{Text: &value} }
	// Neither key order nor its reverse.
	fields := []vaultdoc.Field{{Key: "url", Name: "URL", Type: vaultdoc.Text}, {Key: "company", Name: "Company", Type: vaultdoc.Text}, {Key: "password", Name: "Password", Type: vaultdoc.Secret}}
	values := func(prefix string) map[string]vaultdoc.Value {
		return map[string]vaultdoc.Value{"url": text("http://" + prefix), "company": text(prefix), "password": text(prefix + "-secret")}
	}
	if _, err := store.CommitVault(ctx, VaultCommit{OperationID: "order-create", Actor: admin, NamespaceKey: "ops", ItemKey: "app", ItemName: "App", Snapshot: vaultdoc.Snapshot{
		Fields: fields, Variants: []vaultdoc.Variant{{Environments: []string{"dev"}, Values: values("dev")}, {Environments: []string{"prod"}, Values: values("prod")}},
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "order-put-field", Actor: writer, EnvironmentKey: "dev", NamespaceKey: "ops", ItemKey: "app", ExpectedRevision: 1, Action: "put_field", Field: fields[0], Value: text("http://dev-2")})
	if err != nil || result.Revision != 2 {
		t.Fatal("dev-only token could not change a dev value", result, err)
	}
	// The same Fields listed in another order are still the same structure, and do not reorder the shared Item.
	permuted := []vaultdoc.Field{fields[2], fields[0], fields[1]}
	result, err = store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "order-put-item", Actor: writer, EnvironmentKey: "dev", NamespaceKey: "ops", ItemKey: "app", ExpectedRevision: 2, Action: "put_item", Fields: permuted, Values: values("dev-3")})
	if err != nil || result.Revision != 3 {
		t.Fatal("dev-only token could not replace dev values", result, err)
	}
	item, err := store.ReadVaultItem(ctx, "ops", "app", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range item.Snapshot.Variants {
		want := "http://dev-3"
		if variant.Environments[0] == "prod" {
			want = "http://prod"
		}
		if got := *variant.Values["url"].Text; got != want {
			t.Fatalf("%v url = %q, want %q", variant.Environments, got, want)
		}
	}
	keys := make([]string, 0, len(item.Snapshot.Fields))
	for _, field := range item.Snapshot.Fields {
		keys = append(keys, field.Key)
	}
	if len(keys) != 3 || keys[0] != "url" || keys[1] != "company" || keys[2] != "password" {
		t.Fatal("scoped writes changed the Field order", keys)
	}

	// A real structure change still needs every Environment the Item is bound to.
	if _, err := store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "order-rename", Actor: writer, EnvironmentKey: "dev", NamespaceKey: "ops", ItemKey: "app", ExpectedRevision: 3, Action: "put_field", Field: vaultdoc.Field{Key: "url", Name: "Service URL", Type: vaultdoc.Text}, Value: text("http://dev-3")}); err != machine.ErrForbidden {
		t.Fatal("renaming a shared Field with a dev-only token", err)
	}
}
