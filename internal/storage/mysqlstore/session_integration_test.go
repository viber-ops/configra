//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestManagementSessionStoreUsesSchemaV1OnMySQL8022(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_management_session")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sessions := store.NewManagementSessionStore(0)
	sessions.StopCleanup()

	const token = "0123456789012345678901234567890123456789012"
	want := []byte("encoded-session-data")
	if err := sessions.Commit(token, want, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Commit Session: %v", err)
	}
	got, found, err := sessions.Find(token)
	if err != nil || !found || string(got) != string(want) {
		t.Fatalf("Find Session = %q, %v, %v", got, found, err)
	}
	if err := sessions.Delete(token); err != nil {
		t.Fatalf("Delete Session: %v", err)
	}
	if _, found, err := sessions.Find(token); err != nil || found {
		t.Fatalf("Find deleted Session = found %v, error %v", found, err)
	}
	if err := sessions.Commit(token, want, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("Commit expired Session: %v", err)
	}
	if _, found, err := sessions.Find(token); err != nil || found {
		t.Fatalf("Find expired Session = found %v, error %v", found, err)
	}

	manager := humanauth.NewSessionManager(sessions)
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := manager.Load(cancelled, token); !errors.Is(err, context.Canceled) {
		t.Fatalf("session lookup ignored request cancellation: %v", err)
	}
	if err := sessions.CommitCtx(ctx, token, want, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := sessions.CommitCtx(cancelled, token, []byte("must-not-be-written"), time.Now().Add(time.Hour)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled commit = %v", err)
	}
	if err := sessions.DeleteCtx(cancelled, token); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled delete = %v", err)
	}
	if data, found, err := sessions.FindCtx(ctx, token); err != nil || !found || string(data) != string(want) {
		t.Fatal("cancelled write changed the stored session")
	}

	// A request waiting for a pooled connection must also observe cancellation.
	store.db.SetMaxOpenConns(1)
	connection, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	if _, err := manager.Load(short, token); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("session pool wait = %v", err)
	}
	cleaning := store.NewManagementSessionStore(time.Millisecond)
	defer cleaning.StopCleanup()
	deadline := time.Now().Add(time.Second)
	for store.db.Stats().WaitCount < 2 {
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not start while the connection was occupied")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan struct{})
	go func() { cleaning.StopCleanup(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session cleanup blocked shutdown behind an occupied connection pool")
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sessions.CommitCtx(ctx, token, want, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := sessions.deleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM management_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired session cleanup = %d rows, %v", count, err)
	}
}
