//go:build integration

package mysqlstore

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// Single-line Text values are stored without edge whitespace on every write path (#62). Multi-line Text, Secret and
// File values, values a scoped write did not submit, and restored revisions keep their exact bytes.
func TestVaultTextValuesAreStoredWithoutEdgeWhitespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	admin := Actor{Type: "user", ID: "trim-admin"}
	for _, key := range []string{"dev", "prod"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "trim-env-" + key, Actor: admin, Action: EnvironmentCreate, Key: key, DisplayName: key}); err != nil {
			t.Fatal(err)
		}
	}
	issued, err := store.CreateToken(ctx, TokenCreate{OperationID: "trim-writer", Actor: admin, Kind: machine.TokenWriteScoped, DisplayName: "Writer", EnvironmentKeys: []string{"dev"}, NamespaceKeys: []string{"ops"}, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	writer := Actor{Type: "token", ID: issued.PublicID}

	const certificate = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	text := func(value string) vaultdoc.Value { return vaultdoc.Value{Text: &value} }
	fields := []vaultdoc.Field{
		{Key: "url", Name: "URL", Type: vaultdoc.Text},
		{Key: "company", Name: "Company", Type: vaultdoc.Text},
		{Key: "certificate", Name: "Certificate", Type: vaultdoc.Text},
		{Key: "blank", Name: "Blank", Type: vaultdoc.Text},
		{Key: "password", Name: "Password", Type: vaultdoc.Secret},
		{Key: "bundle", Name: "Bundle", Type: vaultdoc.File},
	}
	submitted := func() vaultdoc.Snapshot {
		return vaultdoc.Snapshot{Fields: fields, Variants: []vaultdoc.Variant{{Environments: []string{"dev", "prod"}, Values: map[string]vaultdoc.Value{
			"url":         text(" \t　http://service.example.com:8918 \r\n"),
			"company":     text("XOOM INTERNET LIMITED"),
			"certificate": text(certificate),
			"blank":       text(" \t\n "),
			"password":    text(" p@ss word "),
			"bundle":      {File: &vaultdoc.FileValue{Filename: "bundle.txt", ContentType: "text/plain", Bytes: []byte(" data\n")}},
		}}}}
	}
	read := func(item string, revision uint64) VaultItem {
		t.Helper()
		result, err := store.ReadVaultItem(ctx, "ops", item, revision, true)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	value := func(item VaultItem, environment, field string) vaultdoc.Value {
		t.Helper()
		for _, variant := range item.Snapshot.Variants {
			for _, key := range variant.Environments {
				if key == environment {
					return variant.Values[field]
				}
			}
		}
		t.Fatalf("no Variant covers %s", environment)
		return vaultdoc.Value{}
	}
	textOf := func(item VaultItem, environment, field string) string {
		t.Helper()
		got := value(item, environment, field)
		if got.Text == nil {
			t.Fatalf("%s/%s has no text", environment, field)
		}
		return *got.Text
	}

	create := VaultCommit{OperationID: "trim-create", Actor: admin, NamespaceKey: "ops", ItemKey: "app", ItemName: "App", Snapshot: submitted()}
	result, err := store.CommitVault(ctx, create)
	if err != nil || result.Revision != 1 {
		t.Fatal("create", result, err)
	}
	item := read("app", 0)
	for field, want := range map[string]string{
		"url":         "http://service.example.com:8918",
		"company":     "XOOM INTERNET LIMITED",
		"certificate": certificate,
		"blank":       "",
		"password":    " p@ss word ",
	} {
		if got := textOf(item, "prod", field); got != want {
			t.Fatalf("%s = %q, want %q", field, got, want)
		}
	}
	if got := value(item, "prod", "bundle").File; got == nil || !bytes.Equal(got.Bytes, []byte(" data\n")) {
		t.Fatal("File bytes changed", got)
	}
	if *create.Snapshot.Variants[0].Values["url"].Text != " \t　http://service.example.com:8918 \r\n" {
		t.Fatal("normalization wrote through the caller's request")
	}

	// The same request with the same Idempotency-Key replays; the digest covers the bytes the client sent.
	if replay, err := store.CommitVault(ctx, create); err != nil || replay.Outcome != OutcomeSuccess || replay.Revision != 1 {
		t.Fatal("replay of the untrimmed request", replay, err)
	}
	// Saving the untrimmed values again, as the editor does with the Variant ID it read, stores nothing new.
	again := submitted()
	again.Variants[0].ID = item.Snapshot.Variants[0].ID
	resave := VaultCommit{OperationID: "trim-resave", Actor: admin, NamespaceKey: "ops", ItemKey: "app", ItemName: "App", ExpectedRevision: 1, Snapshot: again}
	if result, err := store.CommitVault(ctx, resave); !errorsIsNoChange(result, err) || result.Revision != 1 {
		t.Fatal("re-save with edge whitespace", result, err)
	}
	// A scoped write of the same value with edge whitespace neither splits the shared Variant nor adds a revision.
	padded := "  http://service.example.com:8918 \n"
	if result, err := store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "trim-scoped-same", Actor: writer, EnvironmentKey: "dev", NamespaceKey: "ops", ItemKey: "app", ExpectedRevision: 1, Action: "put_field", Field: fields[0], Value: text(padded)}); !errorsIsNoChange(result, err) {
		t.Fatal("scoped write of an equal value", result, err)
	}
	if item := read("app", 0); item.Revision != 1 || len(item.Snapshot.Variants) != 1 {
		t.Fatal("scoped no-change wrote a revision or split the Variant", item.Revision, len(item.Snapshot.Variants))
	}
	if padded != "  http://service.example.com:8918 \n" {
		t.Fatal("scoped normalization wrote through the request")
	}

	// A Config referencing the Field resolves the stored value.
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "trim-config", Actor: admin, EnvironmentKey: "dev", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("url: \"{vault.ops.app.url}\"\ncompany: \"{vault.ops.app.company}\"\n")}); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ReadResolvedConfig(ctx, "dev", "server", "")
	if err != nil || resolved.Content != "url: \"http://service.example.com:8918\"\ncompany: \"XOOM INTERNET LIMITED\"\n" {
		t.Fatalf("resolved Config = %q, %v", resolved.Content, err)
	}

	// A legacy Item written before v1.4.1 keeps its values until someone saves it. action "restore" skips
	// normalization exactly as a v1.4.0 write did.
	legacy := vaultdoc.Snapshot{Fields: fields[:3], Variants: []vaultdoc.Variant{
		{Environments: []string{"dev"}, Values: map[string]vaultdoc.Value{"url": text(" http://dev.example.com "), "company": text(" Dev "), "certificate": text(certificate)}},
		{Environments: []string{"prod"}, Values: map[string]vaultdoc.Value{"url": text(" http://prod.example.com "), "company": text(" Prod "), "certificate": text(certificate)}},
	}}
	if _, err := store.CommitVault(ctx, VaultCommit{OperationID: "trim-legacy", Actor: admin, NamespaceKey: "ops", ItemKey: "legacy", ItemName: "Legacy", Snapshot: legacy, action: "restore"}); err != nil {
		t.Fatal(err)
	}
	// A scoped write normalizes only the value it submits: dev's other values and prod's Variant stay byte for byte.
	if _, err := store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "trim-scoped-edit", Actor: writer, EnvironmentKey: "dev", NamespaceKey: "ops", ItemKey: "legacy", ExpectedRevision: 1, Action: "put_field", Field: fields[1], Value: text(" Dev 2 \n")}); err != nil {
		t.Fatal(err)
	}
	item = read("legacy", 0)
	if item.Revision != 2 || textOf(item, "dev", "company") != "Dev 2" {
		t.Fatal("scoped write did not normalize the submitted value", item.Revision)
	}
	if textOf(item, "dev", "url") != " http://dev.example.com " || textOf(item, "prod", "url") != " http://prod.example.com " || textOf(item, "prod", "company") != " Prod " {
		t.Fatal("scoped write changed values it did not submit")
	}
	// An Admin save submits the whole Item, so it cleans every single-line Text value in every Variant, even one in
	// a Variant the Admin did not edit, and leaves multi-line Text unchanged.
	current := read("legacy", 0)
	current.Snapshot.Variants[0].Values["company"] = text("Development")
	if result, err := store.CommitVault(ctx, VaultCommit{OperationID: "trim-admin-edit", Actor: admin, NamespaceKey: "ops", ItemKey: "legacy", ItemName: "Legacy", ExpectedRevision: 2, Snapshot: current.Snapshot}); err != nil || result.Revision != 3 {
		t.Fatal("Admin save", result, err)
	}
	item = read("legacy", 0)
	for _, check := range []struct{ environment, field, want string }{
		{"dev", "url", "http://dev.example.com"}, {"prod", "url", "http://prod.example.com"}, {"prod", "company", "Prod"},
		{"dev", "certificate", certificate}, {"prod", "certificate", certificate},
	} {
		if got := textOf(item, check.environment, check.field); got != check.want {
			t.Fatalf("%s/%s = %q, want %q", check.environment, check.field, got, check.want)
		}
	}
	// Restore brings back the source revision exactly.
	if result, err := store.RestoreVault(ctx, VaultRestore{OperationID: "trim-restore", Actor: admin, NamespaceKey: "ops", ItemKey: "legacy", SourceRevision: 1, ExpectedRevision: 3}); err != nil || result.Revision != 4 {
		t.Fatal("restore", result, err)
	}
	item = read("legacy", 0)
	if textOf(item, "dev", "url") != " http://dev.example.com " || textOf(item, "prod", "company") != " Prod " {
		t.Fatal("restore did not keep the source revision's bytes")
	}
}

func errorsIsNoChange(result VaultCommitResult, err error) bool {
	return result.Outcome == OutcomeNoChange && (err == nil || err == outcomeError(OutcomeNoChange))
}
