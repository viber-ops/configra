//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestApplyEnvironmentChangeCreatesRenamesArchivesAndUnarchives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_environment_mutation")
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

	create := EnvironmentChange{OperationID: "operation-environment-create", Actor: actor, Action: EnvironmentCreate, Key: "a", DisplayName: "Environment A"}
	created, err := store.ApplyEnvironmentChange(ctx, create)
	if err != nil || created.Outcome != OutcomeSuccess || created.Key != "a" || created.Archived {
		t.Fatalf("create result = %#v, %v", created, err)
	}
	replayed, err := store.ApplyEnvironmentChange(ctx, create)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("idempotent replay = %#v, %v; want %#v", replayed, err, created)
	}
	reused := create
	reused.DisplayName = "Different"
	if _, err := store.ApplyEnvironmentChange(ctx, reused); !errors.Is(err, ErrOperationReuse) {
		t.Fatalf("OperationID reuse error = %v, want ErrOperationReuse", err)
	}

	rename := EnvironmentChange{OperationID: "operation-environment-rename", Actor: actor, Action: EnvironmentRename, Key: "a", DisplayName: "Primary"}
	renamed, err := store.ApplyEnvironmentChange(ctx, rename)
	if err != nil || renamed.Outcome != OutcomeSuccess || renamed.DisplayName != "Primary" {
		t.Fatalf("rename result = %#v, %v", renamed, err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "operation-environment-config", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		ExpectedRevision: 0, Format: configdoc.YAML, Content: []byte("port: 6379\n"),
	}); err != nil {
		t.Fatalf("CommitConfig before Archive: %v", err)
	}

	archive := EnvironmentChange{OperationID: "operation-environment-archive", Actor: actor, Action: EnvironmentArchive, Key: "a"}
	archived, err := store.ApplyEnvironmentChange(ctx, archive)
	if err != nil || archived.Outcome != OutcomeSuccess || !archived.Archived {
		t.Fatalf("archive result = %#v, %v", archived, err)
	}
	if _, err := store.ReadResolvedConfig(ctx, "a", "payment", ""); !errors.Is(err, machine.ErrNotFound) {
		t.Fatalf("ReadResolvedConfig in Archived Environment = %v, want ErrNotFound", err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "operation-environment-archived-write", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		ExpectedRevision: 1, Format: configdoc.YAML, Content: []byte("port: 6380\n"),
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("CommitConfig in Archived Environment = %v, want ErrValidation", err)
	}
	active, err := store.ListEnvironments(ctx, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("active Environments = %#v, %v", active, err)
	}
	all, err := store.ListEnvironments(ctx, true)
	if err != nil || len(all) != 1 || all[0].Key != "a" || all[0].DisplayName != "Primary" || !all[0].Archived ||
		all[0].CreatedAt.IsZero() || all[0].UpdatedAt.IsZero() {
		t.Fatalf("all Environments = %#v, %v", all, err)
	}

	unarchive := EnvironmentChange{OperationID: "operation-environment-unarchive", Actor: actor, Action: EnvironmentUnarchive, Key: "a"}
	unarchived, err := store.ApplyEnvironmentChange(ctx, unarchive)
	if err != nil || unarchived.Outcome != OutcomeSuccess || unarchived.Archived {
		t.Fatalf("unarchive result = %#v, %v", unarchived, err)
	}
	resolved, err := store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || resolved.ConfigRevision != 1 || resolved.Content != "port: 6379\n" {
		t.Fatalf("restored Environment content = %#v, %v", resolved, err)
	}
	active, err = store.ListEnvironments(ctx, false)
	if err != nil || len(active) != 1 || active[0].Archived {
		t.Fatalf("unarchived Environments = %#v, %v", active, err)
	}

	noChange := unarchive
	noChange.OperationID = "operation-environment-unarchive-no-change"
	unchanged, err := store.ApplyEnvironmentChange(ctx, noChange)
	if err != nil || unchanged.Outcome != OutcomeNoChange {
		t.Fatalf("repeated Unarchive = %#v, %v", unchanged, err)
	}
}
