package management_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestConfigMutationsReturnSafeSpecificFeedback(t *testing.T) {
	for _, tc := range []struct {
		reason, field string
		status        int
	}{
		{"clone_target_exists", "target_config", 409},
		{"target_config_exists", "expected_target_revision", 409},
		{"target_environment_unavailable", "target_environment", 422},
		{"source_revision_unavailable", "source_revision", 422},
		{"config_document_invalid", "content", 422},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			repository := &recordingConfigWriter{mergeErr: &mysqlstore.ConfigMutationError{Field: tc.field, Reason: tc.reason}}
			handler := management.NewHandler(repository, nil, nil)
			req := httptest.NewRequest(http.MethodPost, "/v1/environments/test/configs/server/clone", strings.NewReader(`{"target_environment":"prod","target_config":"server_copy","target_name":"Server","source_revision":7}`))
			req.Header.Set("Idempotency-Key", "config-feedback-test")
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(humanauth.WithPrincipal(req.Context(), humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			var body struct {
				Error struct {
					Code, Field, Reason string
					RequestID           string `json:"request_id"`
				}
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Error.Field != tc.field || body.Error.Reason != tc.reason || body.Error.RequestID == "" {
				t.Fatalf("feedback = %d %s", response.Code, response.Body)
			}
			if len(repository.clones) != 1 || repository.clones[0].SourceRevision != 7 {
				t.Fatalf("clone source revision lost: %#v", repository.clones)
			}
		})
	}
}

func TestConfigFirstCopyRequiresExplicitZeroTargetRevisions(t *testing.T) {
	for _, fields := range []string{"", `,"target_revision":0`, `,"expected_target_revision":0`, `,"target_revision":0,"expected_target_revision":0`} {
		repository := &recordingConfigWriter{mergeResult: mysqlstore.ConfigCommitResult{Outcome: mysqlstore.OutcomeSuccess, Revision: 1}}
		handler := management.NewHandler(repository, nil, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/environments/prod/configs/server/replace", strings.NewReader(`{"source_environment":"test","source_config":"server","source_revision":3`+fields+`}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "copy-explicit-revisions")
		req = req.WithContext(humanauth.WithPrincipal(req.Context(), humanauth.Principal{Subject: "admin-1", Role: humanauth.RoleAdmin}))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if fields == `,"target_revision":0,"expected_target_revision":0` {
			if res.Code != 200 || len(repository.replaces) != 1 {
				t.Fatalf("explicit first copy = %d %s", res.Code, res.Body)
			}
		} else if res.Code != 422 || len(repository.replaces) != 0 {
			t.Fatalf("missing concurrency preconditions reached storage: %d %s", res.Code, res.Body)
		}
	}
}
