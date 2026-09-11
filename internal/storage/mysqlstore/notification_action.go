package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type NotificationTestRequest struct {
	OperationID    string
	Actor          Actor
	DestinationKey string
}

type NotificationRedeliveryRequest struct {
	OperationID    string
	Actor          Actor
	DestinationKey string
	DeliveryID     string
}

type NotificationQueueResult struct {
	Outcome          Outcome `json:"outcome"`
	DestinationKey   string  `json:"destination_key"`
	OutboxEventID    string  `json:"outbox_event_id,omitempty"`
	SourceDeliveryID string  `json:"source_delivery_id,omitempty"`
}

func (store *Store) QueueNotificationTest(ctx context.Context, request NotificationTestRequest) (NotificationQueueResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return NotificationQueueResult{}, ErrValidation
	}
	transaction, replay, result, err := store.beginNotificationAction(ctx, request.OperationID, request.Actor, request, request.DestinationKey)
	if err != nil || replay {
		return result, err
	}
	defer transaction.Rollback()
	if !validResourceKey(request.DestinationKey) {
		return finishNotificationActionFailure(ctx, transaction, request.OperationID, request.Actor, request.DestinationKey, "test")
	}
	var destinationID []byte
	var active bool
	if err := transaction.QueryRowContext(ctx, `
		SELECT id, enabled AND archived_at IS NULL
		FROM notification_destinations WHERE resource_key = ? FOR UPDATE
	`, request.DestinationKey).Scan(&destinationID, &active); errors.Is(err, sql.ErrNoRows) || !active {
		return finishNotificationActionFailure(ctx, transaction, request.OperationID, request.Actor, request.DestinationKey, "test")
	} else if err != nil {
		return NotificationQueueResult{}, fmt.Errorf("lock Notification Destination for test: %w", err)
	}
	payload, err := json.Marshal(struct {
		Time           time.Time `json:"time"`
		OperationID    string    `json:"operation_id"`
		Actor          Actor     `json:"actor"`
		Action         string    `json:"action"`
		Outcome        Outcome   `json:"outcome"`
		EnvironmentKey string    `json:"environment_key"`
		ResourceType   string    `json:"resource_type"`
		ResourceKey    string    `json:"resource_key"`
	}{
		Time: time.Now().UTC(), OperationID: request.OperationID, Actor: request.Actor,
		Action: "notification_destination.test", Outcome: OutcomeSuccess,
		ResourceType: "notification_destination", ResourceKey: request.DestinationKey,
	})
	if err != nil {
		return NotificationQueueResult{}, errors.New("encode Notification test payload")
	}
	outboxID, err := insertOutbox(ctx, transaction, "notification", "notification.test", request.OperationID, payload)
	if err != nil {
		return NotificationQueueResult{}, err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO notification_targets (outbox_event_id, destination_id) VALUES (?, ?)
	`, outboxID, destinationID); err != nil {
		return NotificationQueueResult{}, fmt.Errorf("queue Notification test target: %w", err)
	}
	result = NotificationQueueResult{
		Outcome: OutcomeSuccess, DestinationKey: request.DestinationKey, OutboxEventID: hex.EncodeToString(outboxID),
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.test_queued", Action: "notification_destination.test",
		ResourceType: "notification_destination", ResourceKey: request.DestinationKey,
	}, false); err != nil {
		return NotificationQueueResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationQueueResult{}, fmt.Errorf("commit Notification test: %w", err)
	}
	return result, nil
}

func (store *Store) RedeliverNotification(ctx context.Context, request NotificationRedeliveryRequest) (NotificationQueueResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return NotificationQueueResult{}, ErrValidation
	}
	transaction, replay, result, err := store.beginNotificationAction(ctx, request.OperationID, request.Actor, request, request.DestinationKey)
	if err != nil || replay {
		return result, err
	}
	defer transaction.Rollback()
	deliveryID, decodeErr := hex.DecodeString(request.DeliveryID)
	if !validResourceKey(request.DestinationKey) || decodeErr != nil || len(deliveryID) != 16 {
		return finishNotificationActionFailure(ctx, transaction, request.OperationID, request.Actor, request.DestinationKey, "redeliver")
	}
	var (
		destinationID, payload         []byte
		eventType, originalOperationID string
	)
	err = transaction.QueryRowContext(ctx, `
		SELECT destination.id, event.event_type, event.operation_id, event.payload
		FROM notification_deliveries AS delivery
		JOIN notification_destinations AS destination ON destination.id = delivery.destination_id
		JOIN outbox_events AS event ON event.id = delivery.outbox_event_id
		WHERE delivery.id = ? AND destination.resource_key = ?
		  AND destination.enabled = TRUE AND destination.archived_at IS NULL
		FOR UPDATE
	`, deliveryID, request.DestinationKey).Scan(&destinationID, &eventType, &originalOperationID, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return finishNotificationActionFailure(ctx, transaction, request.OperationID, request.Actor, request.DestinationKey, "redeliver")
	}
	if err != nil {
		return NotificationQueueResult{}, fmt.Errorf("lock Notification Delivery for redelivery: %w", err)
	}
	outboxID, err := insertOutbox(ctx, transaction, "notification", eventType, originalOperationID, payload)
	if err != nil {
		return NotificationQueueResult{}, err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO notification_targets (outbox_event_id, destination_id) VALUES (?, ?)
	`, outboxID, destinationID); err != nil {
		return NotificationQueueResult{}, fmt.Errorf("queue Notification redelivery target: %w", err)
	}
	result = NotificationQueueResult{
		Outcome: OutcomeSuccess, DestinationKey: request.DestinationKey,
		OutboxEventID: hex.EncodeToString(outboxID), SourceDeliveryID: request.DeliveryID,
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.redelivery_queued", Action: "notification_destination.redeliver",
		ResourceType: "notification_destination", ResourceKey: request.DestinationKey,
	}, false); err != nil {
		return NotificationQueueResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationQueueResult{}, fmt.Errorf("commit Notification redelivery: %w", err)
	}
	return result, nil
}

func (store *Store) beginNotificationAction(
	ctx context.Context,
	operationID string,
	actor Actor,
	request any,
	destinationKey string,
) (*sql.Tx, bool, NotificationQueueResult, error) {
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, NotificationQueueResult{}, fmt.Errorf("begin Notification action: %w", err)
	}
	replayed, replay, err := beginOperation(ctx, transaction, operationID, tokenRequestDigest(request), actor)
	if err != nil {
		_ = transaction.Rollback()
		return nil, false, NotificationQueueResult{}, err
	}
	if !replayed {
		return transaction, false, NotificationQueueResult{}, nil
	}
	defer transaction.Rollback()
	var result NotificationQueueResult
	if err := json.Unmarshal(replay.Response, &result); err != nil || result.Outcome != replay.Outcome || result.DestinationKey != destinationKey {
		return nil, false, NotificationQueueResult{}, errors.New("existing Notification action failed integrity validation")
	}
	return nil, true, result, outcomeError(result.Outcome)
}

func finishNotificationActionFailure(
	ctx context.Context,
	transaction *sql.Tx,
	operationID string,
	actor Actor,
	destinationKey string,
	action string,
) (NotificationQueueResult, error) {
	result := NotificationQueueResult{Outcome: OutcomeValidationFailed, DestinationKey: destinationKey}
	if err := finishOperation(ctx, transaction, operationID, actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.validation_failed", Action: "notification_destination." + action,
		ResourceType: "notification_destination", ResourceKey: destinationKey,
	}, false); err != nil {
		return NotificationQueueResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationQueueResult{}, fmt.Errorf("commit Notification action validation Audit: %w", err)
	}
	return result, ErrValidation
}
