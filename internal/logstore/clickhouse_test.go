package logstore_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestDecodeAuditOutboxAcceptsOnlyTheValueFreeMetadataContract(t *testing.T) {
	id := mysqlstore.OutboxID{1, 2, 3, 4}
	payload := json.RawMessage(`{
  "time":"2026-08-27T03:04:05.000000006Z",
  "operation_id":"operation-config-123",
  "actor":{"type":"user","id":"issuer|subject"},
  "action":"config.commit",
  "outcome":"success",
  "environment_key":"production",
  "resource_type":"config",
  "resource_key":"payment",
  "revision":21
}`)
	record, err := logstore.DecodeAuditOutbox(mysqlstore.OutboxEvent{
		ID:          id,
		Kind:        mysqlstore.OutboxAudit,
		Type:        "config.commit.succeeded",
		OperationID: "operation-config-123",
		Payload:     payload,
		Attempts:    2,
	})
	if err != nil {
		t.Fatalf("DecodeAuditOutbox: %v", err)
	}
	if record.ID != "01020304000000000000000000000000" || record.EventType != "config.commit.succeeded" ||
		record.Time != time.Date(2026, 8, 27, 3, 4, 5, 6, time.UTC) || record.ActorID != "issuer|subject" ||
		record.Action != "config.commit" || record.Outcome != "success" || record.Environment != "production" ||
		record.ResourceType != "config" || record.Resource != "payment" || record.Revision != 21 || record.Attempt != 2 {
		t.Fatalf("Audit Record = %#v", record)
	}
}

func TestDecodeAuditOutboxPreservesVaultNamespaceAndRejectsItsAbsence(t *testing.T) {
	payload := json.RawMessage(`{"time":"2026-08-27T03:04:05Z","operation_id":"operation-vault-123","actor":{"type":"user","id":"subject"},"action":"vault.commit","outcome":"success","namespace_key":"platform","resource_type":"vault_item","resource_key":"redis","revision":2}`)
	event := mysqlstore.OutboxEvent{ID: mysqlstore.OutboxID{1}, Kind: mysqlstore.OutboxAudit, Type: "vault.updated", OperationID: "operation-vault-123", Payload: payload, Attempts: 1}
	record, err := logstore.DecodeAuditOutbox(event)
	if err != nil || record.Namespace != "platform" {
		t.Fatalf("DecodeAuditOutbox = %#v, %v", record, err)
	}
	event.Payload = json.RawMessage(`{"time":"2026-08-27T03:04:05Z","operation_id":"operation-vault-123","actor":{"type":"user","id":"subject"},"action":"vault.commit","outcome":"success","resource_type":"vault_item","resource_key":"redis","revision":2}`)
	if _, err := logstore.DecodeAuditOutbox(event); err == nil {
		t.Fatal("DecodeAuditOutbox accepted a Vault Item without Namespace")
	}
}

func TestDecodeAuditOutboxRejectsCorruptionAndUnknownValueFieldsWithoutLeakingThem(t *testing.T) {
	valid := `{"time":"2026-08-27T03:04:05Z","operation_id":"operation-config-123","actor":{"type":"user","id":"subject"},"action":"config.commit","outcome":"success","environment_key":"a","resource_type":"config","resource_key":"payment","revision":1}`
	tests := []mysqlstore.OutboxEvent{
		{Kind: mysqlstore.OutboxNotification, OperationID: "operation-config-123", Payload: json.RawMessage(valid), Attempts: 1},
		{Kind: mysqlstore.OutboxAudit, OperationID: "different-operation", Payload: json.RawMessage(valid), Attempts: 1},
		{Kind: mysqlstore.OutboxAudit, OperationID: "operation-config-123", Payload: json.RawMessage(strings.TrimSuffix(valid, "}") + `,"content":"vault-secret-sentinel"}`), Attempts: 1},
		{Kind: mysqlstore.OutboxAudit, OperationID: "operation-config-123", Payload: json.RawMessage(`{"broken":"vault-secret-sentinel"}`), Attempts: 1},
	}
	for index, event := range tests {
		event.ID[0] = byte(index + 1)
		event.Type = "config.commit.succeeded"
		_, err := logstore.DecodeAuditOutbox(event)
		if err == nil {
			t.Fatalf("case %d succeeded", index)
		}
		if strings.Contains(err.Error(), "vault-secret-sentinel") {
			t.Fatalf("case %d leaked payload: %v", index, err)
		}
	}
}

func TestNewClickHouseLogStoreRejectsAnEmptyOrInvalidDSN(t *testing.T) {
	for _, dsn := range []string{"", "://not-a-dsn"} {
		if _, err := logstore.New(dsn); err == nil {
			t.Fatalf("New(%q) succeeded", dsn)
		}
	}
}
