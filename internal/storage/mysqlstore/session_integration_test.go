//go:build integration

package mysqlstore

import (
	"context"
	"testing"
	"time"

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
}
