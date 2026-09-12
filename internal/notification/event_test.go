package notification

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestDecodeEventRejectsUnknownFieldsAndMismatchedOperation(t *testing.T) {
	var id mysqlstore.OutboxID
	copy(id[:], []byte("0123456789abcdef"))
	payload := map[string]any{
		"time": time.Now().UTC(), "operation_id": "operation-create",
		"actor":  map[string]string{"type": "user", "id": "admin@example.com"},
		"action": "config.commit", "outcome": "success", "environment_key": "production",
		"resource_type": "config", "resource_key": "payment", "revision": 1,
	}
	encoded, _ := json.Marshal(payload)
	event, err := DecodeEvent(mysqlstore.OutboxEvent{
		ID: id, Kind: mysqlstore.OutboxNotification, Type: "config.created",
		OperationID: "operation-create", Payload: encoded, Attempts: 1,
	})
	if err != nil || event.DeliveryID != "30313233343536373839616263646566" || event.Revision != 1 {
		t.Fatalf("DecodeEvent = %#v, %v", event, err)
	}

	payload["secret"] = "must-not-leave-the-system"
	encoded, _ = json.Marshal(payload)
	if _, err := DecodeEvent(mysqlstore.OutboxEvent{
		ID: id, Kind: mysqlstore.OutboxNotification, Type: "config.created",
		OperationID: "operation-create", Payload: encoded, Attempts: 1,
	}); err == nil {
		t.Fatal("DecodeEvent accepted an unknown secret field")
	}
	delete(payload, "secret")
	payload["operation_id"] = "another-operation"
	encoded, _ = json.Marshal(payload)
	if _, err := DecodeEvent(mysqlstore.OutboxEvent{
		ID: id, Kind: mysqlstore.OutboxNotification, Type: "config.created",
		OperationID: "operation-create", Payload: encoded, Attempts: 1,
	}); err == nil {
		t.Fatal("DecodeEvent accepted mismatched OperationID")
	}
}

func TestDecodeEventPreservesVaultNamespaceAndRejectsItsAbsence(t *testing.T) {
	var id mysqlstore.OutboxID
	copy(id[:], []byte("0123456789abcdef"))
	payload := map[string]any{
		"time": time.Now().UTC(), "operation_id": "operation-vault",
		"actor":  map[string]string{"type": "user", "id": "admin@example.com"},
		"action": "vault.commit", "outcome": "success", "namespace_key": "platform",
		"resource_type": "vault_item", "resource_key": "redis", "revision": 2,
	}
	encoded, _ := json.Marshal(payload)
	outbox := mysqlstore.OutboxEvent{ID: id, Kind: mysqlstore.OutboxNotification, Type: "vault.updated", OperationID: "operation-vault", Payload: encoded, Attempts: 1}
	event, err := DecodeEvent(outbox)
	if err != nil || event.Namespace != "platform" {
		t.Fatalf("DecodeEvent = %#v, %v", event, err)
	}
	delete(payload, "namespace_key")
	outbox.Payload, _ = json.Marshal(payload)
	if _, err := DecodeEvent(outbox); err == nil {
		t.Fatal("DecodeEvent accepted a Vault Item without Namespace")
	}
}
