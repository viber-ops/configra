package accessnats

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/machine"
)

const (
	accessBatchSize     = 500
	accessFlushInterval = 250 * time.Millisecond
)

type Consumer struct {
	connection   *nats.Conn
	subscription *nats.Subscription
	messages     chan *nats.Msg
	logger       *zap.Logger
}

func ConnectConsumer(config Config, logger *zap.Logger) (*Consumer, error) {
	messages := make(chan *nats.Msg, 8192)
	connection, err := connect(config, "configra-management-v1", logger,
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			if errors.Is(err, nats.ErrSlowConsumer) {
				logger.Warn("Access Event dropped by bounded NATS Consumer")
				return
			}
			logger.Warn("NATS asynchronous Consumer error")
		}),
	)
	if err != nil {
		return nil, err
	}
	subscription, err := connection.ChanQueueSubscribe(Subject, "configra-management-v1", messages)
	if err != nil {
		connection.Close()
		return nil, errors.New("subscribe to NATS Access Events")
	}
	if connection.IsConnected() {
		if err := connection.FlushTimeout(5 * time.Second); err != nil {
			_ = subscription.Unsubscribe()
			connection.Close()
			return nil, errors.New("activate NATS Access Event Subscription")
		}
	}
	return &Consumer{
		connection:   connection,
		subscription: subscription,
		messages:     messages,
		logger:       logger,
	}, nil
}

func (consumer *Consumer) Run(ctx context.Context, store *logstore.Store) error {
	if consumer == nil || store == nil {
		return errors.New("NATS Consumer and ClickHouse Store are required")
	}
	defer consumer.Close()
	ticker := time.NewTicker(accessFlushInterval)
	defer ticker.Stop()
	events := make([]machine.AccessEvent, 0, accessBatchSize)
	flush := func(flushContext context.Context) {
		if len(events) == 0 {
			return
		}
		if err := store.AppendAccess(flushContext, events); err != nil {
			consumer.logger.Warn("Access Event batch dropped", zap.Int("count", len(events)))
			if ctx.Err() == nil {
				initializeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
				_ = store.Initialize(initializeContext)
				cancel()
			}
		}
		events = events[:0]
	}
	for {
		select {
		case message, open := <-consumer.messages:
			if !open {
				return errors.New("NATS Access Event channel closed")
			}
			if message == nil {
				continue
			}
			event, err := decodeAccessEvent(message.Data)
			if err != nil {
				consumer.logger.Warn("Invalid Access Event dropped")
				continue
			}
			events = append(events, event)
			if len(events) == accessBatchSize {
				flushContext, cancel := context.WithTimeout(ctx, 5*time.Second)
				flush(flushContext)
				cancel()
			}
		case <-ticker.C:
			flushContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			flush(flushContext)
			cancel()
		case <-ctx.Done():
			flushContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			flush(flushContext)
			cancel()
			return nil
		}
	}
}

func (consumer *Consumer) Close() {
	if consumer == nil {
		return
	}
	_ = consumer.subscription.Unsubscribe()
	consumer.connection.Close()
}
