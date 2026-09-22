package management_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestRevisionQueriesAreBoundedBeforeReachingStorage(t *testing.T) {
	for _, path := range []string{"/v1/environments/a/configs/payment/revisions", "/v1/vault-items/platform/redis/revisions"} {
		t.Run(path, func(t *testing.T) {
			repository := &recordingConfigWriter{}
			handler := management.NewHandler(repository, nil, nil)
			get := func(query string, authenticated bool) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
				r.URL.RawQuery = query
				if authenticated {
					r = r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "viewer", Role: humanauth.RoleViewer}))
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			for _, query := range []string{
				"limit=0", "limit=-1", "limit=101", "limit=999999999999999999999999", "limit=", "limit=no",
				"before=0", "before=-1", "before=1.5", "before=18446744073709551616", "before=",
				"offset=50", "limit=1&limit=2", "before=2&before=3", "before=%zz", "limit=10;before=2",
			} {
				response := get(query, true)
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
					t.Errorf("query %q = %d %s", query, response.Code, response.Body)
				}
			}
			if response := get("limit=1", false); response.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous status = %d", response.Code)
			}
			if len(repository.revisionQueries) != 0 {
				t.Fatal("invalid or unauthenticated request reached storage")
			}
			for _, test := range []struct {
				query string
				want  mysqlstore.RevisionQuery
			}{
				{"", mysqlstore.RevisionQuery{Limit: 50}},
				{"before=51&limit=1", mysqlstore.RevisionQuery{Before: 51, Limit: 1}},
				{"limit=100&before=18446744073709551615", mysqlstore.RevisionQuery{Before: ^uint64(0), Limit: 100}},
			} {
				response := get(test.query, true)
				if response.Code != http.StatusOK {
					t.Fatalf("query %q = %d %s", test.query, response.Code, response.Body)
				}
				if got := repository.revisionQueries[len(repository.revisionQueries)-1]; got != test.want {
					t.Fatalf("query %q passed %#v, want %#v", test.query, got, test.want)
				}
			}
		})
	}
}
