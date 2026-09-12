//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestCommitVaultCreatesWholeEncryptedRevisionsAndResolvesByEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_vault_mutation")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, "INSERT INTO environments (id, resource_key, display_name) VALUES (?, 'a', 'Environment A'), (?, 'b', 'Environment B')", repeatedID(1), repeatedID(2)); err != nil {
		t.Fatalf("seed Environments: %v", err)
	}

	create := VaultCommit{
		OperationID:      "operation-vault-create",
		Actor:            Actor{Type: "user", ID: "admin@example.com"},
		NamespaceKey:     "platform",
		ItemKey:          "redis",
		ItemName:         "Redis",
		ExpectedRevision: 0,
		Snapshot:         testVaultSnapshot("", []string{"b", "a"}, "redis-user", "sentinel-secret-1"),
	}
	created, err := store.CommitVault(ctx, create)
	if err != nil {
		t.Fatalf("CommitVault create: %v", err)
	}
	if created.Outcome != OutcomeSuccess || created.Revision != 1 || len(created.VariantIDs) != 1 || len(created.VariantIDs[0]) != 32 {
		t.Fatalf("create result = %#v", created)
	}
	replayed, err := store.CommitVault(ctx, create)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("idempotent replay = %#v, %v; want %#v", replayed, err, created)
	}
	reused := create
	reused.Snapshot.Variants[0].Values["password"] = vaultdoc.Value{Text: stringPointer("different")}
	if _, err := store.CommitVault(ctx, reused); !errors.Is(err, ErrOperationReuse) {
		t.Fatalf("OperationID reuse error = %v, want ErrOperationReuse", err)
	}
	secondary := create
	secondary.OperationID = "operation-vault-create-secondary"
	secondary.NamespaceKey = "cloud"
	secondary.Snapshot = testVaultSnapshot("", []string{"a"}, "cloud-user", "cloud-secret")
	if result, err := store.CommitVault(ctx, secondary); err != nil || result.Revision != 1 {
		t.Fatalf("CommitVault same Item Key in second Namespace = %#v, %v", result, err)
	}

	configResult, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID:      "operation-config-with-vault",
		Actor:            create.Actor,
		EnvironmentKey:   "a",
		ConfigKey:        "payment",
		ConfigName:       "Payment",
		ExpectedRevision: 0,
		Format:           configdoc.YAML,
		Content:          []byte("database:\n  username: \"{vault.platform.redis.username}\"\n  password: \"{vault.platform.redis.password}\"\n  cloud_password: \"{vault.cloud.redis.password}\"\n"),
	})
	if err != nil || len(configResult.Warnings) != 0 {
		t.Fatalf("CommitConfig with resolvable Vault References = %#v, %v", configResult, err)
	}
	resolved, err := store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || !strings.Contains(resolved.Content, "sentinel-secret-1") || !strings.Contains(resolved.Content, "cloud-secret") ||
		resolved.VaultRevisions["platform.redis"] != 1 || resolved.VaultRevisions["cloud.redis"] != 1 {
		t.Fatalf("resolved Config = %#v, %v", resolved, err)
	}
	file, err := store.ReadFile(ctx, "b", "platform", "redis", "tls_cert", "")
	if err != nil || string(file.Bytes) != "sentinel-private-file-bytes" || file.VaultRevision != 1 {
		t.Fatalf("File = %#v, %v", file, err)
	}

	update := create
	update.OperationID = "operation-vault-update"
	update.ExpectedRevision = 1
	update.Snapshot = testVaultSnapshot(created.VariantIDs[0], []string{"a", "b"}, "redis-user", "sentinel-secret-2")
	updated, err := store.CommitVault(ctx, update)
	if err != nil || updated.Outcome != OutcomeSuccess || updated.Revision != 2 {
		t.Fatalf("update result = %#v, %v", updated, err)
	}
	resolved, err = store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || !strings.Contains(resolved.Content, "sentinel-secret-2") || !strings.Contains(resolved.Content, "cloud-secret") ||
		resolved.VaultRevisions["platform.redis"] != 2 || resolved.VaultRevisions["cloud.redis"] != 1 {
		t.Fatalf("resolved updated Config = %#v, %v", resolved, err)
	}

	noChange := update
	noChange.OperationID = "operation-vault-no-change"
	noChange.ExpectedRevision = 2
	noChange.Snapshot.Fields[0], noChange.Snapshot.Fields[2] = noChange.Snapshot.Fields[2], noChange.Snapshot.Fields[0]
	noChange.Snapshot.Variants[0].Environments = []string{"b", "a"}
	unchanged, err := store.CommitVault(ctx, noChange)
	if err != nil || unchanged.Outcome != OutcomeNoChange || unchanged.Revision != 2 {
		t.Fatalf("no-change result = %#v, %v", unchanged, err)
	}

	stale := update
	stale.OperationID = "operation-vault-conflict"
	stale.ExpectedRevision = 1
	if _, err := store.CommitVault(ctx, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v, want ErrConflict", err)
	}
	overlap := update
	overlap.OperationID = "operation-vault-overlap"
	overlap.ExpectedRevision = 2
	overlap.Snapshot.Variants = append(overlap.Snapshot.Variants, testVaultSnapshot("02020202020202020202020202020202", []string{"a"}, "other-user", "other-secret").Variants[0])
	if _, err := store.CommitVault(ctx, overlap); !errors.Is(err, ErrValidation) {
		t.Fatalf("overlapping Environment error = %v, want ErrValidation", err)
	}
	typeChange := update
	typeChange.OperationID = "operation-vault-field-type"
	typeChange.ExpectedRevision = 2
	typeChange.Snapshot.Fields[1].Type = vaultdoc.Text
	if _, err := store.CommitVault(ctx, typeChange); !errors.Is(err, ErrValidation) {
		t.Fatalf("Field type change error = %v, want ErrValidation", err)
	}
	resolved, err = store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || resolved.VaultRevisions["platform.redis"] != 2 || resolved.VaultRevisions["cloud.redis"] != 1 || !strings.Contains(resolved.Content, "sentinel-secret-2") {
		t.Fatalf("Vault changed after rejected mutations: %#v, %v", resolved, err)
	}
}

func testVaultSnapshot(variantID string, environments []string, username, password string) vaultdoc.Snapshot {
	return vaultdoc.Snapshot{
		Fields: []vaultdoc.Field{
			{Key: "username", Name: "Username", Type: vaultdoc.Text},
			{Key: "password", Name: "Password", Type: vaultdoc.Secret},
			{Key: "tls_cert", Name: "TLS Certificate", Type: vaultdoc.File},
		},
		Variants: []vaultdoc.Variant{{
			ID:           variantID,
			Environments: environments,
			Values: map[string]vaultdoc.Value{
				"username": {Text: &username},
				"password": {Text: &password},
				"tls_cert": {File: &vaultdoc.FileValue{Filename: "server.pem", ContentType: "application/x-pem-file", Bytes: []byte("sentinel-private-file-bytes")}},
			},
		}},
	}
}

func stringPointer(value string) *string { return &value }
