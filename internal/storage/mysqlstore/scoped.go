package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

var ErrUnauthorized = errors.New("credential inactive")

// Hold the Token row through commit. Revocation and scoped writes therefore
// serialize, including replays and writes on an already-open TLS connection.
func authorizeScopedWrite(ctx context.Context, tx *sql.Tx, actor Actor, environment, config, namespace string) (machine.Token, error) {
	if actor.Type != "token" {
		return machine.Token{}, nil
	}
	var token machine.Token
	var configs, namespaces []byte
	var revoked, expires sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT public_id, kind, expires_at, revoked_at, config_keys, namespace_keys,
		EXISTS (SELECT 1 FROM api_token_environments AS grant_record JOIN environments AS environment ON environment.id = grant_record.environment_id
		WHERE grant_record.token_id = api_tokens.id AND environment.resource_key = ? AND environment.archived_at IS NULL)
		FROM api_tokens WHERE public_id = ? FOR SHARE`, environment, actor.ID).
		Scan(&token.PublicID, &token.Kind, &expires, &revoked, &configs, &namespaces, &token.EnvironmentGranted)
	if errors.Is(err, sql.ErrNoRows) {
		return token, ErrUnauthorized
	}
	if err != nil {
		return token, errors.New("read scoped credential")
	}
	if token.Kind != machine.TokenWriteScoped {
		return token, machine.ErrForbidden
	}
	if !expires.Valid || revoked.Valid || !time.Now().Before(expires.Time) {
		return token, ErrUnauthorized
	}
	token.ExpiresAt = expires.Time.UTC()
	if decodeTokenScope(configs, &token.ConfigKeys) != nil || decodeTokenScope(namespaces, &token.NamespaceKeys) != nil {
		return token, errors.New("invalid scoped credential")
	}
	if token.Kind != machine.TokenWriteScoped || !token.EnvironmentGranted ||
		(config != "" && !token.AllowsConfig(config)) || (namespace != "" && !token.AllowsNamespace(namespace)) {
		return token, machine.ErrForbidden
	}
	return token, nil
}

// Global Item/schema operations require every current binding, including
// archived Environments. Unbound Variants have no Environment authorization.
func allVaultBindingsGranted(ctx context.Context, tx *sql.Tx, publicID string, itemID []byte, revision uint64) (bool, error) {
	var forbidden bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM vault_revision_variants AS variant
		LEFT JOIN vault_revision_variant_environments AS binding ON binding.item_id = variant.item_id AND binding.revision = variant.revision AND binding.variant_id = variant.variant_id
		WHERE variant.item_id = ? AND variant.revision = ? AND (binding.environment_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM api_token_environments AS grant_record JOIN api_tokens AS token ON token.id = grant_record.token_id
			WHERE token.public_id = ? AND grant_record.environment_id = binding.environment_id)))`, itemID, revision, publicID).Scan(&forbidden)
	return !forbidden, err
}

type auditSourceIPKey struct{}

// Use the TCP peer address; caller-supplied forwarding headers are not trusted.
func WithAuditSourceIP(ctx context.Context, remoteAddr string) context.Context {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ctx
	}
	return context.WithValue(ctx, auditSourceIPKey{}, ip.String())
}

func auditSourceIP(ctx context.Context) string {
	ip, _ := ctx.Value(auditSourceIPKey{}).(string)
	return ip
}
