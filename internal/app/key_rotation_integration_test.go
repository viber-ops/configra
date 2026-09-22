//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestRunningAPIBecomesUnreadyAfterKeyRotationAndRecoversOnRestart(t *testing.T) {
	root := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if root == "" {
		t.Skip("CONFIGRA_TEST_MYSQL_ROOT_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := mysql.ParseDSN(root)
	if err != nil {
		t.Fatal("invalid fixture DSN")
	}
	admin, err := sql.Open("mysql", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var version string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil || version != "8.0.22" {
		t.Fatal("requires MySQL 8.0.22", err)
	}
	config.DBName = "configra_rotation_api_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+config.DBName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = admin.ExecContext(cleanup, "DROP DATABASE "+config.DBName)
	})
	key, replacement := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	old, err := vaultcrypto.NewLocalKeyProvider(key)
	if err != nil {
		t.Fatal(err)
	}
	next, err := vaultcrypto.NewLocalKeyProvider(replacement)
	if err != nil {
		t.Fatal(err)
	}
	store, err := mysqlstore.OpenManagement(ctx, config.FormatDSN(), old)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("CONFIGRA_ROTATION_API_DSN", config.FormatDSN())
	certFile, privateFile, certificate := processTLSFiles(t)
	directory := t.TempDir()
	keyFile, yamlFile := filepath.Join(directory, "key"), filepath.Join(directory, "api.yaml")
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	yaml := fmt.Sprintf("version: 1\nlisten: %s\ntls:\n  certificate_file: %q\n  private_key_file: %q\nmysql:\n  dsn_env: CONFIGRA_ROTATION_API_DSN\nkey_provider:\n  master_key_file: %q\nnats:\n  urls: [nats://127.0.0.1:1]\nlogging:\n  level: error\n", address, certFile, privateFile, keyFile)
	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	defer client.CloseIdleConnections()
	start := func() (context.CancelFunc, chan error) {
		serverContext, stop := context.WithCancel(ctx)
		t.Cleanup(stop)
		done := make(chan error, 1)
		go func() { done <- run(serverContext, bootstrap.API, yamlFile) }()
		return stop, done
	}
	waitFor := func(path string, code int, done chan error) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err := client.Get("https://" + address + path)
			if err == nil {
				body, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				if readErr != nil || bytes.Contains(body, []byte("cryptographic")) || bytes.Contains(body, []byte(base64.StdEncoding.EncodeToString(key))) {
					t.Fatal("health response leaked key/dependency details")
				}
				if response.StatusCode == code {
					return
				}
			}
			select {
			case err := <-done:
				t.Fatal("API stopped unexpectedly", err)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not reach %d", path, code)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	stop, done := start()
	waitFor("/health/ready", http.StatusNoContent, done)
	// Deliberately leave this replica up to exercise the forgotten-pod guard.
	if _, err := store.RotateMasterKey(ctx, next); err != nil {
		t.Fatal(err)
	}
	waitFor("/health/ready", http.StatusServiceUnavailable, done)
	waitFor("/health/live", http.StatusNoContent, done)
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(replacement)), 0o600); err != nil {
		t.Fatal(err)
	}
	stop, done = start()
	waitFor("/health/ready", http.StatusNoContent, done)
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
