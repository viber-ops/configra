package logworker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestDeliverNotificationsOncePersistsRetryAndCompletesTerminalTargets(t *testing.T) {
	now := time.Now().UTC()
	event := validNotificationEvent(t)
	store := &fakeNotificationStore{
		events: []mysqlstore.OutboxEvent{event},
		targets: []mysqlstore.NotificationTarget{
			{OutboxID: event.ID, DestinationID: notificationDestinationID(1), DestinationKey: "retry", Attempt: 1, Active: true},
			{OutboxID: event.ID, DestinationID: notificationDestinationID(2), DestinationKey: "last", Attempt: 8, Active: true},
		},
	}
	sender := fakeNotificationSender{results: map[string]notification.AttemptResult{
		"retry": {Status: mysqlstore.NotificationDeliveryRetrying, HTTPStatus: 503, ErrorCode: "http_503", HasRetryAfter: true, RetryAfter: 10 * time.Second},
		"last":  {Status: mysqlstore.NotificationDeliveryRetrying, ErrorCode: "network_error"},
	}}

	delivered, err := deliverNotificationsOnce(context.Background(), store, sender)
	if err != nil || delivered != 0 || len(store.records) != 2 || store.records[0].Status != mysqlstore.NotificationDeliveryRetrying ||
		store.records[1].Status != mysqlstore.NotificationDeliveryDead || store.completed != 0 || store.retried != 1 {
		t.Fatalf("delivery = %d, records = %#v, complete/retry = %d/%d, err = %v", delivered, store.records, store.completed, store.retried, err)
	}
	if store.records[0].NextAttempt.Before(now.Add(9*time.Second)) || store.next.Before(store.records[0].NextAttempt) {
		t.Fatalf("retry times = record %v, outbox %v", store.records[0].NextAttempt, store.next)
	}

	store.targets = nil
	store.pending = false
	time.Sleep(10 * time.Millisecond)
	delivered, err = deliverNotificationsOnce(context.Background(), store, sender)
	if err != nil || delivered != 1 || store.completed != 1 {
		t.Fatalf("terminal delivery = %d, complete = %d, err = %v", delivered, store.completed, err)
	}
}

func TestDeliverNotificationsOnceDeadLettersInvalidPayload(t *testing.T) {
	event := validNotificationEvent(t)
	event.Payload = json.RawMessage(`{"secret":"must-not-be-sent"}`)
	store := &fakeNotificationStore{events: []mysqlstore.OutboxEvent{event}}
	delivered, err := deliverNotificationsOnce(context.Background(), store, fakeNotificationSender{})
	if err == nil || delivered != 0 || store.dead != 1 || store.lastError != "invalid_notification_payload" {
		t.Fatalf("delivery = %d, dead = %d, error = %q, err = %v", delivered, store.dead, store.lastError, err)
	}
}

type fakeNotificationStore struct {
	events    []mysqlstore.OutboxEvent
	targets   []mysqlstore.NotificationTarget
	records   []mysqlstore.NotificationDeliveryRecord
	pending   bool
	next      time.Time
	completed int
	retried   int
	dead      int
	lastError string
}

func (store *fakeNotificationStore) ClaimOutbox(context.Context, mysqlstore.OutboxKind, int, time.Duration) ([]mysqlstore.OutboxEvent, error) {
	return store.events, nil
}

func (store *fakeNotificationStore) ClaimNotificationTargets(context.Context, mysqlstore.OutboxID, int) ([]mysqlstore.NotificationTarget, error) {
	return store.targets, nil
}

func (store *fakeNotificationStore) RecordNotificationDelivery(_ context.Context, record mysqlstore.NotificationDeliveryRecord) error {
	store.records = append(store.records, record)
	if record.Status == mysqlstore.NotificationDeliveryRetrying {
		store.pending = true
		store.next = record.NextAttempt
	}
	return nil
}

func (store *fakeNotificationStore) NotificationProgress(context.Context, mysqlstore.OutboxID) (mysqlstore.NotificationProgressResult, error) {
	return mysqlstore.NotificationProgressResult{Complete: !store.pending, NextAttempt: store.next}, nil
}

func (store *fakeNotificationStore) CompleteOutbox(context.Context, mysqlstore.OutboxID) error {
	store.completed++
	return nil
}

func (store *fakeNotificationStore) RetryOutbox(_ context.Context, _ mysqlstore.OutboxID, next time.Time, code string, dead bool) error {
	store.next = next
	store.lastError = code
	if dead {
		store.dead++
	} else {
		store.retried++
	}
	return nil
}

type fakeNotificationSender struct {
	results map[string]notification.AttemptResult
}

func (sender fakeNotificationSender) Send(_ context.Context, target mysqlstore.NotificationTarget, _ notification.Event) notification.AttemptResult {
	return sender.results[target.DestinationKey]
}

func validNotificationEvent(t *testing.T) mysqlstore.OutboxEvent {
	t.Helper()
	var id mysqlstore.OutboxID
	copy(id[:], []byte("0123456789abcdef"))
	payload, err := json.Marshal(map[string]any{
		"time": time.Now().UTC(), "operation_id": "operation-create",
		"actor":  map[string]string{"type": "user", "id": "admin@example.com"},
		"action": "config.commit", "outcome": "success", "environment_key": "production",
		"resource_type": "config", "resource_key": "payment", "revision": 1,
	})
	if err != nil {
		t.Fatalf("marshal Event: %v", err)
	}
	return mysqlstore.OutboxEvent{
		ID: id, Kind: mysqlstore.OutboxNotification, Type: "config.created",
		OperationID: "operation-create", Payload: payload, Attempts: 1,
	}
}

func notificationDestinationID(value byte) mysqlstore.NotificationDestinationID {
	var id mysqlstore.NotificationDestinationID
	id[len(id)-1] = value
	return id
}
