package management_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type recordingAuthorityWriter struct {
	recordingConfigWriter
	creates []mysqlstore.AuthorityCreate
	issues  []mysqlstore.ClientCertificateIssue
	revokes []mysqlstore.AuthorityRevoke
}

func (writer *recordingAuthorityWriter) ListCertificateAuthorities(context.Context, bool) ([]mysqlstore.CertificateAuthority, error) {
	return []mysqlstore.CertificateAuthority{{ID: "00112233445566778899aabbccddeeff", DisplayName: "Workloads"}}, nil
}
func (writer *recordingAuthorityWriter) CreateCertificateAuthority(_ context.Context, request mysqlstore.AuthorityCreate) (mysqlstore.AuthorityResult, error) {
	writer.creates = append(writer.creates, request)
	return mysqlstore.AuthorityResult{Outcome: mysqlstore.OutcomeSuccess, ExportBundle: "initial-export"}, nil
}
func (writer *recordingAuthorityWriter) IssueClientCertificate(_ context.Context, request mysqlstore.ClientCertificateIssue) (mysqlstore.ClientCertificateIssueResult, error) {
	writer.issues = append(writer.issues, request)
	return mysqlstore.ClientCertificateIssueResult{Outcome: mysqlstore.OutcomeSuccess, ExportBundle: "initial-export"}, nil
}
func (writer *recordingAuthorityWriter) RevokeCertificateAuthority(_ context.Context, request mysqlstore.AuthorityRevoke) (mysqlstore.AuthorityResult, error) {
	writer.revokes = append(writer.revokes, request)
	return mysqlstore.AuthorityResult{Outcome: mysqlstore.OutcomeSuccess}, nil
}

func TestAuthorityManagementRequiresAdminAndCSRFProtection(t *testing.T) {
	tests := []struct{ method, path, body string }{
		{http.MethodGet, "/v1/certificate-authorities", ""},
		{http.MethodPost, "/v1/certificate-authorities", `{"display_name":"Workloads","valid_days":365}`},
		{http.MethodPost, "/v1/certificate-authorities/00112233445566778899aabbccddeeff/revoke", ""},
		{http.MethodPost, "/v1/client-certificates/issue", `{"authority_id":"00112233445566778899aabbccddeeff","display_name":"payments","valid_days":90}`},
	}
	for _, test := range tests {
		for _, role := range []humanauth.Role{"", humanauth.RoleViewer, humanauth.RoleAdmin} {
			writer := &recordingAuthorityWriter{}
			handler := management.NewHandler(writer, nil, nil)
			request := httptest.NewRequest(test.method, "https://configra.test"+test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "operation-1")
			if role != "" {
				request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "operator", Role: role}))
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			want := http.StatusOK
			if role == "" {
				want = http.StatusUnauthorized
			} else if role != humanauth.RoleAdmin {
				want = http.StatusForbidden
			}
			if response.Code != want {
				t.Fatalf("%s %s (%s): status %d, want %d", test.method, test.path, role, response.Code, want)
			}
			if role != humanauth.RoleAdmin && len(writer.creates)+len(writer.issues)+len(writer.revokes) != 0 {
				t.Fatal("unauthorized mutation reached the repository")
			}
			if role == humanauth.RoleAdmin && response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("credential response may be cached")
			}
			if role == humanauth.RoleAdmin && test.method == http.MethodPost {
				before := len(writer.creates) + len(writer.issues) + len(writer.revokes)
				request = httptest.NewRequest(test.method, "https://configra.test"+test.path, strings.NewReader(test.body))
				request.Header.Set("Origin", "https://attacker.example")
				request.Header.Set("Content-Type", "application/json")
				request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "operator", Role: role}))
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden || len(writer.creates)+len(writer.issues)+len(writer.revokes) != before {
					t.Fatal("cross-origin credential mutation was accepted")
				}
			}
		}
	}
}
