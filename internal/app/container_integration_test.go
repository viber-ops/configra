//go:build integration

package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestProductionAPIImageStartsReadyStopsCleanlyAndRejectsWrongMasterKey(t *testing.T) {
	image := os.Getenv("CONFIGRA_TEST_IMAGE")
	hostDSN := os.Getenv("CONFIGRA_TEST_MYSQL_DSN")
	containerDSN := os.Getenv("CONFIGRA_TEST_CONTAINER_MYSQL_DSN")
	network := os.Getenv("CONFIGRA_TEST_DOCKER_NETWORK")
	if image == "" || hostDSN == "" || containerDSN == "" || network == "" {
		t.Skip("production image test environment is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := sql.Open("mysql", hostDSN)
	if err != nil {
		t.Fatalf("open MySQL version check: %v", err)
	}
	defer database.Close()
	var mysqlVersion string
	if err := database.QueryRowContext(ctx, "SELECT VERSION()").Scan(&mysqlVersion); err != nil || mysqlVersion != "8.0.22" {
		t.Fatalf("MySQL version = %q, %v; want 8.0.22", mysqlVersion, err)
	}

	masterKey := []byte("0123456789abcdef0123456789abcdef")
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := mysqlstore.OpenManagement(ctx, hostDSN, provider)
	if err != nil {
		t.Fatalf("initialize image test database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close image test database: %v", err)
	}

	certificateFile, privateKeyFile, certificate := processTLSFiles(t)
	directory := t.TempDir()
	copyTestFile(t, certificateFile, filepath.Join(directory, "tls.crt"))
	copyTestFile(t, privateKeyFile, filepath.Join(directory, "tls.key"))
	copyTestFile(t, certificateFile, filepath.Join(directory, "client-ca.pem"))
	masterKeyFile := filepath.Join(directory, "master-key")
	if err := os.WriteFile(masterKeyFile, []byte(base64.StdEncoding.EncodeToString(masterKey)), 0o444); err != nil {
		t.Fatalf("write image test Master Key: %v", err)
	}
	config := fmt.Sprintf(`version: 1
listen: :9443
tls:
  certificate_file: /run/configra/tls.crt
  private_key_file: /run/configra/tls.key
  client_ca_file: /run/configra/client-ca.pem
mysql:
  dsn_env: CONFIGRA_CONTAINER_TEST_DSN
key_provider:
  master_key_file: /run/configra/master-key
nats:
  urls: [nats://nats:4222]
logging:
  level: info
`)
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(config), 0o444); err != nil {
		t.Fatalf("write image test Config: %v", err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("chmod image test directory: %v", err)
	}

	containerName := "configra-api-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() {
		cleanup := exec.Command("docker", "rm", "-f", containerName)
		_ = cleanup.Run()
	})
	run := exec.CommandContext(ctx, "docker", "run", "--detach",
		"--name", containerName,
		"--network", network,
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--publish", "127.0.0.1::9443",
		"--mount", "type=bind,src="+directory+",dst=/run/configra,readonly",
		"--env", "CONFIGRA_CONTAINER_TEST_DSN="+containerDSN,
		image, "api", "--config", "/run/configra/config.yaml",
	)
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("start production API image: %v\n%s", err, output)
	}
	portOutput, err := exec.CommandContext(ctx, "docker", "port", containerName, "9443/tcp").Output()
	if err != nil {
		t.Fatalf("read production API image port: %v", err)
	}
	hostPort := strings.TrimSpace(string(portOutput))
	if index := strings.LastIndexByte(hostPort, ':'); index >= 0 {
		hostPort = hostPort[index+1:]
	}
	if _, err := strconv.ParseUint(hostPort, 10, 16); err != nil {
		t.Fatalf("invalid mapped API port %q", hostPort)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, RootCAs: roots,
		}},
	}
	defer client.CloseIdleConnections()
	readyURL := "https://127.0.0.1:" + hostPort + "/health/ready"
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, requestErr := client.Get(readyURL)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				break
			}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", containerName).CombinedOutput()
			t.Fatalf("production API image did not become ready: %v; logs=%s", requestErr, logs)
		}
		time.Sleep(50 * time.Millisecond)
	}
	userOutput, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Config.User}}", containerName).Output()
	if err != nil || strings.TrimSpace(string(userOutput)) != "65532:65532" {
		t.Fatalf("production API image user = %q, %v", userOutput, err)
	}
	if output, err := exec.CommandContext(ctx, "docker", "stop", "--time", "10", containerName).CombinedOutput(); err != nil {
		t.Fatalf("stop production API image: %v\n%s", err, output)
	}
	exitOutput, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.ExitCode}}", containerName).Output()
	if err != nil || strings.TrimSpace(string(exitOutput)) != "0" {
		t.Fatalf("production API image exit = %q, %v", exitOutput, err)
	}
	logs, err := exec.CommandContext(ctx, "docker", "logs", containerName).CombinedOutput()
	if err != nil || strings.Contains(string(logs), string(masterKey)) ||
		strings.Contains(string(logs), base64.StdEncoding.EncodeToString(masterKey)) {
		t.Fatalf("production API logs leaked Master Key or could not be read: %v", err)
	}

	wrongKey := []byte("abcdef0123456789abcdef0123456789")
	if err := os.Chmod(masterKeyFile, 0o600); err != nil {
		t.Fatalf("make image test Master Key writable: %v", err)
	}
	if err := os.WriteFile(masterKeyFile, []byte(base64.StdEncoding.EncodeToString(wrongKey)), 0o444); err != nil {
		t.Fatalf("write wrong image test Master Key: %v", err)
	}
	if err := os.Chmod(masterKeyFile, 0o444); err != nil {
		t.Fatalf("make image test Master Key read-only: %v", err)
	}
	wrong := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--network", network,
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--mount", "type=bind,src="+directory+",dst=/run/configra,readonly",
		"--env", "CONFIGRA_CONTAINER_TEST_DSN="+containerDSN,
		image, "api", "--config", "/run/configra/config.yaml",
	)
	wrongOutput, wrongErr := wrong.CombinedOutput()
	if wrongErr == nil || strings.Contains(string(wrongOutput), string(wrongKey)) ||
		strings.Contains(string(wrongOutput), base64.StdEncoding.EncodeToString(wrongKey)) {
		t.Fatalf("wrong Master Key image result = %v; leaked=%v", wrongErr,
			strings.Contains(string(wrongOutput), string(wrongKey)))
	}
}

func copyTestFile(t *testing.T, source, target string) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}
	if err := os.WriteFile(target, content, 0o444); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
