package mysqlstore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

func TestTokenValidationIdentifiesInvalidFieldWithoutEchoingInput(t *testing.T) {
	for _, test := range []struct {
		name, field, reason string
		ttl                 time.Duration
		change              func(*TokenCreate)
	}{
		{"ninety one days", "expires_at", "write_token_expiry_limit", 0, func(r *TokenCreate) { r.ExpiresAt = time.Now().Add(91 * 24 * time.Hour) }},
		{"shorter server policy", "expires_at", "write_token_expiry_limit", 2 * 24 * time.Hour, func(r *TokenCreate) { r.ExpiresAt = time.Now().Add(3 * 24 * time.Hour) }},
		{"missing expiry", "expires_at", "expiry_required", 0, func(r *TokenCreate) { r.ExpiresAt = time.Time{} }},
		{"past expiry", "expires_at", "expiry_in_past", 0, func(r *TokenCreate) { r.ExpiresAt = time.Now().Add(-time.Hour) }},
		{"unicode dash", "config_keys", "scope_key_invalid", 0, func(r *TokenCreate) { r.ConfigKeys = []string{"private–sentinel"} }},
		{"duplicate namespace", "namespace_keys", "scope_keys_duplicate", 0, func(r *TokenCreate) { r.NamespaceKeys = []string{"private_sentinel", "private_sentinel"} }},
		{"too many keys", "config_keys", "scope_keys_too_many", 0, func(r *TokenCreate) { r.ConfigKeys = make([]string, 257) }},
		{"missing environment", "environment_keys", "environment_required", 0, func(r *TokenCreate) { r.EnvironmentKeys = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := TokenCreate{Actor: Actor{Type: "user", ID: "admin"}, Kind: machine.TokenWriteScoped, DisplayName: "private-sentinel-name", EnvironmentKeys: []string{"prod"}, ExpiresAt: time.Now().Add(time.Hour)}
			test.change(&request)
			store := &Store{WriteTokenMaxTTL: test.ttl}
			err := store.validateTokenCreate(request)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("validation identity: %v", err)
			}
			encoded, _ := json.Marshal(err)
			var issue struct {
				Field      string `json:"field"`
				Reason     string `json:"reason"`
				MaxTTLDays int    `json:"max_ttl_days"`
			}
			_ = json.Unmarshal(encoded, &issue)
			if issue.Field != test.field || issue.Reason != test.reason {
				t.Fatalf("lost field-specific reason: %s", encoded)
			}
			if test.reason == "write_token_expiry_limit" {
				want := 90
				if test.ttl > 0 {
					want = 2
				}
				if issue.MaxTTLDays != want {
					t.Fatalf("effective lifetime missing: %s", encoded)
				}
			}
			if strings.Contains(string(encoded)+err.Error(), "private") {
				t.Fatal("validation echoed submitted values")
			}
		})
	}
}
