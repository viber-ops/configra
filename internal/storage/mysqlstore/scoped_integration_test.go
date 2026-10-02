//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestScopedTokenMigrationFromV2PreservesLegacyReadCredentials(t *testing.T) {
	ctx := context.Background()
	dsn, db, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{Type: "user", ID: "migration-admin"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "migration-env", Actor: actor, Action: EnvironmentCreate, Key: "production", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.CreateToken(ctx, TokenCreate{OperationID: "migration-read-token", Actor: actor, DisplayName: "Legacy", EnvironmentKeys: []string{"production"}, AllowWithoutMTLS: true, NeverExpires: true})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	// Reconstruct the released v2 schema around an existing read credential.
	if _, err := db.ExecContext(ctx, `ALTER TABLE api_tokens DROP FOREIGN KEY api_tokens_parent_fk,
		DROP CHECK api_tokens_kind, DROP CHECK api_tokens_write_auth, DROP INDEX api_tokens_parent,
		DROP COLUMN kind, DROP COLUMN config_keys, DROP COLUMN namespace_keys,
		DROP COLUMN parent_public_id, DROP COLUMN deployment_certificate_fingerprint`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 3"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	wrong, _ := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if unexpected, err := OpenManagement(ctx, dsn, wrong); unexpected != nil || !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatal("migration accepted a wrong Master Key")
	}
	var version int
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 2 {
		t.Fatal("wrong-key startup altered the schema")
	}
	if api, err := OpenAPI(ctx, dsn, provider); api != nil || err == nil {
		t.Fatal("API silently migrated the schema")
	}
	upgraded, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	token, err := upgraded.TokenForEnvironment(ctx, legacy.PublicID, "production")
	if err != nil || token.Kind != machine.TokenReadOnly || !token.EnvironmentGranted || !token.AllowWithoutMTLS || !token.ExpiresAt.IsZero() || token.ConfigKeys != nil || token.NamespaceKeys != nil {
		t.Fatal("migration changed legacy credentials")
	}
	api, err := OpenAPI(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	api.Close()
}

func TestScopedTokenExpiryPolicyAndScopeNullVersusEmpty(t *testing.T) {
	ctx := context.Background()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.WriteTokenMaxTTL = 2 * 24 * time.Hour
	actor := Actor{Type: "user", ID: "scope-admin"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "scope-policy-env", Actor: actor, Action: EnvironmentCreate, Key: "production", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	valid := TokenCreate{OperationID: "scope-policy-token", Actor: actor, Kind: machine.TokenWriteScoped, DisplayName: "Writer", EnvironmentKeys: []string{"production"}, ExpiresAt: time.Now().Add(time.Hour)}
	for _, change := range []func(*TokenCreate){
		func(r *TokenCreate) { r.ExpiresAt = time.Time{} }, func(r *TokenCreate) { r.ExpiresAt = time.Now().Add(-time.Hour) },
		func(r *TokenCreate) { r.ExpiresAt = time.Now().Add(3 * 24 * time.Hour) }, func(r *TokenCreate) { r.AllowWithoutMTLS = true },
		func(r *TokenCreate) { r.NeverExpires = true }, func(r *TokenCreate) { r.EnvironmentKeys = nil },
		func(r *TokenCreate) { r.ConfigKeys = []string{"server", "server"} }, func(r *TokenCreate) { r.NamespaceKeys = []string{"invalid.key"} },
	} {
		request := valid
		change(&request)
		if store.validateTokenCreate(request) == nil {
			t.Fatal("invalid scoped Token policy was accepted")
		}
	}
	first, err := store.CreateToken(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	valid.ConfigKeys, valid.NamespaceKeys = []string{}, []string{}
	if _, err := store.CreateToken(ctx, valid); !errors.Is(err, ErrOperationReuse) {
		t.Fatal("empty deny-all scope replayed an unrestricted creation")
	}
	valid.OperationID = "scope-policy-deny-all"
	second, err := store.CreateToken(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		publicID string
		allowed  bool
	}{{first.PublicID, true}, {second.PublicID, false}} {
		token, err := store.TokenForEnvironment(ctx, test.publicID, "production")
		if err != nil || token.AllowsConfig("server") != test.allowed || token.AllowsNamespace("deployment") != test.allowed {
			t.Fatal("null/empty allowlists changed meaning")
		}
	}
}
