//go:build integration

package mysqlstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/viber-ops/configra/internal/configdoc"
	"testing"
	"time"
)

func TestOutboxPruningRequiresArchiveAndPreservesPendingHistoryAndReplay(t *testing.T) {
	ctx := context.Background()
	dsn, db, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := EnvironmentChange{OperationID: "prune-original-change", Actor: Actor{Type: "user", ID: "prune-test"}, Action: EnvironmentCreate, Key: "production", DisplayName: "Production"}
	original, err := store.ApplyEnvironmentChange(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://notification.example.com/archive-sink"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "prune-destination", Actor: request.Actor, Key: "archive-sink", DisplayName: "Archive", Provider: NotificationGenericWebhook, URL: &url, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueueNotificationTest(ctx, NotificationTestRequest{OperationID: "prune-delivered-notification", Actor: request.Actor, DestinationKey: "archive-sink"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []OutboxKind{OutboxAudit, OutboxNotification} {
		events, err := store.ClaimOutbox(ctx, kind, 100, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if kind == OutboxNotification {
				targets, err := store.ClaimNotificationTargets(ctx, event.ID, 100)
				if err != nil {
					t.Fatal(err)
				}
				for _, target := range targets {
					if err := store.RecordNotificationDelivery(ctx, NotificationDeliveryRecord{Target: target, Status: NotificationDeliverySucceeded, HTTPStatus: 204}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := store.CompleteOutbox(ctx, event.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	dead, err := store.QueueNotificationTest(ctx, NotificationTestRequest{OperationID: "prune-dead-notification", Actor: request.Actor, DestinationKey: "archive-sink"})
	if err != nil {
		t.Fatal(err)
	}
	deadBytes, _ := hex.DecodeString(dead.OutboxEventID)
	var deadID OutboxID
	copy(deadID[:], deadBytes)
	claimed, err := store.ClaimOutbox(ctx, OutboxNotification, 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal("claim failed fixture")
	}
	targets, err := store.ClaimNotificationTargets(ctx, deadID, 1)
	if err != nil || len(targets) != 1 {
		t.Fatal("claim failed target")
	}
	if err := store.RecordNotificationDelivery(ctx, NotificationDeliveryRecord{Target: targets[0], Status: NotificationDeliveryDead, HTTPStatus: 400, ErrorCode: "invalid_response"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOutbox(ctx, deadID); err != nil {
		t.Fatal(err)
	}
	// Age the external fixture clock; no implementation result is manufactured.
	if _, err := db.ExecContext(ctx, `UPDATE outbox_events SET completed_at=DATE_SUB(UTC_TIMESTAMP(6),INTERVAL 2 DAY) WHERE status='completed'`); err != nil {
		t.Fatal(err)
	}
	request2 := request
	request2.Key = "testing"
	request2.OperationID = "prune-pending-change"
	if _, err := store.ApplyEnvironmentChange(ctx, request2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "prune-preserve-config", Actor: request.Actor, EnvironmentKey: "production", ConfigKey: "server", ConfigName: "server", Format: configdoc.YAML, Content: []byte("version: 1\n")}); err != nil {
		t.Fatal(err)
	}
	var database string
	if db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database) != nil {
		t.Fatal("fixture database")
	}
	maintenance, err := OpenOutboxMaintenance(ctx, dsn, database)
	if err != nil {
		t.Fatal(err)
	}
	defer maintenance.Close()
	prune := OutboxPrune{OperationID: "prune-archive-once", Before: time.Now().Add(-24 * time.Hour).UTC(), Limit: 100, ArchiveID: "test-archive", Apply: false}
	preview, err := maintenance.Prune(ctx, prune, nil)
	if err != nil || preview.Candidates != 5 || preview.Pruned != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	prune.Apply = true
	if _, err := maintenance.Prune(ctx, prune, func([]byte) error { return errors.New("archive offline") }); err == nil {
		t.Fatal("pruning ignored failed archive")
	}
	prune.Apply = false
	preview, err = maintenance.Prune(ctx, prune, nil)
	if err != nil || preview.Candidates != 5 {
		t.Fatal("failed archive deleted receipts")
	}
	prune.Apply = true
	var archive []byte
	result, err := maintenance.Prune(ctx, prune, func(data []byte) error { archive = append([]byte{}, data...); return nil })
	if err != nil || result.Pruned != 5 || len(archive) == 0 || len(result.ArchiveSHA256) != 64 {
		t.Fatalf("prune result %+v %v", result, err)
	}
	var saved struct {
		Targets    []json.RawMessage `json:"notification_targets"`
		Deliveries []json.RawMessage `json:"notification_deliveries"`
	}
	if json.Unmarshal(archive, &saved) != nil || len(saved.Targets) != 1 || len(saved.Deliveries) != 1 {
		t.Fatal("archive omitted completed delivery metadata")
	}
	replay, err := maintenance.Prune(ctx, prune, func([]byte) error { t.Fatal("replay archived twice"); return nil })
	if err != nil || replay != result {
		t.Fatal("prune replay changed the result")
	}
	again, err := store.ApplyEnvironmentChange(ctx, request)
	if err != nil || again != original {
		t.Fatal("pruning broke original mutation replay")
	}
	if page, err := store.ListEnvironments(ctx, EnvironmentQuery{InventoryQuery: InventoryQuery{Limit: 1, Key: "production"}}); err != nil || len(page.Items) != 1 {
		t.Fatal("pruning affected live data")
	}
	if config, err := store.ReadRawConfigRevision(ctx, "production", "server", 1); err != nil || config.Content != "version: 1\n" {
		t.Fatal("pruning affected immutable history")
	}
	deliveries, err := store.ListNotificationDeliveries(ctx, "archive-sink", 100)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != NotificationDeliveryDead {
		t.Fatal("pruning removed a failed delivery")
	}
	pending, err := store.ClaimOutbox(ctx, OutboxAudit, 100, time.Minute)
	if err != nil || len(pending) < 2 {
		t.Fatal("pruning deleted pending or its own audit")
	}
}
