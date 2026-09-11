package logworker

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

const notificationBatchSize = 100

type notificationOutbox interface {
	ClaimOutbox(context.Context, mysqlstore.OutboxKind, int, time.Duration) ([]mysqlstore.OutboxEvent, error)
	ClaimNotificationTargets(context.Context, mysqlstore.OutboxID, int) ([]mysqlstore.NotificationTarget, error)
	RecordNotificationDelivery(context.Context, mysqlstore.NotificationDeliveryRecord) error
	NotificationProgress(context.Context, mysqlstore.OutboxID) (mysqlstore.NotificationProgressResult, error)
	CompleteOutbox(context.Context, mysqlstore.OutboxID) error
	RetryOutbox(context.Context, mysqlstore.OutboxID, time.Time, string, bool) error
}

type notificationAttemptSender interface {
	Send(context.Context, mysqlstore.NotificationTarget, notification.Event) notification.AttemptResult
}

func DeliverNotificationsOnce(
	ctx context.Context,
	outbox *mysqlstore.Store,
	sender *notification.Sender,
) (int, error) {
	if outbox == nil || sender == nil {
		return 0, errors.New("Notification Outbox and Sender are required")
	}
	return deliverNotificationsOnce(ctx, outbox, sender)
}

func deliverNotificationsOnce(ctx context.Context, outbox notificationOutbox, sender notificationAttemptSender) (int, error) {
	events, err := outbox.ClaimOutbox(ctx, mysqlstore.OutboxNotification, notificationBatchSize, time.Minute)
	if err != nil || len(events) == 0 {
		return 0, err
	}
	completed := 0
	var resultErr error
	for _, outboxEvent := range events {
		if err := ctx.Err(); err != nil {
			return completed, errors.Join(resultErr, err)
		}
		event, err := notification.DecodeEvent(outboxEvent)
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			if retryErr := outbox.RetryOutbox(ctx, outboxEvent.ID, time.Time{}, "invalid_notification_payload", true); retryErr != nil {
				resultErr = errors.Join(resultErr, retryErr)
			}
			continue
		}
		targets, err := outbox.ClaimNotificationTargets(ctx, outboxEvent.ID, 1000)
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			continue
		}
		recordFailed := false
		for _, target := range targets {
			attempt := sender.Send(ctx, target, event)
			if err := ctx.Err(); err != nil {
				return completed, errors.Join(resultErr, err)
			}
			record := mysqlstore.NotificationDeliveryRecord{
				Target: target, Status: attempt.Status, HTTPStatus: attempt.HTTPStatus,
				LatencyMS: attempt.LatencyMS, ErrorCode: attempt.ErrorCode,
			}
			if record.Status == mysqlstore.NotificationDeliveryRetrying {
				if target.Attempt == 8 {
					record.Status = mysqlstore.NotificationDeliveryDead
				} else {
					delay := notificationRetryDelay(target.Attempt)
					if attempt.HasRetryAfter && attempt.RetryAfter > delay {
						delay = attempt.RetryAfter
					}
					record.NextAttempt = time.Now().UTC().Add(delay)
				}
			}
			if err := outbox.RecordNotificationDelivery(ctx, record); err != nil {
				resultErr = errors.Join(resultErr, err)
				recordFailed = true
				break
			}
		}
		if recordFailed {
			continue
		}
		progress, err := outbox.NotificationProgress(ctx, outboxEvent.ID)
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			continue
		}
		if progress.Complete {
			if err := outbox.CompleteOutbox(ctx, outboxEvent.ID); err != nil {
				resultErr = errors.Join(resultErr, err)
				continue
			}
			completed++
			continue
		}
		if err := outbox.RetryOutbox(ctx, outboxEvent.ID, progress.NextAttempt, "delivery_retry", false); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}
	return completed, resultErr
}

func notificationRetryDelay(attempt uint32) time.Duration {
	maximum := time.Second
	for current := uint32(1); current < attempt && maximum < 5*time.Minute; current++ {
		maximum *= 2
	}
	if maximum > 5*time.Minute {
		maximum = 5 * time.Minute
	}
	return time.Duration(rand.Int64N(int64(maximum) + 1))
}
