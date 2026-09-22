package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/alexedwards/scs/v2"
)

// The session manager uses CtxStore so disconnected requests and dependency
// outages cannot leave session queries or shutdown waiting indefinitely.
type managementSessionStore struct {
	db     *sql.DB
	cancel context.CancelFunc
	done   chan struct{}
}

var _ scs.CtxStore = (*managementSessionStore)(nil)

func (store *Store) NewManagementSessionStore(cleanupInterval time.Duration) *managementSessionStore {
	ctx, cancel := context.WithCancel(context.Background())
	sessions := &managementSessionStore{db: store.db, cancel: cancel, done: make(chan struct{})}
	if cleanupInterval <= 0 {
		close(sessions.done)
		return sessions
	}
	go func() {
		defer close(sessions.done)
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sessions.deleteExpired(ctx); err != nil && ctx.Err() == nil {
					log.Print("Management session cleanup failed; will retry")
				}
			}
		}
	}()
	return sessions
}

func (sessions *managementSessionStore) StopCleanup() {
	sessions.cancel()
	<-sessions.done
}

func (sessions *managementSessionStore) Find(token string) ([]byte, bool, error) {
	return sessions.FindCtx(context.Background(), token)
}

func (sessions *managementSessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var data []byte
	err := sessions.db.QueryRowContext(ctx, `SELECT data FROM management_sessions WHERE token = ? AND expiry > UTC_TIMESTAMP(6)`, token).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (sessions *managementSessionStore) Commit(token string, data []byte, expiry time.Time) error {
	return sessions.CommitCtx(context.Background(), token, data, expiry)
}

func (sessions *managementSessionStore) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := sessions.db.ExecContext(ctx, `
		INSERT INTO management_sessions (token, data, expiry) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE data = VALUES(data), expiry = VALUES(expiry)
	`, token, data, expiry.UTC())
	return err
}

func (sessions *managementSessionStore) Delete(token string) error {
	return sessions.DeleteCtx(context.Background(), token)
}

func (sessions *managementSessionStore) DeleteCtx(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := sessions.db.ExecContext(ctx, `DELETE FROM management_sessions WHERE token = ?`, token)
	return err
}

func (sessions *managementSessionStore) deleteExpired(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		result, err := sessions.db.ExecContext(ctx, `DELETE FROM management_sessions WHERE expiry < UTC_TIMESTAMP(6) LIMIT 1000`)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count < 1000 {
			return err
		}
	}
}
