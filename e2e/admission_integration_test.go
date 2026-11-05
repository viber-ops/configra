//go:build integration

package e2e_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	configra "github.com/viber-ops/configra-go"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestMixedFileAndCredentialBurstPreservesMachineReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := newStore(t, ctx)
	admin := mysqlstore.Actor{Type: "user", ID: "admission-fixture"}
	if _, err := store.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{OperationID: "admission-env", Actor: admin, Action: mysqlstore.EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitConfig(ctx, mysqlstore.ConfigCommit{OperationID: "admission-config", Actor: admin, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("version: 1\n")}); err != nil {
		t.Fatal(err)
	}
	authority, err := store.CreateCertificateAuthority(ctx, mysqlstore.AuthorityCreate{OperationID: "admission-ca", Actor: admin, DisplayName: "CA", ValidDays: 2})
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := store.IssueClientCertificate(ctx, mysqlstore.ClientCertificateIssue{OperationID: "admission-client", Actor: admin, AuthorityID: authority.Authority.ID, DisplayName: "Operator", ValidDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := scopedBundle(t, certificate.ExportBundle)
	token, err := store.CreateToken(ctx, mysqlstore.TokenCreate{OperationID: "admission-token", Actor: admin, Kind: machine.TokenWriteScoped, DisplayName: "Writer", EnvironmentKeys: []string{"prod"}, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AppendCertsFromPEM([]byte(authority.Authority.CertificatePEM))
	server := httptest.NewUnstartedServer(management.NewMachineHandler(store, nil, management.WriteLimits{Concurrent: 2, RequestsPerSecond: 20, Burst: 16, PerTokenRequestsPerSecond: 10}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots}
	server.StartTLS()
	defer server.Close()
	serverRoots := x509.NewCertPool()
	serverRoots.AddCert(server.Certificate())
	client, err := configra.NewClient(configra.ClientOptions{BaseURL: server.URL, Token: token.Token, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: serverRoots, Certificates: []tls.Certificate{identity}}, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	metricsDB, err := sql.Open("mysql", os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN"))
	if err != nil {
		t.Fatal("open fixture metrics")
	}
	defer metricsDB.Close()
	deadlocks := func() uint64 {
		var name, value string
		if metricsDB.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Innodb_deadlocks'").Scan(&name, &value) != nil {
			return 0
		}
		number, _ := strconv.ParseUint(value, 10, 64)
		return number
	}
	beforeDeadlocks := deadlocks()
	var accepted, limited, failed atomic.Int64
	write := func(worker, index int) {
		operation := fmt.Sprintf("mixed-burst-%02d-%02d", worker, index)
		var err error
		if worker%2 == 0 {
			_, err = client.WriteFile(ctx, "prod", "ops", fmt.Sprintf("item_%02d_%02d", worker, index), "key", configra.FileWrite{RevisionWrite: configra.RevisionWrite{OperationID: operation}, Name: "Key", File: configra.VaultFile{Filename: "key.bin", ContentType: "application/octet-stream", Bytes: make([]byte, 128<<10)}})
		} else {
			_, err = client.IssueDeploymentCredential(ctx, "prod", configra.DeploymentCredentialIssue{OperationID: operation, DisplayName: "Host", AuthorityID: authority.Authority.ID, ExpiresAt: time.Now().Add(30 * time.Minute)})
		}
		if err == nil {
			accepted.Add(1)
			return
		}
		var api *configra.APIError
		if errors.As(err, &api) && api.StatusCode == 429 && api.RetryAfter > 0 {
			limited.Add(1)
		} else {
			failed.Add(1)
			if api != nil {
				t.Logf("unexpected write response: status=%d code=%s", api.StatusCode, api.Code)
			} else {
				t.Log("unexpected write transport failure")
			}
		}
	}
	// Establish both write paths, then run a simultaneous read/write burst.
	write(0, 0)
	write(1, 0)
	start := make(chan struct{})
	latencies := make(chan time.Duration, 80)
	var workers sync.WaitGroup
	for reader := 0; reader < 4; reader++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for i := 0; i < 20; i++ {
				began := time.Now()
				value, err := client.ReadResolvedConfig(ctx, "prod", "server", "")
				if err != nil || value.ConfigRevision != 1 {
					failed.Add(1)
				}
				latencies <- time.Since(began)
			}
		}()
	}
	for writer := 0; writer < 8; writer++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			<-start
			for i := 1; i <= 8; i++ {
				write(worker, i)
			}
		}(writer)
	}
	close(start)
	workers.Wait()
	close(latencies)
	var measured []time.Duration
	for duration := range latencies {
		measured = append(measured, duration)
	}
	slices.Sort(measured)
	if failed.Load() != 0 || accepted.Load() < 2 || limited.Load() == 0 || len(measured) != 80 || measured[75] > 2*time.Second {
		t.Fatalf("mixed admission: accepted=%d limited=%d errors=%d reads=%d p95=%s deadlocks=%d", accepted.Load(), limited.Load(), failed.Load(), len(measured), measured[75], deadlocks()-beforeDeadlocks)
	}
	t.Logf("bounded mixed acceptance: reads=80 p95=%s accepted_writes=%d limited_writes=%d", measured[75], accepted.Load(), limited.Load())
}
