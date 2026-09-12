package notification

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type Event struct {
	SchemaVersion int                `json:"schema_version"`
	DeliveryID    string             `json:"delivery_id"`
	Type          string             `json:"event_type"`
	Time          time.Time          `json:"time"`
	OperationID   string             `json:"operation_id"`
	Actor         mysqlstore.Actor   `json:"actor"`
	Action        string             `json:"action"`
	Outcome       mysqlstore.Outcome `json:"outcome"`
	Environment   string             `json:"environment,omitempty"`
	Namespace     string             `json:"namespace,omitempty"`
	ResourceType  string             `json:"resource_type"`
	Resource      string             `json:"resource"`
	Revision      uint64             `json:"revision,omitempty"`
}

func DecodeEvent(outbox mysqlstore.OutboxEvent) (Event, error) {
	if outbox.Kind != mysqlstore.OutboxNotification || outbox.ID == (mysqlstore.OutboxID{}) ||
		outbox.Type == "" || len(outbox.Type) > 64 || outbox.OperationID == "" || outbox.Attempts == 0 ||
		len(outbox.Payload) == 0 || len(outbox.Payload) > 64<<10 {
		return Event{}, errors.New("invalid Notification Outbox metadata")
	}
	var payload struct {
		Time           time.Time          `json:"time"`
		OperationID    string             `json:"operation_id"`
		Actor          mysqlstore.Actor   `json:"actor"`
		Action         string             `json:"action"`
		Outcome        mysqlstore.Outcome `json:"outcome"`
		EnvironmentKey string             `json:"environment_key"`
		NamespaceKey   string             `json:"namespace_key,omitempty"`
		ResourceType   string             `json:"resource_type"`
		ResourceKey    string             `json:"resource_key"`
		Revision       uint64             `json:"revision,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(outbox.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return Event{}, errors.New("invalid Notification Outbox payload")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Event{}, errors.New("invalid Notification Outbox payload")
	}
	if payload.Time.IsZero() || payload.OperationID != outbox.OperationID ||
		(payload.Actor.Type != "user" && payload.Actor.Type != "system") || payload.Actor.ID == "" || len(payload.Actor.ID) > 255 ||
		payload.Action == "" || len(payload.Action) > 128 || payload.Outcome != mysqlstore.OutcomeSuccess ||
		payload.ResourceType == "" || len(payload.ResourceType) > 64 ||
		payload.ResourceKey == "" || len(payload.ResourceKey) > 63 || len(payload.EnvironmentKey) > 63 ||
		len(payload.NamespaceKey) > 63 || (payload.ResourceType == "vault_item" && payload.NamespaceKey == "") {
		return Event{}, errors.New("invalid Notification Outbox payload")
	}
	return Event{
		SchemaVersion: 1,
		DeliveryID:    hex.EncodeToString(outbox.ID[:]),
		Type:          outbox.Type,
		Time:          payload.Time.UTC(),
		OperationID:   payload.OperationID,
		Actor:         payload.Actor,
		Action:        payload.Action,
		Outcome:       payload.Outcome,
		Environment:   payload.EnvironmentKey,
		Namespace:     payload.NamespaceKey,
		ResourceType:  payload.ResourceType,
		Resource:      payload.ResourceKey,
		Revision:      payload.Revision,
	}, nil
}
