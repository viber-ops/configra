//go:build integration

package mysqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// This is a storage query-budget regression, not public behavior acceptance.
// The latter lives in e2e/revision_history_integration_test.go and uses HTTP only.
func TestRevisionPagesSeekInsteadOfScanningHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_revision_budget")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// Session counters must surround reads on the same physical connection.
	store.db.SetMaxOpenConns(1)
	store.db.SetMaxIdleConns(1)
	actor := Actor{Type: "user", ID: "query-budget"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
		OperationID: "revision-budget-environment", Actor: actor, Action: EnvironmentCreate, Key: "a", DisplayName: "A",
	}); err != nil {
		t.Fatal(err)
	}
	variantID := ""
	for revision := uint64(1); revision <= 1005; revision++ {
		value := fmt.Sprintf("value-%d", revision)
		if _, err := store.CommitConfig(ctx, ConfigCommit{
			OperationID: fmt.Sprintf("revision-budget-config-%d", revision), Actor: actor,
			EnvironmentKey: "a", ConfigKey: "app", ConfigName: "App", ExpectedRevision: revision - 1,
			Format: configdoc.YAML, Content: []byte("value: " + value + "\n"),
		}); err != nil {
			t.Fatal(err)
		}
		result, err := store.CommitVault(ctx, VaultCommit{
			OperationID: fmt.Sprintf("revision-budget-vault-%d", revision), Actor: actor,
			NamespaceKey: "platform", ItemKey: "app", ItemName: "App", ExpectedRevision: revision - 1,
			Snapshot: vaultdoc.Snapshot{
				Fields:   []vaultdoc.Field{{Key: "value", Name: "Value", Type: vaultdoc.Secret}},
				Variants: []vaultdoc.Variant{{ID: variantID, Environments: []string{"a"}, Values: map[string]vaultdoc.Value{"value": {Text: &value}}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		variantID = result.VariantIDs[0]
	}
	// Handler_read_* includes index seeks, index traversal and table reads.
	// No global counters or elapsed-time thresholds: other test databases and
	// shared-host load must not change this per-connection budget.
	reads := func() uint64 { return handlerReadCount(t, ctx, store.db) }
	sortRows := func() uint64 {
		t.Helper()
		var count uint64
		if err := store.db.QueryRowContext(ctx, `SHOW SESSION STATUS LIKE 'Sort_rows'`).Scan(new(string), &count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	baseline := reads()
	t.Logf("counter sampling alone: %d handler reads", reads()-baseline)
	for _, before := range []uint64{0, 1000, 900, 500, 256, 129, 101} {
		for _, limit := range []int{1, 50, 100} {
			query := RevisionQuery{Before: before, Limit: limit}
			// Includes the parent identity lookup, one lookahead row, optional
			// source joins and SHOW STATUS's own small temporary-table overhead.
			budget := uint64(4*(limit+1) + 64)
			for _, kind := range []string{"Config", "Vault"} {
				sortStart := sortRows()
				start := reads()
				var count int
				if kind == "Config" {
					page, err := store.ListConfigRevisions(ctx, "a", "app", query)
					if err != nil {
						t.Fatal(err)
					}
					count = len(page.Items)
				} else {
					page, err := store.ListVaultRevisions(ctx, "platform", "app", query)
					if err != nil {
						t.Fatal(err)
					}
					count = len(page.Items)
				}
				used := reads() - start
				sorted := sortRows() - sortStart
				if count != limit || used > budget {
					t.Fatalf("%s before=%d limit=%d: items=%d, reads=%d, sorted=%d, budget=%d", kind, before, limit, count, used, sorted, budget)
				}
				t.Logf("%s before=%d limit=%d: %d handler reads, %d sorted (budget %d)", kind, before, limit, used, sorted, budget)
			}
		}
	}
	// Reuse this history to guard the composite-key rotation cursor against
	// repeated full scans, including crossing from one Item to another.
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "rotation-budget-other-item", Actor: actor, NamespaceKey: "platform", ItemKey: "other", ItemName: "Other",
		Snapshot: testVaultSnapshot("", []string{"a"}, "user", "rotation-budget-secret"),
	}); err != nil {
		t.Fatal(err)
	}
	next, err := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	start := reads()
	rotated, err := store.RotateMasterKey(ctx, next)
	used := reads() - start
	const rotationBudget = 16*1006 + 256
	if err != nil || rotated.VaultRevisions != 1006 || used > rotationBudget {
		t.Fatalf("history rotation: records=%d, reads=%d, budget=%d, err=%v", rotated.VaultRevisions, used, rotationBudget, err)
	}
	fresh, err := OpenAPI(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if verified, err := fresh.VerifyEncryptedState(ctx); err != nil || verified != rotated.EncryptionVerification {
		t.Fatalf("rotated full-history verification: %#v, %v", verified, err)
	}
	t.Logf("rotation of 1006 revisions across two Items: %d handler reads (budget %d)", used, rotationBudget)
}

// The caller pins its pool to one physical connection so counters cannot mix.
func handlerReadCount(t *testing.T, ctx context.Context, database *sql.DB) uint64 {
	t.Helper()
	rows, err := database.QueryContext(ctx, `SHOW SESSION STATUS LIKE 'Handler_read_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var total uint64
	count := 0
	for rows.Next() {
		var name string
		var value uint64
		if err := rows.Scan(&name, &value); err != nil {
			t.Fatal(err)
		}
		total += value
		count++
	}
	if err := rows.Err(); err != nil || count < 7 {
		t.Fatalf("read counters: count=%d, err=%v", count, err)
	}
	return total
}
