//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/logworker"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/notification"
	"go.uber.org/zap"
)

// OIDC itself is exercised by docs-smoke against real Casdoor. This handler seam
// supplies authenticated Principals and keeps storage and log delivery real.
func auditRequest(handler http.Handler, role humanauth.Role, method, path, operation, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://management.test"+path, strings.NewReader(body))
	r.Header.Set("Origin", "https://management.test")
	r.Header.Set("Content-Type", "application/json")
	if operation != "" {
		r.Header.Set("Idempotency-Key", operation)
	}
	if role != "" {
		r = r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "audit-integration-user", Role: role}))
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestRejectedMutationFailsClosedWhenAuditReceiptCannotBeStored(t *testing.T) {
	store := newStore(t, context.Background())
	handler := management.NewHandler(store, nil, nil)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	response := auditRequest(handler, humanauth.RoleViewer, http.MethodPost, "/v1/environments", "unused-operation", `{"key":"private_value_marker","display_name":"private-value-marker"}`)
	if response.Code != 503 {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	var result map[string]map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal("response must be one JSON envelope")
	}
	if result["error"]["code"] != "audit_unavailable" || len(result["error"]["request_id"]) != 24 {
		t.Fatal("missing safe audit failure and Request ID")
	}
	if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "forbidden") {
		t.Fatal("original error or values leaked into replacement response")
	}
	if anonymous := auditRequest(handler, "", http.MethodPost, "/v1/environments", "", "{}"); anonymous.Code != 401 {
		t.Fatalf("anonymous status = %d", anonymous.Code)
	}
}

func TestRejectedRequestDoesNotConsumeOperationIDAndBusinessReplayDoesNotDuplicateAudit(t *testing.T) {
	if os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN") == "" || os.Getenv("CONFIGRA_TEST_NATS_URL") == "" {
		t.Skip("real ClickHouse and NATS are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := newStore(t, ctx)
	logs, err := logstore.New(os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if err := logs.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	consumer, err := accessnats.ConnectConsumer(accessnats.Config{URLs: []string{os.Getenv("CONFIGRA_TEST_NATS_URL")}}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sender, err := notification.NewSender(notification.Config{})
	if err != nil {
		consumer.Close()
		t.Fatal(err)
	}
	defer sender.CloseIdleConnections()
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- logworker.Run(workerCtx, store, logs, consumer, sender, zap.NewNop()) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	handler := management.NewHandler(store, nil, nil, logs)
	operation := fmt.Sprintf("audit-operation-%d", time.Now().UnixNano())
	body := `{"key":"audit_environment","display_name":"Audit integration"}`
	denied := auditRequest(handler, humanauth.RoleViewer, http.MethodPost, "/v1/environments", operation, body)
	if denied.Code != 403 {
		t.Fatalf("denied status = %d", denied.Code)
	}
	var errorBody struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if json.Unmarshal(denied.Body.Bytes(), &errorBody) != nil || errorBody.Error.RequestID == "" {
		t.Fatal("missing request identity")
	}
	for range 2 {
		created := auditRequest(handler, humanauth.RoleAdmin, http.MethodPost, "/v1/environments", operation, body)
		if created.Code != 200 {
			t.Fatalf("create/replay status = %d", created.Code)
		}
	}
	read := func(q string) []logstore.AuditRecord {
		r := auditRequest(handler, humanauth.RoleViewer, http.MethodGet, "/v1/audit?limit=100&q="+q, "", "")
		if r.Code != 200 {
			t.Fatalf("Audit query status = %d", r.Code)
		}
		var page logstore.AuditPage
		if json.Unmarshal(r.Body.Bytes(), &page) != nil {
			t.Fatal("invalid Audit page")
		}
		return page.Items
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		rejected, committed := read(errorBody.Error.RequestID), read(operation)
		if len(rejected) == 1 && len(committed) == 1 {
			if rejected[0].ErrorCode != "forbidden" || rejected[0].OperationID != "" || committed[0].Outcome != "success" || committed[0].RequestID != "" {
				t.Fatal("request/operation identities were conflated")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rejected/committed audit counts = %d/%d", len(rejected), len(committed))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
