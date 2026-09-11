package mysqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type OutboxKind string

const (
	OutboxAudit        OutboxKind = "audit"
	OutboxNotification OutboxKind = "notification"
)

type OutboxID [16]byte

type OutboxEvent struct {
	ID          OutboxID
	Kind        OutboxKind
	Type        string
	OperationID string
	Payload     json.RawMessage
	Attempts    uint32
}

func (store *Store) ClaimOutbox(
	ctx context.Context,
	kind OutboxKind,
	limit int,
	claimTimeout time.Duration,
) ([]OutboxEvent, error) {
	if (kind != OutboxAudit && kind != OutboxNotification) || limit < 1 || limit > 1000 || claimTimeout <= 0 {
		return nil, errors.New("invalid Outbox claim")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin Outbox claim: %w", err)
	}
	defer transaction.Rollback()
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, event_type, COALESCE(operation_id, ''), payload, attempts
		FROM outbox_events
		WHERE kind = ? AND (
			(status = 'pending' AND next_attempt_at <= UTC_TIMESTAMP(6)) OR
			(status = 'processing' AND claimed_at <= ?)
		)
		ORDER BY created_at, id
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`, kind, time.Now().UTC().Add(-claimTimeout), limit)
	if err != nil {
		return nil, fmt.Errorf("select Outbox Events: %w", err)
	}
	var events []OutboxEvent
	for rows.Next() {
		var id, payload []byte
		var event OutboxEvent
		if err := rows.Scan(&id, &event.Type, &event.OperationID, &payload, &event.Attempts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Outbox Event: %w", err)
		}
		if len(id) != len(event.ID) || !json.Valid(payload) {
			_ = rows.Close()
			return nil, errors.New("Outbox Event failed integrity validation")
		}
		copy(event.ID[:], id)
		event.Kind = kind
		event.Payload = payload
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate Outbox Events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close Outbox Events: %w", err)
	}
	for index := range events {
		if _, err := transaction.ExecContext(ctx, `
			UPDATE outbox_events
			SET status = 'processing', attempts = attempts + 1, claimed_at = UTC_TIMESTAMP(6)
			WHERE id = ?
		`, events[index].ID[:]); err != nil {
			return nil, fmt.Errorf("claim Outbox Event: %w", err)
		}
		events[index].Attempts++
	}
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit Outbox claim: %w", err)
	}
	return events, nil
}

func (store *Store) CompleteOutbox(ctx context.Context, id OutboxID) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET status = 'completed', completed_at = UTC_TIMESTAMP(6), claimed_at = NULL, last_error_code = NULL
		WHERE id = ? AND status = 'processing'
	`, id[:])
	if err != nil {
		return fmt.Errorf("complete Outbox Event: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect completed Outbox Event: %w", err)
	}
	if changed != 1 {
		return errors.New("Outbox Event is not claimed")
	}
	return nil
}

func (store *Store) RetryOutbox(ctx context.Context, id OutboxID, next time.Time, errorCode string, dead bool) error {
	if !validOutboxErrorCode(errorCode) || (!dead && next.IsZero()) {
		return errors.New("invalid Outbox retry")
	}
	status := "pending"
	var nextAttempt any = next.UTC()
	if dead {
		status = "dead"
		nextAttempt = time.Now().UTC()
	}
	result, err := store.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET status = ?, next_attempt_at = ?, claimed_at = NULL, last_error_code = ?
		WHERE id = ? AND status = 'processing'
	`, status, nextAttempt, errorCode, id[:])
	if err != nil {
		return fmt.Errorf("retry Outbox Event: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect retried Outbox Event: %w", err)
	}
	if changed != 1 {
		return errors.New("Outbox Event is not claimed")
	}
	return nil
}

func validOutboxErrorCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}
