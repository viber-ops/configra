package mysqlstore

import (
	"context"
	"errors"
	"fmt"
)

// AuditRecoveryPage advances over failed rows, including those still invalid.
// Callers retain the cursor so a malformed row cannot starve later valid events.
type AuditRecoveryPage struct {
	After    OutboxID
	Done     bool
	Requeued int
}

// RequeueAuditFailures reconsiders one bounded page of decoder-rejected events.
// The caller supplies the current strict metadata decoder; payloads and event IDs
// are never rewritten. Atomic status checks make concurrent recovery idempotent.
func (store *Store) RequeueAuditFailures(ctx context.Context, after OutboxID, limit int, valid func(OutboxEvent) bool) (AuditRecoveryPage, error) {
	page := AuditRecoveryPage{After: after}
	if limit < 1 || limit > 500 || valid == nil {
		return page, errors.New("invalid Audit recovery request")
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, event_type, COALESCE(operation_id, ''),
		       IF(OCTET_LENGTH(payload) <= 65536, payload, NULL), attempts
		FROM outbox_events
		WHERE kind = 'audit' AND status = 'dead'
		  AND last_error_code = 'invalid_audit_payload' AND id > ?
		ORDER BY id LIMIT ?
	`, after[:], limit)
	if err != nil {
		return page, fmt.Errorf("read rejected Audit Events: %w", err)
	}
	var events []OutboxEvent
	for rows.Next() {
		var id, payload []byte
		event := OutboxEvent{Kind: OutboxAudit}
		if err := rows.Scan(&id, &event.Type, &event.OperationID, &payload, &event.Attempts); err != nil {
			_ = rows.Close()
			return page, fmt.Errorf("read rejected Audit metadata: %w", err)
		}
		if len(id) != len(event.ID) {
			_ = rows.Close()
			return page, errors.New("invalid Audit Event identity")
		}
		copy(event.ID[:], id)
		event.Payload = payload
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return page, err
	}
	if err := rows.Close(); err != nil {
		return page, err
	}
	for _, event := range events {
		if valid(event) {
			result, err := store.db.ExecContext(ctx, `
				UPDATE outbox_events SET status = 'pending', next_attempt_at = UTC_TIMESTAMP(6),
				claimed_at = NULL, completed_at = NULL, last_error_code = NULL
				WHERE id = ? AND kind = 'audit' AND status = 'dead'
				  AND last_error_code = 'invalid_audit_payload'
			`, event.ID[:])
			if err != nil {
				return page, fmt.Errorf("requeue Audit Event: %w", err)
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return page, err
			}
			page.Requeued += int(changed)
		}
		page.After = event.ID
	}
	page.Done = len(events) < limit
	return page, nil
}
