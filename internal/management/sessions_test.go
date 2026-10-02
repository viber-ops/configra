package management_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
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
