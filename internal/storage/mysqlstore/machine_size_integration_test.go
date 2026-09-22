//go:build integration

package mysqlstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
)

func TestMachineHTTPRejectsOversizedExpansionWithoutLeakingValues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	actor := Actor{Type: "user", ID: "document-budget-test"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "budget-env", Actor: actor,
		Action: EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	const sentinel = "protected-expansion-value-"
	secret := sentinel + strings.Repeat("x", (512<<10)-len(sentinel))
	if _, err := store.CommitVault(ctx, VaultCommit{OperationID: "budget-vault", Actor: actor,
		NamespaceKey: "ops", ItemKey: "db", ItemName: "Database", Snapshot: testVaultSnapshot("", []string{"prod"}, "user", secret)}); err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateToken(ctx, TokenCreate{OperationID: "budget-token", Actor: actor,
		DisplayName: "Budget test", EnvironmentKeys: []string{"prod"}, AllowWithoutMTLS: true, NeverExpires: true})
	if err != nil {
		t.Fatal(err)
	}
	handler := machine.NewHandler(store, nil)
	for _, format := range []configdoc.Format{configdoc.JSON, configdoc.YAML} {
		key := "large-" + string(format)
		source := []byte("[" + strings.Repeat(`"{vault.ops.db.password}",`, 15) + `"{vault.ops.db.password}"]`)
		if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "budget-" + key, Actor: actor,
			EnvironmentKey: "prod", ConfigKey: key, ConfigName: key, Format: format, Content: source}); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/prod/configs/"+key, nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token.Token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || response.Body.Len() > 1024 ||
			strings.Contains(response.Body.String(), sentinel) || strings.Contains(response.Body.String(), token.Token) {
			t.Fatalf("%s oversized response must be a bounded, value-free 422; status=%d bytes=%d", format, response.Code, response.Body.Len())
		}
		// A rejected read must release its transaction and allow a later revision.
		if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "budget-small-" + key, Actor: actor,
			EnvironmentKey: "prod", ConfigKey: key, ConfigName: key, ExpectedRevision: 1, Format: format, Content: []byte(`{"ok":true}`)}); err != nil {
			t.Fatal(err)
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s valid replacement status=%d", format, response.Code)
		}
	}
}
