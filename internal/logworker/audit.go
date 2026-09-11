package logworker

import (
	"context"
	"errors"
	"time"

	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

const auditBatchSize = 500

func DeliverAuditOnce(ctx context.Context, outbox *mysqlstore.Store, logs *logstore.Store) (int, error) {
	if outbox == nil || logs == nil {
		return 0, errors.New("MySQL Outbox and ClickHouse Log Store are required")
	}
	events, err := outbox.ClaimOutbox(ctx, mysqlstore.OutboxAudit, auditBatchSize, time.Minute)
	if err != nil || len(events) == 0 {
		return 0, err
	}
	records := make([]logstore.AuditRecord, 0, len(events))
	validEvents := make([]mysqlstore.OutboxEvent, 0, len(events))
	var resultErr error
	for _, event := range events {
		record, decodeErr := logstore.DecodeAuditOutbox(event)
		if decodeErr != nil {
			resultErr = errors.Join(resultErr, decodeErr)
			if retryErr := outbox.RetryOutbox(ctx, event.ID, time.Time{}, "invalid_audit_payload", true); retryErr != nil {
				resultErr = errors.Join(resultErr, retryErr)
			}
			continue
		}
		records = append(records, record)
		validEvents = append(validEvents, event)
	}
	if len(records) == 0 {
		return 0, resultErr
	}
	if err := logs.AppendAudits(ctx, records); err != nil {
		resultErr = errors.Join(resultErr, err)
		now := time.Now().UTC()
		for _, event := range validEvents {
			next := now.Add(auditRetryDelay(event.Attempts))
			if retryErr := outbox.RetryOutbox(ctx, event.ID, next, "clickhouse_unavailable", false); retryErr != nil {
				resultErr = errors.Join(resultErr, retryErr)
			}
		}
		return 0, resultErr
	}
	completed := 0
	for _, event := range validEvents {
		if err := outbox.CompleteOutbox(ctx, event.ID); err != nil {
			resultErr = errors.Join(resultErr, err)
			continue
		}
		completed++
	}
	return completed, resultErr
}

func auditRetryDelay(attempt uint32) time.Duration {
	delay := time.Second
	for current := uint32(1); current < attempt && delay < 5*time.Minute; current++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
