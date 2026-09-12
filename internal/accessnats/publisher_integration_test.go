//go:build integration

package accessnats_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/machine"
)

func TestPublisherSendsOneValueFreeAccessEventOverCoreNATS(t *testing.T) {
	url := os.Getenv("CONFIGRA_TEST_NATS_URL")
	if url == "" {
		t.Skip("CONFIGRA_TEST_NATS_URL is not set")
	}
	subscriber, err := nats.Connect(url, nats.Timeout(5*time.Second))
	if err != nil {
		t.Fatalf("connect subscriber: %v", err)
	}
	defer subscriber.Close()
	subscription, err := subscriber.SubscribeSync(accessnats.Subject)
	if err != nil {
		t.Fatalf("SubscribeSync: %v", err)
	}
	if err := subscriber.FlushTimeout(5 * time.Second); err != nil {
		t.Fatalf("flush Subscription: %v", err)
	}

	publisher, err := accessnats.Connect(accessnats.Config{URLs: []string{url}}, zap.NewNop())
	if err != nil {
		t.Fatalf("Connect Publisher: %v", err)
	}
	event := machine.AccessEvent{
		Time:           time.Date(2026, 8, 27, 2, 3, 4, 0, time.UTC),
		Principal:      "public-id",
		Authentication: machine.AuthenticationTokenOnly,
		Environment:    "a",
		ResourceType:   "config",
		Resource:       "payment",
		ConfigRevision: 3,
		VaultRevisions: map[string]uint64{"platform.database": 8},
	}
	if !publisher.TryPublish(event) {
		t.Fatal("TryPublish failed")
	}
	flushContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publisher.Close(flushContext); err != nil {
		t.Fatalf("Close Publisher: %v", err)
	}
	message, err := subscription.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("NextMsg: %v", err)
	}
	var payload struct {
		SchemaVersion  int                    `json:"schema_version"`
		Principal      string                 `json:"principal"`
		Authentication machine.Authentication `json:"authentication"`
		Environment    string                 `json:"environment"`
		Resource       string                 `json:"resource"`
		ConfigRevision uint64                 `json:"config_revision"`
		VaultRevisions map[string]uint64      `json:"vault_revisions"`
	}
	if err := json.Unmarshal(message.Data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if message.Subject != accessnats.Subject || payload.SchemaVersion != 1 ||
		payload.Principal != "public-id" || payload.Authentication != machine.AuthenticationTokenOnly ||
		payload.Environment != "a" || payload.Resource != "payment" || payload.ConfigRevision != 3 ||
		payload.VaultRevisions["platform.database"] != 8 {
		t.Fatalf("Access Event = %s", message.Data)
	}
}
