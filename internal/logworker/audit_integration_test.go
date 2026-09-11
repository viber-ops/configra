//go:build integration

package logworker_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/logworker"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestRunDeliversAccessAndAuditEventsUntilCanceled(t *testing.T) {
	natsURL := os.Getenv("CONFIGRA_TEST_NATS_URL")
	clickHouseDSN := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	if natsURL == "" || clickHouseDSN == "" {
		t.Skip("CONFIGRA_TEST_NATS_URL and CONFIGRA_TEST_CLICKHOUSE_DSN are required")
	}
	mysqlStore := newAuditMySQLStore(t, context.Background())
	logs, err := logstore.New(clickHouseDSN)
	if err != nil {
		t.Fatalf("New Log Store: %v", err)
	}
	defer logs.Close()
	consumer, err := accessnats.ConnectConsumer(accessnats.Config{URLs: []string{natsURL}}, zap.NewNop())
	if err != nil {
		t.Fatalf("Connect Consumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	sender, err := notification.NewSender(notification.Config{})
	if err != nil {
		t.Fatalf("New Notification Sender: %v", err)
	}
	defer sender.CloseIdleConnections()
	go func() { done <- logworker.Run(ctx, mysqlStore, logs, consumer, sender, zap.NewNop()) }()

	unique := fmt.Sprintf("worker-%d", time.Now().UnixNano())
	if _, err := mysqlStore.ApplyEnvironmentChange(context.Background(), mysqlstore.EnvironmentChange{
		OperationID: unique,
		Actor:       mysqlstore.Actor{Type: "user", ID: "issuer|subject"},
		Action:      mysqlstore.EnvironmentCreate,
		Key:         unique,
		DisplayName: "Worker Test",
	}); err != nil {
		t.Fatalf("create audited Environment: %v", err)
	}
	publisher, err := accessnats.Connect(accessnats.Config{URLs: []string{natsURL}}, zap.NewNop())
	if err != nil {
		t.Fatalf("Connect Publisher: %v", err)
	}
	if !publisher.TryPublish(machine.AccessEvent{
		Time:           time.Now().UTC(),
		Principal:      unique,
		Authentication: machine.AuthenticationTokenOnly,
		Environment:    unique,
		ResourceType:   "config",
		Resource:       "worker-test",
		ConfigRevision: 1,
	}) {
		t.Fatal("publish Access Event")
	}
	flushContext, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := publisher.Close(flushContext); err != nil {
		flushCancel()
		t.Fatalf("close Publisher: %v", err)
	}
	flushCancel()

	options, _ := clickhouse.ParseDSN(clickHouseDSN)
	reader, err := clickhouse.Open(options)
	if err != nil {
		t.Fatalf("open ClickHouse reader: %v", err)
	}
	defer reader.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var accessCount, auditCount uint64
		accessErr := reader.QueryRow(context.Background(), "SELECT count() FROM access_events WHERE principal = ?", unique).Scan(&accessCount)
		auditErr := reader.QueryRow(context.Background(), "SELECT count() FROM audit_events WHERE operation_id = ?", unique).Scan(&auditCount)
		if accessErr == nil && auditErr == nil && accessCount == 1 && auditCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivered Access/Audit = %d/%d, errors = %v/%v", accessCount, auditCount, accessErr, auditErr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestDeliverAuditOnceCompletesOutboxOnlyAfterClickHouseInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mysqlStore := newAuditMySQLStore(t, ctx)
	clickHouseDSN := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	if clickHouseDSN == "" {
		t.Skip("CONFIGRA_TEST_CLICKHOUSE_DSN is not set")
	}
	clickHouseStore, err := logstore.New(clickHouseDSN)
	if err != nil {
		t.Fatalf("New ClickHouse Store: %v", err)
	}
	defer clickHouseStore.Close()
	if err := clickHouseStore.Initialize(ctx); err != nil {
		t.Fatalf("Initialize ClickHouse: %v", err)
	}
	operationID := fmt.Sprintf("audit-delivery-%d", time.Now().UnixNano())
	if _, err := mysqlStore.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{
		OperationID: operationID,
		Actor:       mysqlstore.Actor{Type: "user", ID: "issuer|subject"},
		Action:      mysqlstore.EnvironmentCreate,
		Key:         "production",
		DisplayName: "Production",
	}); err != nil {
		t.Fatalf("create audited Environment: %v", err)
	}

	delivered, err := logworker.DeliverAuditOnce(ctx, mysqlStore, clickHouseStore)
	if err != nil || delivered != 1 {
		t.Fatalf("DeliverAuditOnce = %d, %v", delivered, err)
	}
	remaining, err := mysqlStore.ClaimOutbox(ctx, mysqlstore.OutboxAudit, 10, time.Minute)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("remaining Audit Outbox = %d, %v", len(remaining), err)
	}
	options, _ := clickhouse.ParseDSN(clickHouseDSN)
	reader, err := clickhouse.Open(options)
	if err != nil {
		t.Fatalf("open ClickHouse reader: %v", err)
	}
	defer reader.Close()
	var actorID, action, environment string
	if err := reader.QueryRow(ctx, `
		SELECT actor_id, action, environment FROM audit_events
		WHERE operation_id = ? ORDER BY ingested_at DESC LIMIT 1
	`, operationID).Scan(&actorID, &action, &environment); err != nil {
		t.Fatalf("read delivered Audit Event: %v", err)
	}
	if actorID != "issuer|subject" || action != "environment.create" || environment != "production" {
		t.Fatalf("Audit Event = %q %q %q", actorID, action, environment)
	}
}

func TestDeliverAuditOnceRetainsOutboxWhenClickHouseIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mysqlStore := newAuditMySQLStore(t, ctx)
	operationID := fmt.Sprintf("audit-retry-%d", time.Now().UnixNano())
	if _, err := mysqlStore.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{
		OperationID: operationID,
		Actor:       mysqlstore.Actor{Type: "user", ID: "issuer|subject"},
		Action:      mysqlstore.EnvironmentCreate,
		Key:         "production",
		DisplayName: "Production",
	}); err != nil {
		t.Fatalf("create audited Environment: %v", err)
	}
	unavailable, err := logstore.New("clickhouse://default@127.0.0.1:1/default?dial_timeout=100ms")
	if err != nil {
		t.Fatalf("New unavailable ClickHouse Store: %v", err)
	}
	defer unavailable.Close()
	if delivered, err := logworker.DeliverAuditOnce(ctx, mysqlStore, unavailable); err == nil || delivered != 0 {
		t.Fatalf("DeliverAuditOnce unavailable = %d, %v", delivered, err)
	}
	time.Sleep(1100 * time.Millisecond)
	retried, err := mysqlStore.ClaimOutbox(ctx, mysqlstore.OutboxAudit, 10, time.Minute)
	if err != nil || len(retried) != 1 || retried[0].OperationID != operationID || retried[0].Attempts != 2 {
		t.Fatalf("retried Audit Outbox = %#v, %v", retried, err)
	}
}

func newAuditMySQLStore(t *testing.T, ctx context.Context) *mysqlstore.Store {
	t.Helper()
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	configuration, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse Root DSN: %v", err)
	}
	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open Root MySQL: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	databaseName := fmt.Sprintf("configra_audit_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+databaseName+" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create Audit database: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(cleanupContext, "DROP DATABASE "+databaseName)
	})
	configuration.DBName = databaseName
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := mysqlstore.OpenManagement(ctx, configuration.FormatDSN(), provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
