//go:build integration

package mysqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestConfigManagementReadsListHistoryAndImmutableRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_read")
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
	for _, key := range []string{"a", "b"} {
		if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
			OperationID: "config-read-environment-" + key, Actor: actor, Action: EnvironmentCreate,
			Key: key, DisplayName: "Environment " + key,
		}); err != nil {
			t.Fatalf("create Environment %s: %v", key, err)
		}
	}
	commits := []ConfigCommit{
		{OperationID: "config-read-a-1", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment", Format: configdoc.YAML, Content: []byte("port: 6379\n")},
		{OperationID: "config-read-a-2", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment", ExpectedRevision: 1, Format: configdoc.YAML, Content: []byte("port: 6380\n")},
		{OperationID: "config-read-b-1", Actor: actor, EnvironmentKey: "b", ConfigKey: "payment", ConfigName: "Payment", Format: configdoc.JSON, Content: []byte(`{"port": 6379}`)},
	}
	for _, commit := range commits {
		if _, err := store.CommitConfig(ctx, commit); err != nil {
			t.Fatalf("CommitConfig %s: %v", commit.OperationID, err)
		}
	}

	configPage, err := store.ListConfigs(ctx, ConfigQuery{InventoryQuery: InventoryQuery{Limit: 50}})
	configs := configPage.Items
	if err != nil || len(configs) != 1 || configs[0].Key != "payment" || configs[0].DisplayName != "Payment" || configs[0].Archived ||
		len(configs[0].Environments) != 2 || configs[0].Environments[0].Key != "a" || configs[0].Environments[0].Revision != 2 ||
		configs[0].Environments[1].Key != "b" || configs[0].Environments[1].Revision != 1 {
		t.Fatalf("ListConfigs = %#v, %v", configs, err)
	}
	page, err := store.ListConfigRevisions(ctx, "a", "payment", RevisionQuery{Limit: 50})
	history := page.Items
	if err != nil || len(history) != 2 || history[0].Revision != 2 || history[1].Revision != 1 ||
		history[0].OperationID != "config-read-a-2" || history[0].ActorID != actor.ID || history[0].CreatedAt.IsZero() {
		t.Fatalf("ListConfigRevisions = %#v, %v", history, err)
	}
	revision, err := store.ReadRawConfigRevision(ctx, "a", "payment", 1)
	if err != nil || revision.Revision != 1 || revision.Content != "port: 6379\n" || revision.Format != "yaml" {
		t.Fatalf("ReadRawConfigRevision = %#v, %v", revision, err)
	}
	if _, err := store.ReplaceConfig(ctx, ConfigTransfer{
		OperationID: "config-read-transfer", Actor: actor,
		SourceEnvironmentKey: "a", SourceConfigKey: "payment", SourceRevision: 1,
		TargetEnvironmentKey: "b", TargetConfigKey: "payment", TargetRevision: 1, ExpectedTargetRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	transferred, err := store.ListConfigRevisions(ctx, "b", "payment", RevisionQuery{Limit: 1})
	if err != nil || len(transferred.Items) != 1 || transferred.NextBefore != 2 || transferred.Items[0].Source == nil ||
		*transferred.Items[0].Source != (ConfigRevisionSource{EnvironmentKey: "a", ConfigKey: "payment", Revision: 1}) {
		t.Fatalf("paged history lost transfer origin: %#v, %v", transferred, err)
	}

	if _, err := store.ApplyConfigLifecycleChange(ctx, ConfigLifecycleChange{
		OperationID: "config-read-archive", Actor: actor, Action: ConfigArchive, Key: "payment",
	}); err != nil {
		t.Fatalf("Archive Config: %v", err)
	}
	configPage, err = store.ListConfigs(ctx, ConfigQuery{InventoryQuery: InventoryQuery{Limit: 50}})
	if err != nil || len(configPage.Items) != 0 || configPage.Total != 0 {
		t.Fatalf("active Configs = %#v, %v", configPage, err)
	}
	configPage, err = store.ListConfigs(ctx, ConfigQuery{InventoryQuery: InventoryQuery{Limit: 50, IncludeInactive: true}})
	if err != nil || len(configPage.Items) != 1 || configPage.Total != 1 || !configPage.Items[0].Archived {
		t.Fatalf("all Configs = %#v, %v", configPage, err)
	}
	if page, err = store.ListConfigRevisions(ctx, "a", "payment", RevisionQuery{Limit: 50}); err != nil || len(page.Items) != 2 {
		t.Fatalf("Archived Config history = %#v, %v", page, err)
	}
}
