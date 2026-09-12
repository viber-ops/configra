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
)

func TestConfigMergeReplaceAndRestoreUseImmutableSourceRevisions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_transfer")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO environments (id, resource_key, display_name)
		VALUES (?, 'a', 'Environment A'), (?, 'b', 'Environment B')
	`, repeatedID(1), repeatedID(2)); err != nil {
		t.Fatalf("seed Environments: %v", err)
	}
	actor := Actor{Type: "user", ID: "admin@example.com"}
	for _, seed := range []ConfigCommit{
		{
			OperationID: "operation-transfer-source", Actor: actor,
			EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
			Format: configdoc.YAML, Content: []byte("database:\n  host: source\n  ports: [2]\nsource_only: true\n"),
		},
		{
			OperationID: "operation-transfer-target", Actor: actor,
			EnvironmentKey: "b", ConfigKey: "payment", ConfigName: "Payment",
			Format: configdoc.YAML, Content: []byte("# target\ndatabase:\n  host: target\n  ports: [1]\n  target_only: keep\ntarget_only: yes\n"),
		},
	} {
		if _, err := store.CommitConfig(ctx, seed); err != nil {
			t.Fatalf("seed Config: %v", err)
		}
	}
	targetUpdate := ConfigCommit{
		OperationID: "operation-transfer-target-update", Actor: actor,
		EnvironmentKey: "b", ConfigKey: "payment", ConfigName: "Payment", ExpectedRevision: 1,
		Format: configdoc.YAML, Content: []byte("# current target\ndatabase:\n  host: newer-target\ncurrent_only: discard\n"),
	}
	if result, err := store.CommitConfig(ctx, targetUpdate); err != nil || result.Revision != 2 {
		t.Fatalf("update target Config = %#v, %v", result, err)
	}

	transfer := ConfigTransfer{
		OperationID: "operation-config-merge", Actor: actor,
		SourceEnvironmentKey: "a", SourceConfigKey: "payment", SourceRevision: 1,
		TargetEnvironmentKey: "b", TargetConfigKey: "payment", TargetRevision: 1, ExpectedTargetRevision: 2,
	}
	merged, err := store.MergeConfig(ctx, transfer)
	if err != nil || merged.Outcome != OutcomeSuccess || merged.Revision != 3 {
		t.Fatalf("MergeConfig = %#v, %v", merged, err)
	}
	wantMerged := "# target\ndatabase:\n  host: source\n  ports: [2]\n  target_only: keep\ntarget_only: yes\nsource_only: true\n"
	assertRawConfig(t, ctx, store, "b", "payment", 3, wantMerged)
	assertRawConfig(t, ctx, store, "a", "payment", 1, "database:\n  host: source\n  ports: [2]\nsource_only: true\n")

	stale := transfer
	stale.OperationID = "operation-config-merge-stale"
	if _, err := store.MergeConfig(ctx, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale MergeConfig error = %v, want ErrConflict", err)
	}

	replace := transfer
	replace.OperationID = "operation-config-replace"
	replace.ExpectedTargetRevision = 3
	replaced, err := store.ReplaceConfig(ctx, replace)
	if err != nil || replaced.Outcome != OutcomeSuccess || replaced.Revision != 4 {
		t.Fatalf("ReplaceConfig = %#v, %v", replaced, err)
	}
	assertRawConfig(t, ctx, store, "b", "payment", 4, "database:\n  host: source\n  ports: [2]\nsource_only: true\n")

	restore := ConfigRestore{
		OperationID: "operation-config-restore", Actor: actor,
		EnvironmentKey: "b", ConfigKey: "payment", SourceRevision: 3, ExpectedRevision: 4,
	}
	restored, err := store.RestoreConfig(ctx, restore)
	if err != nil || restored.Outcome != OutcomeSuccess || restored.Revision != 5 {
		t.Fatalf("RestoreConfig = %#v, %v", restored, err)
	}
	assertRawConfig(t, ctx, store, "b", "payment", 5, wantMerged)

	noChange := restore
	noChange.OperationID = "operation-config-restore-no-change"
	noChange.SourceRevision = 5
	noChange.ExpectedRevision = 5
	unchanged, err := store.RestoreConfig(ctx, noChange)
	if err != nil || unchanged.Outcome != OutcomeNoChange || unchanged.Revision != 5 {
		t.Fatalf("no-change RestoreConfig = %#v, %v", unchanged, err)
	}

	var sourceConfigID, sourceEnvironmentID, recordedSourceConfigID, recordedSourceEnvironmentID []byte
	var recordedSourceRevision uint64
	if err := store.db.QueryRowContext(ctx, `
		SELECT c.id, e.id
		FROM configs c JOIN environments e ON e.resource_key = 'a'
		WHERE c.resource_key = 'payment'
	`).Scan(&sourceConfigID, &sourceEnvironmentID); err != nil {
		t.Fatalf("read Source identity: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT source_config_id, source_environment_id, source_revision
		FROM config_revisions cr
		JOIN configs c ON c.id = cr.config_id
		JOIN environments e ON e.id = cr.environment_id
		WHERE c.resource_key = 'payment' AND e.resource_key = 'b' AND cr.revision = 3
	`).Scan(&recordedSourceConfigID, &recordedSourceEnvironmentID, &recordedSourceRevision); err != nil {
		t.Fatalf("read Merge provenance: %v", err)
	}
	if string(recordedSourceConfigID) != string(sourceConfigID) || string(recordedSourceEnvironmentID) != string(sourceEnvironmentID) || recordedSourceRevision != 1 {
		t.Fatalf("Merge provenance = %x/%x@%d, want %x/%x@1", recordedSourceConfigID, recordedSourceEnvironmentID, recordedSourceRevision, sourceConfigID, sourceEnvironmentID)
	}

	audits, err := store.ClaimOutbox(ctx, OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatalf("Claim Audit Outbox: %v", err)
	}
	if len(audits) != 8 {
		t.Fatalf("Audit Outbox Events = %d, want 8", len(audits))
	}
	for _, event := range audits {
		if strings.Contains(string(event.Payload), "source_only") || strings.Contains(string(event.Payload), "target_only") {
			t.Fatalf("Audit Event leaked Config content: %s", event.Payload)
		}
	}
}

func TestCloneConfigCopiesCurrentSnapshotOnceIntoANewConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_clone")
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
	for index, key := range []string{"a", "b"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
			OperationID: "operation-clone-environment-" + key,
			Actor:       actor,
			Action:      EnvironmentCreate,
			Key:         key,
			DisplayName: "Environment " + string(rune('A'+index)),
		}); err != nil {
			t.Fatalf("create Environment %s: %v", key, err)
		}
	}
	create := ConfigCommit{
		OperationID: "operation-clone-source-create", Actor: actor,
		EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		Format: configdoc.YAML, Content: []byte("port: 6379\n"),
	}
	if _, err := store.CommitConfig(ctx, create); err != nil {
		t.Fatalf("create Source Config: %v", err)
	}
	update := create
	update.OperationID = "operation-clone-source-update"
	update.ExpectedRevision = 1
	update.Content = []byte("port: 6380\n")
	if _, err := store.CommitConfig(ctx, update); err != nil {
		t.Fatalf("update Source Config: %v", err)
	}

	request := ConfigClone{
		OperationID: "operation-config-clone", Actor: actor,
		SourceEnvironmentKey: "a", SourceConfigKey: "payment",
		TargetEnvironmentKey: "b", TargetConfigKey: "payment_copy", TargetConfigName: "Payment Copy",
	}
	cloned, err := store.CloneConfig(ctx, request)
	if err != nil || cloned.Outcome != OutcomeSuccess || cloned.Revision != 1 {
		t.Fatalf("CloneConfig = %#v, %v", cloned, err)
	}
	target, err := store.ReadResolvedConfig(ctx, "b", "payment_copy", "")
	if err != nil || target.ConfigRevision != 1 || target.Content != "port: 6380\n" {
		t.Fatalf("cloned Config = %#v, %v", target, err)
	}

	update.OperationID = "operation-clone-source-update-again"
	update.ExpectedRevision = 2
	update.Content = []byte("port: 6381\n")
	if _, err := store.CommitConfig(ctx, update); err != nil {
		t.Fatalf("update Source after Clone: %v", err)
	}
	replayed, err := store.CloneConfig(ctx, request)
	if err != nil || !reflect.DeepEqual(replayed, cloned) {
		t.Fatalf("Clone replay after Source changed = %#v, %v; want %#v", replayed, err, cloned)
	}
	target, err = store.ReadResolvedConfig(ctx, "b", "payment_copy", "")
	if err != nil || target.ConfigRevision != 1 || target.Content != "port: 6380\n" {
		t.Fatalf("Clone replay changed Target = %#v, %v", target, err)
	}
	source, err := store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || source.ConfigRevision != 3 || source.Content != "port: 6381\n" {
		t.Fatalf("Clone changed Source = %#v, %v", source, err)
	}

	reused := request
	reused.TargetConfigKey = "another_copy"
	if _, err := store.CloneConfig(ctx, reused); !errors.Is(err, ErrOperationReuse) {
		t.Fatalf("Clone OperationID reuse = %v, want ErrOperationReuse", err)
	}
	conflict := request
	conflict.OperationID = "operation-config-clone-existing"
	conflict.TargetConfigKey = "payment"
	if _, err := store.CloneConfig(ctx, conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("Clone to existing Config = %v, want ErrConflict", err)
	}
}

func assertRawConfig(t *testing.T, ctx context.Context, store *Store, environmentKey, configKey string, revision uint64, want string) {
	t.Helper()
	var content []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT cr.content
		FROM config_revisions cr
		JOIN configs c ON c.id = cr.config_id
		JOIN environments e ON e.id = cr.environment_id
		WHERE c.resource_key = ? AND e.resource_key = ? AND cr.revision = ?
	`, configKey, environmentKey, revision).Scan(&content); err != nil {
		t.Fatalf("read %s/%s@%d: %v", environmentKey, configKey, revision, err)
	}
	if string(content) != want {
		t.Fatalf("%s/%s@%d =\n%s\nwant:\n%s", environmentKey, configKey, revision, content, want)
	}
}
