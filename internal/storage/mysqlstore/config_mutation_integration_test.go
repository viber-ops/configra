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

func TestCommitConfigIsCanonicalOptimisticAndIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_mutation")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, "INSERT INTO environments (id, resource_key, display_name) VALUES (?, 'a', 'Environment A')", repeatedID(1)); err != nil {
		t.Fatalf("seed Environment: %v", err)
	}

	create := ConfigCommit{
		OperationID:      "operation-config-create",
		Actor:            Actor{Type: "user", ID: "admin@example.com"},
		EnvironmentKey:   "a",
		ConfigKey:        "payment",
		ConfigName:       "Payment",
		ExpectedRevision: 0,
		Format:           configdoc.YAML,
		Content:          []byte("database:\n    port: 6379\n"),
	}
	created, err := store.CommitConfig(ctx, create)
	if err != nil {
		t.Fatalf("CommitConfig create: %v", err)
	}
	if created.Outcome != OutcomeSuccess || created.Revision != 1 {
		t.Fatalf("create result = %#v", created)
	}
	resolved, err := store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil {
		t.Fatalf("ReadResolvedConfig after create: %v", err)
	}
	if resolved.ConfigRevision != 1 || resolved.Content != "database:\n  port: 6379\n" {
		t.Fatalf("created Config = %#v", resolved)
	}

	replayed, err := store.CommitConfig(ctx, create)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("idempotent replay = %#v, %v; want %#v", replayed, err, created)
	}
	reused := create
	reused.Content = []byte("database:\n  port: 6380\n")
	if _, err := store.CommitConfig(ctx, reused); !errors.Is(err, ErrOperationReuse) {
		t.Fatalf("OperationID reuse error = %v, want ErrOperationReuse", err)
	}

	update := create
	update.OperationID = "operation-config-update"
	update.ExpectedRevision = 1
	update.Content = []byte("database:\n  port: 6380\n")
	updated, err := store.CommitConfig(ctx, update)
	if err != nil || updated.Outcome != OutcomeSuccess || updated.Revision != 2 {
		t.Fatalf("update result = %#v, %v", updated, err)
	}

	noChange := update
	noChange.OperationID = "operation-config-no-change"
	noChange.ExpectedRevision = 2
	noChange.Content = []byte("database:\n    port: 6380\n")
	unchanged, err := store.CommitConfig(ctx, noChange)
	if err != nil || unchanged.Outcome != OutcomeNoChange || unchanged.Revision != 2 {
		t.Fatalf("no-change result = %#v, %v", unchanged, err)
	}

	stale := update
	stale.OperationID = "operation-config-conflict"
	stale.ExpectedRevision = 1
	stale.Content = []byte("database:\n  port: 6381\n")
	if _, err := store.CommitConfig(ctx, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v, want ErrConflict", err)
	}
	invalid := update
	invalid.OperationID = "operation-config-invalid"
	invalid.ExpectedRevision = 2
	invalid.Content = []byte("database: [\n")
	if _, err := store.CommitConfig(ctx, invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid update error = %v, want ErrValidation", err)
	}
	resolved, err = store.ReadResolvedConfig(ctx, "a", "payment", "")
	if err != nil {
		t.Fatalf("ReadResolvedConfig after failures: %v", err)
	}
	if resolved.ConfigRevision != 2 || resolved.Content != "database:\n  port: 6380\n" {
		t.Fatalf("Config changed after no-change/conflict/validation failure: %#v", resolved)
	}
	audits, err := store.ClaimOutbox(ctx, OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatalf("Claim Audit Outbox: %v", err)
	}
	if len(audits) != 5 {
		t.Fatalf("Audit Outbox Events = %d, want 5", len(audits))
	}
	notifications, err := store.ClaimOutbox(ctx, OutboxNotification, 100, time.Minute)
	if err != nil {
		t.Fatalf("Claim Notification Outbox: %v", err)
	}
	if len(notifications) != 2 {
		t.Fatalf("Notification Outbox Events = %d, want 2", len(notifications))
	}
	for _, event := range append(audits, notifications...) {
		payload := string(event.Payload)
		if strings.Contains(payload, "6379") || strings.Contains(payload, "6380") || strings.Contains(payload, "database:") {
			t.Fatalf("Outbox Event leaked Config content: %s", payload)
		}
	}
}
