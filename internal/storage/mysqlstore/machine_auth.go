package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

type authenticationQuery uint8

const (
	tokenAuthenticationQuery authenticationQuery = iota
	certificateAuthenticationQuery
)

var authenticationQueries = [...]string{
	`
		SELECT token.id, token.secret_digest, token.allow_without_mtls, token.expires_at, token.revoked_at,
		       token.kind, token.config_keys, token.namespace_keys, token.deployment_certificate_fingerprint,COALESCE(token.parent_public_id,''),
		       EXISTS (
		           SELECT 1 FROM api_token_environments AS grant_record
		           JOIN environments AS environment ON environment.id = grant_record.environment_id
		           WHERE grant_record.token_id = token.id AND environment.resource_key = ?
		       )
		FROM api_tokens AS token
		WHERE token.public_id = ?
	`,
	`
		SELECT EXISTS (
			SELECT 1
			FROM client_certificates AS certificate
			LEFT JOIN certificate_authorities AS authority ON authority.id = certificate.authority_id
			WHERE certificate.fingerprint_sha256 = ?
			  AND certificate.revoked_at IS NULL
			  AND certificate.not_before <= UTC_TIMESTAMP(6)
			  AND certificate.not_after > UTC_TIMESTAMP(6)
			  AND (certificate.authority_id IS NULL OR (authority.revoked_at IS NULL
			       AND authority.not_before <= UTC_TIMESTAMP(6) AND authority.not_after > UTC_TIMESTAMP(6)))
		)
	`,
}

// Cache only two fixed SQL statements, never authorization results. database/sql
// manages prepared statements on each pooled connection, including reconnection.
// Lifecycle, scope and issuer changes are still read from MySQL on every request.
type authenticationStatements struct {
	mu         sync.Mutex
	statements [2]*sql.Stmt
	closed     bool
}

func (store *Store) authenticationStatement(ctx context.Context, query authenticationQuery) (*sql.Stmt, error) {
	cache := &store.authentication
	cache.mu.Lock()
	closed, statement := cache.closed, cache.statements[query]
	cache.mu.Unlock()
	if closed {
		return nil, sql.ErrConnDone
	}
	if statement != nil {
		return statement, nil
	}
	// Do not hold a mutex across database I/O: a failing first request must not
	// keep other requests waiting beyond their own cancellation deadline.
	prepared, err := store.db.PrepareContext(ctx, authenticationQueries[query])
	if err != nil {
		return nil, fmt.Errorf("prepare machine authentication query: %w", err)
	}
	cache.mu.Lock()
	if cache.closed {
		cache.mu.Unlock()
		_ = prepared.Close()
		return nil, sql.ErrConnDone
	}
	if existing := cache.statements[query]; existing != nil {
		cache.mu.Unlock()
		_ = prepared.Close()
		return existing, nil
	}
	cache.statements[query] = prepared
	cache.mu.Unlock()
	return prepared, nil
}

func (cache *authenticationStatements) close() error {
	cache.mu.Lock()
	cache.closed = true
	statements := cache.statements
	cache.statements = [2]*sql.Stmt{}
	cache.mu.Unlock()
	var result error
	for _, statement := range statements {
		if statement != nil {
			result = errors.Join(result, statement.Close())
		}
	}
	return result
}
