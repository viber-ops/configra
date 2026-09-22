//go:build integration && load

package e2e_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-sql-driver/mysql"
	"github.com/nats-io/nats.go"
	configrago "github.com/viber-ops/configra-go"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

const loadRate = 1000

func TestProductionImageSustainsConfiguredLoadWithoutLeak(t *testing.T) {
	durationText := os.Getenv("CONFIGRA_LOAD_DURATION")
	if durationText == "" {
		t.Skip("CONFIGRA_LOAD_DURATION is not set")
	}
	duration, err := time.ParseDuration(durationText)
	if err != nil || duration < 10*time.Second {
		t.Fatalf("CONFIGRA_LOAD_DURATION = %q; want at least 10s", durationText)
	}
	required := map[string]string{
		"image":               os.Getenv("CONFIGRA_TEST_IMAGE"),
		"mysql_root_dsn":      os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN"),
		"mysql_dsn":           os.Getenv("CONFIGRA_TEST_MYSQL_DSN"),
		"container_mysql_dsn": os.Getenv("CONFIGRA_TEST_CONTAINER_MYSQL_DSN"),
		"nats_url":            os.Getenv("CONFIGRA_TEST_NATS_URL"),
		"clickhouse_dsn":      os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN"),
		"docker_network":      os.Getenv("CONFIGRA_TEST_DOCKER_NETWORK"),
		"vegeta":              os.Getenv("CONFIGRA_TEST_VEGETA"),
	}
	for name, value := range required {
		if value == "" {
			t.Fatalf("load-test setting %s is required", name)
		}
	}
	if _, err := os.Stat(required["vegeta"]); err != nil {
		t.Fatalf("Vegeta is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration+3*time.Minute)
	defer cancel()
	startedAt := time.Now().UTC()
	suffix := strconv.FormatInt(startedAt.UnixNano(), 10)
	masterKey := sha256.Sum256([]byte("configra-load-master-key-" + suffix))
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey[:])
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	database := newLoadDatabase(t, ctx, required["mysql_root_dsn"], required["mysql_dsn"],
		required["container_mysql_dsn"], provider, suffix)
	tlsMaterial := createLoadTLS(t)
	actor := mysqlstore.Actor{Type: "user", ID: "load-test-admin"}
	if _, err := database.store.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{
		OperationID: "load-environment-" + suffix, Actor: actor, Action: mysqlstore.EnvironmentCreate,
		Key: "load", DisplayName: "Load Test",
	}); err != nil {
		t.Fatalf("create load Environment: %v", err)
	}
	vaultSecret := "load-vault-secret-" + suffix
	fileSecret := "load-file-secret-" + suffix
	if _, err := database.store.CommitVault(ctx, mysqlstore.VaultCommit{
		OperationID: "load-vault-" + suffix, Actor: actor, NamespaceKey: "platform", ItemKey: "database", ItemName: "Database",
		Snapshot: loadVaultSnapshot(vaultSecret, fileSecret),
	}); err != nil {
		t.Fatalf("create load Vault: %v", err)
	}
	content := "database:\n  password: '{vault.platform.database.password}'\n" +
		"representative_payload: '" + strings.Repeat("x", 4<<10) + "'\n"
	if _, err := database.store.CommitConfig(ctx, mysqlstore.ConfigCommit{
		OperationID: "load-config-" + suffix, Actor: actor, EnvironmentKey: "load",
		ConfigKey: "service", ConfigName: "Service", Format: configdoc.YAML, Content: []byte(content),
	}); err != nil {
		t.Fatalf("create load Config: %v", err)
	}
	token, err := database.store.CreateToken(ctx, mysqlstore.TokenCreate{
		OperationID: "load-token-" + suffix, Actor: actor, DisplayName: "Load Client",
		EnvironmentKeys: []string{"load"}, NeverExpires: true,
	})
	if err != nil || token.Token == "" {
		t.Fatalf("create load API Token: %v", err)
	}
	if _, err := database.store.RegisterVerifiedClientCertificate(ctx, mysqlstore.ClientCertificateRegister{
		OperationID: "load-certificate-" + suffix, Actor: actor, DisplayName: "Load Client",
		Certificate: tlsMaterial.clientCertificate,
	}); err != nil {
		t.Fatalf("register load Client Certificate: %v", err)
	}
	if err := database.store.Close(); err != nil {
		t.Fatalf("close load Management Store: %v", err)
	}
	privateValues := append([][]byte{[]byte(token.Token), []byte(vaultSecret), []byte(fileSecret), masterKey[:]}, tlsMaterial.privateKeyMaterial...)
	sentinels := loadSecretForms(privateValues...)
	finishCapture := captureLoadAccess(t, required["nats_url"], sentinels)

	logs, err := logstore.New(required["clickhouse_dsn"])
	if err != nil {
		t.Fatalf("open ClickHouse log Store: %v", err)
	}
	defer logs.Close()
	if err := logs.Initialize(ctx); err != nil {
		t.Fatalf("initialize ClickHouse log Store: %v", err)
	}
	consumer, err := accessnats.ConnectConsumer(accessnats.Config{URLs: []string{required["nats_url"]}}, zap.NewNop())
	if err != nil {
		t.Fatalf("connect Access Event Consumer: %v", err)
	}
	consumerContext, stopConsumer := context.WithCancel(context.Background())
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- consumer.Run(consumerContext, logs) }()
	consumerStopped := false
	defer func() {
		if consumerStopped {
			return
		}
		stopConsumer()
		select {
		case <-consumerDone:
		case <-time.After(5 * time.Second):
		}
	}()

	runtimeDirectory := t.TempDir()
	if err := os.Chmod(runtimeDirectory, 0o755); err != nil {
		t.Fatalf("chmod load runtime directory: %v", err)
	}
	copyLoadFile(t, tlsMaterial.caFile, filepath.Join(runtimeDirectory, "ca.pem"))
	copyLoadFile(t, tlsMaterial.serverCertificateFile, filepath.Join(runtimeDirectory, "tls.crt"))
	copyLoadFile(t, tlsMaterial.serverPrivateKeyFile, filepath.Join(runtimeDirectory, "tls.key"))
	masterKeyFile := filepath.Join(runtimeDirectory, "master-key")
	if err := os.WriteFile(masterKeyFile, []byte(base64.StdEncoding.EncodeToString(masterKey[:])), 0o444); err != nil {
		t.Fatalf("write load Master Key: %v", err)
	}
	configFile := filepath.Join(runtimeDirectory, "config.yaml")
	config := `version: 1
listen: :9443
tls:
  certificate_file: /run/configra/tls.crt
  private_key_file: /run/configra/tls.key
  client_ca_file: /run/configra/ca.pem
mysql:
  dsn_env: CONFIGRA_LOAD_MYSQL_DSN
key_provider:
  master_key_file: /run/configra/master-key
nats:
  urls: [nats://nats:4222]
logging:
  level: info
`
	if err := os.WriteFile(configFile, []byte(config), 0o444); err != nil {
		t.Fatalf("write load Config: %v", err)
	}
	// Match the non-root image's mount permissions independently of the runner's umask.
	for _, name := range []string{"ca.pem", "tls.crt", "tls.key", "master-key", "config.yaml"} {
		if err := os.Chmod(filepath.Join(runtimeDirectory, name), 0o444); err != nil {
			t.Fatalf("chmod load runtime mount: %v", err)
		}
	}
	containerName := "configra-load-" + suffix
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", containerName).Run() })
	measurementDirectory := t.TempDir()
	warmResult := filepath.Join(measurementDirectory, "warm.gob")
	resultFile := filepath.Join(measurementDirectory, "results.gob")
	imageDescription, _ := exec.CommandContext(ctx, "docker", "image", "inspect", required["image"],
		"--format", "{{.Id}} {{.Architecture}} {{.Os}} {{.Size}}").Output()
	evidence := loadEvidence{
		StartedAt: startedAt, Stage: "startup", Duration: duration.String(), RequestedQPS: loadRate,
		HarnessGoVersion: runtime.Version(), HostOS: runtime.GOOS, HostArch: runtime.GOARCH, HostCPUs: runtime.NumCPU(),
		Image: strings.TrimSpace(string(imageDescription)), ContainerCPUs: 2, ContainerMemoryBytes: 512 << 20,
		MySQLVersion: "8.0.22", ClickHouseVersion: "26.7.3.19",
		MySQLInterpolateParams: database.interpolateParams,
	}
	// Run before container/TempDir cleanup, including when a gate calls Fatal.
	t.Cleanup(func() {
		if t.Failed() {
			evidence.Passed = false
			preserveLoadFailure(t, evidence, sentinels, containerName, warmResult, resultFile)
		}
	})
	if output, err := exec.CommandContext(ctx, "docker", "run", "--detach",
		"--name", containerName,
		"--network", required["docker_network"],
		"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--memory", "512m", "--cpus", "2", "--pids-limit", "256",
		"--publish", "127.0.0.1::9443",
		"--mount", "type=bind,src="+runtimeDirectory+",dst=/run/configra,readonly",
		"--env", "CONFIGRA_LOAD_MYSQL_DSN="+database.containerDSN,
		"--env", "GODEBUG=gctrace=1,schedtrace=30000",
		required["image"], "api", "--config", "/run/configra/config.yaml",
	).CombinedOutput(); err != nil {
		t.Fatalf("start load API image: %v\n%s", err, output)
	}
	baseURL := waitForLoadAPI(t, ctx, containerName, tlsMaterial.roots)
	clientCertificate, err := tls.LoadX509KeyPair(tlsMaterial.clientCertificateFile, tlsMaterial.clientPrivateKeyFile)
	if err != nil {
		t.Fatalf("load load-test Client Certificate: %v", err)
	}
	sdk, err := configrago.NewClient(configrago.ClientOptions{
		BaseURL: baseURL, Token: token.Token,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: tlsMaterial.roots,
			Certificates: []tls.Certificate{clientCertificate}},
	})
	if err != nil {
		t.Fatalf("create load SDK Client: %v", err)
	}
	preflight, err := sdk.ReadResolvedConfig(ctx, "load", "service", "")
	if err != nil || !strings.Contains(preflight.Content, vaultSecret) || len(preflight.Content) < 4<<10 {
		t.Fatalf("load preflight Config was not representative: %v", err)
	}
	file, err := sdk.ReadFile(ctx, "load", "platform", "database", "tls_cert", "")
	if err != nil || string(file.Bytes) != fileSecret {
		t.Fatalf("load preflight File: %v", err)
	}

	targetFile := filepath.Join(measurementDirectory, "target.http")
	target := fmt.Sprintf("GET %s/v1/environments/load/configs/service\nAuthorization: Bearer %s\nAccept: application/json\n\n",
		baseURL, token.Token)
	if err := os.WriteFile(targetFile, []byte(target), 0o600); err != nil {
		t.Fatalf("write Vegeta target: %v", err)
	}
	warmDuration := duration / 5
	if warmDuration > 15*time.Second {
		warmDuration = 15 * time.Second
	}
	if warmDuration < 2*time.Second {
		warmDuration = 2 * time.Second
	}
	evidence.Stage = "warmup"
	warmReport := runVegeta(t, ctx, required["vegeta"], warmDuration, targetFile, warmResult, tlsMaterial)
	evidence.Warmup = warmReport
	validateVegetaReport(t, warmReport, warmDuration)

	evidence.Stage = "load"
	sampleContext, stopSampling := context.WithCancel(ctx)
	samplesDone := make(chan []loadSample, 1)
	go func() { samplesDone <- sampleLoad(sampleContext, containerName, database.admin) }()
	defer func() {
		stopSampling()
		if samplesDone != nil {
			evidence.Samples = <-samplesDone
		}
	}()
	report := runVegeta(t, ctx, required["vegeta"], duration, targetFile, resultFile, tlsMaterial)
	stopSampling()
	samples := <-samplesDone
	samplesDone = nil
	evidence.Report, evidence.Samples = report, samples
	validateVegetaReport(t, report, duration)
	resourceSummary := validateLoadSamples(t, samples, duration)
	evidence.Resources = resourceSummary

	evidence.Stage = "authorization-and-shutdown"
	negativeClient := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: tlsMaterial.roots,
			Certificates: []tls.Certificate{clientCertificate}},
	}}
	negativeRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/v1/environments/load/configs/service", nil)
	negativeRequest.Header.Set("Authorization", "Bearer cfg_0000000000000000_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	negativeResponse, err := negativeClient.Do(negativeRequest)
	if err != nil {
		t.Fatalf("send load negative request: %v", err)
	}
	negativeBody, _ := io.ReadAll(io.LimitReader(negativeResponse.Body, 64<<10))
	_ = negativeResponse.Body.Close()
	negativeClient.CloseIdleConnections()
	if negativeResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("load negative response = %d", negativeResponse.StatusCode)
	}

	if output, err := exec.CommandContext(ctx, "docker", "stop", "--time", "15", containerName).CombinedOutput(); err != nil {
		t.Fatalf("stop load API image: %v\n%s", err, output)
	}
	exitCode, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.ExitCode}}", containerName).Output()
	if err != nil || strings.TrimSpace(string(exitCode)) != "0" {
		t.Fatalf("load API exit = %q, %v", exitCode, err)
	}
	containerLogs, err := exec.CommandContext(ctx, "docker", "logs", containerName).CombinedOutput()
	if err != nil {
		t.Fatalf("read load API logs: %v", err)
	}

	clickHouseOptions, err := clickhouse.ParseDSN(required["clickhouse_dsn"])
	if err != nil {
		t.Fatalf("parse ClickHouse DSN: %v", err)
	}
	reader, err := clickhouse.Open(clickHouseOptions)
	if err != nil {
		t.Fatalf("open ClickHouse evidence reader: %v", err)
	}
	defer reader.Close()
	expectedEvents := report.Requests + warmReport.Requests + 2
	evidence.Stage = "access-events"
	evidence.ExpectedAccessEvents = expectedEvents
	accessEvents := waitForAccessEvents(ctx, reader, token.PublicID, expectedEvents)
	evidence.PersistedAccessEvents = accessEvents
	stopConsumer()
	select {
	case err := <-consumerDone:
		consumerStopped = true
		if err != nil {
			t.Fatalf("Access Event Consumer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Access Event Consumer did not stop")
	}
	if accessEvents*100 < expectedEvents*95 {
		t.Fatalf("persisted Access Events = %d/%d; below 95%%", accessEvents, expectedEvents)
	}
	capturedEvents, wireLeak := finishCapture()
	evidence.CapturedAccessEvents = capturedEvents
	if wireLeak || capturedEvents*100 < expectedEvents*95 {
		t.Fatalf("NATS capture failed: events=%d/%d protected_material=%t", capturedEvents, expectedEvents, wireLeak)
	}
	evidence.NATSLeakScanPassed = true

	evidence.Stage = "leak-scan"
	composeFile, err := filepath.Abs(filepath.Join("..", "deploy", "compose.test.yaml"))
	if err != nil {
		t.Fatalf("resolve Compose file: %v", err)
	}
	dependencyLogs, err := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile,
		"logs", "--no-color", "mysql", "nats", "clickhouse").CombinedOutput()
	if err != nil {
		t.Fatalf("read dependency logs: %v", err)
	}
	for source, content := range map[string][]byte{
		"API logs": containerLogs, "dependency logs": dependencyLogs, "HTTP error": negativeBody,
	} {
		assertNoSentinels(t, source, content, sentinels)
	}
	assertFileHasNoSentinels(t, resultFile, sentinels)
	assertClickHouseHasNoSentinels(t, ctx, reader, token.PublicID, sentinels)
	if bytes.Contains(containerLogs, []byte("panic:")) || bytes.Contains(containerLogs, []byte("fatal error:")) ||
		bytes.Contains(containerLogs, []byte("out of memory")) {
		t.Fatal("load API logs contain a runtime failure")
	}

	evidence.Passed, evidence.LeakScanPassed, evidence.Stage = true, true, "complete"
	evidence.GCTraceLines = bytes.Count(containerLogs, []byte("gc "))
	evidence.SchedulerTraceLines = bytes.Count(containerLogs, []byte("SCHED "))
	evidencePath := writeLoadEvidence(t, evidence, sentinels)
	t.Logf("1000 QPS evidence: requests=%d throughput=%.2f/s success=%.5f p95=%s p99=%s RSS=%s..%s access=%d/%d report=%s",
		report.Requests, report.Throughput, report.Success,
		time.Duration(report.Latencies.P95), time.Duration(report.Latencies.P99),
		formatBytes(resourceSummary.MinMemoryBytes), formatBytes(resourceSummary.MaxMemoryBytes),
		accessEvents, expectedEvents, evidencePath)
}

type loadDatabase struct {
	store             *mysqlstore.Store
	admin             *sql.DB
	containerDSN      string
	interpolateParams bool
}

func newLoadDatabase(
	t *testing.T,
	ctx context.Context,
	rootDSN, hostDSN, containerDSN string,
	provider *vaultcrypto.LocalKeyProvider,
	suffix string,
) loadDatabase {
	t.Helper()
	admin, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatalf("open load MySQL Root connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var version string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil || version != "8.0.22" {
		t.Fatalf("MySQL version = %q, %v; want 8.0.22", version, err)
	}
	databaseName := "configra_load_" + suffix
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+databaseName+
		" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create load database: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "GRANT ALL PRIVILEGES ON "+databaseName+".* TO 'configra'@'%'"); err != nil {
		t.Fatalf("grant load database: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(cleanupContext, "DROP DATABASE IF EXISTS "+databaseName)
	})
	hostConfiguration, err := mysql.ParseDSN(hostDSN)
	if err != nil || hostConfiguration.User != "configra" {
		t.Fatalf("parse load host DSN: %v", err)
	}
	hostConfiguration.DBName = databaseName
	containerConfiguration, err := mysql.ParseDSN(containerDSN)
	if err != nil || containerConfiguration.User != "configra" {
		t.Fatalf("parse load container DSN: %v", err)
	}
	containerConfiguration.DBName = databaseName
	store, err := mysqlstore.OpenManagement(ctx, hostConfiguration.FormatDSN(), provider)
	if err != nil {
		t.Fatalf("OpenManagement load database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return loadDatabase{store: store, admin: admin, containerDSN: containerConfiguration.FormatDSN(),
		interpolateParams: containerConfiguration.InterpolateParams}
}

func loadVaultSnapshot(secret, file string) vaultdoc.Snapshot {
	return vaultdoc.Snapshot{
		Fields: []vaultdoc.Field{
			{Key: "password", Name: "Password", Type: vaultdoc.Secret},
			{Key: "tls_cert", Name: "TLS Certificate", Type: vaultdoc.File},
		},
		Variants: []vaultdoc.Variant{{
			Environments: []string{"load"},
			Values: map[string]vaultdoc.Value{
				"password": {Text: &secret},
				"tls_cert": {File: &vaultdoc.FileValue{
					Filename: "client.pem", ContentType: "application/x-pem-file", Bytes: []byte(file),
				}},
			},
		}},
	}
}

type loadTLSMaterial struct {
	caFile                string
	serverCertificateFile string
	serverPrivateKeyFile  string
	clientCertificateFile string
	clientPrivateKeyFile  string
	privateKeyMaterial    [][]byte
	clientCertificate     *x509.Certificate
	roots                 *x509.CertPool
}

func createLoadTLS(t *testing.T) loadTLSMaterial {
	t.Helper()
	directory := t.TempDir()
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate load CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Configra Load CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatalf("create load CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse load CA: %v", err)
	}
	createLeaf := func(serial int64, commonName string, usages []x509.ExtKeyUsage, server bool) ([]byte, ed25519.PrivateKey, *x509.Certificate) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate load TLS key: %v", err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName},
			NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usages,
		}
		if server {
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
			template.DNSNames = []string{"localhost"}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, publicKey, caPrivate)
		if err != nil {
			t.Fatalf("create load TLS Certificate: %v", err)
		}
		certificate, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("parse load TLS Certificate: %v", err)
		}
		return der, privateKey, certificate
	}
	serverDER, serverKey, _ := createLeaf(2, "localhost", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	clientDER, clientKey, clientCertificate := createLeaf(3, "load-client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverPEM := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), caPEM...)
	clientPEM := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}), caPEM...)
	serverKeyDER, _ := x509.MarshalPKCS8PrivateKey(serverKey)
	clientKeyDER, _ := x509.MarshalPKCS8PrivateKey(clientKey)
	caKeyDER, _ := x509.MarshalPKCS8PrivateKey(caPrivate)
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER})
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clientKeyDER})
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caKeyDER})
	material := loadTLSMaterial{
		caFile:                filepath.Join(directory, "ca.pem"),
		serverCertificateFile: filepath.Join(directory, "server.crt"),
		serverPrivateKeyFile:  filepath.Join(directory, "server.key"),
		clientCertificateFile: filepath.Join(directory, "client.crt"),
		clientPrivateKeyFile:  filepath.Join(directory, "client.key"),
		privateKeyMaterial:    [][]byte{serverKey, serverKeyDER, serverKeyPEM, clientKey, clientKeyDER, clientKeyPEM, caPrivate, caKeyDER, caKeyPEM},
		clientCertificate:     clientCertificate,
		roots:                 x509.NewCertPool(),
	}
	material.roots.AddCert(ca)
	for path, content := range map[string][]byte{
		material.caFile: caPEM, material.serverCertificateFile: serverPEM,
		material.serverPrivateKeyFile: serverKeyPEM, material.clientCertificateFile: clientPEM,
		material.clientPrivateKeyFile: clientKeyPEM,
	} {
		if err := os.WriteFile(path, content, 0o444); err != nil {
			t.Fatalf("write load TLS material: %v", err)
		}
	}
	return material
}

func copyLoadFile(t *testing.T, source, target string) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read load file: %v", err)
	}
	if err := os.WriteFile(target, content, 0o444); err != nil {
		t.Fatalf("write load file: %v", err)
	}
}

func waitForLoadAPI(t *testing.T, ctx context.Context, container string, roots *x509.CertPool) string {
	t.Helper()
	portOutput, err := exec.CommandContext(ctx, "docker", "port", container, "9443/tcp").Output()
	if err != nil {
		t.Fatalf("read load API port: %v", err)
	}
	address := strings.TrimSpace(string(portOutput))
	index := strings.LastIndexByte(address, ':')
	if index < 0 {
		t.Fatalf("invalid load API port %q", address)
	}
	baseURL := "https://127.0.0.1:" + address[index+1:]
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
	}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(20 * time.Second)
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health/ready", nil)
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return baseURL
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("load API did not become ready: %v", requestErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type vegetaReport struct {
	Latencies struct {
		Mean int64 `json:"mean"`
		P50  int64 `json:"50th"`
		P95  int64 `json:"95th"`
		P99  int64 `json:"99th"`
		Max  int64 `json:"max"`
	} `json:"latencies"`
	Requests    uint64            `json:"requests"`
	Rate        float64           `json:"rate"`
	Throughput  float64           `json:"throughput"`
	Success     float64           `json:"success"`
	StatusCodes map[string]uint64 `json:"status_codes"`
	Errors      []string          `json:"errors"`
}

func runVegeta(
	t *testing.T,
	ctx context.Context,
	binary string,
	duration time.Duration,
	targetFile, resultFile string,
	material loadTLSMaterial,
) vegetaReport {
	t.Helper()
	arguments := []string{
		"attack", "-name=configra-machine-read", "-rate=1000/s", "-duration=" + duration.String(),
		"-workers=100", "-max-workers=1000", "-connections=200", "-max-connections=500",
		"-timeout=5s", "-redirects=-1", "-max-body=0", "-http2=true",
		"-root-certs=" + material.caFile, "-cert=" + material.clientCertificateFile,
		"-key=" + material.clientPrivateKeyFile, "-targets=" + targetFile, "-output=" + resultFile,
	}
	if output, err := exec.CommandContext(ctx, binary, arguments...).CombinedOutput(); err != nil {
		t.Fatalf("Vegeta attack: %v\n%s", err, output)
	}
	encoded, err := exec.CommandContext(ctx, binary, "report", "-type=json", resultFile).Output()
	if err != nil {
		t.Fatalf("Vegeta report: %v", err)
	}
	var report vegetaReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("decode Vegeta report: %v", err)
	}
	return report
}

func validateVegetaReport(t *testing.T, report vegetaReport, duration time.Duration) {
	t.Helper()
	minimumRequests := uint64(duration.Seconds() * loadRate * 0.98)
	if report.Requests < minimumRequests || report.Rate < loadRate*0.99 || report.Throughput < loadRate*0.98 ||
		report.Success != 1 || len(report.Errors) != 0 || report.StatusCodes["200"] != report.Requests || len(report.StatusCodes) != 1 {
		t.Fatalf("Vegeta gate failed: requests=%d rate=%.2f throughput=%.2f success=%.5f codes=%v errors=%v",
			report.Requests, report.Rate, report.Throughput, report.Success, report.StatusCodes, report.Errors)
	}
}

type loadSample struct {
	ElapsedSeconds  float64 `json:"elapsed_seconds"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemoryBytes     uint64  `json:"memory_bytes"`
	PIDs            uint64  `json:"pids"`
	MySQLConnected  uint64  `json:"mysql_connected"`
	MySQLRunning    uint64  `json:"mysql_running"`
	SamplingFailure string  `json:"sampling_failure,omitempty"`
}

func TestSampleLoadIgnoresCancellationAsSamplingFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if samples := sampleLoad(ctx, "unused", nil); len(samples) != 0 {
		t.Fatalf("cancelled sampler returned %d samples, want none", len(samples))
	}
}

func sampleLoad(ctx context.Context, container string, database *sql.DB) []loadSample {
	started := time.Now()
	samples := make([]loadSample, 0, 128)
	for {
		if ctx.Err() != nil {
			return samples
		}
		sample := oneLoadSample(ctx, container, database, time.Since(started).Seconds())
		if ctx.Err() != nil && sample.SamplingFailure != "" {
			return samples
		}
		samples = append(samples, sample)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return samples
		case <-timer.C:
		}
	}
}

func oneLoadSample(ctx context.Context, container string, database *sql.DB, elapsed float64) loadSample {
	sample := loadSample{ElapsedSeconds: elapsed}
	output, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", container).Output()
	if err != nil {
		sample.SamplingFailure = "docker_stats"
		return sample
	}
	var stats struct {
		CPUPercent string `json:"CPUPerc"`
		Memory     string `json:"MemUsage"`
		PIDs       string `json:"PIDs"`
	}
	if json.Unmarshal(output, &stats) != nil {
		sample.SamplingFailure = "decode_docker_stats"
		return sample
	}
	sample.CPUPercent, _ = strconv.ParseFloat(strings.TrimSuffix(stats.CPUPercent, "%"), 64)
	sample.MemoryBytes, err = parseByteSize(strings.TrimSpace(strings.Split(stats.Memory, "/")[0]))
	if err != nil {
		sample.SamplingFailure = "decode_memory"
		return sample
	}
	sample.PIDs, _ = strconv.ParseUint(strings.TrimSpace(stats.PIDs), 10, 64)
	if err := database.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Threads_connected'").Scan(new(string), &sample.MySQLConnected); err != nil {
		sample.SamplingFailure = "mysql_connected"
		return sample
	}
	if err := database.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Threads_running'").Scan(new(string), &sample.MySQLRunning); err != nil {
		sample.SamplingFailure = "mysql_running"
	}
	return sample
}

func parseByteSize(value string) (uint64, error) {
	units := []struct {
		suffix     string
		multiplier float64
	}{
		{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
			if err != nil || number < 0 {
				return 0, fmt.Errorf("invalid byte size")
			}
			return uint64(number * unit.multiplier), nil
		}
	}
	return 0, fmt.Errorf("unknown byte size")
}

type loadResourceSummary struct {
	MinMemoryBytes    uint64  `json:"min_memory_bytes"`
	MaxMemoryBytes    uint64  `json:"max_memory_bytes"`
	FirstMemoryMedian uint64  `json:"first_memory_median"`
	LastMemoryMedian  uint64  `json:"last_memory_median"`
	MaxCPUPercent     float64 `json:"max_cpu_percent"`
	MaxPIDs           uint64  `json:"max_pids"`
	MaxMySQLConnected uint64  `json:"max_mysql_connected"`
	MaxMySQLRunning   uint64  `json:"max_mysql_running"`
}

func validateLoadSamples(t *testing.T, samples []loadSample, duration time.Duration) loadResourceSummary {
	t.Helper()
	if len(samples) < 2 {
		t.Fatalf("load resource samples = %d, want at least 2", len(samples))
	}
	result := loadResourceSummary{MinMemoryBytes: ^uint64(0)}
	memories := make([]uint64, 0, len(samples))
	for _, sample := range samples {
		if sample.SamplingFailure != "" {
			t.Fatalf("load resource sampling failed: %s", sample.SamplingFailure)
		}
		result.MinMemoryBytes = min(result.MinMemoryBytes, sample.MemoryBytes)
		result.MaxMemoryBytes = max(result.MaxMemoryBytes, sample.MemoryBytes)
		result.MaxCPUPercent = max(result.MaxCPUPercent, sample.CPUPercent)
		result.MaxPIDs = max(result.MaxPIDs, sample.PIDs)
		result.MaxMySQLConnected = max(result.MaxMySQLConnected, sample.MySQLConnected)
		result.MaxMySQLRunning = max(result.MaxMySQLRunning, sample.MySQLRunning)
		memories = append(memories, sample.MemoryBytes)
	}
	if result.MaxMemoryBytes >= 480<<20 || result.MaxPIDs >= 256 {
		t.Fatalf("load resource ceiling exceeded: memory=%s pids=%d", formatBytes(result.MaxMemoryBytes), result.MaxPIDs)
	}
	window := max(1, len(memories)/5)
	result.FirstMemoryMedian = median(memories[:window])
	result.LastMemoryMedian = median(memories[len(memories)-window:])
	// ponytail: RSS medians are the coarse container leak gate; add authenticated heap profiles if this trend grows or becomes noisy in CI.
	if duration >= 2*time.Minute && result.LastMemoryMedian > result.FirstMemoryMedian+(64<<20) &&
		result.LastMemoryMedian*100 > result.FirstMemoryMedian*125 {
		t.Fatalf("load RSS grew from %s to %s", formatBytes(result.FirstMemoryMedian), formatBytes(result.LastMemoryMedian))
	}
	return result
}

func median(values []uint64) uint64 {
	copyOfValues := slices.Clone(values)
	sort.Slice(copyOfValues, func(left, right int) bool { return copyOfValues[left] < copyOfValues[right] })
	return copyOfValues[len(copyOfValues)/2]
}

func waitForAccessEvents(ctx context.Context, connection clickhouse.Conn, principal string, expected uint64) uint64 {
	deadline := time.Now().Add(15 * time.Second)
	var count uint64
	for {
		_ = connection.QueryRow(ctx, "SELECT count() FROM access_events WHERE principal = ?", principal).Scan(&count)
		if count >= expected {
			return count
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return count
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func loadSecretForms(values ...[]byte) [][]byte {
	var forms [][]byte
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		encodedHex := hex.EncodeToString(value)
		forms = append(forms, value, []byte(base64.StdEncoding.EncodeToString(value)), []byte(encodedHex), []byte(strings.ToUpper(encodedHex)))
	}
	return forms
}

func containsLoadSecret(content []byte, sentinels [][]byte) bool {
	for _, sentinel := range sentinels {
		if len(sentinel) > 0 && bytes.Contains(content, sentinel) {
			return true
		}
	}
	return false
}

func assertNoSentinels(t *testing.T, source string, content []byte, sentinels [][]byte) {
	t.Helper()
	if containsLoadSecret(content, sentinels) {
		t.Fatalf("%s contains protected load-test material", source)
	}
}

// Scan in the subscriber callback, without retaining the full load's payloads.
func captureLoadAccess(t *testing.T, url string, sentinels [][]byte) func() (uint64, bool) {
	t.Helper()
	var count atomic.Uint64
	var leaked, failed atomic.Bool
	closed := make(chan struct{})
	connection, err := nats.Connect(url, nats.NoReconnect(), nats.Timeout(5*time.Second), nats.DrainTimeout(5*time.Second),
		nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) { failed.Store(true) }),
		nats.ClosedHandler(func(*nats.Conn) { close(closed) }),
	)
	if err != nil {
		t.Fatal("connect load NATS capture")
	}
	t.Cleanup(connection.Close)
	subscription, err := connection.Subscribe(accessnats.Subject, func(message *nats.Msg) {
		if containsLoadSecret(message.Data, sentinels) {
			leaked.Store(true)
		}
		count.Add(1)
	})
	if err != nil {
		t.Fatal("subscribe load NATS capture")
	}
	if err := subscription.SetPendingLimits(8192, 8<<20); err != nil {
		t.Fatal("bound load NATS capture")
	}
	if err := connection.FlushTimeout(5 * time.Second); err != nil {
		t.Fatal("activate load NATS capture")
	}
	return func() (uint64, bool) {
		if err := connection.Drain(); err != nil {
			t.Fatal("drain load NATS capture")
		}
		select {
		case <-closed:
		case <-time.After(12 * time.Second):
			t.Fatal("load NATS capture did not close")
		}
		if failed.Load() || connection.LastError() != nil {
			t.Fatal("load NATS capture lost messages or failed")
		}
		return count.Load(), leaked.Load()
	}
}

func TestLoadLeakSecretForms(t *testing.T) {
	value := []byte("load-key-\x00\xff\r\n")
	forms := loadSecretForms(nil, value)
	for _, encoded := range [][]byte{value, []byte("bG9hZC1rZXktAP8NCg=="), []byte("6c6f61642d6b65792d00ff0d0a"), []byte("6C6F61642D6B65792D00FF0D0A")} {
		if !containsLoadSecret(append([]byte("metadata:"), encoded...), forms) {
			t.Fatal("raw/base64/hex protected material went undetected")
		}
	}
	if containsLoadSecret([]byte("value-free metadata"), forms) {
		t.Fatal("unrelated metadata matched a secret")
	}
}

func TestLoadLeakCaptureFindsEncodedPayload(t *testing.T) {
	url := os.Getenv("CONFIGRA_TEST_NATS_URL")
	if url == "" {
		t.Skip("CONFIGRA_TEST_NATS_URL is not set")
	}
	value := []byte("load-capture-regression")
	finish := captureLoadAccess(t, url, loadSecretForms(value))
	publisher, err := nats.Connect(url, nats.NoReconnect(), nats.Timeout(5*time.Second))
	if err != nil {
		t.Fatal("connect load capture regression publisher")
	}
	defer publisher.Close()
	for _, body := range []string{"value-free metadata", base64.StdEncoding.EncodeToString(value)} {
		if err := publisher.Publish(accessnats.Subject, []byte(body)); err != nil {
			t.Fatal("publish load capture regression payload")
		}
	}
	if err := publisher.FlushTimeout(5 * time.Second); err != nil {
		t.Fatal("flush load capture regression publisher")
	}
	if count, leaked := finish(); count != 2 || !leaked {
		t.Fatalf("capture = %d messages, leaked=%t; want two messages and a detected leak", count, leaked)
	}
}

func assertFileHasNoSentinels(t *testing.T, path string, sentinels [][]byte) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open Vegeta results: %v", err)
	}
	defer file.Close()
	maxSentinel := 1
	for _, sentinel := range sentinels {
		maxSentinel = max(maxSentinel, len(sentinel))
	}
	buffer := make([]byte, (1<<20)+maxSentinel)
	carried := 0
	for {
		read, readErr := file.Read(buffer[carried : carried+(1<<20)])
		chunk := buffer[:carried+read]
		assertNoSentinels(t, "Vegeta results", chunk, sentinels)
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			t.Fatalf("scan Vegeta results: %v", readErr)
		}
		carried = min(maxSentinel-1, len(chunk))
		copy(buffer, chunk[len(chunk)-carried:])
	}
}

func assertClickHouseHasNoSentinels(
	t *testing.T,
	ctx context.Context,
	connection clickhouse.Conn,
	principal string,
	sentinels [][]byte,
) {
	t.Helper()
	// Stream metadata once. Do not send protected values as SQL parameters: the
	// database's own query log could otherwise record the scan's secret needles.
	rows, err := connection.Query(ctx, `
		SELECT concat(principal, authentication, environment, namespace, resource_type,
		              resource, toString(config_revision), toString(vault_revisions))
		FROM access_events WHERE principal = ?
	`, principal)
	if err != nil {
		t.Fatalf("read ClickHouse Access Events for leak scan: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			t.Fatalf("scan ClickHouse Access Events: %v", err)
		}
		assertNoSentinels(t, "ClickHouse Access Events", []byte(content), sentinels)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate ClickHouse Access Events for leak scan: %v", err)
	}
}

type loadEvidence struct {
	StartedAt              time.Time           `json:"started_at"`
	Passed                 bool                `json:"passed"`
	Stage                  string              `json:"stage"`
	Duration               string              `json:"duration"`
	RequestedQPS           int                 `json:"requested_qps"`
	HarnessGoVersion       string              `json:"harness_go_version"`
	HostOS                 string              `json:"host_os"`
	HostArch               string              `json:"host_arch"`
	HostCPUs               int                 `json:"host_cpus"`
	Image                  string              `json:"image"`
	ContainerCPUs          int                 `json:"container_cpus"`
	ContainerMemoryBytes   uint64              `json:"container_memory_bytes"`
	MySQLVersion           string              `json:"mysql_version"`
	MySQLInterpolateParams bool                `json:"mysql_interpolate_params"`
	ClickHouseVersion      string              `json:"clickhouse_version"`
	Warmup                 vegetaReport        `json:"warmup"`
	Report                 vegetaReport        `json:"vegeta"`
	Samples                []loadSample        `json:"samples"`
	Resources              loadResourceSummary `json:"resources"`
	ExpectedAccessEvents   uint64              `json:"expected_access_events"`
	PersistedAccessEvents  uint64              `json:"persisted_access_events"`
	CapturedAccessEvents   uint64              `json:"captured_access_events"`
	NATSLeakScanPassed     bool                `json:"nats_leak_scan_passed"`
	LeakScanPassed         bool                `json:"leak_scan_passed"`
	GCTraceLines           int                 `json:"gc_trace_lines"`
	SchedulerTraceLines    int                 `json:"scheduler_trace_lines"`
}

func preserveLoadFailure(t *testing.T, evidence loadEvidence, sentinels [][]byte, container string, results ...string) {
	t.Helper()
	path := writeLoadEvidence(t, evidence, sentinels)
	directory, err := os.MkdirTemp(filepath.Dir(path), "failed-"+strconv.FormatInt(evidence.StartedAt.UnixNano(), 10)+"-")
	if err != nil {
		t.Errorf("create private load failure directory: %v", err)
		return
	}
	for _, source := range append([]string{path}, results...) {
		if err := copyLoadEvidenceFile(source, filepath.Join(directory, filepath.Base(source))); err != nil {
			t.Errorf("preserve load evidence: %v", err)
		}
	}
	// Raw logs/results may contain a newly discovered leak: keep them private,
	// never print them or retain target.http (which contains the Bearer Token).
	logFile, err := os.OpenFile(filepath.Join(directory, "api.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Errorf("create private load API log: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "logs", container)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Run(); err != nil {
		t.Errorf("preserve load API log: %v", err)
	}
	if err := logFile.Close(); err != nil {
		t.Errorf("close load API log: %v", err)
	}
	t.Logf("failed load evidence (private; do not publish raw logs/results): %s", directory)
}

func copyLoadEvidenceFile(source, target string) error {
	input, err := os.Open(source)
	if os.IsNotExist(err) {
		return nil // The test may fail before this measurement starts.
	}
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func TestLoadEvidenceCopyIsPrivateAndDoesNotOverwrite(t *testing.T) {
	directory := t.TempDir()
	source, target := filepath.Join(directory, "source"), filepath.Join(directory, "target")
	if err := copyLoadEvidenceFile(source, target); err != nil {
		t.Fatalf("missing measurement: %v", err)
	}
	if err := os.WriteFile(source, []byte("measurement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyLoadEvidenceFile(source, target); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "measurement" {
		t.Fatalf("copied measurement = %q, %v", content, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence permissions = %v; want 0600", info.Mode().Perm())
	}
	if err := copyLoadEvidenceFile(source, target); !os.IsExist(err) {
		t.Fatalf("existing evidence overwrite = %v; want existence error", err)
	}
}

func TestLoadEvidenceRejectsSecretsInReport(t *testing.T) {
	evidence := loadEvidence{Passed: true, Stage: "complete", MySQLInterpolateParams: true}
	secret := []byte("private-report-sentinel")
	sentinels := loadSecretForms(secret)
	if encoded, err := encodeLoadEvidence(evidence, sentinels); err != nil || !bytes.Contains(encoded, []byte(`"mysql_interpolate_params": true`)) {
		t.Fatalf("clean report must record the query mode without a DSN: %v", err)
	}
	evidence.Report.Errors = []string{base64.StdEncoding.EncodeToString(secret)}
	if encoded, err := encodeLoadEvidence(evidence, sentinels); err == nil || len(encoded) != 0 {
		t.Fatal("secret-bearing report must return an error without encoded content")
	}
}

func encodeLoadEvidence(evidence loadEvidence, sentinels [][]byte) ([]byte, error) {
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode load evidence: %w", err)
	}
	if containsLoadSecret(encoded, sentinels) {
		return nil, fmt.Errorf("load evidence contains protected load-test material")
	}
	return encoded, nil
}

func writeLoadEvidence(t *testing.T, evidence loadEvidence, sentinels [][]byte) string {
	t.Helper()
	repository, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository: %v", err)
	}
	directory := filepath.Join(repository, ".cache", "load")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create load evidence directory: %v", err)
	}
	encoded, err := encodeLoadEvidence(evidence, sentinels)
	if err != nil {
		t.Errorf("load report redacted: %v", err)
		// Never leave a previous success as "latest" when the report itself leaks.
		encoded = []byte(`{"passed":false,"stage":"evidence-redacted"}`)
	}
	path := filepath.Join(directory, "latest.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write load evidence: %v", err)
	}
	return path
}

func formatBytes(value uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
}
