//go:build integration

package mysqlstore

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestMySQLBackupRestoresEncryptedStateAndFailsClosed(t *testing.T) {
	network := os.Getenv("CONFIGRA_TEST_DOCKER_NETWORK")
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if network == "" || rootDSN == "" {
		t.Skip("Docker network and MySQL Root DSN are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sourceDatabase := "configra_backup_source_" + suffix
	sourceDSN := createIntegrationDatabase(t, ctx, sourceDatabase)
	masterKey := sha256.Sum256([]byte("backup-master-key-sentinel"))
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey[:])
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, sourceDSN, provider)
	if err != nil {
		t.Fatalf("OpenManagement source: %v", err)
	}
	actor := Actor{Type: "user", ID: "backup-test@example.com"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
		OperationID: "backup-environment", Actor: actor, Action: EnvironmentCreate,
		Key: "prod", DisplayName: "Production",
	}); err != nil {
		t.Fatalf("seed Environment: %v", err)
	}
	const vaultSecret = "backup-vault-secret-sentinel"
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "backup-vault", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis",
		Snapshot: testVaultSnapshot("", []string{"prod"}, "redis-user", vaultSecret),
	}); err != nil {
		t.Fatalf("seed Vault: %v", err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "backup-config", Actor: actor, EnvironmentKey: "prod",
		ConfigKey: "service", ConfigName: "Service", Format: configdoc.YAML,
		Content: []byte("redis:\n  password: \"{vault.platform.redis.password}\"\n"),
	}); err != nil {
		t.Fatalf("seed Config: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close source Store: %v", err)
	}

	workDirectory := t.TempDir()
	if err := os.Chmod(workDirectory, 0o755); err != nil {
		t.Fatalf("chmod backup work directory: %v", err)
	}
	clientConfig := filepath.Join(workDirectory, "client.cnf")
	if err := os.WriteFile(clientConfig, []byte("[client]\nhost=mysql\nport=3306\nprotocol=tcp\nuser=root\npassword=configra-test-root\n"), 0o600); err != nil {
		t.Fatalf("write MySQL client config: %v", err)
	}
	backupDirectory := filepath.Join(workDirectory, "backup")
	if err := os.Mkdir(backupDirectory, 0o755); err != nil {
		t.Fatalf("create backup directory: %v", err)
	}
	toolsDirectory, err := filepath.Abs(filepath.Join("..", "..", "..", "deploy", "backup"))
	if err != nil {
		t.Fatalf("resolve backup tools: %v", err)
	}
	if output, err := runMySQLBackupTool(ctx, network, toolsDirectory, workDirectory,
		"mysql-backup.sh", "/work/client.cnf", sourceDatabase, "/work/backup"); err != nil {
		t.Fatalf("backup MySQL: %v\n%s", err, output)
	}

	dump, err := readGzipFile(filepath.Join(backupDirectory, "mysql.sql.gz"), 64<<20)
	if err != nil {
		t.Fatalf("read backup dump: %v", err)
	}
	for _, forbidden := range [][]byte{
		[]byte(vaultSecret), masterKey[:], []byte(base64.StdEncoding.EncodeToString(masterKey[:])),
	} {
		if strings.Contains(string(dump), string(forbidden)) {
			t.Fatal("MySQL backup contains plaintext Vault or Master Key material")
		}
	}

	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open cleanup MySQL: %v", err)
	}
	defer admin.Close()
	targetDatabase := "configra_backup_restored_" + suffix
	corruptDatabase := "configra_backup_corrupt_" + suffix
	missingDatabase := "configra_backup_missing_" + suffix
	for _, database := range []string{targetDatabase, corruptDatabase, missingDatabase} {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+database); err != nil {
			t.Fatalf("drop stale restore database: %v", err)
		}
		database := database
		t.Cleanup(func() {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = admin.ExecContext(cleanupContext, "DROP DATABASE IF EXISTS "+database)
		})
	}

	corruptDirectory := filepath.Join(workDirectory, "corrupt")
	copyBackupDirectory(t, backupDirectory, corruptDirectory)
	corruptDump := filepath.Join(corruptDirectory, "mysql.sql.gz")
	encoded, err := os.ReadFile(corruptDump)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("read corruptible dump: %v", err)
	}
	encoded[len(encoded)/2] ^= 0xff
	if err := os.WriteFile(corruptDump, encoded, 0o600); err != nil {
		t.Fatalf("corrupt dump: %v", err)
	}
	if _, err := runMySQLBackupTool(ctx, network, toolsDirectory, workDirectory,
		"mysql-restore.sh", "/work/client.cnf", "/work/corrupt", corruptDatabase); err == nil {
		t.Fatal("restore accepted a checksum-invalid backup")
	}
	assertDatabaseMissing(t, ctx, admin, corruptDatabase)

	missingDirectory := filepath.Join(workDirectory, "missing")
	if err := os.Mkdir(missingDirectory, 0o755); err != nil {
		t.Fatalf("create incomplete backup directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(missingDirectory, "mysql.sql.gz"), encoded, 0o600); err != nil {
		t.Fatalf("write incomplete backup: %v", err)
	}
	if _, err := runMySQLBackupTool(ctx, network, toolsDirectory, workDirectory,
		"mysql-restore.sh", "/work/client.cnf", "/work/missing", missingDatabase); err == nil {
		t.Fatal("restore accepted a backup without a manifest")
	}
	assertDatabaseMissing(t, ctx, admin, missingDatabase)

	if output, err := runMySQLBackupTool(ctx, network, toolsDirectory, workDirectory,
		"mysql-restore.sh", "/work/client.cnf", "/work/backup", targetDatabase); err != nil {
		t.Fatalf("restore MySQL: %v\n%s", err, output)
	}
	targetConfiguration, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatalf("parse target DSN: %v", err)
	}
	targetConfiguration.DBName = targetDatabase
	restored, err := OpenAPI(ctx, targetConfiguration.FormatDSN(), provider)
	if err != nil {
		t.Fatalf("OpenAPI restored: %v", err)
	}
	resolved, err := restored.ReadResolvedConfig(ctx, "prod", "service", "")
	if err != nil || !strings.Contains(resolved.Content, vaultSecret) {
		t.Fatalf("read restored Config: %v", err)
	}
	if err := restored.Close(); err != nil {
		t.Fatalf("close restored Store: %v", err)
	}

	wrongKey := sha256.Sum256([]byte("wrong-backup-master-key"))
	wrongProvider, err := vaultcrypto.NewLocalKeyProvider(wrongKey[:])
	if err != nil {
		t.Fatalf("NewLocalKeyProvider wrong key: %v", err)
	}
	if wrongStore, err := OpenAPI(ctx, targetConfiguration.FormatDSN(), wrongProvider); err == nil {
		_ = wrongStore.Close()
		t.Fatal("restored database accepted the wrong Master Key")
	} else if !errors.Is(err, vaultcrypto.ErrIntegrity) || strings.Contains(err.Error(), vaultSecret) ||
		strings.Contains(err.Error(), base64.StdEncoding.EncodeToString(wrongKey[:])) {
		t.Fatalf("wrong Master Key error was unsafe or unexpected: %v", err)
	}

	if _, err := runMySQLBackupTool(ctx, network, toolsDirectory, workDirectory,
		"mysql-restore.sh", "/work/client.cnf", "/work/backup", targetDatabase); err == nil {
		t.Fatal("restore overwrote an existing database")
	}
	reopened, err := OpenAPI(ctx, targetConfiguration.FormatDSN(), provider)
	if err != nil {
		t.Fatalf("existing restored database changed after rejected overwrite: %v", err)
	}
	_ = reopened.Close()
}

func runMySQLBackupTool(
	ctx context.Context,
	network string,
	toolsDirectory string,
	workDirectory string,
	script string,
	arguments ...string,
) ([]byte, error) {
	commandArguments := []string{
		"run", "--rm", "--platform", "linux/amd64", "--network", network,
		"--mount", "type=bind,src=" + toolsDirectory + ",dst=/tools,readonly",
		"--mount", "type=bind,src=" + workDirectory + ",dst=/work",
		"--entrypoint", "/bin/sh", "mysql:8.0.22", "/tools/" + script,
	}
	commandArguments = append(commandArguments, arguments...)
	return exec.CommandContext(ctx, "docker", commandArguments...).CombinedOutput()
}

func readGzipFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, limit+1))
}

func copyBackupDirectory(t *testing.T, source, target string) {
	t.Helper()
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create copied backup directory: %v", err)
	}
	for _, name := range []string{"mysql.sql.gz", "manifest.sha256"} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("read backup artifact %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o600); err != nil {
			t.Fatalf("copy backup artifact %s: %v", name, err)
		}
	}
}

func assertDatabaseMissing(t *testing.T, ctx context.Context, database *sql.DB, name string) {
	t.Helper()
	var count uint64
	if err := database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", name).Scan(&count); err != nil {
		t.Fatalf("check restore database: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed restore left database %q behind", name)
	}
}
