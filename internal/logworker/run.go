package logworker

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

// Run gives durable Audit, Notification delivery and best-effort Access ingestion
// independent budgets. A failed consumer cancels/join all workers and is returned
// to the owning process; dependency outages remain retryable.
func Run(ctx context.Context, outbox *mysqlstore.Store, logs *logstore.Store, consumer *accessnats.Consumer,
	sender notificationAttemptSender, logger *zap.Logger, observers ...func(string, bool)) error {
	if outbox == nil || logs == nil || consumer == nil || sender == nil || logger == nil {
		return errors.New("Log Worker dependencies are required")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	observe := func(worker string, ok bool) {
		for _, observer := range observers {
			if observer != nil {
				observer(worker, ok)
			}
		}
	}
	results := make(chan error, 3)
	ready := make(chan struct{})
	go func() {
		results <- runPeriodic(ctx, "notification", 15*time.Second, observe, logger, func(ctx context.Context) error {
			_, err := DeliverNotificationsOnce(ctx, outbox, sender)
			return err
		})
	}()
	go func() {
		initialized := false
		var cursor mysqlstore.OutboxID
		recovered := false
		results <- runPeriodic(ctx, "audit", 10*time.Second, observe, logger, func(ctx context.Context) error {
			if !initialized {
				if err := logs.Initialize(ctx); err != nil {
					return err
				}
				initialized = true
				close(ready)
			}
			if !recovered {
				page, err := outbox.RequeueAuditFailures(ctx, cursor, 100, func(event mysqlstore.OutboxEvent) bool {
					_, err := logstore.DecodeAuditOutbox(event)
					return err == nil
				})
				if err != nil {
					return err
				}
				cursor, recovered = page.After, page.Done
			}
			_, err := DeliverAuditOnce(ctx, outbox, logs)
			if err != nil {
				// A restore can remove log tables; recreate the schema before retry.
				_ = logs.Initialize(ctx)
			}
			return err
		})
	}()
	go func() {
		select {
		case <-ctx.Done():
			consumer.Close()
			results <- nil
			return
		case <-ready:
		}
		err := consumer.Run(ctx, logs)
		if ctx.Err() == nil && err == nil {
			err = errors.New("NATS Access Event Consumer stopped unexpectedly")
		}
		results <- err
	}()
	var result error
	for range 3 {
		if err := <-results; err != nil {
			result = errors.Join(result, err)
			cancel()
		}
	}
	return result
}

func runPeriodic(ctx context.Context, worker string, budget time.Duration, observe func(string, bool), logger *zap.Logger, work func(context.Context) error) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		attempt, cancel := context.WithTimeout(ctx, budget)
		err := work(attempt)
		cancel()
		observe(worker, err == nil)
		if err != nil && ctx.Err() == nil {
			logger.Warn("Background delivery will retry", zap.String("worker", worker))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
