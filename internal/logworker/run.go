package logworker

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func Run(
	ctx context.Context,
	outbox *mysqlstore.Store,
	logs *logstore.Store,
	consumer *accessnats.Consumer,
	sender *notification.Sender,
	logger *zap.Logger,
) error {
	if outbox == nil || logs == nil || consumer == nil || sender == nil || logger == nil {
		return errors.New("Log Worker dependencies are required")
	}
	for {
		initializeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := logs.Initialize(initializeContext)
		cancel()
		if err == nil {
			break
		}
		logger.Warn("ClickHouse unavailable; Log Worker will retry")
		deliveryContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, deliveryErr := DeliverNotificationsOnce(deliveryContext, outbox, sender)
		cancel()
		if deliveryErr != nil && ctx.Err() == nil {
			logger.Warn("Notification delivery did not complete")
		}
		select {
		case <-ctx.Done():
			consumer.Close()
			return nil
		case <-time.After(2 * time.Second):
		}
	}

	consumerDone := make(chan error, 1)
	go func() { consumerDone <- consumer.Run(ctx, logs) }()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-consumerDone:
			if err == nil && ctx.Err() != nil {
				return nil
			}
			if err == nil {
				return errors.New("NATS Access Event Consumer stopped unexpectedly")
			}
			return err
		case <-ticker.C:
			deliveryContext, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := DeliverAuditOnce(deliveryContext, outbox, logs)
			cancel()
			if err != nil {
				logger.Warn("Audit Event delivery did not complete")
				initializeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
				_ = logs.Initialize(initializeContext)
				cancel()
			}
			deliveryContext, cancel = context.WithTimeout(ctx, 15*time.Second)
			_, err = DeliverNotificationsOnce(deliveryContext, outbox, sender)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Warn("Notification delivery did not complete")
			}
		case <-ctx.Done():
			return <-consumerDone
		}
	}
}
