package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

const (
	schemaVersion     = 2
	migrationLockName = "configra:schema-migration"
)

type Store struct {
	db       *sql.DB
	provider *vaultcrypto.LocalKeyProvider
}

func OpenManagement(ctx context.Context, dsn string, provider *vaultcrypto.LocalKeyProvider) (*Store, error) {
	return open(ctx, dsn, provider, true)
}

func OpenAPI(ctx context.Context, dsn string, provider *vaultcrypto.LocalKeyProvider) (*Store, error) {
	return open(ctx, dsn, provider, false)
}

func open(ctx context.Context, dsn string, provider *vaultcrypto.LocalKeyProvider, management bool) (*Store, error) {
	if dsn == "" {
		return nil, errors.New("MySQL DSN is required")
	}
	if provider == nil {
		return nil, errors.New("LocalKeyProvider is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open MySQL: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	var startupErr error
	if management {
		startupErr = initializeOrVerify(ctx, db, provider)
	} else {
		startupErr = verifyInitialized(ctx, db, provider)
	}
	if startupErr != nil {
		_ = db.Close()
		return nil, startupErr
	}
	return &Store{db: db, provider: provider}, nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) Ping(ctx context.Context) error {
	// A replica with a stale mounted key must not remain ready after rotation.
	return verifySentinel(ctx, store.db, store.provider)
}

func (store *Store) SchemaVersion(ctx context.Context) (uint64, error) {
	var version uint64
	if err := store.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func initializeOrVerify(ctx context.Context, db *sql.DB, provider *vaultcrypto.LocalKeyProvider) error {
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer connection.Close()

	var locked int
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", migrationLockName).Scan(&locked); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if locked != 1 {
		return errors.New("timed out acquiring migration lock")
	}
	defer releaseMigrationLock(connection)

	if _, err := connection.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("create schema migration table: %w", err)
	}
	var version uint64
	if err := connection.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == 0 {
		if err := initializeSchema(ctx, connection, provider); err != nil {
			return err
		}
	} else if err := verifySentinel(ctx, connection, provider); err != nil {
		return err
	}
	if version == 1 {
		if err := applySchemaV2(ctx, connection); err != nil {
			return err
		}
		if _, err := connection.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (2)"); err != nil {
			return fmt.Errorf("record schema v2: %w", err)
		}
	}
	return nil
}

func verifyInitialized(ctx context.Context, db *sql.DB, provider *vaultcrypto.LocalKeyProvider) error {
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve verification connection: %w", err)
	}
	defer connection.Close()
	return verifyDatabase(ctx, connection, provider)
}

// Both startup connections and the recovery check's consistent snapshot use
// the same schema and Sentinel verification.
type verificationReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verifyDatabase(ctx context.Context, connection verificationReader, provider *vaultcrypto.LocalKeyProvider) error {
	var version uint64
	if err := connection.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version != schemaVersion {
		return fmt.Errorf("database schema version %d does not match supported version %d", version, schemaVersion)
	}
	return verifySentinel(ctx, connection, provider)
}

func initializeSchema(ctx context.Context, connection *sql.Conn, provider *vaultcrypto.LocalKeyProvider) error {
	if _, err := connection.ExecContext(ctx, cryptoSentinelDDL); err != nil {
		return fmt.Errorf("create crypto sentinel table: %w", err)
	}
	for index, statement := range schemaV1DDL {
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize schema v1 statement %d: %w", index+1, err)
		}
	}
	if err := applySchemaV2(ctx, connection); err != nil {
		return err
	}
	sentinel, err := provider.CreateSentinel()
	if err != nil {
		return fmt.Errorf("create crypto sentinel: %w", err)
	}
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema initialization: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO crypto_sentinel (id, algorithm, key_version, nonce, ciphertext)
		VALUES (1, ?, ?, ?, ?)
	`, sentinel.Algorithm, sentinel.KeyVersion, sentinel.Nonce, sentinel.Ciphertext); err != nil {
		return fmt.Errorf("insert crypto sentinel: %w", err)
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT INTO schema_migrations (version) VALUES (?)", schemaVersion,
	); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit schema initialization: %w", err)
	}
	return nil
}

func verifySentinel(ctx context.Context, connection verificationReader, provider *vaultcrypto.LocalKeyProvider) error {
	sentinel, err := readSentinel(ctx, connection, "")
	if err != nil {
		return err
	}
	return provider.VerifySentinel(sentinel)
}

// Encrypted writes take this lock before any resource/Operation locks. Rotation
// takes the exclusive form, so a stale writer cannot commit old-key data after it.
func (store *Store) lockMasterKey(ctx context.Context, transaction *sql.Tx, exclusive bool) error {
	lock := " FOR SHARE"
	if exclusive {
		lock = " FOR UPDATE"
	}
	sentinel, err := readSentinel(ctx, transaction, lock)
	if err != nil {
		return err
	}
	return store.provider.VerifySentinel(sentinel)
}

func readSentinel(ctx context.Context, connection verificationReader, lock string) (vaultcrypto.EncryptedSentinel, error) {
	var sentinel vaultcrypto.EncryptedSentinel
	err := connection.QueryRowContext(ctx, `
		SELECT algorithm, key_version, nonce, ciphertext
		FROM crypto_sentinel
		WHERE id = 1
	`+lock).Scan(&sentinel.Algorithm, &sentinel.KeyVersion, &sentinel.Nonce, &sentinel.Ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return sentinel, fmt.Errorf("crypto sentinel is missing: %w", vaultcrypto.ErrIntegrity)
	}
	if err != nil {
		return sentinel, fmt.Errorf("read crypto sentinel: %w", err)
	}
	return sentinel, nil
}

func releaseMigrationLock(connection *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released sql.NullInt64
	_ = connection.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", migrationLockName).Scan(&released)
}

const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB
`

const cryptoSentinelDDL = `
CREATE TABLE IF NOT EXISTS crypto_sentinel (
    id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
    algorithm VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    key_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    nonce VARBINARY(32) NOT NULL,
    ciphertext VARBINARY(512) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT crypto_sentinel_singleton CHECK (id = 1)
) ENGINE=InnoDB
`
