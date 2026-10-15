//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestTokenValidationHTTPPreservesSafeReasonsOnReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := newStore(t, ctx)
	store.WriteTokenMaxTTL = 7 * 24 * time.Hour
	if _, err := store.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{OperationID: "validation-environment", Actor: mysqlstore.Actor{Type: "user", ID: "admin"}, Action: mysqlstore.EnvironmentCreate, Key: "production", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	handler := management.NewHandler(store, nil, nil)
	call := func(method, path, operation string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequest(method, "https://management.test"+path, bytes.NewReader(encoded)).WithContext(humanauth.WithPrincipal(ctx, humanauth.Principal{Subject: "admin", Role: humanauth.RoleAdmin, MFAVerified: true}))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", operation)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	me := call("GET", "/v1/me", "", nil)
	if me.Code != 200 || !strings.Contains(me.Body.String(), `"write_token_max_ttl_days":7`) {
		t.Fatal("effective expiry policy missing from Management metadata")
	}
	for i, test := range []struct {
		reason, field string
		change        func(map[string]any)
	}{
		{"write_token_expiry_limit", "expires_at", func(body map[string]any) { body["expires_at"] = time.Now().Add(8 * 24 * time.Hour) }},
		{"scope_key_invalid", "config_keys", func(body map[string]any) { body["config_keys"] = []string{"private–sentinel"} }},
		{"scope_keys_duplicate", "namespace_keys", func(body map[string]any) { body["namespace_keys"] = []string{"private_sentinel", "private_sentinel"} }},
		{"environment_unavailable", "environment_keys", func(body map[string]any) { body["environment_keys"] = []string{"missing"} }},
	} {
		t.Run(test.reason, func(t *testing.T) {
			body := map[string]any{"kind": "write-scoped", "display_name": "private-sentinel-name", "environment_keys": []string{"production"}, "expires_at": time.Now().Add(time.Hour)}
			test.change(body)
			for range 2 {
				response := call("POST", "/v1/api-tokens", fmt.Sprintf("validation-case-%d", i), body)
				var result struct {
					Error struct {
						Code, Field, Reason string
						RequestID           string `json:"request_id"`
						MaxTTLDays          int    `json:"max_ttl_days"`
					}
				}
				if response.Code != http.StatusUnprocessableEntity || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Error.Code != "validation_failed" || result.Error.Field != test.field || result.Error.Reason != test.reason || result.Error.RequestID == "" {
					t.Fatalf("lost safe field reason: %d %s", response.Code, response.Body)
				}
				if test.reason == "write_token_expiry_limit" && result.Error.MaxTTLDays != 7 {
					t.Fatal("policy lost on failure/replay")
				}
				if strings.Contains(response.Body.String(), "private") {
					t.Fatal("HTTP validation echoed submitted values")
				}
			}
		})
	}
	tokens, err := store.ListTokens(ctx, mysqlstore.InventoryQuery{Limit: 50})
	if err != nil || tokens.Total != 0 {
		t.Fatal("rejected request created a credential", err)
	}
	events, err := store.ClaimOutbox(ctx, mysqlstore.OutboxAudit, 100, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "private") {
			t.Fatal("validation audit leaked input")
		}
		if strings.HasPrefix(event.OperationID, "validation-case-") {
			counts[event.OperationID]++
		}
	}
	if len(counts) != 4 {
		t.Fatal("missing validation audits")
	}
	for _, count := range counts {
		if count != 1 {
			t.Fatal("replay duplicated logical mutation audit")
		}
	}
}
