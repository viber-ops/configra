//go:build integration

package mysqlstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestNotificationDestinationEncryptsCredentialsAndSnapshotsSubscribedTargets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_notification_destination")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "admin@example.com"}
	url := "https://hooks.example.test/services/url-token-sentinel"
	secret := "notification-signing-secret-sentinel"
	created, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{
		OperationID: "notification-destination-create", Actor: actor, Key: "operations", DisplayName: "Operations",
		Provider: NotificationGenericWebhook, URL: &url, Secret: &secret, Enabled: true,
		EventTypes: []string{"config.created"},
	})
	if err != nil || created.Outcome != OutcomeSuccess || created.SafeHost != "hooks.example.test" ||
		strings.Contains(created.MaskedSuffix, "url-token-sentinel") {
		t.Fatalf("CommitNotificationDestination = %#v, %v", created, err)
	}
	destinations, err := store.ListNotificationDestinations(ctx, false)
	encoded, encodeErr := json.Marshal(destinations)
	if err != nil || encodeErr != nil || len(destinations) != 1 ||
		strings.Contains(string(encoded), url) || strings.Contains(string(encoded), secret) {
		t.Fatalf("ListNotificationDestinations = %s, %v, %v", encoded, err, encodeErr)
	}
	var ciphertext, encryptedDEK []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT ciphertext, encrypted_dek FROM notification_destinations WHERE resource_key = 'operations'
	`).Scan(&ciphertext, &encryptedDEK); err != nil || bytes.Contains(ciphertext, []byte(url)) ||
		bytes.Contains(ciphertext, []byte(secret)) || bytes.Contains(encryptedDEK, []byte(url)) ||
		bytes.Contains(encryptedDEK, []byte(secret)) {
		t.Fatalf("Notification credentials are not safely encrypted: %v", err)
	}
	invalidFeishuURL := "https://evil.example.test/open-apis/bot/v2/hook/token-sentinel"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{
		OperationID: "notification-destination-invalid-feishu", Actor: actor,
		Key: "invalid-feishu", DisplayName: "Invalid Feishu", Provider: NotificationFeishuBot,
		URL: &invalidFeishuURL, Enabled: true,
	}); !errors.Is(err, ErrValidation) || strings.Contains(err.Error(), invalidFeishuURL) {
		t.Fatalf("invalid Feishu Destination error = %v", err)
	}

	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
		OperationID: "notification-environment-create", Actor: actor, Action: EnvironmentCreate,
		Key: "a", DisplayName: "Environment A",
	}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "notification-config-create", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		Format: configdoc.YAML, Content: []byte("port: 6379\n"),
	}); err != nil {
		t.Fatalf("create Config: %v", err)
	}

	updatedSecret := "latest-notification-signing-secret-sentinel"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{
		OperationID: "notification-destination-update", Actor: actor, Key: "operations", DisplayName: "Operations Updated",
		Provider: NotificationGenericWebhook, Secret: &updatedSecret, Enabled: true,
		EventTypes: []string{"config.created"},
	}); err != nil {
		t.Fatalf("update Notification Destination: %v", err)
	}
	secondURL := "https://new.example.test/hook/new-token-sentinel"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{
		OperationID: "notification-destination-second", Actor: actor, Key: "late", DisplayName: "Late",
		Provider: NotificationGenericWebhook, URL: &secondURL, Enabled: true, EventTypes: []string{"config.created"},
	}); err != nil {
		t.Fatalf("create late Notification Destination: %v", err)
	}

	events, err := store.ClaimOutbox(ctx, OutboxNotification, 100, time.Minute)
	if err != nil {
		t.Fatalf("ClaimOutbox: %v", err)
	}
	var configEvent OutboxEvent
	found := false
	for _, event := range events {
		if event.Type == "config.created" {
			configEvent = event
			found = true
			continue
		}
		progress, progressErr := store.NotificationProgress(ctx, event.ID)
		if progressErr != nil || !progress.Complete {
			t.Fatalf("NotificationProgress without subscribed targets = %#v, %v", progress, progressErr)
		}
		if err := store.CompleteOutbox(ctx, event.ID); err != nil {
			t.Fatalf("complete unrelated Notification Event: %v", err)
		}
	}
	if !found {
		t.Fatal("config.created Notification Outbox Event is missing")
	}
	targets, err := store.ClaimNotificationTargets(ctx, configEvent.ID, 100)
	if err != nil || len(targets) != 1 || targets[0].DestinationKey != "operations" || targets[0].URL != url ||
		targets[0].Secret != updatedSecret || targets[0].Attempt != 1 {
		t.Fatalf("ClaimNotificationTargets = %#v, %v", targets, err)
	}
	nextAttempt := time.Now().UTC().Add(5 * time.Millisecond)
	if err := store.RecordNotificationDelivery(ctx, NotificationDeliveryRecord{
		Target: targets[0], Status: NotificationDeliveryRetrying, HTTPStatus: 503,
		LatencyMS: 12, ErrorCode: "http_503", NextAttempt: nextAttempt,
	}); err != nil {
		t.Fatalf("record retrying Notification Delivery: %v", err)
	}
	progress, err := store.NotificationProgress(ctx, configEvent.ID)
	if err != nil || progress.Complete || progress.NextAttempt.IsZero() {
		t.Fatalf("NotificationProgress retry = %#v, %v", progress, err)
	}
	if err := store.RetryOutbox(ctx, configEvent.ID, progress.NextAttempt, "delivery_retry", false); err != nil {
		t.Fatalf("RetryOutbox: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	var retriedEvents []OutboxEvent
	for len(retriedEvents) == 0 && time.Now().Before(deadline) {
		retriedEvents, err = store.ClaimOutbox(ctx, OutboxNotification, 10, time.Minute)
		if err != nil {
			break
		}
		if len(retriedEvents) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if err != nil || len(retriedEvents) != 1 || retriedEvents[0].ID != configEvent.ID {
		t.Fatalf("retried Notification Events = %#v, %v", retriedEvents, err)
	}
	targets, err = store.ClaimNotificationTargets(ctx, configEvent.ID, 100)
	if err != nil || len(targets) != 1 || targets[0].Attempt != 2 {
		t.Fatalf("retried Notification targets = %#v, %v", targets, err)
	}
	if err := store.RecordNotificationDelivery(ctx, NotificationDeliveryRecord{
		Target: targets[0], Status: NotificationDeliverySucceeded, HTTPStatus: 204, LatencyMS: 8,
	}); err != nil {
		t.Fatalf("record successful Notification Delivery: %v", err)
	}
	progress, err = store.NotificationProgress(ctx, configEvent.ID)
	if err != nil || !progress.Complete {
		t.Fatalf("NotificationProgress success = %#v, %v", progress, err)
	}
	if err := store.CompleteOutbox(ctx, configEvent.ID); err != nil {
		t.Fatalf("CompleteOutbox: %v", err)
	}
	deliveries, err := store.ListNotificationDeliveries(ctx, "operations", 100)
	deliveryJSON, encodeErr := json.Marshal(deliveries)
	if err != nil || encodeErr != nil || len(deliveries) != 2 || deliveries[0].Status != NotificationDeliverySucceeded ||
		strings.Contains(string(deliveryJSON), url) || strings.Contains(string(deliveryJSON), updatedSecret) {
		t.Fatalf("Notification Deliveries = %s, %v, %v", deliveryJSON, err, encodeErr)
	}

	testResult, err := store.QueueNotificationTest(ctx, NotificationTestRequest{
		OperationID: "notification-destination-test", Actor: actor, DestinationKey: "operations",
	})
	if err != nil || testResult.Outcome != OutcomeSuccess || testResult.OutboxEventID == "" {
		t.Fatalf("QueueNotificationTest = %#v, %v", testResult, err)
	}
	testReplay, err := store.QueueNotificationTest(ctx, NotificationTestRequest{
		OperationID: "notification-destination-test", Actor: actor, DestinationKey: "operations",
	})
	if err != nil || testReplay.OutboxEventID != testResult.OutboxEventID {
		t.Fatalf("QueueNotificationTest replay = %#v, %v", testReplay, err)
	}
	assertQueuedNotificationTarget(t, ctx, store, "notification.test", "operations")

	redelivery, err := store.RedeliverNotification(ctx, NotificationRedeliveryRequest{
		OperationID: "notification-redelivery-create", Actor: actor,
		DestinationKey: "operations", DeliveryID: deliveries[0].ID,
	})
	if err != nil || redelivery.Outcome != OutcomeSuccess || redelivery.OutboxEventID == "" || redelivery.SourceDeliveryID != deliveries[0].ID {
		t.Fatalf("RedeliverNotification = %#v, %v", redelivery, err)
	}
	assertQueuedNotificationTarget(t, ctx, store, "config.created", "operations")

	archived, err := store.ApplyNotificationDestinationLifecycle(ctx, NotificationDestinationLifecycle{
		OperationID: "notification-destination-archive", Actor: actor, Action: NotificationDestinationArchive, Key: "operations",
	})
	if err != nil || !archived.Archived {
		t.Fatalf("Archive Notification Destination = %#v, %v", archived, err)
	}
	if active, err := store.ListNotificationDestinations(ctx, false); err != nil || len(active) != 1 || active[0].Key != "late" {
		t.Fatalf("active Notification Destinations = %#v, %v", active, err)
	}
}

func assertQueuedNotificationTarget(t *testing.T, ctx context.Context, store *Store, eventType, destinationKey string) {
	t.Helper()
	events, err := store.ClaimOutbox(ctx, OutboxNotification, 10, time.Minute)
	if err != nil || len(events) != 1 || events[0].Type != eventType {
		t.Fatalf("queued Notification Event = %#v, %v", events, err)
	}
	targets, err := store.ClaimNotificationTargets(ctx, events[0].ID, 10)
	if err != nil || len(targets) != 1 || targets[0].DestinationKey != destinationKey {
		t.Fatalf("queued Notification target = %#v, %v", targets, err)
	}
	if err := store.RecordNotificationDelivery(ctx, NotificationDeliveryRecord{
		Target: targets[0], Status: NotificationDeliverySucceeded, HTTPStatus: 204,
	}); err != nil {
		t.Fatalf("record queued Notification Delivery: %v", err)
	}
	if err := store.CompleteOutbox(ctx, events[0].ID); err != nil {
		t.Fatalf("complete queued Notification Event: %v", err)
	}
}
