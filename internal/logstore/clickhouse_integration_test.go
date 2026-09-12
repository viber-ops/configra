//go:build integration

package logstore_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/machine"
)

func TestClickHouseLogStoreInitializesAndBatchesAccessAndAuditMetadata(t *testing.T) {
	dsn := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("CONFIGRA_TEST_CLICKHOUSE_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := logstore.New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	unique := fmt.Sprintf("test-%d", time.Now().UnixNano())
	accessTime := time.Now().UTC()
	if err := store.AppendAccess(ctx, []machine.AccessEvent{{
		Time:           accessTime,
		Principal:      unique,
		Authentication: machine.AuthenticationMTLS,
		Environment:    "production",
		ResourceType:   "config",
		Resource:       "payment",
		ConfigRevision: 21,
		VaultRevisions: map[string]uint64{"platform.mysql": 8},
	}}); err != nil {
		t.Fatalf("AppendAccess: %v", err)
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		t.Fatalf("random Audit ID: %v", err)
	}
	auditID := hex.EncodeToString(idBytes)
	if err := store.AppendAudits(ctx, []logstore.AuditRecord{{
		ID:           auditID,
		Time:         accessTime,
		EventType:    "config.commit.succeeded",
		OperationID:  unique,
		ActorType:    "user",
		ActorID:      "issuer|subject",
		Action:       "config.commit",
		Outcome:      "success",
		Environment:  "production",
		ResourceType: "config",
		Resource:     "payment",
		Revision:     21,
		Attempt:      1,
	}}); err != nil {
		t.Fatalf("AppendAudits: %v", err)
	}

	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	reader, err := clickhouse.Open(options)
	if err != nil {
		t.Fatalf("Open reader: %v", err)
	}
	defer reader.Close()
	var principal, authentication, environment, resource string
	var configRevision uint64
	var vaultRevisions map[string]uint64
	if err := reader.QueryRow(ctx, `
		SELECT principal, authentication, environment, resource, config_revision, vault_revisions
		FROM access_events WHERE principal = ? ORDER BY ingested_at DESC LIMIT 1
	`, unique).Scan(&principal, &authentication, &environment, &resource, &configRevision, &vaultRevisions); err != nil {
		t.Fatalf("read Access Event: %v", err)
	}
	if principal != unique || authentication != "mtls" || environment != "production" || resource != "payment" ||
		configRevision != 21 || vaultRevisions["platform.mysql"] != 8 {
		t.Fatalf("Access Event = %q %q %q %q %d %v", principal, authentication, environment, resource, configRevision, vaultRevisions)
	}
	var operationID, actorID, outcome string
	var attempt uint32
	if err := reader.QueryRow(ctx, `
		SELECT operation_id, actor_id, outcome, delivery_attempt
		FROM audit_events WHERE event_id = ? LIMIT 1
	`, auditID).Scan(&operationID, &actorID, &outcome, &attempt); err != nil {
		t.Fatalf("read Audit Event: %v", err)
	}
	if operationID != unique || actorID != "issuer|subject" || outcome != "success" || attempt != 1 {
		t.Fatalf("Audit Event = %q %q %q %d", operationID, actorID, outcome, attempt)
	}
	accessRecords, err := store.ListAccess(ctx, 500)
	if err != nil {
		t.Fatalf("ListAccess: %v", err)
	}
	foundAccess := false
	for _, record := range accessRecords {
		if record.Principal == unique {
			foundAccess = record.Time.Equal(accessTime) && record.ConfigRevision == 21 && record.VaultRevisions["platform.mysql"] == 8
		}
	}
	if !foundAccess {
		t.Fatalf("ListAccess omitted inserted record for %q", unique)
	}
	auditPage, err := store.ListAudits(ctx, logstore.AuditQuery{Limit: 500, Search: unique})
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	foundAudit := false
	for _, record := range auditPage.Items {
		if record.ID == auditID {
			foundAudit = record.OperationID == unique && record.ActorID == "issuer|subject" && record.Attempt == 1
		}
	}
	if !foundAudit {
		t.Fatalf("ListAudits omitted inserted record %q", auditID)
	}
}
