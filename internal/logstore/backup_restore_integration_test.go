//go:build integration

package logstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

func TestClickHouseNativeBackupRestoresLogsAndRejectsInvalidArtifacts(t *testing.T) {
	dsn := os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN")
	network := os.Getenv("CONFIGRA_TEST_DOCKER_NETWORK")
	if dsn == "" || network == "" {
		t.Skip("ClickHouse DSN and Docker network are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	var version string
	if err := store.connection.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil || version != "26.7.3.19" {
		t.Fatalf("ClickHouse version = %q, %v; want 26.7.3.19", version, err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	principal := "backup-" + suffix
	eventID := fmt.Sprintf("%032x", time.Now().UnixNano())
	now := time.Now().UTC()
	if err := store.AppendAccess(ctx, []machine.AccessEvent{{
		Time: now, Principal: principal, Authentication: machine.AuthenticationTokenOnly,
		Environment: "prod", ResourceType: "config", Resource: "service", ConfigRevision: 7,
	}}); err != nil {
		t.Fatalf("seed Access Event: %v", err)
	}
	if err := store.AppendAudits(ctx, []AuditRecord{{
		ID: eventID, Time: now, EventType: "config.commit", OperationID: principal,
		ActorType: "user", ActorID: "backup@example.com", Action: "commit",
		Outcome: "success", Environment: "prod", ResourceType: "config", Resource: "service", Revision: 7, Attempt: 1,
	}}); err != nil {
		t.Fatalf("seed Audit Event: %v", err)
	}

	backupName := "configra-" + suffix + ".zip"
	restoredDatabase := "configra_log_restored_" + suffix
	missingDatabase := "configra_log_missing_" + suffix
	corruptDatabase := "configra_log_corrupt_" + suffix
	workDirectory := t.TempDir()
	if err := os.Chmod(workDirectory, 0o755); err != nil {
		t.Fatalf("chmod ClickHouse backup work directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDirectory, "client.xml"), []byte(`<config>
    <host>clickhouse</host>
    <port>9000</port>
    <user>configra</user>
    <password>configra-test</password>
</config>
`), 0o600); err != nil {
		t.Fatalf("write ClickHouse client config: %v", err)
	}
	toolsDirectory, err := filepath.Abs(filepath.Join("..", "..", "deploy", "backup"))
	if err != nil {
		t.Fatalf("resolve backup tools: %v", err)
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, database := range []string{restoredDatabase, missingDatabase, corruptDatabase} {
			_ = store.connection.Exec(cleanupContext, "DROP DATABASE IF EXISTS "+database+" SYNC")
		}
	}()

	if output, err := runClickHouseBackupTool(ctx, network, toolsDirectory, workDirectory,
		"clickhouse-backup.sh", "/work/client.xml", "configra", "backups", backupName); err != nil {
		t.Fatalf("BACKUP DATABASE: %v\n%s", err, output)
	}
	if output, err := runClickHouseBackupTool(ctx, network, toolsDirectory, workDirectory,
		"clickhouse-restore.sh", "/work/client.xml", "configra", "backups", backupName, restoredDatabase); err != nil {
		t.Fatalf("RESTORE DATABASE: %v\n%s", err, output)
	}
	var accessCount, auditCount uint64
	if err := store.connection.QueryRow(ctx,
		"SELECT count() FROM "+restoredDatabase+".access_events WHERE principal = ?", principal).Scan(&accessCount); err != nil {
		t.Fatalf("read restored Access Events: %v", err)
	}
	if err := store.connection.QueryRow(ctx,
		"SELECT count() FROM "+restoredDatabase+".audit_events WHERE operation_id = ?", principal).Scan(&auditCount); err != nil {
		t.Fatalf("read restored Audit Events: %v", err)
	}
	if accessCount != 1 || auditCount != 1 {
		t.Fatalf("restored Access/Audit count = %d/%d, want 1/1", accessCount, auditCount)
	}
	if _, err := runClickHouseBackupTool(ctx, network, toolsDirectory, workDirectory,
		"clickhouse-restore.sh", "/work/client.xml", "configra", "backups", backupName, restoredDatabase); err == nil {
		t.Fatal("ClickHouse restore overwrote an existing database")
	}

	missingName := "missing-" + suffix + ".zip"
	if _, err := runClickHouseBackupTool(ctx, network, toolsDirectory, workDirectory,
		"clickhouse-restore.sh", "/work/client.xml", "configra", "backups", missingName, missingDatabase); err == nil {
		t.Fatal("ClickHouse restored a missing backup")
	}
	assertClickHouseDatabaseMissing(t, ctx, store, missingDatabase)

	composeFile, err := filepath.Abs(filepath.Join("..", "..", "deploy", "compose.test.yaml"))
	if err != nil {
		t.Fatalf("resolve Compose file: %v", err)
	}
	containerOutput, err := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile,
		"ps", "-q", "clickhouse").Output()
	if err != nil || strings.TrimSpace(string(containerOutput)) == "" {
		t.Fatalf("resolve ClickHouse container: %v", err)
	}
	container := strings.TrimSpace(string(containerOutput))
	encoded, err := exec.CommandContext(ctx, "docker", "exec", container,
		"cat", "/backups/"+backupName).Output()
	if err != nil || len(encoded) < 2 {
		t.Fatalf("read ClickHouse backup: %v", err)
	}
	encoded = encoded[:len(encoded)/2]
	corruptName := "corrupt-" + backupName
	defer func() {
		_, _ = exec.Command("docker", "exec", container, "rm", "-f",
			"/backups/"+backupName, "/backups/"+corruptName).CombinedOutput()
	}()
	installCorrupt := exec.CommandContext(ctx, "docker", "exec", "-i", container,
		"tee", "/backups/"+corruptName)
	installCorrupt.Stdin = bytes.NewReader(encoded)
	installCorrupt.Stdout = io.Discard
	var installError bytes.Buffer
	installCorrupt.Stderr = &installError
	if err := installCorrupt.Run(); err != nil {
		t.Fatalf("install corrupt ClickHouse backup: %v\n%s", err, installError.Bytes())
	}
	if _, err := runClickHouseBackupTool(ctx, network, toolsDirectory, workDirectory,
		"clickhouse-restore.sh", "/work/client.xml", "configra", "backups", corruptName, corruptDatabase); err == nil {
		t.Fatal("ClickHouse restored a corrupt backup")
	}
	assertClickHouseDatabaseMissing(t, ctx, store, corruptDatabase)
}

func runClickHouseBackupTool(
	ctx context.Context,
	network string,
	toolsDirectory string,
	workDirectory string,
	script string,
	arguments ...string,
) ([]byte, error) {
	commandArguments := []string{
		"run", "--rm", "--network", network,
		"--mount", "type=bind,src=" + toolsDirectory + ",dst=/tools,readonly",
		"--mount", "type=bind,src=" + workDirectory + ",dst=/work,readonly",
		"--entrypoint", "/bin/sh", "clickhouse/clickhouse-server:26.7.3.19", "/tools/" + script,
	}
	commandArguments = append(commandArguments, arguments...)
	return exec.CommandContext(ctx, "docker", commandArguments...).CombinedOutput()
}

func assertClickHouseDatabaseMissing(t *testing.T, ctx context.Context, store *Store, database string) {
	t.Helper()
	var count uint64
	if err := store.connection.QueryRow(ctx,
		"SELECT count() FROM system.databases WHERE name = ?", database).Scan(&count); err != nil {
		t.Fatalf("check ClickHouse restore database: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed ClickHouse restore left database %q behind", database)
	}
}
