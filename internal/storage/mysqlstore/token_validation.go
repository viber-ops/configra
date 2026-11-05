package mysqlstore

import (
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

// TokenValidationError contains only server-selected field/reason identifiers
// and the public lifetime policy. Never attach input values or parser errors.
type TokenValidationError struct {
	Field      string `json:"field"`
	Reason     string `json:"reason"`
	MaxTTLDays int    `json:"max_ttl_days,omitempty"`
}

func (issue *TokenValidationError) Error() string { return "API Token validation failed" }
func (issue *TokenValidationError) Unwrap() error { return ErrValidation }

func (store *Store) WriteTokenLifetime() time.Duration {
	if store.WriteTokenMaxTTL <= 0 || store.WriteTokenMaxTTL > 90*24*time.Hour {
		return 90 * 24 * time.Hour
	}
	return store.WriteTokenMaxTTL
}

func (store *Store) validateTokenCreate(request TokenCreate) error {
	if request.Kind != "" && request.Kind != machine.TokenReadOnly && request.Kind != machine.TokenWriteScoped {
		return &TokenValidationError{Field: "kind", Reason: "token_kind_invalid"}
	}
	for _, scope := range []struct {
		field string
		keys  []string
	}{{"config_keys", request.ConfigKeys}, {"namespace_keys", request.NamespaceKeys}} {
		if len(scope.keys) > 256 {
			return &TokenValidationError{Field: scope.field, Reason: "scope_keys_too_many"}
		}
		seen := map[string]bool{}
		for _, key := range scope.keys {
			if !validResourceKey(key) {
				return &TokenValidationError{Field: scope.field, Reason: "scope_key_invalid"}
			}
			if seen[key] {
				return &TokenValidationError{Field: scope.field, Reason: "scope_keys_duplicate"}
			}
			seen[key] = true
		}
	}
	if request.Kind == machine.TokenWriteScoped {
		if request.Actor.Type != "user" {
			return &TokenValidationError{Field: "kind", Reason: "write_token_requires_user"}
		}
		if len(request.EnvironmentKeys) == 0 {
			return &TokenValidationError{Field: "environment_keys", Reason: "environment_required"}
		}
		if request.AllowWithoutMTLS {
			return &TokenValidationError{Field: "allow_without_mtls", Reason: "write_token_requires_mtls"}
		}
		if request.NeverExpires || request.ExpiresAt.IsZero() {
			return &TokenValidationError{Field: "expires_at", Reason: "expiry_required"}
		}
		if request.ExpiresAt.After(time.Now().Add(store.WriteTokenLifetime())) {
			return &TokenValidationError{Field: "expires_at", Reason: "write_token_expiry_limit", MaxTTLDays: int(store.WriteTokenLifetime() / (24 * time.Hour))}
		}
	}
	if request.DisplayName == "" || len(request.DisplayName) > 255 {
		return &TokenValidationError{Field: "display_name", Reason: "token_name_invalid"}
	}
	if request.NeverExpires && !request.ExpiresAt.IsZero() {
		return &TokenValidationError{Field: "expires_at", Reason: "expiry_conflicts_with_never"}
	}
	if !request.ExpiresAt.IsZero() && !time.Now().Before(request.ExpiresAt) {
		return &TokenValidationError{Field: "expires_at", Reason: "expiry_in_past"}
	}
	seen := map[string]bool{}
	for _, key := range request.EnvironmentKeys {
		if !validResourceKey(key) || seen[key] {
			return &TokenValidationError{Field: "environment_keys", Reason: "environment_keys_invalid"}
		}
		seen[key] = true
	}
	return nil
}
