package management_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestInventoryQueriesAreBoundedBeforeReachingStorage(t *testing.T) {
	for _, path := range []string{"/v1/environments", "/v1/configs", "/v1/vault-items", "/v1/api-tokens", "/v1/client-certificates", "/v1/certificate-authorities", "/v1/notification-destinations"} {
		t.Run(path, func(t *testing.T) {
			repository := &recordingConfigWriter{}
			handler := management.NewHandler(repository, nil, nil)
			get := func(query string, role humanauth.Role) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
				r.URL.RawQuery = query
				if role != "" {
					r = r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "reader", Role: role}))
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			for _, query := range []string{"limit=0", "limit=-1", "limit=101", "limit=", "limit=no", "limit=1&limit=2", "limit=1000000000000000000000", "offset=-1", "offset=", "offset=1.5", "offset=2&offset=3", "offset=1000000000000000000000", "q=%zz", "q=a&q=b", "q=" + strings.Repeat("x", 257), "key=" + strings.Repeat("x", 129), "limit=10;offset=2", "unknown=x", "status=invalid"} {
				if response := get(query, humanauth.RoleAdmin); response.Code != http.StatusBadRequest {
					t.Errorf("query %q = %d %s", query, response.Code, response.Body)
				}
			}
			if response := get("", ""); response.Code != http.StatusUnauthorized {
				t.Errorf("anonymous = %d", response.Code)
			}
			if len(repository.inventoryQueries) != 0 {
				t.Fatal("invalid request reached storage")
			}
			if path == "/v1/client-certificates" || path == "/v1/certificate-authorities" || path == "/v1/notification-destinations" || path == "/v1/api-tokens" {
				if response := get("", humanauth.RoleViewer); response.Code != http.StatusForbidden || len(repository.inventoryQueries) != 0 {
					t.Fatal("Viewer reached administrative inventory")
				}
			}
			for _, test := range []struct {
				query string
				want  mysqlstore.InventoryQuery
			}{
				{"", mysqlstore.InventoryQuery{Limit: 50}},
				{"limit=100&offset=500&q=%20app%20&status=all&key=app", mysqlstore.InventoryQuery{Limit: 100, Offset: 500, Search: "app", Key: "app", IncludeInactive: true}},
			} {
				response := get(test.query, humanauth.RoleAdmin)
				if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":0`) {
					t.Fatalf("valid query = %d %s", response.Code, response.Body)
				}
				if got := repository.inventoryQueries[len(repository.inventoryQueries)-1]; got != test.want {
					t.Fatalf("parsed %#v, want %#v", got, test.want)
				}
			}
			if path == "/v1/environments" {
				calls := len(repository.inventoryQueries)
				if response := get("token=0123456789abcdef", humanauth.RoleViewer); response.Code != http.StatusForbidden || len(repository.inventoryQueries) != calls {
					t.Fatal("viewer can inspect token grants")
				}
				if response := get("token=0123456789abcdef&config=app&namespace=platform&item=db", humanauth.RoleAdmin); response.Code != http.StatusOK {
					t.Fatal(response.Body)
				}
				got := repository.environmentQueries[len(repository.environmentQueries)-1]
				if got.TokenPublicID != "0123456789abcdef" || got.Config != "app" || got.Namespace != "platform" || got.Item != "db" {
					t.Fatalf("lost scope filters: %#v", got)
				}
			}
		})
	}
}

func TestVaultImpactPreviewIsMetadataOnlyAdminRead(t *testing.T) {
	repository := &recordingConfigWriter{vaultUsages: []mysqlstore.VaultUsage{{FieldKey: "password", ConfigKey: "app", EnvironmentKey: "production"}}}
	handler := management.NewHandler(repository, nil, nil)
	call := func(method, path, body string, role humanauth.Role, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://configra.test"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r = r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "admin", Role: role}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	path := "/v1/vault-items/platform/db/impact-preview"
	valid := `{"fields":["host"],"environments":["production"]}`
	for _, test := range []struct {
		path, body string
		role       humanauth.Role
		origin     string
		status     int
	}{
		{path, valid, humanauth.RoleViewer, "", 403},
		{path, valid, humanauth.RoleAdmin, "https://attacker.example", 403},
		{path + "?limit=101", valid, humanauth.RoleAdmin, "", 400},
		{path + "?q=unrelated", valid, humanauth.RoleAdmin, "", 400},
		{path, `{"fields":["host"]}`, humanauth.RoleAdmin, "", 400},
		{path, `{"fields":null,"environments":[]}`, humanauth.RoleAdmin, "", 400},
		{path, `{"fields":[],"environments":[],"values":{"password":"secret"}}`, humanauth.RoleAdmin, "", 400},
	} {
		if response := call("POST", test.path, test.body, test.role, test.origin); response.Code != test.status {
			t.Fatalf("preview = %d %s", response.Code, response.Body)
		}
	}
	if len(repository.vaultUsageQueries) != 0 || len(repository.rejectedMutations) != 0 {
		t.Fatal("invalid preview reached storage or wrote a mutation audit")
	}
	response := call("POST", path+"?limit=1&offset=100", valid, humanauth.RoleAdmin, "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatal(response.Body)
	}
	query := repository.vaultUsageQueries[0]
	if !query.Impact || query.Limit != 1 || query.Offset != 100 || !reflect.DeepEqual(query.Fields, []string{"host"}) || !reflect.DeepEqual(query.Environments, []string{"production"}) {
		t.Fatalf("lost impact scope: %#v", query)
	}
	if response := call("GET", "/v1/vault-items/platform/db/usages?limit=1&offset=100", "", humanauth.RoleViewer, ""); response.Code != 200 {
		t.Fatal(response.Body)
	}
	for _, suffix := range []string{"?limit=0", "?offset=-1", "?status=all", "?key=app", "?q=a&q=b"} {
		if response := call("GET", "/v1/notification-destinations/ops/event-types"+suffix, "", humanauth.RoleAdmin, ""); response.Code != 400 {
			t.Fatal(response.Body)
		}
	}
	if response := call("GET", "/v1/notification-destinations/ops/event-types", "", humanauth.RoleViewer, ""); response.Code != 403 {
		t.Fatal(response.Body)
	}
	if response := call("GET", "/v1/notification-destinations/ops/event-types?limit=1&q=config", "", humanauth.RoleAdmin, ""); response.Code != 200 {
		t.Fatal(response.Body)
	}
}

func TestTokenGrantPatchIsAdminOnlyAuditedAndDoesNotReturnAReplacementSet(t *testing.T) {
	repository := &recordingConfigWriter{tokenEnvironmentResult: mysqlstore.TokenEnvironmentResult{Outcome: mysqlstore.OutcomeSuccess, PublicID: "0123456789abcdef"}}
	handler := management.NewHandler(repository, nil, nil)
	patch := func(role humanauth.Role, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPatch, "https://configra.test/v1/api-tokens/0123456789abcdef/environments", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "grant-patch-test")
		r = r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "reader", Role: role}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if response := patch(humanauth.RoleViewer, `{"add":["a"]}`); response.Code != http.StatusForbidden || len(repository.tokenEnvironmentChanges) != 0 {
		t.Fatal("viewer patch was not rejected")
	}
	if response := patch(humanauth.RoleAdmin, `{"environment_keys":["a"]}`); response.Code != http.StatusBadRequest || len(repository.tokenEnvironmentChanges) != 0 {
		t.Fatal("replacement body accepted as patch")
	}
	if len(repository.rejectedMutations) != 2 {
		t.Fatal("patch rejections were not audited")
	}
	for _, rejected := range repository.rejectedMutations {
		if rejected.Action != "token.patch_environments" || rejected.ResourceKey != "0123456789abcdef" || rejected.RequestID == "" {
			t.Fatalf("patch rejection metadata: %#v", rejected)
		}
	}
	response := patch(humanauth.RoleAdmin, `{"add":["a"],"remove":["b"]}`)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "environment_keys") {
		t.Fatalf("patch response: %d %s", response.Code, response.Body)
	}
	change := repository.tokenEnvironmentChanges[0]
	if !change.Patch || change.OperationID != "grant-patch-test" || change.PublicID != "0123456789abcdef" || !reflect.DeepEqual(change.Add, []string{"a"}) || !reflect.DeepEqual(change.Remove, []string{"b"}) || change.EnvironmentKeys != nil {
		t.Fatalf("patch request: %#v", change)
	}
}
