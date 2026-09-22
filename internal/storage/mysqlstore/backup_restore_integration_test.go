//go:build integration

package mysqlstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "backup-test@example.com"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{
		OperationID: "backup-environment", Actor: actor, Action: EnvironmentCreate,
		Key: "prod", DisplayName: "Production",
	}); err != nil {
		t.Fatalf("seed Environment: %v", err)
	}
	const vaultSecret = "backup-vault-secret-sentinel"
	created, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "backup-vault", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis",
		Snapshot: testVaultSnapshot("", []string{"prod"}, "redis-user", "old-backup-vault-secret-sentinel"),
	})
	if err != nil {
		t.Fatalf("seed Vault: %v", err)
	}
	if _, err := store.CommitVault(ctx, VaultCommit{
		OperationID: "backup-vault-update", Actor: actor, NamespaceKey: "platform", ItemKey: "redis", ItemName: "Redis", ExpectedRevision: 1,
		Snapshot: testVaultSnapshot(created.VariantIDs[0], []string{"prod"}, "redis-user", vaultSecret),
	}); err != nil {
		t.Fatalf("update Vault: %v", err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "backup-config", Actor: actor, EnvironmentKey: "prod",
		ConfigKey: "service", ConfigName: "Service", Format: configdoc.YAML,
		Content: []byte("redis:\n  password: \"{vault.platform.redis.password}\"\n"),
	}); err != nil {
		t.Fatalf("seed Config: %v", err)
	}
	ca, err := store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "backup-ca", Actor: actor, DisplayName: "Backup CA", ValidDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	caFiles := unpackCredential(t, ca.ExportBundle)
	caPrivate, _ := pem.Decode(caFiles["ca.key"])
	if caPrivate == nil {
		t.Fatal("missing exported CA key")
	}
	defer clear(caPrivate.Bytes)
	const notificationURL = "https://hooks.example.test/backup-url-credential-sentinel"
	url, secret := notificationURL, "backup-notification-signing-secret-sentinel"
	if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "backup-notification", Actor: actor,
		Key: "ops", DisplayName: "Ops", Provider: NotificationGenericWebhook, URL: &url, Secret: &secret, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	wantVerification := EncryptionVerification{VaultRevisions: 2, CertificateAuthorities: 1, NotificationDestinations: 1}
	if report, err := store.VerifyEncryptedState(ctx); err != nil || report != wantVerification {
		t.Fatalf("source verification: %#v, %v", report, err)
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
	for _, name := range []string{"mysql.sql.gz", "manifest.sha256"} {
		info, err := os.Stat(filepath.Join(backupDirectory, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("backup artifact %s must remain owner-only: %v", name, err)
		}
	}

	dump, err := readGzipFile(filepath.Join(backupDirectory, "mysql.sql.gz"), 64<<20)
	if err != nil {
		t.Fatalf("read backup dump: %v", err)
	}
	lowerDump := bytes.ToLower(dump)
	for _, forbidden := range [][]byte{
		[]byte(vaultSecret), []byte(notificationURL), []byte(secret), []byte("sentinel-private-file-bytes"),
		caFiles["ca.key"], caPrivate.Bytes,
		masterKey[:], []byte(base64.StdEncoding.EncodeToString(masterKey[:])),
	} {
		// --hex-blob can encode accidentally stored plaintext. Check that form as
		// well as literal bytes so encryption regressions cannot hide in the dump.
		if bytes.Contains(dump, forbidden) || bytes.Contains(lowerDump, []byte(hex.EncodeToString(forbidden))) {
			t.Fatal("MySQL backup contains unencrypted credential material")
		}
	}

	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open cleanup MySQL: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	targetDatabase := "configra_backup_restored_" + suffix
	corruptDatabase := "configra_backup_corrupt_" + suffix
	missingDatabase := "configra_backup_missing_" + suffix
	for _, database := range []string{targetDatabase, corruptDatabase, missingDatabase} {
		assertDatabaseMissing(t, ctx, admin, database)
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
	t.Cleanup(func() { _ = restored.Close() })
	if report, err := restored.VerifyEncryptedState(ctx); err != nil || report != wantVerification {
		t.Fatalf("restored verification: %#v, %v", report, err)
	}
	resolved, err := restored.ReadResolvedConfig(ctx, "prod", "service", "")
	if err != nil || !strings.Contains(resolved.Content, vaultSecret) {
		t.Fatalf("read restored Config: %v", err)
	}
	issued, err := restored.IssueClientCertificate(ctx, ClientCertificateIssue{OperationID: "after-restore-issue", Actor: actor,
		AuthorityID: ca.Authority.ID, DisplayName: "After restore", ValidDays: 1})
	if err != nil || issued.ExportBundle == "" {
		t.Fatalf("issue using restored CA: %v", err)
	}
	files := unpackCredential(t, issued.ExportBundle)
	if _, err := tls.X509KeyPair(files["client.crt"], files["client.key"]); err != nil {
		t.Fatal("client certificate and key do not match after restore")
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

	// Use the real CLI process with SELECT-only credentials. It needs no server
	// TLS files, OIDC secret, NATS or ClickHouse, and must not migrate/init a DB.
	readUser := "doctor_" + suffix
	if _, err := admin.ExecContext(ctx, "CREATE USER '"+readUser+"'@'%' IDENTIFIED BY 'doctor-password-sentinel'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP USER '" + readUser + "'@'%'") })
	if _, err := admin.ExecContext(ctx, "GRANT SELECT ON "+targetDatabase+".* TO '"+readUser+"'@'%'"); err != nil {
		t.Fatal(err)
	}
	readConfig := *targetConfiguration
	readConfig.User, readConfig.Passwd = readUser, "doctor-password-sentinel"
	// Exercise the documented maintenance grants, not a privileged/root CLI.
	rotationUser := "rotator_" + suffix
	if _, err := admin.ExecContext(ctx, "CREATE USER '"+rotationUser+"'@'%' IDENTIFIED BY 'rotator-password-sentinel'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP USER '" + rotationUser + "'@'%'") })
	for table, grants := range map[string]string{
		"schema_migrations": "SELECT", "crypto_sentinel": "SELECT, UPDATE",
		"vault_item_revisions": "SELECT, UPDATE", "certificate_authorities": "SELECT, UPDATE",
		"notification_destinations": "SELECT, UPDATE", "operations": "SELECT, INSERT, UPDATE", "outbox_events": "INSERT",
	} {
		if _, err := admin.ExecContext(ctx, "GRANT "+grants+" ON "+targetDatabase+"."+table+" TO '"+rotationUser+"'@'%'"); err != nil {
			t.Fatal(err)
		}
	}
	rotationConfig := *targetConfiguration
	rotationConfig.User, rotationConfig.Passwd = rotationUser, "rotator-password-sentinel"
	reader, err := sql.Open("mysql", readConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if _, err := reader.ExecContext(ctx, "DELETE FROM crypto_sentinel WHERE id = 255"); err == nil {
		t.Fatal("doctor account has write access")
	}
	binary := filepath.Join(workDirectory, "configra")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/configra")
	build.Dir = filepath.Join("..", "..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build doctor process: %v\n%s", err, output)
	}
	keyPath, configPath := filepath.Join(workDirectory, "master-key"), filepath.Join(workDirectory, "doctor.yaml")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(masterKey[:])), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, fmt.Appendf(nil, "version: 1\nmysql:\n  dsn_env: CONFIGRA_DOCTOR_DSN\nkey_provider:\n  master_key_file: %q\n", keyPath), 0o600); err != nil {
		t.Fatal(err)
	}
	checkNoLeak := func(output string) {
		t.Helper()
		for _, forbidden := range []string{vaultSecret, notificationURL, secret, string(caFiles["ca.key"]), base64.StdEncoding.EncodeToString(caPrivate.Bytes), "doctor-password-sentinel", "rotator-password-sentinel", base64.StdEncoding.EncodeToString(masterKey[:]), base64.StdEncoding.EncodeToString(wrongKey[:])} {
			if strings.Contains(output, forbidden) {
				t.Fatal("maintenance process leaked credential material")
			}
		}
	}
	runDoctor := func(dsn string, succeeds bool, extra ...string) {
		t.Helper()
		args := append([]string{"doctor", "--config", configPath, "--verify-vault"}, extra...)
		command := exec.CommandContext(ctx, binary, args...)
		command.Env = append(os.Environ(), "CONFIGRA_DOCTOR_DSN="+dsn)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		checkNoLeak(stdout.String() + stderr.String())
		if succeeds {
			var report EncryptionVerification
			if err != nil || stderr.Len() != 0 || json.Unmarshal(stdout.Bytes(), &report) != nil || report != wantVerification {
				t.Fatalf("doctor success report: %q, stderr=%q, err=%v", stdout.String(), stderr.String(), err)
			}
		} else if err == nil || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("doctor did not fail without a success report: stdout=%q, stderr=%q, err=%v", stdout.String(), stderr.String(), err)
		}
	}
	runDoctor(readConfig.FormatDSN(), true)
	runDoctor(readConfig.FormatDSN(), false, "--timeout=1ns")
	runDoctor("doctor-password-sentinel", false)
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(wrongKey[:])), 0o600); err != nil {
		t.Fatal(err)
	}
	runDoctor(readConfig.FormatDSN(), false)
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(masterKey[:])), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+corruptDatabase); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "GRANT SELECT ON "+corruptDatabase+".* TO '"+readUser+"'@'%'"); err != nil {
		t.Fatal(err)
	}
	emptyConfig := readConfig
	emptyConfig.DBName = corruptDatabase
	runDoctor(emptyConfig.FormatDSN(), false)
	var tables uint64
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", corruptDatabase).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("doctor initialized an empty recovery target")
	}
	newKeyPath := filepath.Join(workDirectory, "new-master-key")
	if err := os.WriteFile(newKeyPath, []byte(base64.StdEncoding.EncodeToString(wrongKey[:])), 0o600); err != nil {
		t.Fatal(err)
	}
	runRotation := func(dsn string, succeeds bool, extra ...string) {
		t.Helper()
		args := append([]string{"rotate-master-key", "--config", configPath, "--new-key-file", newKeyPath,
			"--confirm-database", targetDatabase, "--confirm-offline"}, extra...)
		command := exec.CommandContext(ctx, binary, args...)
		command.Env = append(os.Environ(), "CONFIGRA_DOCTOR_DSN="+dsn)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		checkNoLeak(stdout.String() + stderr.String())
		if succeeds {
			var report KeyRotationResult
			if err != nil || stderr.Len() != 0 || json.Unmarshal(stdout.Bytes(), &report) != nil || report.EncryptionVerification != wantVerification || report.OperationID == "" {
				t.Fatalf("rotation process failed: stdout=%q, stderr=%q, error=%v", stdout.String(), stderr.String(), err)
			}
		} else if err == nil || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatal("rotation process did not fail closed")
		}
	}
	runRotation(readConfig.FormatDSN(), false) // SELECT-only credentials cannot rotate.
	for _, extra := range [][]string{{"--confirm-database", corruptDatabase}, {"--confirm-offline=false"}, {"--new-key-file", keyPath}, {"--timeout=1ns"}} {
		runRotation(rotationConfig.FormatDSN(), false, extra...)
		runDoctor(readConfig.FormatDSN(), true)
	}
	runRotation(rotationConfig.FormatDSN(), true)
	runDoctor(readConfig.FormatDSN(), false) // Old file still contains the old key.
	for path, want := range map[string]string{keyPath: base64.StdEncoding.EncodeToString(masterKey[:]), newKeyPath: base64.StdEncoding.EncodeToString(wrongKey[:])} {
		if value, err := os.ReadFile(path); err != nil || string(value) != want {
			t.Fatal("rotation modified a mounted key file")
		}
	}
	if err := os.WriteFile(configPath, fmt.Appendf(nil, "version: 1\nmysql:\n  dsn_env: CONFIGRA_DOCTOR_DSN\nkey_provider:\n  master_key_file: %q\n", newKeyPath), 0o600); err != nil {
		t.Fatal(err)
	}
	runDoctor(readConfig.FormatDSN(), true)
	rotated, err := OpenAPI(ctx, targetConfiguration.FormatDSN(), wrongProvider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rotated.Close() })
	if value, err := rotated.ReadResolvedConfig(ctx, "prod", "service", ""); err != nil || value.Content != resolved.Content || value.ETag != resolved.ETag {
		t.Fatal("rotation changed restored Config or ETag", err)
	}
	if value, err := rotated.ReadFile(ctx, "prod", "platform", "redis", "tls_cert", ""); err != nil || string(value.Bytes) != "sentinel-private-file-bytes" {
		t.Fatal("rotation broke restored File", err)
	}
	if issued, err := rotated.IssueClientCertificate(ctx, ClientCertificateIssue{OperationID: "after-rotation-issue", Actor: actor, AuthorityID: ca.Authority.ID, DisplayName: "After rotation", ValidDays: 1}); err != nil || issued.ExportBundle == "" {
		t.Fatal("rotation broke restored CA issuance", err)
	}
	// Corrupt an old revision, leaving the current Config and Sentinel readable.
	if _, err := admin.ExecContext(ctx, "UPDATE "+targetDatabase+".vault_item_revisions SET nonce = REPEAT(0x00, 12) WHERE revision = 1"); err != nil {
		t.Fatal(err)
	}
	runDoctor(readConfig.FormatDSN(), false)
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
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
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
