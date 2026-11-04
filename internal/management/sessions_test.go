package management_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestSessionPolicyChangesRequireAdministratorMFA(t *testing.T) {
	for _, test := range []struct {
		role humanauth.Role
		mfa  bool
		want int
	}{{humanauth.RoleViewer, true, 403}, {humanauth.RoleAdmin, false, 403}, {humanauth.RoleAdmin, true, 200}} {
		request := httptest.NewRequest("POST", "https://configra.test/v1/session-policies", strings.NewReader(`{"issuer":"https://identity.example.com","subject":"user-1","action":"block"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://configra.test")
		request.Header.Set("Idempotency-Key", "session-policy-test")
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: "admin", Role: test.role, MFAVerified: test.mfa}))
		response := httptest.NewRecorder()
		management.NewHandler(&recordingConfigWriter{}, nil, nil).ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("role=%s MFA=%v: HTTP %d", test.role, test.mfa, response.Code)
		}
	}
}

func TestSessionSettingsAndTokenRenewalRoutes(t *testing.T) {
	repository := &recordingConfigWriter{
		tokenRenewResult: mysqlstore.TokenRenewResult{
			Outcome:      mysqlstore.OutcomeSuccess,
			PublicID:     "0123456789abcdef",
			NeverExpires: true,
		},
	}
	handler := management.NewHandler(repository, nil, nil)

	putReq := httptest.NewRequest("PUT", "https://configra.test/v1/session-settings", strings.NewReader(`{"idle_timeout":"24h","lifetime":"7d"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("Origin", "https://configra.test")
	putReq.Header.Set("Idempotency-Key", "session-settings-test")
	putReq = putReq.WithContext(humanauth.WithPrincipal(putReq.Context(), humanauth.Principal{Subject: "admin", Role: humanauth.RoleAdmin}))
	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, putReq)
	if putRes.Code != 200 {
		t.Fatalf("PUT /v1/session-settings HTTP %d: %s", putRes.Code, putRes.Body.String())
	}

	renewReq := httptest.NewRequest("PATCH", "https://configra.test/v1/api-tokens/0123456789abcdef/expiry", strings.NewReader(`{"never_expires":true}`))
	renewReq.Header.Set("Content-Type", "application/json")
	renewReq.Header.Set("Origin", "https://configra.test")
	renewReq.Header.Set("Idempotency-Key", "token-renew-test")
	renewReq = renewReq.WithContext(humanauth.WithPrincipal(renewReq.Context(), humanauth.Principal{Subject: "admin", Role: humanauth.RoleAdmin}))
	renewRes := httptest.NewRecorder()
	handler.ServeHTTP(renewRes, renewReq)
	if renewRes.Code != 200 || len(repository.tokenRenews) != 1 || !repository.tokenRenews[0].NeverExpires {
		t.Fatalf("PATCH /v1/api-tokens/{id}/expiry HTTP %d, renews=%+v", renewRes.Code, repository.tokenRenews)
	}
}

