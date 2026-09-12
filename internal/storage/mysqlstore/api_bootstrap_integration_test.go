//go:build integration

package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestAPIServerVerifiesButNeverInitializesSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_api_bootstrap")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}

	apiStore, err := OpenAPI(ctx, dsn, provider)
	if apiStore != nil {
		_ = apiStore.Close()
		t.Fatal("OpenAPI initialized an empty database")
	}
	if err == nil {
		t.Fatal("OpenAPI accepted an empty database")
	}
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	var migrationTables int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'schema_migrations'
	`).Scan(&migrationTables); err != nil {
		t.Fatalf("inspect empty schema: %v", err)
	}
	if migrationTables != 0 {
		t.Fatal("OpenAPI created schema_migrations")
	}

	managementStore, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	if err := managementStore.Close(); err != nil {
		t.Fatalf("close Management Store: %v", err)
	}
	apiStore, err = OpenAPI(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenAPI after Management initialization: %v", err)
	}
	if err := apiStore.Close(); err != nil {
		t.Fatalf("close API Store: %v", err)
	}

	wrongProvider, err := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatalf("wrong provider: %v", err)
	}
	apiStore, err = OpenAPI(ctx, dsn, wrongProvider)
	if apiStore != nil {
		_ = apiStore.Close()
		t.Fatal("OpenAPI accepted the wrong Master Key")
	}
	if !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatalf("OpenAPI with wrong Master Key = %v, want ErrIntegrity", err)
	}
}
