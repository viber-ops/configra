//go:build integration

package mysqlstore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestClientCertificateRegistrationAllowsAnyActiveCertificateAndRevokesPermanently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_certificate_mutation")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "admin@example.com"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "operation-certificate-environment", Actor: actor, Action: EnvironmentCreate, Key: "a", DisplayName: "Environment A"}); err != nil {
		t.Fatalf("create Environment: %v", err)
	}
	if _, err := store.CommitConfig(ctx, ConfigCommit{
		OperationID: "operation-certificate-config", Actor: actor, EnvironmentKey: "a", ConfigKey: "payment", ConfigName: "Payment",
		ExpectedRevision: 0, Format: configdoc.YAML, Content: []byte("port: 6379\n"),
	}); err != nil {
		t.Fatalf("create Config: %v", err)
	}
	token, err := store.CreateToken(ctx, TokenCreate{
		OperationID: "operation-certificate-token", Actor: actor, DisplayName: "Client", EnvironmentKeys: []string{"a"}, NeverExpires: true,
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	firstCertificate := selfSignedClientCertificate(t, 1, "client-one")
	secondCertificate := selfSignedClientCertificate(t, 2, "client-two")
	first, err := store.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{
		OperationID: "operation-certificate-first", Actor: actor, DisplayName: "First", Certificate: firstCertificate,
	})
	if err != nil || first.Outcome != OutcomeSuccess || len(first.FingerprintSHA256) != 64 {
		t.Fatalf("register first Certificate = %#v, %v", first, err)
	}
	second, err := store.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{
		OperationID: "operation-certificate-second", Actor: actor, DisplayName: "Second", Certificate: secondCertificate,
	})
	if err != nil || second.Outcome != OutcomeSuccess {
		t.Fatalf("register second Certificate = %#v, %v", second, err)
	}

	handler := machine.NewHandler(store, nil)
	request := func(certificate *x509.Certificate) *httptest.ResponseRecorder {
		httpRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/environments/a/configs/payment", nil)
		httpRequest.Header.Set("Authorization", "Bearer "+token.Token)
		httpRequest.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	if response := request(firstCertificate); response.Code != http.StatusOK {
		t.Fatalf("first Certificate request = %d %s", response.Code, response.Body)
	}
	if response := request(secondCertificate); response.Code != http.StatusOK {
		t.Fatalf("second Certificate request = %d %s", response.Code, response.Body)
	}

	revoked, err := store.RevokeClientCertificate(ctx, ClientCertificateRevoke{
		OperationID: "operation-certificate-revoke", Actor: actor, FingerprintSHA256: first.FingerprintSHA256,
	})
	if err != nil || revoked.Outcome != OutcomeSuccess || !revoked.Revoked {
		t.Fatalf("RevokeClientCertificate = %#v, %v", revoked, err)
	}
	if response := request(firstCertificate); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked Certificate request = %d %s", response.Code, response.Body)
	}
	if response := request(secondCertificate); response.Code != http.StatusOK {
		t.Fatalf("other active Certificate request = %d %s", response.Code, response.Body)
	}
	if _, err := store.RegisterVerifiedClientCertificate(ctx, ClientCertificateRegister{
		OperationID: "operation-certificate-reregister", Actor: actor, DisplayName: "First", Certificate: firstCertificate,
	}); err == nil {
		t.Fatal("RegisterVerifiedClientCertificate restored a revoked Certificate")
	}
}

func selfSignedClientCertificate(t *testing.T, serial int64, commonName string) *x509.Certificate {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return certificate
}
