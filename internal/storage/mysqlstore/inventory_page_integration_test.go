//go:build integration

package mysqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestInventoryPagesBoundParentsAndAssociationsAndPatchOnlyNamedGrants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_inventory_pages")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "inventory-review"}
	const size = 1005
	keys := make([]string, size)
	for index := range keys {
		keys[index] = fmt.Sprintf("resource_%04d", index)
		_, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "inventory-env-" + keys[index], Actor: actor, Action: EnvironmentCreate, Key: keys[index], DisplayName: keys[index]})
		if err != nil {
			t.Fatal(err)
		}
	}
	secret := "inventory-secret-sentinel"
	var primary TokenCreateResult
	for index, key := range keys {
		_, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "inventory-config-" + key, Actor: actor, EnvironmentKey: keys[0], ConfigKey: key, ConfigName: key, Format: configdoc.YAML, Content: []byte("secret: source-sentinel\n")})
		if err != nil {
			t.Fatal(err)
		}
		bindings := keys[:5]
		if index == 0 {
			bindings = keys
		}
		_, err = store.CommitVault(ctx, VaultCommit{OperationID: "inventory-vault-" + key, Actor: actor, NamespaceKey: "platform", ItemKey: key, ItemName: key,
			Snapshot: vaultdoc.Snapshot{Fields: []vaultdoc.Field{{Key: "value", Name: "Value", Type: vaultdoc.Secret}}, Variants: []vaultdoc.Variant{{Environments: bindings, Values: map[string]vaultdoc.Value{"value": {Text: &secret}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		created, err := store.CreateToken(ctx, TokenCreate{OperationID: "inventory-token-" + key, Actor: actor, DisplayName: key, EnvironmentKeys: bindings})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			primary = created
		}
	}
	for _, environment := range keys[1:] {
		_, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "inventory-context-" + environment, Actor: actor, EnvironmentKey: environment, ConfigKey: keys[0], ConfigName: keys[0], Format: configdoc.YAML, Content: []byte("value: 1\n")})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, limit := range []int{1, 50, 100} {
		for _, offset := range []int{0, 500, 1000, size} {
			query := InventoryQuery{Limit: limit, Offset: offset}
			wanted := min(limit, size-offset)
			start := time.Now()
			environments, err := store.ListEnvironments(ctx, EnvironmentQuery{InventoryQuery: query})
			if err != nil || len(environments.Items) != wanted || environments.Total != size {
				t.Fatalf("environments limit=%d offset=%d: count=%d total=%d err=%v", limit, offset, len(environments.Items), environments.Total, err)
			}
			configs, err := store.ListConfigs(ctx, ConfigQuery{InventoryQuery: query})
			if err != nil || len(configs.Items) != wanted || configs.Total != size {
				t.Fatalf("configs limit=%d offset=%d: count=%d total=%d err=%v", limit, offset, len(configs.Items), configs.Total, err)
			}
			for _, item := range configs.Items {
				if len(item.Environments) > SummaryEnvironmentLimit {
					t.Fatalf("unbounded config associations: %d", len(item.Environments))
				}
			}
			vault, err := store.ListVaultItems(ctx, VaultQuery{InventoryQuery: query})
			if err != nil || len(vault.Items) != wanted || vault.Total != size {
				t.Fatalf("vault limit=%d offset=%d: count=%d total=%d err=%v", limit, offset, len(vault.Items), vault.Total, err)
			}
			for _, item := range vault.Items {
				if len(item.EnvironmentKeys) > SummaryEnvironmentLimit {
					t.Fatalf("unbounded vault associations: %d", len(item.EnvironmentKeys))
				}
			}
			tokens, err := store.ListTokens(ctx, query)
			if err != nil || len(tokens.Items) != wanted || tokens.Total != size {
				t.Fatalf("tokens limit=%d offset=%d: count=%d total=%d err=%v", limit, offset, len(tokens.Items), tokens.Total, err)
			}
			for _, item := range tokens.Items {
				if len(item.EnvironmentKeys) > SummaryEnvironmentLimit {
					t.Fatalf("unbounded token grants: %d", len(item.EnvironmentKeys))
				}
			}
			encoded, err := json.Marshal([]any{environments, configs, vault, tokens})
			if err != nil || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "source-sentinel") || strings.Contains(string(encoded), primary.Token) {
				t.Fatal("inventory leaked value material")
			}
			if len(encoded) > 2500*limit+512 {
				t.Fatalf("inventory response budget: %d bytes for limit %d", len(encoded), limit)
			}
			t.Logf("four inventories limit=%d offset=%d: %d bytes, %s (diagnostic, not a load gate)", limit, offset, len(encoded), time.Since(start))
		}
	}
	query := InventoryQuery{Limit: 1, Key: keys[0]}
	configs, err := store.ListConfigs(ctx, ConfigQuery{InventoryQuery: query, Environment: keys[size-1]})
	if err != nil || configs.Total != 1 || len(configs.Items) != 1 || configs.Items[0].EnvironmentCount != size || configs.Items[0].Revision != 1 {
		t.Fatalf("off-preview config context: %#v, %v", configs, err)
	}
	vault, err := store.ListVaultItems(ctx, VaultQuery{InventoryQuery: query, Namespace: "platform", Environment: keys[size-1]})
	if err != nil || vault.Total != 1 || len(vault.Items) != 1 || vault.Items[0].EnvironmentCount != size {
		t.Fatalf("off-preview vault context: %#v, %v", vault, err)
	}
	bound, err := store.ListEnvironments(ctx, EnvironmentQuery{InventoryQuery: InventoryQuery{Limit: 5, Offset: 1000}, Config: keys[0], Namespace: "platform", Item: keys[0], TokenPublicID: primary.PublicID})
	if err != nil || bound.Total != size || len(bound.Items) != 5 {
		t.Fatalf("scoped environment tail: %#v, %v", bound, err)
	}
	for index, item := range bound.Items {
		if item.Key != keys[1000+index] || item.Revision != 1 || !item.Granted {
			t.Fatalf("scoped metadata: %#v", item)
		}
	}
	for _, search := range []string{"%", "' OR 1=1 --", "not_present"} {
		page, err := store.ListConfigs(ctx, ConfigQuery{InventoryQuery: InventoryQuery{Limit: 50, Search: search}})
		if err != nil || page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("literal search %q: %#v, %v", search, page, err)
		}
	}
	for _, namespace := range []string{"platform", "other"} {
		page, err := store.ListVaultItems(ctx, VaultQuery{InventoryQuery: InventoryQuery{Limit: 50, Search: "platform." + keys[size-1]}, Namespace: namespace})
		want := uint64(0)
		if namespace == "platform" {
			want = 1
		}
		if err != nil || page.Total != want {
			t.Fatalf("composite search in %s: %#v, %v", namespace, page, err)
		}
	}
	if page, err := store.ListTokens(ctx, InventoryQuery{Limit: 1, Search: primary.PublicID}); err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].PublicID != primary.PublicID {
		t.Fatalf("Token search: %#v, %v", page, err)
	}
	if page, err := store.ListEnvironments(ctx, EnvironmentQuery{InventoryQuery: InventoryQuery{Limit: 1, Search: strings.ToUpper(keys[size-1])}}); err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Key != keys[size-1] {
		t.Fatalf("Environment case-insensitive literal search: %#v, %v", page, err)
	}
	// Archive a grant outside the summary. Removing it must work without
	// rewriting the other 1004 grants, including concurrent independent edits.
	_, err = store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "inventory-archive", Actor: actor, Action: EnvironmentArchive, Key: keys[size-1]})
	if err != nil {
		t.Fatal(err)
	}
	if page, err := store.ListEnvironments(ctx, EnvironmentQuery{InventoryQuery: InventoryQuery{Limit: 50, OnlyInactive: true}}); err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Key != keys[size-1] {
		t.Fatalf("archived Environment filter: %#v, %v", page, err)
	}
	patch := TokenEnvironmentChange{OperationID: "inventory-patch", Actor: actor, PublicID: primary.PublicID, Patch: true, Remove: []string{keys[size-1], keys[0]}}
	result, err := store.SetTokenEnvironments(ctx, patch)
	if err != nil || result.Outcome != OutcomeSuccess || result.EnvironmentKeys != nil {
		t.Fatalf("patch: %#v, %v", result, err)
	}
	if result, err = store.SetTokenEnvironments(ctx, patch); err != nil || result.Outcome != OutcomeSuccess {
		t.Fatalf("patch replay: %#v, %v", result, err)
	}
	patch.OperationID = "inventory-patch-no-change"
	if result, err = store.SetTokenEnvironments(ctx, patch); err != nil || result.Outcome != OutcomeNoChange {
		t.Fatalf("patch no change: %#v, %v", result, err)
	}
	patch.OperationID, patch.Add, patch.Remove = "inventory-invalid-patch", []string{keys[size-1]}, []string{keys[1]}
	if _, err = store.SetTokenEnvironments(ctx, patch); !errors.Is(err, ErrValidation) {
		t.Fatalf("archived addition: %v", err)
	}
	patch.OperationID, patch.Add, patch.Remove = "inventory-overlap", []string{keys[1]}, []string{keys[1]}
	if _, err = store.SetTokenEnvironments(ctx, patch); !errors.Is(err, ErrValidation) {
		t.Fatalf("overlapping patch: %v", err)
	}
	grants, err := store.ListTokens(ctx, InventoryQuery{Limit: 1, Key: primary.PublicID})
	if err != nil || len(grants.Items) != 1 || grants.Items[0].EnvironmentCount != size-2 || grants.Items[0].EnvironmentKeys[0] != keys[1] {
		t.Fatalf("unrelated grants changed: %#v, %v", grants, err)
	}
	start, results := make(chan struct{}), make(chan error, 2)
	for _, key := range keys[1:3] {
		go func() {
			<-start
			result, err := store.SetTokenEnvironments(ctx, TokenEnvironmentChange{OperationID: "inventory-concurrent-" + key, Actor: actor, PublicID: primary.PublicID, Patch: true, Remove: []string{key}})
			if err == nil && result.Outcome != OutcomeSuccess {
				err = fmt.Errorf("concurrent patch outcome = %s", result.Outcome)
			}
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	grants, err = store.ListTokens(ctx, InventoryQuery{Limit: 1, Key: primary.PublicID})
	if err != nil || len(grants.Items) != 1 || grants.Items[0].EnvironmentCount != size-4 || grants.Items[0].EnvironmentKeys[0] != keys[3] {
		t.Fatalf("concurrent patches lost an independent edit: %#v, %v", grants, err)
	}
	if _, err := store.RevokeToken(ctx, TokenRevoke{OperationID: "inventory-revoke", Actor: actor, PublicID: primary.PublicID}); err != nil {
		t.Fatal(err)
	}
	patch.OperationID, patch.Add, patch.Remove = "inventory-revoked-patch", []string{keys[0]}, nil
	if _, err := store.SetTokenEnvironments(ctx, patch); !errors.Is(err, ErrValidation) {
		t.Fatalf("revoked token patch: %v", err)
	}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "inventory-archive-first", Actor: actor, Action: EnvironmentArchive, Key: keys[0]}); err != nil {
		t.Fatal(err)
	}
	// The highly connected first Config still has active contexts, but every
	// other Config is now bound only to an archived environment.
	if page, err := store.ListConfigs(ctx, ConfigQuery{InventoryQuery: InventoryQuery{Limit: 1}, Unbound: true}); err != nil || page.Total != size-1 || len(page.Items) != 1 || page.Items[0].Key != keys[1] {
		t.Fatalf("configuration gaps: %#v, %v", page, err)
	}
}
