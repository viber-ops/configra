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

func TestConfigArchiveAndUnarchivePreserveRevisionHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_lifecycle")
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
		OperationID: "operation-config-lifecycle-environment", Actor: actor,
		Action: EnvironmentCreate, Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	commit := ConfigCommit{
		OperationID: "operation-config-lifecycle-create", Actor: actor,
		EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		Format: configdoc.YAML, Content: []byte("port: 6379\n"),
	}
	if _, err := store.CommitConfig(ctx, commit); err != nil {
		t.Fatalf("create Config: %v", err)
	}

	archive := ConfigLifecycleChange{
		OperationID: "operation-config-archive", Actor: actor,
		Action: ConfigArchive, Key: "payment",
	}
	archived, err := store.ApplyConfigLifecycleChange(ctx, archive)
	if err != nil || archived.Outcome != OutcomeSuccess || !archived.Archived {
		t.Fatalf("Archive Config = %#v, %v", archived, err)
	}
	replayed, err := store.ApplyConfigLifecycleChange(ctx, archive)
	if err != nil || !reflect.DeepEqual(replayed, archived) {
		t.Fatalf("Archive replay = %#v, %v; want %#v", replayed, err, archived)
	}
	if _, err := store.ReadResolvedConfig(ctx, "a", "payment", ""); !errors.Is(err, machine.ErrNotFound) {
		t.Fatalf("read Archived Config = %v, want ErrNotFound", err)
	}
	if _, err := store.ReadRawConfig(ctx, "a", "payment"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read raw Archived Config = %v, want ErrNotFound", err)
	}
	commit.OperationID = "operation-config-archived-write"
	commit.ExpectedRevision = 1
	commit.Content = []byte("port: 6380\n")
	if _, err := store.CommitConfig(ctx, commit); !errors.Is(err, ErrValidation) {
		t.Fatalf("write Archived Config = %v, want ErrValidation", err)
	}

	archive.OperationID = "operation-config-archive-no-change"
	unchanged, err := store.ApplyConfigLifecycleChange(ctx, archive)
	if err != nil || unchanged.Outcome != OutcomeNoChange || !unchanged.Archived {
		t.Fatalf("repeat Archive = %#v, %v", unchanged, err)
	}
	unarchive := ConfigLifecycleChange{
		OperationID: "operation-config-unarchive", Actor: actor,
		Action: ConfigUnarchive, Key: "payment",
	}
	unarchived, err := store.ApplyConfigLifecycleChange(ctx, unarchive)
	if err != nil || unarchived.Outcome != OutcomeSuccess || unarchived.Archived {
		t.Fatalf("Unarchive Config = %#v, %v", unarchived, err)
	}
	resolved, err := store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil || resolved.ConfigRevision != 1 || resolved.Content != "port: 6379\n" {
		t.Fatalf("Config after Unarchive = %#v, %v", resolved, err)
	}
	raw, err := store.ReadRawConfig(ctx, "a", "payment")
	if err != nil || raw.Revision != 1 || raw.Content != "port: 6379\n" {
		t.Fatalf("Raw Config after Unarchive = %#v, %v", raw, err)
	}
}
