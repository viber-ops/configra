package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

type machineQuery uint8

const (
	tokenAuthenticationQuery machineQuery = iota
	certificateAuthenticationQuery
	resolvedConfigQuery
	fileEnvironmentQuery
	configReferencesQuery
	vaultMetadataQuery
	vaultFieldTypesQuery
	vaultPayloadQuery
	machineQueryCount
)

var machineQueries = [...]string{
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
	`
		SELECT environment.id, config.id, state.current_revision, revision_record.format, revision_record.content
		FROM environments AS environment
		JOIN config_env_states AS state ON state.environment_id = environment.id
		JOIN configs AS config ON config.id = state.config_id
		JOIN config_revisions AS revision_record
		  ON revision_record.config_id = state.config_id
		 AND revision_record.environment_id = state.environment_id
		 AND revision_record.revision = state.current_revision
		WHERE environment.resource_key = ?
		  AND environment.archived_at IS NULL
		  AND config.resource_key = ?
		  AND config.archived_at IS NULL
	`,
	`
		SELECT id FROM environments WHERE resource_key = ? AND archived_at IS NULL
	`,
	`
			SELECT namespace_key, item_key, field_key
		FROM config_revision_vault_refs
		WHERE config_id = ? AND environment_id = ? AND revision = ?
			ORDER BY namespace_key, item_key, field_key
	`,
	`
		SELECT item.id, item.current_revision, binding.variant_id
		FROM vault_items AS item
		JOIN vault_revision_variant_environments AS binding
		  ON binding.item_id = item.id
		 AND binding.revision = item.current_revision
			WHERE item.namespace_key = ?
			  AND item.resource_key = ?
			  AND item.archived_at IS NULL
			  AND binding.environment_id = ?
		`,
	`
		SELECT resource_key, field_type
		FROM vault_revision_fields
		WHERE item_id = ? AND revision = ? AND archived = FALSE
	`,
	`
		SELECT algorithm, key_version, nonce, ciphertext, encrypted_dek
		FROM vault_item_revisions
		WHERE item_id = ? AND revision = ?
	`,
}

// Cache a bounded set of fixed machine-read SQL plans, never authorization or
// resource results. database/sql manages plans on each pooled connection,
// including reconnection. Every request still reads current database state.
type machineStatements struct {
	mu         sync.Mutex
	statements [machineQueryCount]*sql.Stmt
	closed     bool
}

func (store *Store) machineStatement(ctx context.Context, query machineQuery) (*sql.Stmt, error) {
	cache := &store.machineReads
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
	prepared, err := store.db.PrepareContext(ctx, machineQueries[query])
	if err != nil {
		return nil, fmt.Errorf("prepare machine query: %w", err)
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

func (cache *machineStatements) close() error {
	cache.mu.Lock()
	cache.closed = true
	statements := cache.statements
	cache.statements = [machineQueryCount]*sql.Stmt{}
	cache.mu.Unlock()
	var result error
	for _, statement := range statements {
		if statement != nil {
			result = errors.Join(result, statement.Close())
		}
	}
	return result
}

// Both ordinary transactions (including release reads) and prepared machine
// snapshots use the same readers and retain their original SQL and snapshot.
type machineSnapshotReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type machineSnapshot struct {
	*sql.Tx
	statements map[string]*sql.Stmt
}

func (store *Store) beginMachineSnapshot(ctx context.Context, queries ...machineQuery) (*machineSnapshot, error) {
	statements := make(map[string]*sql.Stmt, len(queries))
	for _, query := range queries {
		statement, err := store.machineStatement(ctx, query)
		if err != nil {
			return nil, err
		}
		statements[machineQueries[query]] = statement
	}
	// Prepare DB plans before reserving a transaction connection. Preparing via
	// DB inside the transaction would deadlock when all pool connections are in use.
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	for query, statement := range statements {
		// Tx binds each statement to this snapshot's connection and closes the
		// wrapper on commit/rollback; the reusable DB statement remains open.
		statements[query] = transaction.StmtContext(ctx, statement)
	}
	return &machineSnapshot{Tx: transaction, statements: statements}, nil
}

func (snapshot *machineSnapshot) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if statement := snapshot.statements[query]; statement != nil {
		return statement.QueryContext(ctx, args...)
	}
	return snapshot.Tx.QueryContext(ctx, query, args...)
}

func (snapshot *machineSnapshot) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if statement := snapshot.statements[query]; statement != nil {
		return statement.QueryRowContext(ctx, args...)
	}
	return snapshot.Tx.QueryRowContext(ctx, query, args...)
}
