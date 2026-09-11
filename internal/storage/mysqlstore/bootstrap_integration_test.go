//go:build integration

package mysqlstore_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestManagementBootstrapInitializesOnceAndRejectsWrongMasterKey(t *testing.T) {
	dsn := os.Getenv("CONFIGRA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}

	first, err := mysqlstore.OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("first OpenManagement: %v", err)
	}
	version, err := first.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", version)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	second, err := mysqlstore.OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("second OpenManagement: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second store: %v", err)
	}

	wrongProvider, err := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatalf("wrong provider: %v", err)
	}
	wrongStore, err := mysqlstore.OpenManagement(ctx, dsn, wrongProvider)
	if wrongStore != nil {
		_ = wrongStore.Close()
		t.Fatal("OpenManagement returned a store for the wrong Master Key")
	}
	if !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatalf("OpenManagement with wrong Master Key = %v, want ErrIntegrity", err)
	}
}

func TestConcurrentBootstrapInitializesOnceAndMissingSentinelFailsClosed(t *testing.T) {
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open root MySQL: %v", err)
	}
	defer admin.Close()
	const database = "configra_concurrent_bootstrap"
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+database); err != nil {
		t.Fatalf("drop stale test database: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+database+" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS "+database)
	})
	configuration, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse root DSN: %v", err)
	}
	configuration.DBName = database
	dsn := configuration.FormatDSN()
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}

	const starters = 8
	errorsByStarter := make(chan error, starters)
	var wait sync.WaitGroup
	for range starters {
		wait.Add(1)
		go func() {
			defer wait.Done()
			store, openErr := mysqlstore.OpenManagement(ctx, dsn, provider)
			if openErr == nil {
				openErr = store.Close()
			}
			errorsByStarter <- openErr
		}()
	}
	wait.Wait()
	close(errorsByStarter)
	for openErr := range errorsByStarter {
		if openErr != nil {
			t.Fatalf("concurrent OpenManagement: %v", openErr)
		}
	}

	databaseConnection, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open initialized database: %v", err)
	}
	if _, err := databaseConnection.ExecContext(ctx, "DELETE FROM crypto_sentinel WHERE id = 1"); err != nil {
		_ = databaseConnection.Close()
		t.Fatalf("remove sentinel for failure drill: %v", err)
	}
	_ = databaseConnection.Close()
	store, err := mysqlstore.OpenManagement(ctx, dsn, provider)
	if store != nil {
		_ = store.Close()
		t.Fatal("OpenManagement recreated a missing sentinel")
	}
	if !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatalf("OpenManagement with missing sentinel = %v, want ErrIntegrity", err)
	}
}
