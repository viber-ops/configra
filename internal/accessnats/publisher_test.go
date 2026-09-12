package accessnats

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/machine"
)

func TestMarshalAccessEventContainsOnlyTheVersionedMetadataContract(t *testing.T) {
	event := machine.AccessEvent{
		Time:           time.Date(2026, 8, 27, 1, 2, 3, 4, time.UTC),
		Principal:      "token-public-id",
		Authentication: machine.AuthenticationMTLS,
		Environment:    "production",
		ResourceType:   "config",
		Resource:       "payment",
		ConfigRevision: 21,
		VaultRevisions: map[string]uint64{"platform.mysql": 7},
	}
	payload, err := marshalAccessEvent(event)
	if err != nil {
		t.Fatalf("marshalAccessEvent: %v", err)
	}
	want := `{"schema_version":1,"time":"2026-08-27T01:02:03.000000004Z","principal":"token-public-id","authentication":"mtls","environment":"production","resource_type":"config","resource":"payment","config_revision":21,"vault_revisions":{"platform.mysql":7}}`
	if string(payload) != want {
		t.Fatalf("payload = %s\nwant    = %s", payload, want)
	}
	for _, forbidden := range []string{"authorization", "token_secret", "content", "value", "file_bytes"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("payload contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestPublisherBuffersDuringNATSOutageAndFailsFastAfterClose(t *testing.T) {
	publisher, err := Connect(Config{URLs: []string{"nats://127.0.0.1:1"}}, zap.NewNop())
	if err != nil {
		t.Fatalf("Connect during outage: %v", err)
	}
	event := machine.AccessEvent{
		Time:           time.Now().UTC(),
		Principal:      "token-public-id",
		Authentication: machine.AuthenticationTokenOnly,
		Environment:    "a",
		ResourceType:   "config",
		Resource:       "payment",
		ConfigRevision: 1,
	}
	if !publisher.TryPublish(event) {
		t.Fatal("TryPublish did not use the bounded reconnect buffer")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_ = publisher.Close(ctx)
	if publisher.TryPublish(event) {
		t.Fatal("TryPublish succeeded after Close")
	}
}

func TestDecodeAccessEventRejectsUnknownValueFieldsWithoutLeakingThem(t *testing.T) {
	valid := []byte(`{"schema_version":1,"time":"2026-08-27T01:02:03Z","principal":"public-id","authentication":"token_only","environment":"a","resource_type":"config","resource":"payment","config_revision":1}`)
	event, err := decodeAccessEvent(valid)
	if err != nil || event.Principal != "public-id" || event.ConfigRevision != 1 {
		t.Fatalf("decodeAccessEvent = %#v, %v", event, err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"schema_version":1,"time":"2026-08-27T01:02:03Z","principal":"public-id","authentication":"token_only","environment":"a","resource_type":"config","resource":"payment","config_revision":1,"content":"vault-secret-sentinel"}`),
		[]byte(`{"schema_version":2,"time":"2026-08-27T01:02:03Z","principal":"public-id","authentication":"token_only","environment":"a","resource_type":"config","resource":"payment"}`),
		[]byte(`{"broken":"vault-secret-sentinel"}`),
	} {
		_, err := decodeAccessEvent(payload)
		if err == nil {
			t.Fatalf("decodeAccessEvent accepted %s", payload)
		}
		if strings.Contains(err.Error(), "vault-secret-sentinel") {
			t.Fatalf("decodeAccessEvent leaked payload: %v", err)
		}
	}
}

func TestDecodeAccessEventAcceptsOIDCVaultReadAndRejectsInvalidVaultVersion(t *testing.T) {
	valid := []byte(`{"schema_version":1,"time":"2026-08-27T01:02:03Z","principal":"issuer|admin","authentication":"oidc","environment":"","namespace":"platform","resource_type":"vault_item","resource":"redis","vault_revisions":{"platform.redis":2}}`)
	event, err := decodeAccessEvent(valid)
	if err != nil || event.Authentication != machine.AuthenticationOIDC || event.Namespace != "platform" || event.VaultRevisions["platform.redis"] != 2 {
		t.Fatalf("decodeAccessEvent = %#v, %v", event, err)
	}
	invalid := []byte(`{"schema_version":1,"time":"2026-08-27T01:02:03Z","principal":"issuer|admin","authentication":"oidc","environment":"","namespace":"platform","resource_type":"vault_item","resource":"redis","vault_revisions":{"platform.redis":0}}`)
	if _, err := decodeAccessEvent(invalid); err == nil {
		t.Fatal("decodeAccessEvent accepted zero Vault Revision")
	}
}
