//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestVaultManagementReadsPreserveHistoricalNamesStructureAndValues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_vault_management")
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
		OperationID: "vault-management-environment", Actor: actor, Action: EnvironmentCreate,
		Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	passwordOne := "secret-one"
	created, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "vault-management-create", Actor: actor, ItemKey: "redis", ItemName: "Redis One",
		NamespaceKey: "platform",
		Snapshot: vaultdoc.Snapshot{
			Fields: []vaultdoc.Field{{Key: "password", Name: "Password One", Type: vaultdoc.Secret}},
			Variants: []vaultdoc.Variant{{Environments: []string{"a"}, Values: map[string]vaultdoc.Value{
				"password": {Text: &passwordOne},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("create Vault Item: %v", err)
	}
	passwordTwo := "secret-two"
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "vault-management-update", Actor: actor, ItemKey: "redis", ItemName: "Redis Two", ExpectedRevision: 1,
		NamespaceKey: "platform",
		Snapshot: vaultdoc.Snapshot{
			Fields: []vaultdoc.Field{{Key: "password", Name: "Password Two", Type: vaultdoc.Secret}},
			Variants: []vaultdoc.Variant{{ID: created.VariantIDs[0], Environments: []string{"a"}, Values: map[string]vaultdoc.Value{
				"password": {Text: &passwordTwo},
			}}},
		},
	}); err != nil {
		t.Fatalf("update Vault Item: %v", err)
	}

	items, err := store.ListVaultItems(ctx, false)
	if err != nil || len(items) != 1 || items[0].Key != "redis" || items[0].DisplayName != "Redis Two" || items[0].Revision != 2 || items[0].Archived ||
		len(items[0].EnvironmentKeys) != 1 || items[0].EnvironmentKeys[0] != "a" {
		t.Fatalf("ListVaultItems = %#v, %v", items, err)
	}
	current, err := store.ReadVaultItem(ctx, "platform", "redis", 0, true)
	if err != nil || current.DisplayName != "Redis Two" || current.Revision != 2 ||
		len(current.Snapshot.Fields) != 1 || current.Snapshot.Fields[0].Name != "Password Two" ||
		*current.Snapshot.Variants[0].Values["password"].Text != "secret-two" {
		t.Fatalf("current Vault Item = %#v, %v", current, err)
	}
	historical, err := store.ReadVaultItem(ctx, "platform", "redis", 1, true)
	if err != nil || historical.DisplayName != "Redis One" || historical.Revision != 1 ||
		historical.Snapshot.Fields[0].Name != "Password One" ||
		*historical.Snapshot.Variants[0].Values["password"].Text != "secret-one" {
		t.Fatalf("historical Vault Item = %#v, %v", historical, err)
	}
	metadata, err := store.ReadVaultItem(ctx, "platform", "redis", 0, false)
	if err != nil || metadata.Snapshot.Variants[0].Values != nil {
		t.Fatalf("Vault metadata = %#v, %v", metadata, err)
	}
	history, err := store.ListVaultRevisions(ctx, "platform", "redis")
	if err != nil || len(history) != 2 || history[0].Revision != 2 || history[1].Revision != 1 ||
		history[0].OperationID != "vault-management-update" || history[0].ActorID != actor.ID {
		t.Fatalf("ListVaultRevisions = %#v, %v", history, err)
	}
	restored, err := store.RestoreVault(ctx, VaultRestore{
		OperationID: "vault-management-restore", Actor: actor, ItemKey: "redis",
		NamespaceKey:   "platform",
		SourceRevision: 1, ExpectedRevision: 2,
	})
	if err != nil || restored.Outcome != OutcomeSuccess || restored.Revision != 3 {
		t.Fatalf("RestoreVault = %#v, %v", restored, err)
	}
	current, err = store.ReadVaultItem(ctx, "platform", "redis", 0, true)
	if err != nil || current.DisplayName != "Redis One" || current.Snapshot.Fields[0].Name != "Password One" ||
		*current.Snapshot.Variants[0].Values["password"].Text != "secret-one" {
		t.Fatalf("restored Vault Item = %#v, %v", current, err)
	}
	history, err = store.ListVaultRevisions(ctx, "platform", "redis")
	if err != nil || len(history) != 3 || history[0].RestoredFromRevision == nil || *history[0].RestoredFromRevision != 1 {
		t.Fatalf("restored Vault history = %#v, %v", history, err)
	}

	archived, err := store.ApplyVaultLifecycleChange(ctx, VaultLifecycleChange{
		OperationID: "vault-management-archive", Actor: actor, Action: VaultArchive, NamespaceKey: "platform", Key: "redis",
	})
	if err != nil || !archived.Archived {
		t.Fatalf("Archive Vault Item = %#v, %v", archived, err)
	}
	if active, err := store.ListVaultItems(ctx, false); err != nil || len(active) != 0 {
		t.Fatalf("active Vault Items = %#v, %v", active, err)
	}
	if all, err := store.ListVaultItems(ctx, true); err != nil || len(all) != 1 || !all[0].Archived {
		t.Fatalf("all Vault Items = %#v, %v", all, err)
	}
	if _, err := store.ReadVaultItem(ctx, "platform", "redis", 0, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Archived Vault values read = %v, want ErrNotFound", err)
	}
	archivedMetadata, err := store.ReadVaultItem(ctx, "platform", "redis", 0, false)
	if err != nil || !archivedMetadata.Archived || archivedMetadata.Snapshot.Variants[0].Values != nil {
		t.Fatalf("Archived Vault metadata = %#v, %v", archivedMetadata, err)
	}
	if _, err := store.ApplyVaultLifecycleChange(ctx, VaultLifecycleChange{
		OperationID: "vault-management-unarchive", Actor: actor, Action: VaultUnarchive, NamespaceKey: "platform", Key: "redis",
	}); err != nil {
		t.Fatalf("Unarchive Vault Item: %v", err)
	}
}

func TestListVaultUsagesReturnsOnlyActiveCurrentConfigReferences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_vault_usages")
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
	for _, environment := range []string{"a", "b"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
			OperationID: "vault-usage-environment-" + environment, Actor: actor, Action: EnvironmentCreate,
			Key: environment, DisplayName: "Environment " + environment,
		}); err != nil {
			t.Fatalf("create Environment %s: %v", environment, err)
		}
	}
	password := "secret-never-returned"
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "vault-usage-item", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis",
		Snapshot: vaultdoc.Snapshot{
			Fields: []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}, {Key: "username", Name: "Username", Type: vaultdoc.Text}},
			Variants: []vaultdoc.Variant{{Environments: []string{"a"}, Values: map[string]vaultdoc.Value{
				"password": {Text: &password}, "username": {Text: &password},
			}}},
		},
	}); err != nil {
		t.Fatalf("create Vault Item: %v", err)
	}
	commit := func(operation, environment, key, name, content string, expected uint64) {
		t.Helper()
		if _, err := store.CommitConfig(ctx, ConfigCommit{
			OperationID: operation, Actor: actor, EnvironmentKey: environment, ConfigKey: key, ConfigName: name,
			ExpectedRevision: expected, Format: configdoc.YAML, Content: []byte(content),
		}); err != nil {
			t.Fatalf("commit Config %s: %v", key, err)
		}
	}
	commit("vault-usage-payment-v1", "a", "payment", "Payment", "password: '{vault.platform.redis.password}'\n", 0)
	commit("vault-usage-payment-v2", "a", "payment", "Payment", "username: '{vault.platform.redis.username}'\n", 1)
	commit("vault-usage-archived", "a", "archived", "Archived", "password: '{vault.platform.redis.password}'\n", 0)
	if _, err := store.ApplyConfigLifecycleChange(ctx, ConfigLifecycleChange{OperationID: "vault-usage-archive-config", Actor: actor, Action: ConfigArchive, Key: "archived"}); err != nil {
		t.Fatalf("archive Config: %v", err)
	}
	commit("vault-usage-archived-environment", "b", "recovery", "Recovery", "password: '{vault.platform.redis.password}'\n", 0)
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "vault-usage-archive-environment", Actor: actor, Action: EnvironmentArchive, Key: "b"}); err != nil {
		t.Fatalf("archive Environment: %v", err)
	}
	commit("vault-usage-other-namespace", "a", "billing", "Billing", "password: '{vault.cloud.redis.password}'\n", 0)

	usages, err := store.ListVaultUsages(ctx, "platform", "redis")
	if err != nil || len(usages) != 1 || usages[0].FieldKey != "username" || usages[0].EnvironmentKey != "a" ||
		usages[0].ConfigKey != "payment" || usages[0].ConfigName != "Payment" || usages[0].ConfigRevision != 2 {
		t.Fatalf("ListVaultUsages = %#v, %v", usages, err)
	}
	if _, err := store.ListVaultUsages(ctx, "platform", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Vault Item = %v, want ErrNotFound", err)
	}
}
