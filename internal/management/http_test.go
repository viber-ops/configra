package management_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestViewerReadsIdentityAndEnvironmentList(t *testing.T) {
	created := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	repository := &recordingConfigWriter{environments: []mysqlstore.Environment{{
		Key: "a", DisplayName: "Environment A", CreatedAt: created, UpdatedAt: created,
	}}}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Issuer: "https://identity.example.com", Subject: "viewer-1", Email: "viewer@example.com", Role: humanauth.RoleViewer}

	meRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/me", nil)
	meRequest = meRequest.WithContext(humanauth.WithPrincipal(meRequest.Context(), principal))
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK || meResponse.Body.String() != "{\"issuer\":\"https://identity.example.com\",\"subject\":\"viewer-1\",\"email\":\"viewer@example.com\",\"role\":\"viewer\"}\n" {
		t.Fatalf("me response = %d %q", meResponse.Code, meResponse.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments?include_archived=true", nil)
	listRequest = listRequest.WithContext(humanauth.WithPrincipal(listRequest.Context(), principal))
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"key":"a"`) ||
		!strings.Contains(listResponse.Body.String(), `"created_at":"2026-08-27T01:02:03Z"`) || len(repository.environmentLists) != 1 || !repository.environmentLists[0] {
		t.Fatalf("Environment list = %d %q; calls %#v", listResponse.Code, listResponse.Body.String(), repository.environmentLists)
	}

	invalidRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments?include_archived=yes", nil)
	invalidRequest = invalidRequest.WithContext(humanauth.WithPrincipal(invalidRequest.Context(), principal))
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest || len(repository.environmentLists) != 1 {
		t.Fatalf("invalid list = %d %q; calls %#v", invalidResponse.Code, invalidResponse.Body.String(), repository.environmentLists)
	}
}

func TestViewerReadsBoundedValueFreeAccessAndAuditLogs(t *testing.T) {
	logs := &recordingLogReader{
		access: []logstore.AccessRecord{{
			Time: time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC), Principal: "token-public-id",
			Authentication: machine.AuthenticationMTLS, Environment: "production", ResourceType: "config",
			Resource: "payment", ConfigRevision: 7, VaultRevisions: map[string]uint64{"platform.mysql": 4},
		}},
		audits: []logstore.AuditRecord{{
			ID: "00112233445566778899aabbccddeeff", Time: time.Date(2026, 8, 27, 2, 3, 4, 0, time.UTC),
			EventType: "config.updated", OperationID: "operation-1", ActorType: "user", ActorID: "admin-1",
			Action: "config.commit", Outcome: "success", Environment: "production", ResourceType: "config", Resource: "payment", Revision: 8,
		}},
	}
	handler := management.NewHandler(&recordingConfigWriter{}, nil, nil, logs)
	viewer := humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), viewer))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	access := get("/v1/access?limit=25")
	if access.Code != http.StatusOK || !strings.Contains(access.Body.String(), `"principal":"token-public-id"`) ||
		!strings.Contains(access.Body.String(), `"vault_revisions":{"platform.mysql":4}`) || strings.Contains(access.Body.String(), "content") ||
		len(logs.accessLimits) != 1 || logs.accessLimits[0] != 25 {
		t.Fatalf("Access response = %d %q; limits=%#v", access.Code, access.Body.String(), logs.accessLimits)
	}
	audit := get("/v1/audit?limit=30&offset=10&q=payment")
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"operation_id":"operation-1"`) ||
		!strings.Contains(audit.Body.String(), `"has_more":false`) || strings.Contains(audit.Body.String(), "content") ||
		len(logs.auditQueries) != 1 || logs.auditQueries[0] != (logstore.AuditQuery{Limit: 30, Offset: 10, Search: "payment"}) {
		t.Fatalf("Audit response = %d %q; queries=%#v", audit.Code, audit.Body.String(), logs.auditQueries)
	}
	invalid := get("/v1/access?limit=501")
	if invalid.Code != http.StatusBadRequest || len(logs.accessLimits) != 1 {
		t.Fatalf("invalid Access limit = %d %q; limits=%#v", invalid.Code, invalid.Body.String(), logs.accessLimits)
	}
	invalidAudit := get("/v1/audit?offset=-1")
	if invalidAudit.Code != http.StatusBadRequest || len(logs.auditQueries) != 1 {
		t.Fatalf("invalid Audit offset = %d %q; queries=%#v", invalidAudit.Code, invalidAudit.Body.String(), logs.auditQueries)
	}
}

func TestManagementUIShellIsPublicWithoutWeakeningV1Authentication(t *testing.T) {
	handler := management.NewHandler(&recordingConfigWriter{}, nil, nil)

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "https://configra.test/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="root"></div>`) ||
		page.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		!strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("Management UI = %d %#v %q", page.Code, page.Header(), page.Body.String())
	}
	nonceMarker := "style-src 'self' 'nonce-"
	csp := page.Header().Get("Content-Security-Policy")
	nonceStart := strings.Index(csp, nonceMarker)
	if nonceStart < 0 {
		t.Fatalf("Management UI CSP has no style nonce: %q", csp)
	}
	nonceStart += len(nonceMarker)
	nonceEnd := strings.IndexByte(csp[nonceStart:], '\'')
	if nonceEnd < 0 || !strings.Contains(page.Body.String(), `<meta name="csp-nonce" content="`+csp[nonceStart:nonceStart+nonceEnd]+`">`) {
		t.Fatalf("Management UI does not expose its CSP style nonce: CSP=%q body=%q", csp, page.Body.String())
	}
	assetStart := strings.Index(page.Body.String(), `src="/ui/assets/`)
	if assetStart < 0 {
		t.Fatalf("Management UI has no hashed script: %q", page.Body.String())
	}
	assetPath := page.Body.String()[assetStart+len(`src="`):]
	assetPath = assetPath[:strings.IndexByte(assetPath, '"')]
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "https://configra.test"+assetPath, nil))
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Content-Type"), "javascript") ||
		asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("Management UI asset = %d %#v", asset.Code, asset.Header())
	}

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "https://configra.test/v1/me", nil))
	if api.Code != http.StatusUnauthorized || !strings.Contains(api.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("unauthenticated API = %d %q", api.Code, api.Body.String())
	}
}

func TestAdminCreatesRenamesArchivesAndUnarchivesEnvironment(t *testing.T) {
	repository := &recordingConfigWriter{environmentResult: mysqlstore.EnvironmentChangeResult{
		Outcome: mysqlstore.OutcomeSuccess, Key: "a", DisplayName: "Environment A",
	}}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		action mysqlstore.EnvironmentAction
		nameIn string
	}{
		{"create", http.MethodPost, "/v1/environments", `{"key":"a","display_name":"Environment A"}`, mysqlstore.EnvironmentCreate, "Environment A"},
		{"rename", http.MethodPatch, "/v1/environments/a", `{"display_name":"Primary"}`, mysqlstore.EnvironmentRename, "Primary"},
		{"archive", http.MethodPost, "/v1/environments/a/archive", "", mysqlstore.EnvironmentArchive, ""},
		{"unarchive", http.MethodPost, "/v1/environments/a/unarchive", "", mysqlstore.EnvironmentUnarchive, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "https://configra.test"+test.path, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			request.Header.Set("Idempotency-Key", "operation-environment-"+test.name)
			request = request.WithContext(humanauth.WithPrincipal(request.Context(), principal))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body)
			}
			change := repository.environmentChanges[len(repository.environmentChanges)-1]
			if change.OperationID != "operation-environment-"+test.name || change.Actor.ID != "admin-1" ||
				change.Action != test.action || change.Key != "a" || change.DisplayName != test.nameIn {
				t.Fatalf("Environment change = %#v", change)
			}
		})
	}

	viewerRequest := httptest.NewRequest(http.MethodPost, "https://configra.test/v1/environments", strings.NewReader(`{"key":"b","display_name":"B"}`))
	viewerRequest.Header.Set("Content-Type", "application/json")
	viewerRequest = viewerRequest.WithContext(humanauth.WithPrincipal(viewerRequest.Context(), humanauth.Principal{Subject: "viewer", Role: humanauth.RoleViewer}))
	viewerResponse := httptest.NewRecorder()
	handler.ServeHTTP(viewerResponse, viewerRequest)
	if viewerResponse.Code != http.StatusForbidden || len(repository.environmentChanges) != len(tests) {
		t.Fatalf("viewer create = %d %q; changes %d", viewerResponse.Code, viewerResponse.Body.String(), len(repository.environmentChanges))
	}
}

func TestConfigCommitRequiresAdminAndUsesAuthenticatedSubjectAsActor(t *testing.T) {
	repository := &recordingConfigWriter{result: mysqlstore.ConfigCommitResult{
		Outcome:  mysqlstore.OutcomeSuccess,
		Revision: 1,
	}}
	handler := management.NewHandler(repository, nil, nil)
	body := `{"name":"Payment","expected_revision":0,"format":"yaml","content":"port: 6379\n"}`

	tests := []struct {
		name       string
		principal  *humanauth.Principal
		wantStatus int
		wantCode   string
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized, wantCode: "unauthenticated"},
		{name: "viewer", principal: &humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "admin", principal: &humanauth.Principal{Subject: "admin-1", Email: "admin@example.com", Role: humanauth.RoleAdmin}, wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository.requests = nil
			request := httptest.NewRequest(http.MethodPut, "https://configra.test/v1/environments/a/configs/payment", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "operation-management-config-create")
			if test.principal != nil {
				request = request.WithContext(humanauth.WithPrincipal(request.Context(), *test.principal))
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body)
			}
			if test.wantCode != "" && !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("body = %q, want error code %q", response.Body.String(), test.wantCode)
			}
			if test.principal == nil || test.principal.Role != humanauth.RoleAdmin {
				if len(repository.requests) != 0 {
					t.Fatalf("unauthorized request reached repository: %#v", repository.requests)
				}
				return
			}
			if len(repository.requests) != 1 {
				t.Fatalf("repository calls = %d, want 1", len(repository.requests))
			}
			got := repository.requests[0]
			if got.Actor != (mysqlstore.Actor{Type: "user", ID: test.principal.Subject}) {
				t.Fatalf("Actor = %#v, want authenticated subject", got.Actor)
			}
			if got.OperationID != "operation-management-config-create" || got.EnvironmentKey != "a" || got.ConfigKey != "payment" {
				t.Fatalf("Config identity = %#v", got)
			}
			if got.ConfigName != "Payment" || got.ExpectedRevision != 0 || string(got.Content) != "port: 6379\n" {
				t.Fatalf("Config request = %#v", got)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response headers = %#v", response.Header())
			}
			if response.Body.String() != "{\"outcome\":\"success\",\"revision\":1}\n" {
				t.Fatalf("body = %q", response.Body.String())
			}
		})
	}
}

func TestAuthenticatedUserValidatesAndFormatsConfigWithoutMutation(t *testing.T) {
	repository := &recordingConfigWriter{validationResult: mysqlstore.ConfigValidationResult{
		Content:  "database:\n  password: \"{vault.platform.redis.password}\"\n",
		Warnings: []mysqlstore.ConfigWarning{{Code: "missing_vault_item", NamespaceKey: "platform", ItemKey: "redis", FieldKey: "password"}},
	}}
	handler := management.NewHandler(repository, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "https://configra.test/v1/configs/validate",
		strings.NewReader(`{"environment":"a","format":"yaml","content":"database:\n    password: \"{vault.platform.redis.password}\"\n"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
		Subject: "viewer-1", Role: humanauth.RoleViewer,
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"content":"database:\n  password:`) {
		t.Fatalf("validate response = %d %q", response.Code, response.Body.String())
	}
	if len(repository.validations) != 1 || repository.validations[0].EnvironmentKey != "a" ||
		string(repository.validations[0].Format) != "yaml" {
		t.Fatalf("validation calls = %#v", repository.validations)
	}

	repository.validationErr = mysqlstore.ErrValidation
	invalid := httptest.NewRequest(http.MethodPost, "https://configra.test/v1/configs/validate",
		strings.NewReader(`{"environment":"a","format":"yaml","content":"broken: ["}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalid = invalid.WithContext(humanauth.WithPrincipal(invalid.Context(), humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}))
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidResponse.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("invalid validation response = %d %q", invalidResponse.Code, invalidResponse.Body.String())
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "https://configra.test/v1/configs/validate",
		strings.NewReader(`{"environment":"a","format":"yaml","content":"ok: true"}`)))
	if unauthenticated.Code != http.StatusUnauthorized || len(repository.validations) != 2 {
		t.Fatalf("unauthenticated validation = %d %q; calls=%d", unauthenticated.Code, unauthenticated.Body.String(), len(repository.validations))
	}
}

func TestConfigCommitRejectsOversizedRequestBeforeRepository(t *testing.T) {
	repository := &recordingConfigWriter{}
	handler := management.NewHandler(repository, nil, nil)
	const sentinel = "oversized-config-sentinel"
	body := `{"name":"Payment","expected_revision":0,"format":"yaml","content":"` +
		strings.Repeat("x", 32<<20) + sentinel + `"}`
	request := httptest.NewRequest(http.MethodPut, "https://configra.test/v1/environments/a/configs/payment", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "operation-management-config-oversized")
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
		Subject: "admin-1",
		Role:    humanauth.RoleAdmin,
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body = %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("body = %q, want request_too_large", response.Body.String())
	}
	if strings.Contains(response.Body.String(), sentinel) {
		t.Fatalf("error response leaked request content: %q", response.Body.String())
	}
	if len(repository.requests) != 0 {
		t.Fatalf("oversized request reached repository: %#v", repository.requests)
	}
}

func TestViewerReadsRawConfigWithoutResolvingVaultReferences(t *testing.T) {
	repository := &recordingConfigWriter{raw: mysqlstore.RawConfig{
		EnvironmentKey: "a",
		ConfigKey:      "payment",
		ConfigName:     "Payment",
		Format:         "yaml",
		Content:        "database:\n  password: '{vault.platform.mysql.password}'\n",
		Revision:       7,
	}}
	handler := management.NewHandler(repository, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/a/configs/payment", nil)
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
		Subject: "viewer-1",
		Role:    humanauth.RoleViewer,
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	const want = "{\"environment_key\":\"a\",\"config_key\":\"payment\",\"config_name\":\"Payment\",\"format\":\"yaml\",\"content\":\"database:\\n  password: '{vault.platform.mysql.password}'\\n\",\"revision\":7}\n"
	if response.Body.String() != want {
		t.Fatalf("body = %q, want %q", response.Body.String(), want)
	}
	if repository.reads != 1 {
		t.Fatalf("repository reads = %d, want 1", repository.reads)
	}
}

func TestManagementRejectsCrossOriginMutationBeforeRepository(t *testing.T) {
	repository := &recordingConfigWriter{}
	handler := management.NewHandler(repository, nil, nil)
	request := httptest.NewRequest(
		http.MethodPut,
		"https://configra.test/v1/environments/a/configs/payment",
		strings.NewReader(`{"name":"Payment","format":"yaml","content":"port: 6379\n"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "operation-management-cross-origin")
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
		Subject: "admin-1",
		Role:    humanauth.RoleAdmin,
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"code":"csrf_rejected"`) {
		t.Fatalf("body = %q, want csrf_rejected", response.Body.String())
	}
	if len(repository.requests) != 0 {
		t.Fatalf("cross-origin mutation reached repository: %#v", repository.requests)
	}
}

func TestAdminMergesAnImmutableSourceRevisionIntoTarget(t *testing.T) {
	repository := &recordingConfigWriter{mergeResult: mysqlstore.ConfigCommitResult{
		Outcome:  mysqlstore.OutcomeSuccess,
		Revision: 6,
	}}
	handler := management.NewHandler(repository, nil, nil)
	request := httptest.NewRequest(
		http.MethodPost,
		"https://configra.test/v1/environments/b/configs/payment/merge",
		strings.NewReader(`{"source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4,"expected_target_revision":5}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "operation-management-config-merge")
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
		Subject: "admin-1",
		Role:    humanauth.RoleAdmin,
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	if response.Body.String() != "{\"outcome\":\"success\",\"revision\":6}\n" {
		t.Fatalf("body = %q", response.Body.String())
	}
	if len(repository.merges) != 1 {
		t.Fatalf("Merge calls = %d, want 1", len(repository.merges))
	}
	got := repository.merges[0]
	if got.Actor != (mysqlstore.Actor{Type: "user", ID: "admin-1"}) || got.OperationID != "operation-management-config-merge" {
		t.Fatalf("Merge identity = %#v", got)
	}
	if got.SourceEnvironmentKey != "a" || got.SourceConfigKey != "payment" || got.SourceRevision != 3 ||
		got.TargetEnvironmentKey != "b" || got.TargetConfigKey != "payment" || got.TargetRevision != 4 || got.ExpectedTargetRevision != 5 {
		t.Fatalf("Merge request = %#v", got)
	}
}

func TestAdminPreviewsConfigTransferWithoutMutation(t *testing.T) {
	sourceKey := configRevisionRead{"a", "payment", 3}
	targetKey := configRevisionRead{"b", "payment", 4}
	repository := &recordingConfigWriter{
		rawRevisions: map[configRevisionRead]mysqlstore.RawConfig{sourceKey: {
			EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment", Format: "yaml",
			Content: "database:\n  host: source\n  ports: [2]\n", Revision: 3,
		}},
		raw: mysqlstore.RawConfig{
			EnvironmentKey: "b", ConfigKey: "payment", ConfigName: "Payment", Format: "yaml",
			Content: "current: true\n", Revision: 5,
		},
	}
	handler := management.NewHandler(repository, nil, nil)

	for _, test := range []struct {
		name          string
		mode          string
		targetFormat  string
		targetContent string
		want          string
	}{
		{"merge", "merge", "yaml", "# target\ndatabase:\n  host: target\n  target_only: keep\noutside: yes\n", `{"mode":"merge","format":"yaml","content":"# target\ndatabase:\n  host: source\n  target_only: keep\n  ports: [2]\noutside: yes\n","source_revision":3,"target_revision":4,"expected_target_revision":5}` + "\n"},
		{"replace", "replace", "yaml", "value: target\n", `{"mode":"replace","format":"yaml","content":"database:\n  host: source\n  ports: [2]\n","source_revision":3,"target_revision":4,"expected_target_revision":5}` + "\n"},
		{"replace across formats", "replace", "json", "{\"value\":\"target\"}\n", `{"mode":"replace","format":"yaml","content":"database:\n  host: source\n  ports: [2]\n","source_revision":3,"target_revision":4,"expected_target_revision":5}` + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository.rawRevisions[targetKey] = mysqlstore.RawConfig{EnvironmentKey: "b", ConfigKey: "payment", Format: test.targetFormat, Content: test.targetContent, Revision: 4}
			request := httptest.NewRequest(
				http.MethodPost,
				"https://configra.test/v1/environments/b/configs/payment/transfer-preview",
				strings.NewReader(`{"mode":"`+test.mode+`","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4}`),
			)
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{
				Subject: "admin-1",
				Role:    humanauth.RoleAdmin,
			}))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK || response.Body.String() != test.want {
				t.Fatalf("response = %d %q, want 200 %q", response.Code, response.Body.String(), test.want)
			}
		})
	}
	if len(repository.merges) != 0 || len(repository.replaces) != 0 {
		t.Fatalf("preview mutated Config: merges=%d replaces=%d", len(repository.merges), len(repository.replaces))
	}
}

func TestAdminReadsResolvedConfigPreviewAndEmitsValueFreeAccessEvent(t *testing.T) {
	repository := &recordingConfigWriter{resolved: machine.ResolvedConfig{
		Format: "yaml", Content: "password: resolved-secret\n", ConfigRevision: 7,
		VaultRevisions: map[string]uint64{"platform.mysql": 4},
	}}
	publisher := &managementAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := management.NewHandler(repository, publisher, nil)
	request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/a/configs/payment/resolved-preview", nil)
	request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	const want = `{"format":"yaml","content":"password: resolved-secret\n","config_revision":7,"vault_revisions":{"platform.mysql":4}}` + "\n"
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %q, want 200 %q", response.Code, response.Body.String(), want)
	}
	if len(repository.resolvedReads) != 1 || repository.resolvedReads[0] != ([2]string{"a", "payment"}) {
		t.Fatalf("Resolved Config reads = %#v", repository.resolvedReads)
	}
	event := <-publisher.events
	if event.Principal != "admin-1" || event.Authentication != machine.AuthenticationOIDC || event.Environment != "a" ||
		event.ResourceType != "config" || event.Resource != "payment" || event.ConfigRevision != 7 || event.VaultRevisions["platform.mysql"] != 4 {
		t.Fatalf("Access Event = %#v", event)
	}
}

func TestResolvedConfigPreviewRejectsViewerAndDoesNotEmitEventsForFailedReads(t *testing.T) {
	for _, test := range []struct {
		name       string
		role       humanauth.Role
		err        error
		wantStatus int
		wantReads  int
	}{
		{"viewer", humanauth.RoleViewer, nil, http.StatusForbidden, 0},
		{"unresolved reference", humanauth.RoleAdmin, machine.ErrUnresolved, http.StatusUnprocessableEntity, 1},
		{"integrity failure", humanauth.RoleAdmin, machine.ErrIntegrity, http.StatusInternalServerError, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingConfigWriter{resolvedErr: test.err}
			publisher := &managementAccessPublisher{events: make(chan machine.AccessEvent, 1)}
			handler := management.NewHandler(repository, publisher, nil)
			request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/a/configs/payment/resolved-preview", nil)
			request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "subject-1", Role: test.role}))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus || len(repository.resolvedReads) != test.wantReads || len(publisher.events) != 0 {
				t.Fatalf("response=%d reads=%d events=%d, want %d/%d/0", response.Code, len(repository.resolvedReads), len(publisher.events), test.wantStatus, test.wantReads)
			}
		})
	}
}

func TestConfigTransferPreviewRejectsInvalidOrUnauthorizedRequests(t *testing.T) {
	validSource := mysqlstore.RawConfig{EnvironmentKey: "a", ConfigKey: "payment", Format: "yaml", Content: "value: source\n", Revision: 3}
	validTarget := mysqlstore.RawConfig{EnvironmentKey: "b", ConfigKey: "payment", Format: "yaml", Content: "value: target\n", Revision: 4}
	currentTarget := mysqlstore.RawConfig{EnvironmentKey: "b", ConfigKey: "payment", Format: "yaml", Content: "current: true\n", Revision: 5}
	repository := func(source, target mysqlstore.RawConfig) *recordingConfigWriter {
		return &recordingConfigWriter{rawRevisions: map[configRevisionRead]mysqlstore.RawConfig{
			{"a", "payment", 3}: source,
			{"b", "payment", 4}: target,
		}, raw: currentTarget}
	}
	tests := []struct {
		name       string
		role       humanauth.Role
		body       string
		repository *recordingConfigWriter
		wantStatus int
		wantCode   string
	}{
		{"viewer", humanauth.RoleViewer, `{"mode":"merge","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4}`, repository(validSource, validTarget), http.StatusForbidden, "forbidden"},
		{"unknown mode", humanauth.RoleAdmin, `{"mode":"patch","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4}`, repository(validSource, validTarget), http.StatusUnprocessableEntity, "validation_failed"},
		{"zero source revision", humanauth.RoleAdmin, `{"mode":"merge","source_environment":"a","source_config":"payment","source_revision":0,"target_revision":4}`, repository(validSource, validTarget), http.StatusUnprocessableEntity, "validation_failed"},
		{"zero target revision", humanauth.RoleAdmin, `{"mode":"merge","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":0}`, repository(validSource, validTarget), http.StatusUnprocessableEntity, "validation_failed"},
		{"different formats", humanauth.RoleAdmin, `{"mode":"merge","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4}`, repository(validSource, mysqlstore.RawConfig{EnvironmentKey: "b", ConfigKey: "payment", Format: "json", Revision: 4}), http.StatusUnprocessableEntity, "format_mismatch"},
		{"invalid replace source", humanauth.RoleAdmin, `{"mode":"replace","source_environment":"a","source_config":"payment","source_revision":3,"target_revision":4}`, repository(mysqlstore.RawConfig{EnvironmentKey: "a", ConfigKey: "payment", Format: "yaml", Content: "value: [\n", Revision: 3}, validTarget), http.StatusUnprocessableEntity, "validation_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := management.NewHandler(test.repository, nil, nil)
			request := httptest.NewRequest(http.MethodPost, "https://configra.test/v1/environments/b/configs/payment/transfer-preview", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "subject-1", Role: test.role}))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = %d %s, want %d with code %q", response.Code, response.Body, test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestViewerListsConfigsAndReadsImmutableHistory(t *testing.T) {
	repository := &recordingConfigWriter{
		configs:     []mysqlstore.ConfigSummary{{Key: "payment", DisplayName: "Payment", Environments: []mysqlstore.ConfigEnvironment{{Key: "a", Revision: 2}}}},
		revisions:   []mysqlstore.ConfigRevision{{Revision: 2, Format: "yaml", OperationID: "operation-2", ActorType: "user", ActorID: "admin-1"}},
		rawRevision: mysqlstore.RawConfig{EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment", Format: "yaml", Content: "port: 6379\n", Revision: 1},
	}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}
	request := func(path string) *http.Request {
		result := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
		return result.WithContext(humanauth.WithPrincipal(result.Context(), principal))
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, request("/v1/configs?include_archived=true"))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"key":"payment"`) || len(repository.configLists) != 1 || !repository.configLists[0] {
		t.Fatalf("Config list = %d %q; calls %#v", list.Code, list.Body.String(), repository.configLists)
	}
	history := httptest.NewRecorder()
	handler.ServeHTTP(history, request("/v1/environments/a/configs/payment/revisions"))
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"operation_id":"operation-2"`) || len(repository.historyReads) != 1 {
		t.Fatalf("Config history = %d %q; calls %#v", history.Code, history.Body.String(), repository.historyReads)
	}
	revision := httptest.NewRecorder()
	handler.ServeHTTP(revision, request("/v1/environments/a/configs/payment/revisions/1"))
	if revision.Code != http.StatusOK || !strings.Contains(revision.Body.String(), `"content":"port: 6379\n"`) || len(repository.revisionReads) != 1 || repository.revisionReads[0].revision != 1 {
		t.Fatalf("Config Revision = %d %q; calls %#v", revision.Code, revision.Body.String(), repository.revisionReads)
	}
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, request("/v1/environments/a/configs/payment/revisions/0"))
	if invalid.Code != http.StatusBadRequest || len(repository.revisionReads) != 1 {
		t.Fatalf("invalid Config Revision = %d %q", invalid.Code, invalid.Body.String())
	}
}

func TestAdminReplacesRestoresClonesAndArchivesConfig(t *testing.T) {
	repository := &recordingConfigWriter{
		mergeResult:     mysqlstore.ConfigCommitResult{Outcome: mysqlstore.OutcomeSuccess, Revision: 3},
		lifecycleResult: mysqlstore.ConfigLifecycleResult{Outcome: mysqlstore.OutcomeSuccess, Key: "payment", Archived: true},
	}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	mutation := func(method, path, operationID, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "https://configra.test"+path, strings.NewReader(body))
		request.Header.Set("Idempotency-Key", operationID)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := mutation(http.MethodPost, "/v1/environments/b/configs/payment/replace", "operation-replace",
		`{"source_environment":"a","source_config":"payment","source_revision":1,"target_revision":1,"expected_target_revision":2}`)
	if response.Code != http.StatusOK || len(repository.replaces) != 1 || repository.replaces[0].TargetEnvironmentKey != "b" || repository.replaces[0].TargetRevision != 1 {
		t.Fatalf("Replace = %d %q; calls %#v", response.Code, response.Body.String(), repository.replaces)
	}
	response = mutation(http.MethodPost, "/v1/environments/b/configs/payment/restore", "operation-restore",
		`{"source_revision":2,"expected_revision":3}`)
	if response.Code != http.StatusOK || len(repository.restores) != 1 || repository.restores[0].SourceRevision != 2 {
		t.Fatalf("Restore = %d %q; calls %#v", response.Code, response.Body.String(), repository.restores)
	}
	response = mutation(http.MethodPost, "/v1/environments/a/configs/payment/clone", "operation-clone",
		`{"target_environment":"b","target_config":"payment_copy","target_name":"Payment Copy"}`)
	if response.Code != http.StatusOK || len(repository.clones) != 1 || repository.clones[0].TargetConfigKey != "payment_copy" {
		t.Fatalf("Clone = %d %q; calls %#v", response.Code, response.Body.String(), repository.clones)
	}
	response = mutation(http.MethodPost, "/v1/configs/payment/archive", "operation-archive", "")
	if response.Code != http.StatusOK || len(repository.lifecycleChanges) != 1 || repository.lifecycleChanges[0].Action != mysqlstore.ConfigArchive {
		t.Fatalf("Archive = %d %q; calls %#v", response.Code, response.Body.String(), repository.lifecycleChanges)
	}
	response = mutation(http.MethodPost, "/v1/configs/payment/unarchive", "operation-unarchive", "")
	if response.Code != http.StatusOK || len(repository.lifecycleChanges) != 2 || repository.lifecycleChanges[1].Action != mysqlstore.ConfigUnarchive {
		t.Fatalf("Unarchive = %d %q; calls %#v", response.Code, response.Body.String(), repository.lifecycleChanges)
	}
}

func TestVaultMetadataIsViewerSafeAndValuesRequireAdmin(t *testing.T) {
	secret := "vault-secret-sentinel"
	repository := &recordingConfigWriter{
		vaultItems:  []mysqlstore.VaultItemSummary{{NamespaceKey: "platform", Key: "redis", DisplayName: "Redis", Revision: 2, EnvironmentKeys: []string{"a"}}},
		vaultUsages: []mysqlstore.VaultUsage{{FieldKey: "password", EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment", ConfigRevision: 7}},
		vaultItem: mysqlstore.VaultItem{
			NamespaceKey: "platform", Key: "redis", DisplayName: "Redis", Revision: 2,
			Snapshot: vaultdoc.Snapshot{
				Fields: []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}},
				Variants: []vaultdoc.Variant{{ID: "01010101010101010101010101010101", Environments: []string{"a"}, Values: map[string]vaultdoc.Value{
					"password": {Text: &secret},
				}}},
			},
		},
	}
	publisher := &managementAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := management.NewHandler(repository, publisher, nil)
	viewer := humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}
	admin := humanauth.Principal{Issuer: "https://identity.example.com", Subject: "admin-1", Role: humanauth.RoleAdmin}
	get := func(path string, principal humanauth.Principal) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	list := get("/v1/vault-items", viewer)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"environment_keys":["a"]`) || len(repository.vaultLists) != 1 {
		t.Fatalf("Vault list = %d %q", list.Code, list.Body.String())
	}
	usages := get("/v1/vault-items/platform/redis/usages", viewer)
	if usages.Code != http.StatusOK || !strings.Contains(usages.Body.String(), `"field_key":"password"`) ||
		!strings.Contains(usages.Body.String(), `"config_revision":7`) || strings.Contains(usages.Body.String(), secret) ||
		len(repository.vaultUsageReads) != 1 || repository.vaultUsageReads[0] != [2]string{"platform", "redis"} {
		t.Fatalf("Vault usages = %d %q; reads %#v", usages.Code, usages.Body.String(), repository.vaultUsageReads)
	}
	metadata := get("/v1/vault-items/platform/redis", viewer)
	if metadata.Code != http.StatusOK || strings.Contains(metadata.Body.String(), secret) || len(repository.vaultReads) != 1 || repository.vaultReads[0].includeValues {
		t.Fatalf("Vault metadata = %d %q; reads %#v", metadata.Code, metadata.Body.String(), repository.vaultReads)
	}
	viewerValues := get("/v1/vault-items/platform/redis/values", viewer)
	if viewerValues.Code != http.StatusForbidden || len(repository.vaultReads) != 1 {
		t.Fatalf("viewer Vault values = %d %q; reads %#v", viewerValues.Code, viewerValues.Body.String(), repository.vaultReads)
	}
	values := get("/v1/vault-items/platform/redis/values", admin)
	if values.Code != http.StatusOK || !strings.Contains(values.Body.String(), secret) || len(repository.vaultReads) != 2 || !repository.vaultReads[1].includeValues {
		t.Fatalf("admin Vault values = %d %q; reads %#v", values.Code, values.Body.String(), repository.vaultReads)
	}
	event := <-publisher.events
	if event.Principal != admin.ActorID() || event.Authentication != machine.AuthenticationOIDC || event.ResourceType != "vault_item" ||
		event.Namespace != "platform" || event.Resource != "redis" || event.VaultRevisions["platform.redis"] != 2 {
		t.Fatalf("Vault Access Event = %#v", event)
	}
}

func TestViewerReadsVaultHistoryWithoutValues(t *testing.T) {
	secret := "historical-vault-secret-sentinel"
	restoredFrom := uint64(1)
	repository := &recordingConfigWriter{
		vaultRevisions: []mysqlstore.VaultRevision{{Revision: 2, OperationID: "operation-restore", RestoredFromRevision: &restoredFrom}},
		vaultItem: mysqlstore.VaultItem{NamespaceKey: "platform", Key: "redis", DisplayName: "Redis", Revision: 1, Snapshot: vaultdoc.Snapshot{
			Fields: []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}},
			Variants: []vaultdoc.Variant{{ID: "01010101010101010101010101010101", Values: map[string]vaultdoc.Value{
				"password": {Text: &secret},
			}}},
		}},
	}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer}
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "https://configra.test"+path, nil)
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	history := get("/v1/vault-items/platform/redis/revisions")
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"restored_from_revision":1`) {
		t.Fatalf("Vault history = %d %q", history.Code, history.Body.String())
	}
	revision := get("/v1/vault-items/platform/redis/revisions/1")
	if revision.Code != http.StatusOK || strings.Contains(revision.Body.String(), secret) || len(repository.vaultReads) != 1 || repository.vaultReads[0].revision != 1 || repository.vaultReads[0].includeValues {
		t.Fatalf("Vault Revision = %d %q; reads %#v", revision.Code, revision.Body.String(), repository.vaultReads)
	}
	invalid := get("/v1/vault-items/platform/redis/revisions/0")
	if invalid.Code != http.StatusBadRequest || len(repository.vaultReads) != 1 {
		t.Fatalf("invalid Vault Revision = %d %q; reads %#v", invalid.Code, invalid.Body.String(), repository.vaultReads)
	}
}

func TestVaultValueReadDistinguishesMissingAndIntegrityFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"missing", mysqlstore.ErrNotFound, http.StatusNotFound, "not_found"},
		{"integrity", errors.Join(vaultcrypto.ErrIntegrity), http.StatusInternalServerError, "crypto_integrity_failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingConfigWriter{vaultReadErr: test.err}
			handler := management.NewHandler(repository, nil, nil)
			request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/vault-items/platform/redis/values", nil)
			request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = %d %q", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminCommitsRestoresAndArchivesVaultItem(t *testing.T) {
	repository := &recordingConfigWriter{
		vaultCommitResult:    mysqlstore.VaultCommitResult{Outcome: mysqlstore.OutcomeSuccess, Revision: 2},
		vaultLifecycleResult: mysqlstore.VaultLifecycleResult{Outcome: mysqlstore.OutcomeSuccess, NamespaceKey: "platform", Key: "redis", Archived: true},
	}
	handler := management.NewHandler(repository, nil, nil)
	principal := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	mutation := func(path, operationID, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "https://configra.test"+path, strings.NewReader(body))
		if strings.Contains(path, "/vault-items/platform/redis") && !strings.HasSuffix(path, "/restore") && !strings.HasSuffix(path, "/archive") && !strings.HasSuffix(path, "/unarchive") {
			request.Method = http.MethodPut
		}
		request.Header.Set("Idempotency-Key", operationID)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	commit := mutation("/v1/vault-items/platform/redis", "operation-vault-commit", `{"display_name":"Redis","expected_revision":1,"snapshot":{"fields":[{"key":"password","name":"Password","type":"secret"}],"variants":[{"id":"01010101010101010101010101010101","environments":["a"],"values":{"password":{"text":"new-secret"}}}]}}`)
	if commit.Code != http.StatusOK || len(repository.vaultCommits) != 1 || repository.vaultCommits[0].NamespaceKey != "platform" || repository.vaultCommits[0].ItemKey != "redis" || repository.vaultCommits[0].ExpectedRevision != 1 {
		t.Fatalf("Vault Commit = %d %q; calls %#v", commit.Code, commit.Body.String(), repository.vaultCommits)
	}
	restore := mutation("/v1/vault-items/platform/redis/restore", "operation-vault-restore", `{"source_revision":1,"expected_revision":2}`)
	if restore.Code != http.StatusOK || len(repository.vaultRestores) != 1 || repository.vaultRestores[0].SourceRevision != 1 {
		t.Fatalf("Vault Restore = %d %q; calls %#v", restore.Code, restore.Body.String(), repository.vaultRestores)
	}
	archive := mutation("/v1/vault-items/platform/redis/archive", "operation-vault-archive", "")
	if archive.Code != http.StatusOK || len(repository.vaultLifecycleChanges) != 1 || repository.vaultLifecycleChanges[0].Action != mysqlstore.VaultArchive {
		t.Fatalf("Vault Archive = %d %q; calls %#v", archive.Code, archive.Body.String(), repository.vaultLifecycleChanges)
	}
	unarchive := mutation("/v1/vault-items/platform/redis/unarchive", "operation-vault-unarchive", "")
	if unarchive.Code != http.StatusOK || len(repository.vaultLifecycleChanges) != 2 || repository.vaultLifecycleChanges[1].Action != mysqlstore.VaultUnarchive {
		t.Fatalf("Vault Unarchive = %d %q; calls %#v", unarchive.Code, unarchive.Body.String(), repository.vaultLifecycleChanges)
	}
}

func TestAdminManagesEnvironmentScopedAPITokensWithoutListingSecrets(t *testing.T) {
	const publicID = "0123456789abcdef"
	const plaintext = "cfg_0123456789abcdef_one-time-token-sentinel"
	repository := &recordingConfigWriter{
		tokens: []mysqlstore.TokenSummary{{
			PublicID: publicID, DisplayName: "Datacenter A", DisplayPrefix: "cfg_0123456789abcdef",
			EnvironmentKeys: []string{"a"}, AllowWithoutMTLS: true,
		}},
		tokenCreateResult: mysqlstore.TokenCreateResult{
			Outcome: mysqlstore.OutcomeSuccess, PublicID: publicID, DisplayPrefix: "cfg_0123456789abcdef",
			Token: plaintext, EnvironmentKeys: []string{"a"}, AllowWithoutMTLS: true,
		},
		tokenEnvironmentResult: mysqlstore.TokenEnvironmentResult{
			Outcome: mysqlstore.OutcomeSuccess, PublicID: publicID, EnvironmentKeys: []string{"b"},
		},
		tokenRevokeResult: mysqlstore.TokenRevokeResult{Outcome: mysqlstore.OutcomeSuccess, PublicID: publicID, Revoked: true},
	}
	handler := management.NewHandler(repository, nil, nil)
	admin := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	request := func(method, path, operationID, body string, principal humanauth.Principal) *httptest.ResponseRecorder {
		input := httptest.NewRequest(method, "https://configra.test"+path, strings.NewReader(body))
		if body != "" {
			input.Header.Set("Content-Type", "application/json")
		}
		if operationID != "" {
			input.Header.Set("Idempotency-Key", operationID)
		}
		input = input.WithContext(humanauth.WithPrincipal(input.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, input)
		return response
	}

	viewer := request(http.MethodGet, "/v1/api-tokens", "", "", humanauth.Principal{Subject: "viewer-1", Role: humanauth.RoleViewer})
	if viewer.Code != http.StatusForbidden || len(repository.tokenLists) != 0 {
		t.Fatalf("viewer Token list = %d %q", viewer.Code, viewer.Body.String())
	}
	list := request(http.MethodGet, "/v1/api-tokens?include_revoked=true", "", "", admin)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"public_id":"`+publicID+`"`) ||
		strings.Contains(list.Body.String(), plaintext) || len(repository.tokenLists) != 1 || !repository.tokenLists[0] {
		t.Fatalf("Token list = %d %q; calls %#v", list.Code, list.Body.String(), repository.tokenLists)
	}
	created := request(http.MethodPost, "/v1/api-tokens", "operation-token-create",
		`{"display_name":"Datacenter A","environment_keys":["a"],"allow_without_mtls":true,"never_expires":true}`, admin)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), plaintext) || len(repository.tokenCreates) != 1 ||
		!repository.tokenCreates[0].AllowWithoutMTLS || !repository.tokenCreates[0].NeverExpires {
		t.Fatalf("Token create = %d %q; calls %#v", created.Code, created.Body.String(), repository.tokenCreates)
	}
	updated := request(http.MethodPut, "/v1/api-tokens/"+publicID+"/environments", "operation-token-environments",
		`{"environment_keys":["b"]}`, admin)
	if updated.Code != http.StatusOK || len(repository.tokenEnvironmentChanges) != 1 || repository.tokenEnvironmentChanges[0].EnvironmentKeys[0] != "b" {
		t.Fatalf("Token environments = %d %q; calls %#v", updated.Code, updated.Body.String(), repository.tokenEnvironmentChanges)
	}
	revoked := request(http.MethodPost, "/v1/api-tokens/"+publicID+"/revoke", "operation-token-revoke", "", admin)
	if revoked.Code != http.StatusOK || len(repository.tokenRevokes) != 1 || repository.tokenRevokes[0].PublicID != publicID {
		t.Fatalf("Token revoke = %d %q; calls %#v", revoked.Code, revoked.Body.String(), repository.tokenRevokes)
	}
}

func TestAdminImportsOnlyClientCASignedCertificatesAndRevokesByFingerprint(t *testing.T) {
	roots, certificatePEM := signedClientCertificate(t, "datacenter-a")
	_, untrustedPEM := signedClientCertificate(t, "untrusted")
	const fingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	repository := &recordingConfigWriter{
		certificates: []mysqlstore.ClientCertificate{{FingerprintSHA256: fingerprint, DisplayName: "Datacenter A", Subject: "CN=datacenter-a"}},
		certificateRegisterResult: mysqlstore.ClientCertificateResult{
			Outcome: mysqlstore.OutcomeSuccess, FingerprintSHA256: fingerprint, DisplayName: "Datacenter A", Subject: "CN=datacenter-a",
		},
		certificateRevokeResult: mysqlstore.ClientCertificateResult{
			Outcome: mysqlstore.OutcomeSuccess, FingerprintSHA256: fingerprint, DisplayName: "Datacenter A", Revoked: true,
		},
	}
	handler := management.NewHandler(repository, nil, roots)
	admin := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	request := func(method, path, operationID string, body []byte) *httptest.ResponseRecorder {
		input := httptest.NewRequest(method, "https://configra.test"+path, strings.NewReader(string(body)))
		if len(body) > 0 {
			input.Header.Set("Content-Type", "application/json")
		}
		input.Header.Set("Idempotency-Key", operationID)
		input = input.WithContext(humanauth.WithPrincipal(input.Context(), admin))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, input)
		return response
	}

	list := request(http.MethodGet, "/v1/client-certificates?include_revoked=true", "", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), fingerprint) || len(repository.certificateLists) != 1 || !repository.certificateLists[0] {
		t.Fatalf("Certificate list = %d %q; calls %#v", list.Code, list.Body.String(), repository.certificateLists)
	}
	body, _ := json.Marshal(map[string]string{"display_name": "Datacenter A", "certificate_pem": certificatePEM})
	registered := request(http.MethodPost, "/v1/client-certificates", "operation-certificate-register", body)
	if registered.Code != http.StatusOK || len(repository.certificateRegisters) != 1 ||
		repository.certificateRegisters[0].Certificate.Subject.CommonName != "datacenter-a" {
		t.Fatalf("Certificate register = %d %q; calls %#v", registered.Code, registered.Body.String(), repository.certificateRegisters)
	}
	untrustedBody, _ := json.Marshal(map[string]string{"display_name": "Untrusted", "certificate_pem": untrustedPEM})
	untrusted := request(http.MethodPost, "/v1/client-certificates", "operation-certificate-untrusted", untrustedBody)
	if untrusted.Code != http.StatusUnprocessableEntity || len(repository.certificateRegisters) != 1 || strings.Contains(untrusted.Body.String(), untrustedPEM) {
		t.Fatalf("untrusted Certificate = %d %q; calls %#v", untrusted.Code, untrusted.Body.String(), repository.certificateRegisters)
	}
	revoked := request(http.MethodPost, "/v1/client-certificates/"+fingerprint+"/revoke", "operation-certificate-revoke", nil)
	if revoked.Code != http.StatusOK || len(repository.certificateRevokes) != 1 || repository.certificateRevokes[0].FingerprintSHA256 != fingerprint {
		t.Fatalf("Certificate revoke = %d %q; calls %#v", revoked.Code, revoked.Body.String(), repository.certificateRevokes)
	}
}

func TestAdminManagesNotificationDestinationsTestsHistoryAndRedelivery(t *testing.T) {
	const (
		urlSentinel    = "https://hooks.example.com/services/url-token-sentinel"
		secretSentinel = "notification-secret-sentinel"
	)
	repository := &recordingConfigWriter{
		notificationDestinations: []mysqlstore.NotificationDestination{{
			Key: "operations", DisplayName: "Operations", Provider: mysqlstore.NotificationGenericWebhook,
			SafeHost: "hooks.example.com", MaskedSuffix: "••••inel", Enabled: true,
			EventTypes: []string{"config.created"},
		}},
		notificationDestinationResult: mysqlstore.NotificationDestinationResult{
			Outcome: mysqlstore.OutcomeSuccess, Key: "operations", DisplayName: "Operations",
			Provider: mysqlstore.NotificationGenericWebhook, SafeHost: "hooks.example.com", MaskedSuffix: "••••inel", Enabled: true,
			EventTypes: []string{"config.created"},
		},
		notificationLifecycleResult: mysqlstore.NotificationDestinationLifecycleResult{
			Outcome: mysqlstore.OutcomeSuccess, Key: "operations", Archived: true,
		},
		notificationDeliveries: []mysqlstore.NotificationDelivery{{
			ID: "00112233445566778899aabbccddeeff", OutboxEventID: "ffeeddccbbaa99887766554433221100",
			EventType: "config.created", DestinationKey: "operations", Attempt: 1,
			Status: mysqlstore.NotificationDeliverySucceeded, HTTPStatus: 204,
		}},
		notificationQueueResult: mysqlstore.NotificationQueueResult{
			Outcome: mysqlstore.OutcomeSuccess, DestinationKey: "operations", OutboxEventID: "ffeeddccbbaa99887766554433221100",
		},
	}
	handler := management.NewHandler(repository, nil, nil)
	admin := humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}
	request := func(method, path, operationID, body string, principal humanauth.Principal) *httptest.ResponseRecorder {
		input := httptest.NewRequest(method, "https://configra.test"+path, strings.NewReader(body))
		if body != "" {
			input.Header.Set("Content-Type", "application/json")
		}
		if operationID != "" {
			input.Header.Set("Idempotency-Key", operationID)
		}
		input = input.WithContext(humanauth.WithPrincipal(input.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, input)
		return response
	}

	viewer := request(http.MethodGet, "/v1/notification-destinations", "", "", humanauth.Principal{Subject: "viewer", Role: humanauth.RoleViewer})
	if viewer.Code != http.StatusForbidden || len(repository.notificationDestinationLists) != 0 {
		t.Fatalf("viewer Destination list = %d %q", viewer.Code, viewer.Body.String())
	}
	list := request(http.MethodGet, "/v1/notification-destinations?include_archived=true", "", "", admin)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"safe_host":"hooks.example.com"`) ||
		strings.Contains(list.Body.String(), "url-token-sentinel") || strings.Contains(list.Body.String(), secretSentinel) ||
		len(repository.notificationDestinationLists) != 1 || !repository.notificationDestinationLists[0] {
		t.Fatalf("Destination list = %d %q; calls %#v", list.Code, list.Body.String(), repository.notificationDestinationLists)
	}
	commit := request(http.MethodPut, "/v1/notification-destinations/operations", "notification-destination-put",
		`{"display_name":"Operations","provider":"generic_webhook","url":"`+urlSentinel+`","secret":"`+secretSentinel+`","enabled":true,"event_types":["config.created"]}`, admin)
	if commit.Code != http.StatusOK || len(repository.notificationDestinationCommits) != 1 ||
		repository.notificationDestinationCommits[0].URL == nil || *repository.notificationDestinationCommits[0].URL != urlSentinel ||
		repository.notificationDestinationCommits[0].Secret == nil || *repository.notificationDestinationCommits[0].Secret != secretSentinel ||
		strings.Contains(commit.Body.String(), urlSentinel) || strings.Contains(commit.Body.String(), secretSentinel) {
		t.Fatalf("Destination commit = %d %q; calls %#v", commit.Code, commit.Body.String(), repository.notificationDestinationCommits)
	}
	invalidFeishu := request(http.MethodPut, "/v1/notification-destinations/feishu", "notification-destination-feishu",
		`{"display_name":"Feishu","provider":"feishu_bot","url":"https://evil.example.com/hook/token","enabled":true}`, admin)
	if invalidFeishu.Code != http.StatusUnprocessableEntity || len(repository.notificationDestinationCommits) != 1 ||
		strings.Contains(invalidFeishu.Body.String(), "evil.example.com") {
		t.Fatalf("invalid Feishu = %d %q; calls %#v", invalidFeishu.Code, invalidFeishu.Body.String(), repository.notificationDestinationCommits)
	}
	testSend := request(http.MethodPost, "/v1/notification-destinations/operations/test", "notification-destination-test", "", admin)
	if testSend.Code != http.StatusOK || len(repository.notificationTests) != 1 || repository.notificationTests[0].DestinationKey != "operations" {
		t.Fatalf("Destination test = %d %q; calls %#v", testSend.Code, testSend.Body.String(), repository.notificationTests)
	}
	history := request(http.MethodGet, "/v1/notification-destinations/operations/deliveries?limit=50", "", "", admin)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"http_status":204`) ||
		strings.Contains(history.Body.String(), urlSentinel) || strings.Contains(history.Body.String(), secretSentinel) ||
		len(repository.notificationDeliveryLists) != 1 || repository.notificationDeliveryLists[0].limit != 50 {
		t.Fatalf("Delivery history = %d %q; calls %#v", history.Code, history.Body.String(), repository.notificationDeliveryLists)
	}
	redelivery := request(http.MethodPost,
		"/v1/notification-destinations/operations/deliveries/00112233445566778899aabbccddeeff/redeliver",
		"notification-redelivery-create", "", admin)
	if redelivery.Code != http.StatusOK || len(repository.notificationRedeliveries) != 1 ||
		repository.notificationRedeliveries[0].DeliveryID != "00112233445566778899aabbccddeeff" {
		t.Fatalf("redelivery = %d %q; calls %#v", redelivery.Code, redelivery.Body.String(), repository.notificationRedeliveries)
	}
	archive := request(http.MethodPost, "/v1/notification-destinations/operations/archive", "notification-destination-archive", "", admin)
	unarchive := request(http.MethodPost, "/v1/notification-destinations/operations/unarchive", "notification-destination-unarchive", "", admin)
	if archive.Code != http.StatusOK || unarchive.Code != http.StatusOK || len(repository.notificationLifecycleChanges) != 2 ||
		repository.notificationLifecycleChanges[0].Action != mysqlstore.NotificationDestinationArchive ||
		repository.notificationLifecycleChanges[1].Action != mysqlstore.NotificationDestinationUnarchive {
		t.Fatalf("Destination lifecycle = %d/%d; calls %#v", archive.Code, unarchive.Code, repository.notificationLifecycleChanges)
	}
}

func signedClientCertificate(t *testing.T, commonName string) (*x509.CertPool, string) {
	t.Helper()
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Configra Client CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	leafPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Client Certificate key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, leafPublic, caPrivate)
	if err != nil {
		t.Fatalf("create Client Certificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return roots, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}))
}

type recordingConfigWriter struct {
	management.AuthorityRepository
	requests                       []mysqlstore.ConfigCommit
	validations                    []mysqlstore.ConfigValidation
	validationResult               mysqlstore.ConfigValidationResult
	validationErr                  error
	result                         mysqlstore.ConfigCommitResult
	err                            error
	raw                            mysqlstore.RawConfig
	readErr                        error
	reads                          int
	merges                         []mysqlstore.ConfigTransfer
	mergeResult                    mysqlstore.ConfigCommitResult
	mergeErr                       error
	environments                   []mysqlstore.Environment
	environmentLists               []bool
	environmentChanges             []mysqlstore.EnvironmentChange
	environmentResult              mysqlstore.EnvironmentChangeResult
	environmentErr                 error
	configs                        []mysqlstore.ConfigSummary
	configLists                    []bool
	revisions                      []mysqlstore.ConfigRevision
	historyReads                   [][2]string
	rawRevision                    mysqlstore.RawConfig
	rawRevisions                   map[configRevisionRead]mysqlstore.RawConfig
	revisionReads                  []configRevisionRead
	replaces                       []mysqlstore.ConfigTransfer
	restores                       []mysqlstore.ConfigRestore
	clones                         []mysqlstore.ConfigClone
	lifecycleChanges               []mysqlstore.ConfigLifecycleChange
	lifecycleResult                mysqlstore.ConfigLifecycleResult
	vaultItems                     []mysqlstore.VaultItemSummary
	vaultLists                     []bool
	vaultUsages                    []mysqlstore.VaultUsage
	vaultUsageReads                [][2]string
	vaultItem                      mysqlstore.VaultItem
	vaultReadErr                   error
	vaultReads                     []vaultRead
	vaultRevisions                 []mysqlstore.VaultRevision
	vaultCommits                   []mysqlstore.VaultCommit
	vaultCommitResult              mysqlstore.VaultCommitResult
	vaultRestores                  []mysqlstore.VaultRestore
	vaultLifecycleChanges          []mysqlstore.VaultLifecycleChange
	vaultLifecycleResult           mysqlstore.VaultLifecycleResult
	tokens                         []mysqlstore.TokenSummary
	tokenLists                     []bool
	tokenCreates                   []mysqlstore.TokenCreate
	tokenCreateResult              mysqlstore.TokenCreateResult
	tokenEnvironmentChanges        []mysqlstore.TokenEnvironmentChange
	tokenEnvironmentResult         mysqlstore.TokenEnvironmentResult
	tokenRevokes                   []mysqlstore.TokenRevoke
	tokenRevokeResult              mysqlstore.TokenRevokeResult
	certificates                   []mysqlstore.ClientCertificate
	certificateLists               []bool
	certificateRegisters           []mysqlstore.ClientCertificateRegister
	certificateRegisterResult      mysqlstore.ClientCertificateResult
	certificateRevokes             []mysqlstore.ClientCertificateRevoke
	certificateRevokeResult        mysqlstore.ClientCertificateResult
	notificationDestinations       []mysqlstore.NotificationDestination
	notificationDestinationLists   []bool
	notificationDestinationCommits []mysqlstore.NotificationDestinationCommit
	notificationDestinationResult  mysqlstore.NotificationDestinationResult
	notificationLifecycleChanges   []mysqlstore.NotificationDestinationLifecycle
	notificationLifecycleResult    mysqlstore.NotificationDestinationLifecycleResult
	notificationDeliveries         []mysqlstore.NotificationDelivery
	notificationDeliveryLists      []notificationDeliveryList
	notificationTests              []mysqlstore.NotificationTestRequest
	notificationRedeliveries       []mysqlstore.NotificationRedeliveryRequest
	notificationQueueResult        mysqlstore.NotificationQueueResult
	resolved                       machine.ResolvedConfig
	resolvedErr                    error
	resolvedReads                  [][2]string
}

type notificationDeliveryList struct {
	destination string
	limit       int
}

type recordingLogReader struct {
	access       []logstore.AccessRecord
	audits       []logstore.AuditRecord
	accessLimits []int
	auditQueries []logstore.AuditQuery
	err          error
}

func (reader *recordingLogReader) ListAccess(_ context.Context, limit int) ([]logstore.AccessRecord, error) {
	reader.accessLimits = append(reader.accessLimits, limit)
	return reader.access, reader.err
}

func (reader *recordingLogReader) ListAudits(_ context.Context, query logstore.AuditQuery) (logstore.AuditPage, error) {
	reader.auditQueries = append(reader.auditQueries, query)
	return logstore.AuditPage{Items: reader.audits}, reader.err
}

type configRevisionRead struct {
	environment string
	config      string
	revision    uint64
}

type vaultRead struct {
	namespace     string
	item          string
	revision      uint64
	includeValues bool
}

func (writer *recordingConfigWriter) ListEnvironments(_ context.Context, includeArchived bool) ([]mysqlstore.Environment, error) {
	writer.environmentLists = append(writer.environmentLists, includeArchived)
	return writer.environments, writer.environmentErr
}

func (writer *recordingConfigWriter) ApplyEnvironmentChange(_ context.Context, request mysqlstore.EnvironmentChange) (mysqlstore.EnvironmentChangeResult, error) {
	writer.environmentChanges = append(writer.environmentChanges, request)
	return writer.environmentResult, writer.environmentErr
}

func (writer *recordingConfigWriter) ListConfigs(_ context.Context, includeArchived bool) ([]mysqlstore.ConfigSummary, error) {
	writer.configLists = append(writer.configLists, includeArchived)
	return writer.configs, nil
}

func (writer *recordingConfigWriter) ListConfigRevisions(_ context.Context, environment, config string) ([]mysqlstore.ConfigRevision, error) {
	writer.historyReads = append(writer.historyReads, [2]string{environment, config})
	return writer.revisions, nil
}

func (writer *recordingConfigWriter) ReadRawConfigRevision(_ context.Context, environment, config string, revision uint64) (mysqlstore.RawConfig, error) {
	key := configRevisionRead{environment, config, revision}
	writer.revisionReads = append(writer.revisionReads, key)
	if raw, ok := writer.rawRevisions[key]; ok {
		return raw, nil
	}
	return writer.rawRevision, nil
}

func (writer *recordingConfigWriter) ReplaceConfig(_ context.Context, request mysqlstore.ConfigTransfer) (mysqlstore.ConfigCommitResult, error) {
	writer.replaces = append(writer.replaces, request)
	return writer.mergeResult, writer.mergeErr
}

func (writer *recordingConfigWriter) RestoreConfig(_ context.Context, request mysqlstore.ConfigRestore) (mysqlstore.ConfigCommitResult, error) {
	writer.restores = append(writer.restores, request)
	return writer.mergeResult, writer.mergeErr
}

func (writer *recordingConfigWriter) CloneConfig(_ context.Context, request mysqlstore.ConfigClone) (mysqlstore.ConfigCommitResult, error) {
	writer.clones = append(writer.clones, request)
	return writer.mergeResult, writer.mergeErr
}

func (writer *recordingConfigWriter) ApplyConfigLifecycleChange(_ context.Context, request mysqlstore.ConfigLifecycleChange) (mysqlstore.ConfigLifecycleResult, error) {
	writer.lifecycleChanges = append(writer.lifecycleChanges, request)
	return writer.lifecycleResult, writer.mergeErr
}

func (writer *recordingConfigWriter) ListVaultItems(_ context.Context, includeArchived bool) ([]mysqlstore.VaultItemSummary, error) {
	writer.vaultLists = append(writer.vaultLists, includeArchived)
	return writer.vaultItems, nil
}

func (writer *recordingConfigWriter) ListVaultUsages(_ context.Context, namespace, item string) ([]mysqlstore.VaultUsage, error) {
	writer.vaultUsageReads = append(writer.vaultUsageReads, [2]string{namespace, item})
	return writer.vaultUsages, nil
}

func (writer *recordingConfigWriter) ReadVaultItem(_ context.Context, namespace, item string, revision uint64, includeValues bool) (mysqlstore.VaultItem, error) {
	writer.vaultReads = append(writer.vaultReads, vaultRead{namespace, item, revision, includeValues})
	return writer.vaultItem, writer.vaultReadErr
}

func (writer *recordingConfigWriter) ListVaultRevisions(context.Context, string, string) ([]mysqlstore.VaultRevision, error) {
	return writer.vaultRevisions, nil
}

func (writer *recordingConfigWriter) CommitVault(_ context.Context, request mysqlstore.VaultCommit) (mysqlstore.VaultCommitResult, error) {
	writer.vaultCommits = append(writer.vaultCommits, request)
	return writer.vaultCommitResult, nil
}

func (writer *recordingConfigWriter) RestoreVault(_ context.Context, request mysqlstore.VaultRestore) (mysqlstore.VaultCommitResult, error) {
	writer.vaultRestores = append(writer.vaultRestores, request)
	return writer.vaultCommitResult, nil
}

func (writer *recordingConfigWriter) ApplyVaultLifecycleChange(_ context.Context, request mysqlstore.VaultLifecycleChange) (mysqlstore.VaultLifecycleResult, error) {
	writer.vaultLifecycleChanges = append(writer.vaultLifecycleChanges, request)
	return writer.vaultLifecycleResult, nil
}

func (writer *recordingConfigWriter) ListTokens(_ context.Context, includeRevoked bool) ([]mysqlstore.TokenSummary, error) {
	writer.tokenLists = append(writer.tokenLists, includeRevoked)
	return writer.tokens, nil
}

func (writer *recordingConfigWriter) CreateToken(_ context.Context, request mysqlstore.TokenCreate) (mysqlstore.TokenCreateResult, error) {
	writer.tokenCreates = append(writer.tokenCreates, request)
	return writer.tokenCreateResult, nil
}

func (writer *recordingConfigWriter) SetTokenEnvironments(_ context.Context, request mysqlstore.TokenEnvironmentChange) (mysqlstore.TokenEnvironmentResult, error) {
	writer.tokenEnvironmentChanges = append(writer.tokenEnvironmentChanges, request)
	return writer.tokenEnvironmentResult, nil
}

func (writer *recordingConfigWriter) RevokeToken(_ context.Context, request mysqlstore.TokenRevoke) (mysqlstore.TokenRevokeResult, error) {
	writer.tokenRevokes = append(writer.tokenRevokes, request)
	return writer.tokenRevokeResult, nil
}

func (writer *recordingConfigWriter) ListClientCertificates(_ context.Context, includeRevoked bool) ([]mysqlstore.ClientCertificate, error) {
	writer.certificateLists = append(writer.certificateLists, includeRevoked)
	return writer.certificates, nil
}

func (writer *recordingConfigWriter) ActiveCertificateAuthorities(context.Context) ([][]byte, error) {
	return nil, nil
}

func (writer *recordingConfigWriter) RegisterVerifiedClientCertificate(_ context.Context, request mysqlstore.ClientCertificateRegister) (mysqlstore.ClientCertificateResult, error) {
	writer.certificateRegisters = append(writer.certificateRegisters, request)
	return writer.certificateRegisterResult, nil
}

func (writer *recordingConfigWriter) RevokeClientCertificate(_ context.Context, request mysqlstore.ClientCertificateRevoke) (mysqlstore.ClientCertificateResult, error) {
	writer.certificateRevokes = append(writer.certificateRevokes, request)
	return writer.certificateRevokeResult, nil
}

func (writer *recordingConfigWriter) ListNotificationDestinations(_ context.Context, includeArchived bool) ([]mysqlstore.NotificationDestination, error) {
	writer.notificationDestinationLists = append(writer.notificationDestinationLists, includeArchived)
	return writer.notificationDestinations, nil
}

func (writer *recordingConfigWriter) CommitNotificationDestination(_ context.Context, request mysqlstore.NotificationDestinationCommit) (mysqlstore.NotificationDestinationResult, error) {
	writer.notificationDestinationCommits = append(writer.notificationDestinationCommits, request)
	return writer.notificationDestinationResult, nil
}

func (writer *recordingConfigWriter) ApplyNotificationDestinationLifecycle(_ context.Context, request mysqlstore.NotificationDestinationLifecycle) (mysqlstore.NotificationDestinationLifecycleResult, error) {
	writer.notificationLifecycleChanges = append(writer.notificationLifecycleChanges, request)
	return writer.notificationLifecycleResult, nil
}

func (writer *recordingConfigWriter) ListNotificationDeliveries(_ context.Context, destination string, limit int) ([]mysqlstore.NotificationDelivery, error) {
	writer.notificationDeliveryLists = append(writer.notificationDeliveryLists, notificationDeliveryList{destination, limit})
	return writer.notificationDeliveries, nil
}

func (writer *recordingConfigWriter) QueueNotificationTest(_ context.Context, request mysqlstore.NotificationTestRequest) (mysqlstore.NotificationQueueResult, error) {
	writer.notificationTests = append(writer.notificationTests, request)
	return writer.notificationQueueResult, nil
}

func (writer *recordingConfigWriter) RedeliverNotification(_ context.Context, request mysqlstore.NotificationRedeliveryRequest) (mysqlstore.NotificationQueueResult, error) {
	writer.notificationRedeliveries = append(writer.notificationRedeliveries, request)
	result := writer.notificationQueueResult
	result.SourceDeliveryID = request.DeliveryID
	return result, nil
}

type managementAccessPublisher struct {
	events chan machine.AccessEvent
}

func (publisher *managementAccessPublisher) TryPublish(event machine.AccessEvent) bool {
	publisher.events <- event
	return true
}

func (writer *recordingConfigWriter) MergeConfig(_ context.Context, request mysqlstore.ConfigTransfer) (mysqlstore.ConfigCommitResult, error) {
	writer.merges = append(writer.merges, request)
	return writer.mergeResult, writer.mergeErr
}

func (writer *recordingConfigWriter) CommitConfig(_ context.Context, request mysqlstore.ConfigCommit) (mysqlstore.ConfigCommitResult, error) {
	writer.requests = append(writer.requests, request)
	return writer.result, writer.err
}

func (writer *recordingConfigWriter) ValidateConfig(_ context.Context, request mysqlstore.ConfigValidation) (mysqlstore.ConfigValidationResult, error) {
	writer.validations = append(writer.validations, request)
	return writer.validationResult, writer.validationErr
}

func (writer *recordingConfigWriter) ReadRawConfig(context.Context, string, string) (mysqlstore.RawConfig, error) {
	writer.reads++
	return writer.raw, writer.readErr
}

func (writer *recordingConfigWriter) ReadResolvedConfig(_ context.Context, environment, config, _ string) (machine.ResolvedConfig, error) {
	writer.resolvedReads = append(writer.resolvedReads, [2]string{environment, config})
	return writer.resolved, writer.resolvedErr
}
func (writer *recordingConfigWriter) RecordRejectedMutation(context.Context, mysqlstore.RejectedMutation) error {
	return nil
}
