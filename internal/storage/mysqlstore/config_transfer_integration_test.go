//go:build integration

package mysqlstore

import (
	"context"
	"encoding/json"
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

func configCopyStore(t *testing.T) (*Store, context.Context, Actor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	dsn := createIntegrationDatabase(t, ctx, "configra_copy_regression")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "copy-admin"}
	for _, key := range []string{"test", "prod"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "copy-env-" + key, Actor: actor, Action: EnvironmentCreate, Key: key, DisplayName: key}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "copy-source", Actor: actor, EnvironmentKey: "test", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("port: 6379\npassword: '{vault.app.db.password}'\n")}); err != nil {
		t.Fatal(err)
	}
	return store, ctx, actor
}

func TestReplaceInitializesMissingEnvironmentWithoutOverwriting(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	copy := ConfigTransfer{OperationID: "copy-first", Actor: actor, SourceEnvironmentKey: "test", SourceConfigKey: "server", SourceRevision: 1, TargetEnvironmentKey: "prod", TargetConfigKey: "server"}
	result, err := store.ReplaceConfig(ctx, copy)
	if err != nil || result.Revision != 1 {
		t.Fatalf("copy test/server to empty prod = %#v, %v", result, err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "missing_vault_item" {
		t.Fatalf("target Vault warnings = %#v", result.Warnings)
	}
	assertRawConfig(t, ctx, store, "prod", "server", 1, "port: 6379\npassword: '{vault.app.db.password}'\n")
	assertRawConfig(t, ctx, store, "test", "server", 1, "port: 6379\npassword: '{vault.app.db.password}'\n")
	copy.OperationID = "copy-second"
	for range 2 {
		result, err = store.ReplaceConfig(ctx, copy)
		encoded, _ := json.Marshal(result)
		if !errors.Is(err, ErrConflict) || !strings.Contains(string(encoded), `"reason":"target_config_exists"`) {
			t.Fatalf("copy must explain existing target, including replay: %s, %v", encoded, err)
		}
	}
	var revisions, sourceRevision, audits int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(source_revision) FROM config_revisions cr JOIN environments e ON e.id = cr.environment_id WHERE e.resource_key = 'prod'`).Scan(&revisions, &sourceRevision); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 || sourceRevision != 1 {
		t.Fatalf("target revisions/provenance = %d/%d", revisions, sourceRevision)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE operation_id IN ('copy-first', 'copy-second') AND kind = 'audit'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("copy audits = %d, want 2", audits)
	}
}

func TestConfigTransferReplaySurvivesSourceArchiveAndBindsTargetRevision(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	for rev := uint64(0); rev < 2; rev++ {
		if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "prod-seed-" + string(rune('a'+rev)), Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("port: " + string(rune('1'+rev)) + "\n"), ExpectedRevision: rev}); err != nil {
			t.Fatal(err)
		}
	}
	req := ConfigTransfer{OperationID: "replace-replay", Actor: actor, SourceEnvironmentKey: "test", SourceConfigKey: "server", SourceRevision: 1, TargetEnvironmentKey: "prod", TargetConfigKey: "server", TargetRevision: 1, ExpectedTargetRevision: 2}
	first, err := store.ReplaceConfig(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	changed := req
	changed.TargetRevision = 2
	if _, err := store.ReplaceConfig(ctx, changed); !errors.Is(err, ErrOperationReuse) {
		t.Errorf("changed target revision must reject reused operation: %v", err)
	}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "archive-source-env", Actor: actor, Action: EnvironmentArchive, Key: "test"}); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ReplaceConfig(ctx, req)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Errorf("replay after source archive = %#v, %v; want %#v", replayed, err, first)
	}
}

func TestCloneExplainsGlobalNameConflictAndUnavailableEnvironment(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	for _, test := range []struct {
		name, targetEnv, targetConfig, reason string
		want                                  error
	}{
		{"same-key", "prod", "server", "clone_target_exists", ErrConflict},
		{"missing-env", "production", "server_copy", "target_environment_unavailable", ErrValidation},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := ConfigClone{OperationID: "clone-" + test.name, Actor: actor, SourceEnvironmentKey: "test", SourceConfigKey: "server", TargetEnvironmentKey: test.targetEnv, TargetConfigKey: test.targetConfig}
			for range 2 {
				result, err := store.CloneConfig(ctx, req)
				encoded, _ := json.Marshal(result)
				if !errors.Is(err, test.want) || !strings.Contains(string(encoded), `"reason":"`+test.reason+`"`) {
					t.Fatalf("clone rejection = %s, %v; want %s", encoded, err, test.reason)
				}
			}
		})
	}
}

func TestConfigClonePinsSourceAndReplaysAfterArchive(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "source-newer", Actor: actor, EnvironmentKey: "test", ConfigKey: "server", ConfigName: "Server", ExpectedRevision: 1, Format: configdoc.YAML, Content: []byte("newer: true\n")}); err != nil {
		t.Fatal(err)
	}
	req := ConfigClone{OperationID: "clone-pinned", Actor: actor, SourceEnvironmentKey: "test", SourceConfigKey: "server", SourceRevision: 1, TargetEnvironmentKey: "prod", TargetConfigKey: "server_copy"}
	first, err := store.CloneConfig(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	assertRawConfig(t, ctx, store, "prod", "server_copy", 1, "port: 6379\npassword: '{vault.app.db.password}'\n")
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "archive-clone-env", Actor: actor, Action: EnvironmentArchive, Key: "test"}); err != nil {
		t.Fatal(err)
	}
	replay, err := store.CloneConfig(ctx, req)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("clone replay = %#v, %v", replay, err)
	}
}

func TestConfigTransferAcceptsLegacyCompletedOperationDigest(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "legacy-target", Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("old: true\n")}); err != nil {
		t.Fatal(err)
	}
	req := ConfigTransfer{OperationID: "legacy-replace", Actor: actor, SourceEnvironmentKey: "test", SourceConfigKey: "server", SourceRevision: 1, TargetEnvironmentKey: "prod", TargetConfigKey: "server", TargetRevision: 1, ExpectedTargetRevision: 1}
	first, err := store.ReplaceConfig(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.readConfigRevision(ctx, "test", "server", 1)
	if err != nil {
		t.Fatal(err)
	}
	legacy := configCommitDigest(ConfigCommit{OperationID: req.OperationID, Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: source.DisplayName, ExpectedRevision: 1, Format: source.Format, Content: source.Content}, configCommitMeta{Action: "replace", SourceConfigID: source.ConfigID, SourceEnvironmentID: source.EnvironmentID, SourceRevision: 1})
	if _, err := store.db.ExecContext(ctx, `UPDATE operations SET request_sha256 = ? WHERE operation_id = ?`, legacy[:], req.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "archive-legacy-source", Actor: actor, Action: EnvironmentArchive, Key: "test"}); err != nil {
		t.Fatal(err)
	}
	replay, err := store.ReplaceConfig(ctx, req)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("legacy replay = %#v, %v", replay, err)
	}
}

func TestConfigParserFailureNeverEchoesContent(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	content := []byte("SENSITIVE-PARSER-MARKER: 1\nSENSITIVE-PARSER-MARKER: 2\n")
	_, err := store.ValidateConfig(ctx, ConfigValidation{EnvironmentKey: "test", Format: configdoc.YAML, Content: content})
	if !errors.Is(err, ErrValidation) || strings.Contains(err.Error(), "SENSITIVE-PARSER-MARKER") {
		t.Errorf("validation error must be value-free: %v", err)
	}
	req := ConfigCommit{OperationID: "parser-rejection", Actor: actor, EnvironmentKey: "test", ConfigKey: "server", ConfigName: "Server", ExpectedRevision: 1, Format: configdoc.YAML, Content: content}
	for range 2 {
		result, err := store.CommitConfig(ctx, req)
		encoded, _ := json.Marshal(result)
		if !errors.Is(err, ErrValidation) || strings.Contains(err.Error(), "SENSITIVE-PARSER-MARKER") || strings.Contains(string(encoded), "SENSITIVE-PARSER-MARKER") {
			t.Fatalf("commit error must be value-free: %s, %v", encoded, err)
		}
	}
	audits, err := store.ClaimOutbox(ctx, OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, audit := range audits {
		if strings.Contains(string(audit.Payload), "SENSITIVE-PARSER-MARKER") {
			t.Fatal("audit leaked content")
		}
	}
}

func TestConfigFormatOnlyChangeCreatesRevision(t *testing.T) {
	store, ctx, actor := configCopyStore(t)
	req := ConfigCommit{OperationID: "format-json", Actor: actor, EnvironmentKey: "prod", ConfigKey: "empty", ConfigName: "Empty", Format: configdoc.JSON, Content: []byte("{}\n")}
	if _, err := store.CommitConfig(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.OperationID, req.ExpectedRevision, req.Format = "format-yaml", 1, configdoc.YAML
	result, err := store.CommitConfig(ctx, req)
	if err != nil || result.Outcome != OutcomeSuccess || result.Revision != 2 {
		t.Fatalf("format-only change = %#v, %v", result, err)
	}
	raw, err := store.ReadRawConfig(ctx, "prod", "empty")
	if err != nil || raw.Format != "yaml" {
		t.Fatalf("format not updated: %#v, %v", raw, err)
	}
}
