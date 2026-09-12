//go:build integration

package accessnats_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/machine"
)

func TestConsumerBatchesValidNATSEventsIntoClickHouseAndDropsUnknownFields(t *testing.T) {
	natsURL := os.Getenv("CONFIGRA_TEST_NATS_URL")
	clickHouseDSN := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	if natsURL == "" || clickHouseDSN == "" {
		t.Skip("CONFIGRA_TEST_NATS_URL and CONFIGRA_TEST_CLICKHOUSE_DSN are required")
	}
	logs, err := logstore.New(clickHouseDSN)
	if err != nil {
		t.Fatalf("New Log Store: %v", err)
	}
	defer logs.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if err := logs.Initialize(ctx); err != nil {
		t.Fatalf("Initialize Log Store: %v", err)
	}
	consumer, err := accessnats.ConnectConsumer(accessnats.Config{URLs: []string{natsURL}}, zap.NewNop())
	if err != nil {
		t.Fatalf("ConnectConsumer: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- consumer.Run(ctx, logs) }()

	connection, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect raw Publisher: %v", err)
	}
	unique := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
	malformed := fmt.Sprintf(`{"schema_version":1,"time":"2026-08-27T05:06:07Z","principal":"%s-invalid","authentication":"token_only","environment":"a","resource_type":"config","resource":"payment","config_revision":1,"content":"vault-secret-sentinel"}`, unique)
	if err := connection.Publish(accessnats.Subject, []byte(malformed)); err != nil {
		t.Fatalf("publish malformed Event: %v", err)
	}
	publisher, err := accessnats.Connect(accessnats.Config{URLs: []string{natsURL}}, zap.NewNop())
	if err != nil {
		t.Fatalf("Connect Publisher: %v", err)
	}
	if !publisher.TryPublish(machine.AccessEvent{
		Time:           time.Date(2026, 8, 27, 5, 6, 7, 0, time.UTC),
		Principal:      unique,
		Authentication: machine.AuthenticationTokenOnly,
		Environment:    "a",
		ResourceType:   "config",
		Resource:       "payment",
		ConfigRevision: 1,
	}) {
		t.Fatal("TryPublish failed")
	}
	flushContext, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := publisher.Close(flushContext); err != nil {
		flushCancel()
		t.Fatalf("Close Publisher: %v", err)
	}
	flushCancel()
	if err := connection.FlushTimeout(5 * time.Second); err != nil {
		t.Fatalf("flush malformed Event: %v", err)
	}
	connection.Close()

	options, _ := clickhouse.ParseDSN(clickHouseDSN)
	reader, err := clickhouse.Open(options)
	if err != nil {
		t.Fatalf("open ClickHouse reader: %v", err)
	}
	defer reader.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var valid, invalid uint64
		if err := reader.QueryRow(context.Background(), `
			SELECT countIf(principal = ?), countIf(principal = ?)
			FROM access_events
		`, unique, unique+"-invalid").Scan(&valid, &invalid); err != nil {
			t.Fatalf("query Access Events: %v", err)
		}
		if valid == 1 {
			if invalid != 0 {
				t.Fatalf("invalid Access Events = %d", invalid)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("valid Access Event was not batched into ClickHouse")
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Consumer Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Consumer did not stop")
	}
}
