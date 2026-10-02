//go:build integration

package e2e_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configrago "github.com/viber-ops/configra-go"
	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/logworker"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultdoc"
	"go.uber.org/zap"
)

func TestScopedWriteSDKWithRealMySQLNATSClickHouse(t *testing.T) {
	if os.Getenv("CONFIGRA_TEST_NATS_URL") == "" || os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN") == "" {
		t.Skip("real NATS and ClickHouse are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store := newStore(t, ctx)
	actor := mysqlstore.Actor{Type: "user", ID: "scoped-e2e-admin"}
	for _, key := range []string{"testing", "production", "foreign"} {
		if _, err := store.ApplyEnvironmentChange(ctx, mysqlstore.EnvironmentChange{OperationID: "scoped-env-" + key, Actor: actor, Action: mysqlstore.EnvironmentCreate, Key: key, DisplayName: key}); err != nil {
			t.Fatal(err)
		}
	}
	ca, err := store.CreateCertificateAuthority(ctx, mysqlstore.AuthorityCreate{OperationID: "scoped-ca-create", Actor: actor, DisplayName: "Scoped E2E", ValidDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	control, err := store.IssueClientCertificate(ctx, mysqlstore.ClientCertificateIssue{OperationID: "scoped-operator-cert", Actor: actor, AuthorityID: ca.Authority.ID, DisplayName: "Operator", ValidDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	identity, controlFiles := scopedBundle(t, control.ExportBundle)
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM([]byte(ca.Authority.CertificatePEM)) {
		t.Fatal("invalid CA fixture")
	}
	logs, err := logstore.New(os.Getenv("CONFIGRA_TEST_CLICKHOUSE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if err := logs.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	consumer, err := accessnats.ConnectConsumer(accessnats.Config{URLs: []string{os.Getenv("CONFIGRA_TEST_NATS_URL")}}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	consumerCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- consumer.Run(consumerCtx, logs) }()
	defer func() {
		stopConsumer()
		if err := <-consumerDone; err != nil {
			t.Error(err)
		}
	}()
	publisher, err := accessnats.Connect(accessnats.Config{URLs: []string{os.Getenv("CONFIGRA_TEST_NATS_URL")}}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close(ctx)
	managementAPI := management.NewHandler(store, publisher, clientCAs, logs)
	adminRequest := func(method, path, operation string, input any, mfa bool) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(input)
		request := httptest.NewRequest(method, "https://management.test"+path, bytes.NewReader(encoded))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", operation)
		request.RemoteAddr = "192.0.2.10:1234"
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), humanauth.Principal{Subject: actor.ID, Role: humanauth.RoleAdmin, MFAVerified: mfa}))
		response := httptest.NewRecorder()
		managementAPI.ServeHTTP(response, request)
		return response
	}
	issueWriter := func(operation string, expiry time.Time, namespaces []string) mysqlstore.TokenCreateResult {
		body := map[string]any{"kind": "write-scoped", "display_name": "Operator", "environment_keys": []string{"testing", "production"}, "config_keys": []string{"server"}, "namespace_keys": namespaces, "expires_at": expiry}
		if denied := adminRequest("POST", "/v1/api-tokens", operation, body, false); denied.Code != 403 {
			t.Fatalf("MFA-free issuance HTTP %d", denied.Code)
		}
		response := adminRequest("POST", "/v1/api-tokens", operation, body, true)
		var token mysqlstore.TokenCreateResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &token) != nil || token.Token == "" {
			t.Fatalf("writer issuance HTTP %d", response.Code)
		}
		return token
	}
	writer := issueWriter("scoped-writer-create", time.Now().Add(48*time.Hour), []string{"deployment"})
	server := httptest.NewUnstartedServer(management.NewMachineHandler(store, publisher))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: clientCAs}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	clientFor := func(token string, certificate *tls.Certificate) *configrago.Client {
		t.Helper()
		config := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
		if certificate != nil {
			config.Certificates = []tls.Certificate{*certificate}
		}
		client, err := configrago.NewClient(configrago.ClientOptions{BaseURL: server.URL, Token: token, TLSConfig: config})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.CloseIdleConnections)
		return client
	}
	client := clientFor(writer.Token, &identity)
	wantStatus := func(err error, status int) {
		t.Helper()
		var api *configrago.APIError
		if !errors.As(err, &api) || api.StatusCode != status {
			t.Fatalf("expected API HTTP %d; got %v", status, err)
		}
		if strings.Contains(err.Error(), writer.Token) || strings.Contains(err.Error(), "private-value") {
			t.Fatal("error leaked a credential/value")
		}
	}
	configWrite := configrago.ConfigWrite{RevisionWrite: configrago.RevisionWrite{OperationID: "scoped-config-create"}, Name: "server.yaml", Format: "yaml", Content: "setting: private-value-config\n"}
	if _, err := clientFor(writer.Token, nil).WriteConfig(ctx, "testing", "server", configWrite); err == nil {
		t.Fatal("writer accepted without mTLS")
	} else {
		wantStatus(err, 401)
	}
	result, err := client.WriteConfig(ctx, "testing", "server", configWrite)
	if err != nil || result.Revision != 1 {
		t.Fatalf("Config creation: %v", err)
	}
	configWrite.OperationID, configWrite.ExpectedRevision, configWrite.Content = "scoped-config-update", 1, "setting: updated\n"
	for range 2 {
		result, err = client.WriteConfig(ctx, "testing", "server", configWrite)
		if err != nil || result.Revision != 2 {
			t.Fatalf("Config update/replay: %v", err)
		}
	}
	if raw, err := client.ReadRawConfig(ctx, "testing", "server"); err != nil || raw.Revision != 2 {
		t.Fatalf("raw Config: %v", err)
	}
	configWrite.OperationID = "scoped-config-conflict"
	_, err = client.WriteConfig(ctx, "testing", "server", configWrite)
	wantStatus(err, 409)
	for _, scope := range []struct{ env, config string }{{"foreign", "server"}, {"testing", "other"}} {
		_, err := client.WriteConfig(ctx, scope.env, scope.config, configWrite)
		wantStatus(err, 403)
		_, err = client.ReadResolvedConfig(ctx, scope.env, scope.config, "")
		wantStatus(err, 403)
	}
	configWrite.OperationID, configWrite.ExpectedRevision, configWrite.Content = "scoped-reference-denied", 2, "secret: '{vault.foreign.db.password}'\n"
	_, err = client.WriteConfig(ctx, "testing", "server", configWrite)
	wantStatus(err, 403)
	if _, err := store.CommitConfig(ctx, mysqlstore.ConfigCommit{OperationID: "scoped-admin-reference", Actor: actor, EnvironmentKey: "production", ConfigKey: "server", ConfigName: "server", Format: configdoc.YAML, Content: []byte(configWrite.Content)}); err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadResolvedConfig(ctx, "production", "server", "")
	wantStatus(err, 403) // Namespace checked before resolution or a conditional 304.

	var vaultRevision uint64
	fileBytes := []byte("\x00private-value-file\xff")
	for index := 1; index <= 17; index++ {
		result, err := client.WriteFile(ctx, "testing", "deployment", "files", fmt.Sprintf("file_%02d", index), configrago.FileWrite{
			RevisionWrite: configrago.RevisionWrite{OperationID: fmt.Sprintf("scoped-file-%02d", index), ExpectedRevision: vaultRevision}, Name: fmt.Sprintf("File %d", index),
			File: configrago.VaultFile{Filename: "private-filename.key", ContentType: "application/octet-stream", Bytes: fileBytes},
		})
		if err != nil {
			t.Fatalf("File upload %d: %v", index, err)
		}
		vaultRevision = result.Revision
	}
	file, err := client.ReadFile(ctx, "testing", "deployment", "files", "file_17", "")
	if err != nil || !bytes.Equal(file.Bytes, fileBytes) {
		t.Fatalf("File round trip: %v", err)
	}
	_, err = client.ReadFile(ctx, "testing", "foreign", "files", "file_17", file.ETag)
	wantStatus(err, 403)
	_, err = client.WriteFile(ctx, "testing", "foreign", "files", "file_17", configrago.FileWrite{RevisionWrite: configrago.RevisionWrite{OperationID: "scoped-file-forbidden"}})
	wantStatus(err, 403)
	if result, err := client.DeleteVaultField(ctx, "testing", "deployment", "files", "file_01", configrago.RevisionWrite{OperationID: "scoped-field-delete", ExpectedRevision: vaultRevision}); err != nil || result.Revision != 18 {
		t.Fatalf("Field delete: %v", err)
	}
	_, err = client.ReadFile(ctx, "testing", "deployment", "files", "file_01", "")
	wantStatus(err, 404)
	if _, err := client.DeleteVaultItem(ctx, "testing", "deployment", "files", configrago.RevisionWrite{OperationID: "scoped-item-delete", ExpectedRevision: 18}); err != nil {
		t.Fatal(err)
	}
	if history, err := store.ListVaultRevisions(ctx, "deployment", "files", mysqlstore.RevisionQuery{Limit: 50}); err != nil || len(history.Items) != 18 {
		t.Fatal("Vault history was not retained")
	}
	_, err = client.ReadFile(ctx, "testing", "deployment", "files", "file_17", file.ETag)
	wantStatus(err, 404)

	foreignValue := "private-value-foreign"
	if _, err := store.CommitVault(ctx, mysqlstore.VaultCommit{OperationID: "scoped-shared-create", Actor: actor, NamespaceKey: "deployment", ItemKey: "shared", ItemName: "Shared",
		Snapshot: vaultdoc.Snapshot{Fields: []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}}, Variants: []vaultdoc.Variant{{Environments: []string{"testing", "foreign"}, Values: map[string]vaultdoc.Value{"password": {Text: &foreignValue}}}}},
	}); err != nil {
		t.Fatal(err)
	}
	localValue := "private-value-local"
	sharedWrite := configrago.VaultFieldWrite{RevisionWrite: configrago.RevisionWrite{OperationID: "scoped-shared-update", ExpectedRevision: 1}, Name: "Password", Type: "secret", Value: configrago.VaultValue{Text: &localValue}}
	if _, err := client.WriteVaultField(ctx, "testing", "deployment", "shared", "password", sharedWrite); err != nil {
		t.Fatal(err)
	}
	shared, err := store.ReadVaultItem(ctx, "deployment", "shared", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range shared.Snapshot.Variants {
		for _, environment := range variant.Environments {
			if environment == "foreign" && *variant.Values["password"].Text != foreignValue {
				t.Fatal("scoped write changed a foreign Variant")
			}
		}
	}
	view, err := client.ReadVaultItem(ctx, "testing", "deployment", "shared")
	if err != nil || len(view.Snapshot.Variants) != 1 || len(view.Snapshot.Variants[0].Environments) != 1 || *view.Snapshot.Variants[0].Values["password"].Text != localValue {
		t.Fatal("Vault read returned outside the selected Environment")
	}
	sharedWrite.OperationID, sharedWrite.ExpectedRevision = "scoped-shared-schema-denied", 2
	_, err = client.WriteVaultField(ctx, "testing", "deployment", "shared", "new_field", sharedWrite)
	wantStatus(err, 403)
	_, err = client.DeleteVaultItem(ctx, "testing", "deployment", "shared", configrago.RevisionWrite{OperationID: "scoped-shared-delete-denied", ExpectedRevision: 2})
	wantStatus(err, 403)

	deploymentRequest := configrago.DeploymentCredentialIssue{OperationID: "scoped-deploy-issue", DisplayName: "Deployment host", AuthorityID: ca.Authority.ID, ExpiresAt: time.Now().Add(time.Hour)}
	deployment, err := client.IssueDeploymentCredential(ctx, "testing", deploymentRequest)
	if err != nil || deployment.Token.Kind != machine.TokenReadOnly || len(deployment.Token.ConfigKeys) != 1 || deployment.Token.ConfigKeys[0] != "server" || len(deployment.Token.NamespaceKeys) != 1 {
		t.Fatalf("deployment issuance: %v", err)
	}
	deploymentIdentity, _ := scopedBundle(t, deployment.Certificate.ExportBundle)
	reader := clientFor(deployment.Token.Token, &deploymentIdentity)
	if _, err := reader.ReadResolvedConfig(ctx, "testing", "server", ""); err != nil {
		t.Fatal(err)
	}
	_, err = clientFor(deployment.Token.Token, &identity).ReadResolvedConfig(ctx, "testing", "server", "")
	wantStatus(err, 401)
	_, err = reader.ReadResolvedConfig(ctx, "testing", "other", "")
	wantStatus(err, 403)
	_, err = reader.WriteConfig(ctx, "testing", "server", configWrite)
	wantStatus(err, 403)
	_, err = reader.IssueDeploymentCredential(ctx, "testing", deploymentRequest)
	wantStatus(err, 403)
	_, err = client.IssueDeploymentCredential(ctx, "foreign", deploymentRequest)
	wantStatus(err, 403)
	replay, err := client.IssueDeploymentCredential(ctx, "testing", deploymentRequest)
	if err != nil || replay.Token.PublicID != deployment.Token.PublicID || replay.Token.Token != "" || replay.Certificate.ExportBundle != "" {
		t.Fatal("deployment replay exposed one-time credentials")
	}
	if response := adminRequest("PATCH", "/v1/api-tokens/"+deployment.Token.PublicID+"/environments", "scoped-deploy-expand-denied", map[string]any{"add": []string{"foreign"}}, true); response.Code != 422 {
		t.Fatal("deployment scope could be expanded")
	}
	if err := client.RevokeDeploymentCredential(ctx, "testing", deployment.Token.PublicID, "scoped-deploy-revoke"); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ReadResolvedConfig(ctx, "testing", "server", "")
	wantStatus(err, 401)
	fingerprint := sha256.Sum256(deploymentIdentity.Certificate[0])
	if active, err := store.IsCertificateActive(ctx, fingerprint); err != nil || active {
		t.Fatal("deployment certificate was not revoked")
	}

	// Execute the shipped operator against this HTTPS/mTLS fixture with 17 files.
	directory := t.TempDir()
	paths := map[string][]byte{"token": []byte(writer.Token), "client.crt": controlFiles["client.crt"], "client.key": controlFiles["client.key"], "server.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), "server.yaml": []byte("operator: true\n")}
	for name, data := range paths {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"run", "./examples/operator", "-environment", "testing", "-namespace", "deployment", "-authority", ca.Authority.ID, "-config", filepath.Join(directory, "server.yaml"), "-output", filepath.Join(directory, "identity")}
	for index := 1; index <= 17; index++ {
		path := filepath.Join(directory, fmt.Sprintf("secret-%02d.key", index))
		if err := os.WriteFile(path, fileBytes, 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	for _, environment := range []string{"testing", "production"} {
		args[3] = environment
		args[11] = filepath.Join(directory, "identity-"+environment)
		command := exec.CommandContext(ctx, "go", args...)
		command.Dir = "../../configra-go"
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "CONFIGRA_") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "CONFIGRA_URL="+server.URL, "CONFIGRA_TOKEN_FILE="+filepath.Join(directory, "token"), "CONFIGRA_CLIENT_CERT="+filepath.Join(directory, "client.crt"), "CONFIGRA_CLIENT_KEY="+filepath.Join(directory, "client.key"), "CONFIGRA_SERVER_CA="+filepath.Join(directory, "server.crt"))
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("operator example failed: %s", output)
		} else if bytes.Contains(output, []byte(writer.Token)) || bytes.Contains(output, fileBytes) {
			t.Fatal("example logged secret material")
		}
		for _, name := range []string{"token", "client.zip"} {
			if info, err := os.Stat(filepath.Join(directory, "identity-"+environment, name)); err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("example credential file permissions")
			}
		}
	}

	// Forbidden management capabilities are never routed to human handlers.
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{identity}}}
	defer transport.CloseIdleConnections()
	for _, path := range []string{"/v1/environments", "/v1/api-tokens", "/v1/users", "/v1/certificate-authorities", "/v1/master-key/export"} {
		request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+path, strings.NewReader(`{"value":"private-value-request"}`))
		request.Header.Set("Authorization", "Bearer "+writer.Token)
		response, err := (&http.Client{Transport: transport}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 403 || bytes.Contains(body, []byte("private-value")) {
			t.Fatal("forbidden capability accepted or leaked input")
		}
	}
	expiring := issueWriter("scoped-expiring-create", time.Now().Add(500*time.Millisecond), []string{"deployment"})
	expiringClient := clientFor(expiring.Token, &identity)
	time.Sleep(time.Until(*expiring.ExpiresAt) + time.Millisecond)
	_, err = expiringClient.WriteConfig(ctx, "testing", "server", configWrite)
	wantStatus(err, 401)
	_, err = expiringClient.ReadResolvedConfig(ctx, "testing", "server", "")
	wantStatus(err, 401)
	deploymentRequest.OperationID = "scoped-deploy-cascade"
	cascade, err := client.IssueDeploymentCredential(ctx, "testing", deploymentRequest)
	if err != nil {
		t.Fatal(err)
	}
	cascadeIdentity, _ := scopedBundle(t, cascade.Certificate.ExportBundle)
	cascadeReader := clientFor(cascade.Token.Token, &cascadeIdentity)
	if response := adminRequest("POST", "/v1/api-tokens/"+writer.PublicID+"/revoke", "scoped-writer-revoke", nil, true); response.Code != 200 {
		t.Fatalf("writer revoke HTTP %d", response.Code)
	}
	_, err = client.WriteConfig(ctx, "testing", "server", configWrite)
	wantStatus(err, 401)
	_, err = cascadeReader.ReadResolvedConfig(ctx, "testing", "server", "")
	wantStatus(err, 401)

	for {
		count, err := logworker.DeliverAuditOnce(ctx, store, logs)
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			break
		}
	}
	page, err := logs.ListAudits(ctx, logstore.AuditQuery{Limit: 500, Search: writer.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	updates := 0
	for _, record := range page.Items {
		if record.ActorType == "token" {
			ip := net.ParseIP(record.SourceIP)
			if record.ActorID != writer.PublicID || ip == nil || !ip.IsLoopback() || record.Time.IsZero() {
				t.Fatal("invalid Token audit identity/time/peer IP")
			}
			actions[record.Action]++
			if record.Action == "vault.put_field" || record.Action == "vault.delete_field" {
				if record.FieldKey == "" {
					t.Fatal("field audit omitted the object Field key")
				}
			}
			if record.OperationID == "scoped-config-update" {
				updates++
			}
		}
	}
	if updates != 1 || actions["config.commit"] < 2 || actions["vault.put_field"] < 17 || actions["vault.delete_field"] != 1 || actions["vault.delete_item"] < 1 || actions["deployment.issue"] < 2 || actions["deployment.revoke"] != 1 {
		t.Fatalf("missing or duplicate mutation audits: %v", actions)
	}
	encoded, _ := json.Marshal(page)
	for _, marker := range []string{writer.Token, deployment.Token.Token, "private-value", "private-filename", "PRIVATE KEY", control.ExportBundle, deployment.Certificate.ExportBundle} {
		if bytes.Contains(encoded, []byte(marker)) {
			t.Fatal("Audit exposed secret material")
		}
	}
	access, err := logs.ListAccess(ctx, 500)
	if err != nil {
		t.Fatal(err)
	}
	observed := false
	for _, record := range access {
		if record.Principal == writer.PublicID && record.Authentication == machine.AuthenticationMTLS {
			observed = true
		}
	}
	if !observed {
		t.Fatal("real NATS-to-ClickHouse Access pipeline did not observe the writer")
	}
	response := adminRequest("GET", "/v1/audit?q="+writer.PublicID, "", nil, true)
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"source_ip"`)) {
		t.Fatal("Management audit query omitted scoped writes/IP")
	}
}

func scopedBundle(t *testing.T, encoded string) (tls.Certificate, map[string][]byte) {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal("invalid certificate bundle")
	}
	defer clear(data)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal("invalid certificate archive")
	}
	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name], err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	identity, err := tls.X509KeyPair(files["client.crt"], files["client.key"])
	if err != nil {
		t.Fatal("invalid exported client identity")
	}
	return identity, files
}
