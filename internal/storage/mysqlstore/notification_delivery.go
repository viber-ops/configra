package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type NotificationDeliveryStatus string

const (
	NotificationDeliverySucceeded NotificationDeliveryStatus = "succeeded"
	NotificationDeliveryRetrying  NotificationDeliveryStatus = "retrying"
	NotificationDeliveryDead      NotificationDeliveryStatus = "dead"
)

type NotificationDeliveryRecord struct {
	Target      NotificationTarget
	Status      NotificationDeliveryStatus
	HTTPStatus  int
	LatencyMS   uint32
	ErrorCode   string
	NextAttempt time.Time
}

type NotificationDelivery struct {
	ID             string                     `json:"id"`
	OutboxEventID  string                     `json:"outbox_event_id"`
	EventType      string                     `json:"event_type"`
	OperationID    string                     `json:"operation_id,omitempty"`
	DestinationKey string                     `json:"destination_key"`
	Attempt        uint32                     `json:"attempt"`
	Status         NotificationDeliveryStatus `json:"status"`
	HTTPStatus     int                        `json:"http_status,omitempty"`
	LatencyMS      uint32                     `json:"latency_ms"`
	ErrorCode      string                     `json:"error_code,omitempty"`
	CreatedAt      time.Time                  `json:"created_at"`
}

type NotificationProgressResult struct {
	Complete    bool
	NextAttempt time.Time
}

func (store *Store) RecordNotificationDelivery(ctx context.Context, record NotificationDeliveryRecord) error {
	if err := validateNotificationDelivery(record); err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Notification Delivery: %w", err)
	}
	defer transaction.Rollback()

	var httpStatus, errorCode any
	if record.HTTPStatus != 0 {
		httpStatus = record.HTTPStatus
	}
	if record.ErrorCode != "" {
		errorCode = record.ErrorCode
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO notification_deliveries
			(id, outbox_event_id, destination_id, attempt, status, http_status, latency_ms, provider_error_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, record.Target.OutboxID[:], record.Target.DestinationID[:], record.Target.Attempt,
		record.Status, httpStatus, record.LatencyMS, errorCode); err != nil {
		return fmt.Errorf("insert Notification Delivery: %w", err)
	}

	targetStatus := "pending"
	var nextAttempt any = record.NextAttempt.UTC()
	if record.Status != NotificationDeliveryRetrying {
		targetStatus = string(record.Status)
		nextAttempt = time.Now().UTC()
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE notification_targets
		SET status = ?, next_attempt_at = ?, last_error_code = ?
		WHERE outbox_event_id = ? AND destination_id = ? AND status = 'pending' AND attempts = ?
	`, targetStatus, nextAttempt, errorCode, record.Target.OutboxID[:], record.Target.DestinationID[:], record.Target.Attempt)
	if err != nil {
		return fmt.Errorf("update Notification target after Delivery: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect Notification target after Delivery: %w", err)
	}
	if changed != 1 {
		return errors.New("Notification target is not claimed at the recorded attempt")
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit Notification Delivery: %w", err)
	}
	return nil
}

func (store *Store) NotificationProgress(ctx context.Context, outboxID OutboxID) (NotificationProgressResult, error) {
	if outboxID == (OutboxID{}) {
		return NotificationProgressResult{}, errors.New("invalid Notification Event ID")
	}
	var pending uint64
	var next sql.NullTime
	err := store.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(target.status = 'pending'), 0),
		       MIN(CASE WHEN target.status = 'pending' THEN target.next_attempt_at END)
		FROM outbox_events AS event
		LEFT JOIN notification_targets AS target ON target.outbox_event_id = event.id
		WHERE event.id = ? AND event.kind = 'notification'
		GROUP BY event.id
	`, outboxID[:]).Scan(&pending, &next)
	if errors.Is(err, sql.ErrNoRows) {
		return NotificationProgressResult{}, errors.New("Notification Event does not exist")
	}
	if err != nil {
		return NotificationProgressResult{}, fmt.Errorf("read Notification progress: %w", err)
	}
	result := NotificationProgressResult{Complete: pending == 0}
	if next.Valid {
		result.NextAttempt = next.Time.UTC()
	}
	return result, nil
}

func (store *Store) ListNotificationDeliveries(
	ctx context.Context,
	destinationKey string,
	limit int,
) ([]NotificationDelivery, error) {
	if !validResourceKey(destinationKey) || limit < 1 || limit > 1000 {
		return nil, errors.New("invalid Notification Delivery list")
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT delivery.id, delivery.outbox_event_id, event.event_type,
		       COALESCE(event.operation_id, ''), destination.resource_key,
		       delivery.attempt, delivery.status, delivery.http_status,
		       delivery.latency_ms, delivery.provider_error_code, delivery.created_at
		FROM notification_deliveries AS delivery
		JOIN outbox_events AS event ON event.id = delivery.outbox_event_id
		JOIN notification_destinations AS destination ON destination.id = delivery.destination_id
		WHERE destination.resource_key = ?
		ORDER BY delivery.created_at DESC, delivery.id DESC
		LIMIT ?
	`, destinationKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list Notification Deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]NotificationDelivery, 0)
	for rows.Next() {
		var (
			delivery          NotificationDelivery
			id, outboxID      []byte
			httpStatus        sql.NullInt64
			latency           sql.NullInt64
			providerErrorCode sql.NullString
		)
		if err := rows.Scan(
			&id, &outboxID, &delivery.EventType, &delivery.OperationID, &delivery.DestinationKey,
			&delivery.Attempt, &delivery.Status, &httpStatus, &latency, &providerErrorCode, &delivery.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan Notification Delivery: %w", err)
		}
		if len(id) != 16 || len(outboxID) != 16 {
			return nil, errors.New("Notification Delivery failed integrity validation")
		}
		delivery.ID = hex.EncodeToString(id)
		delivery.OutboxEventID = hex.EncodeToString(outboxID)
		if httpStatus.Valid {
			delivery.HTTPStatus = int(httpStatus.Int64)
		}
		if latency.Valid {
			delivery.LatencyMS = uint32(latency.Int64)
		}
		if providerErrorCode.Valid {
			delivery.ErrorCode = providerErrorCode.String
		}
		delivery.CreatedAt = delivery.CreatedAt.UTC()
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Notification Deliveries: %w", err)
	}
	return deliveries, nil
}

func validateNotificationDelivery(record NotificationDeliveryRecord) error {
	if record.Target.OutboxID == (OutboxID{}) || record.Target.DestinationID == (NotificationDestinationID{}) ||
		record.Target.Attempt < 1 || record.Target.Attempt > 8 ||
		(record.HTTPStatus != 0 && (record.HTTPStatus < 100 || record.HTTPStatus > 599)) {
		return errors.New("invalid Notification Delivery")
	}
	switch record.Status {
	case NotificationDeliverySucceeded:
		if record.ErrorCode != "" || !record.NextAttempt.IsZero() {
			return errors.New("invalid successful Notification Delivery")
		}
	case NotificationDeliveryRetrying:
		if record.Target.Attempt == 8 || !validOutboxErrorCode(record.ErrorCode) || record.NextAttempt.IsZero() {
			return errors.New("invalid retrying Notification Delivery")
		}
	case NotificationDeliveryDead:
		if !validOutboxErrorCode(record.ErrorCode) || !record.NextAttempt.IsZero() {
			return errors.New("invalid dead Notification Delivery")
		}
	default:
		return errors.New("invalid Notification Delivery status")
	}
	return nil
}
