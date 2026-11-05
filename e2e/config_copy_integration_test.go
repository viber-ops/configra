//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestConfigCopyHTTPCreateConflictReplayAndSafeFeedback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := newStore(t, ctx)
	handler := management.NewHandler(store, nil, nil)
	call := func(role humanauth.Role, method, path, operation string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "https://management.test"+path, bytes.NewReader(encoded)).WithContext(humanauth.WithPrincipal(ctx, humanauth.Principal{Subject: "admin", Role: role, MFAVerified: true}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", operation)
		req.RemoteAddr = "192.0.2.70:443"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	admin := humanauth.RoleAdmin
	for _, key := range []string{"test", "prod"} {
		res := call(admin, "POST", "/v1/environments", "copy-http-env-"+key, map[string]any{"key": key, "display_name": key})
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body)
		}
	}
	source := map[string]any{"name": "Server", "format": "yaml", "expected_revision": 0, "content": "marker: CONTENT-MUST-NOT-BE-IN-ERRORS\npassword: '{vault.app.db.password}'\n"}
	if res := call(admin, "PUT", "/v1/environments/test/configs/server", "copy-http-source", source); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	transfer := map[string]any{"source_environment": "test", "source_config": "server", "source_revision": 1, "target_revision": 0, "expected_target_revision": 0}
	path := "/v1/environments/prod/configs/server/replace"
	if res := call(humanauth.RoleViewer, "POST", path, "copy-http-viewer", transfer); res.Code != 403 {
		t.Fatal("viewer copied Config", res.Code, res.Body)
	}
	for range 2 {
		res := call(admin, "POST", path, "copy-http-first", transfer)
		if res.Code != 200 || !strings.Contains(res.Body.String(), `"revision":1`) || !strings.Contains(res.Body.String(), `missing_vault_item`) {
			t.Fatal(res.Code, res.Body)
		}
	}
	for i, tc := range []struct {
		path   string
		body   any
		code   int
		reason string
	}{
		{path, transfer, 409, "target_config_exists"},
		{"/v1/environments/test/configs/server/clone", map[string]any{"target_environment": "prod", "target_config": "server", "source_revision": 1}, 409, "clone_target_exists"},
		{"/v1/environments/production/configs/server/replace", transfer, 422, "target_environment_unavailable"},
	} {
		for range 2 {
			res := call(admin, "POST", tc.path, fmt.Sprintf("copy-http-reject-%d", i), tc.body)
			var body struct {
				Error struct {
					Reason    string
					RequestID string `json:"request_id"`
				}
			}
			if json.Unmarshal(res.Body.Bytes(), &body) != nil || res.Code != tc.code || body.Error.Reason != tc.reason || body.Error.RequestID == "" {
				t.Fatal(res.Code, res.Body)
			}
			if strings.Contains(res.Body.String(), "CONTENT-MUST-NOT") {
				t.Fatal("error leaked content")
			}
		}
	}
	events, err := store.ClaimOutbox(ctx, mysqlstore.OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "CONTENT-MUST-NOT") {
			t.Fatal("audit leaked content")
		}
		if strings.HasPrefix(event.OperationID, "copy-http-reject-") || event.OperationID == "copy-http-first" {
			counts[event.OperationID]++
			var metadata struct {
				SourceIP string `json:"source_ip"`
			}
			if json.Unmarshal(event.Payload, &metadata) != nil || metadata.SourceIP != "192.0.2.70" {
				t.Fatal("copy audit lost IP")
			}
		}
	}
	if len(counts) != 4 {
		t.Fatalf("copy audit operations = %#v", counts)
	}
	for key, count := range counts {
		if count != 1 {
			t.Fatalf("replay duplicated audit %s: %d", key, count)
		}
	}
}
