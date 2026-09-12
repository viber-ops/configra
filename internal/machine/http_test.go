package machine_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

func TestHandlerReturnsResolvedConfigForAuthorizedEnvironmentWithRegisteredCertificate(t *testing.T) {
	const publicID = "token1"
	secretBytes := []byte("0123456789abcdef0123456789abcdef")
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	certificateDER := []byte("registered-client-certificate")
	repository := fakeRepository{
		token: machine.Token{
			PublicID:            publicID,
			SecretDigest:        sha256.Sum256(secretBytes),
			AllowedEnvironments: []string{"a"},
			ExpiresAt:           time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		certificateFingerprint: sha256.Sum256(certificateDER),
		config: machine.ResolvedConfig{
			Format:         "yaml",
			Content:        "database:\n  username: redis-user\n  password: resolved-secret\n",
			ConfigRevision: 7,
			VaultRevisions: map[string]uint64{"platform.redis": 4},
			ETag:           `"cfg-7-vault-redis-4"`,
		},
	}
	publisher := &recordingAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := machine.NewHandler(repository, publisher)
	request := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/configs/payment",
		nil,
	)
	request.Header.Set("Authorization", "Bearer cfg_"+publicID+"_"+secret)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{Raw: certificateDER}},
	}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	result := response.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	if result.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(result.Body)
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, body)
	}
	if got := result.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := result.Header.Get("ETag"); got != repository.config.ETag {
		t.Fatalf("ETag = %q, want %q", got, repository.config.ETag)
	}
	const wantBody = "{\"format\":\"yaml\",\"content\":\"database:\\n  username: redis-user\\n  password: resolved-secret\\n\",\"config_revision\":7,\"vault_revisions\":{\"platform.redis\":4}}\n"
	if got := response.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
	event := <-publisher.events
	if event.Principal != publicID || event.Environment != "a" || event.Resource != "payment" {
		t.Fatalf("access event identity = %#v", event)
	}
	if event.ConfigRevision != 7 || event.VaultRevisions["platform.redis"] != 4 {
		t.Fatalf("access event versions = %#v", event)
	}
	if event.Authentication != machine.AuthenticationMTLS {
		t.Fatalf("access event authentication = %q, want %q", event.Authentication, machine.AuthenticationMTLS)
	}
}

func TestHandlerReturnsNotModifiedWithoutPublishingAccessEvent(t *testing.T) {
	const publicID = "token1"
	secretBytes := []byte("0123456789abcdef0123456789abcdef")
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	certificateDER := []byte("registered-client-certificate")
	repository := fakeRepository{
		token: machine.Token{
			PublicID:            publicID,
			SecretDigest:        sha256.Sum256(secretBytes),
			AllowedEnvironments: []string{"a"},
			ExpiresAt:           time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		certificateFingerprint: sha256.Sum256(certificateDER),
		config:                 machine.ResolvedConfig{ETag: `"unchanged"`},
		readErr:                machine.ErrNotModified,
	}
	publisher := &recordingAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := machine.NewHandler(repository, publisher)
	request := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/configs/payment",
		nil,
	)
	request.Header.Set("Authorization", "Bearer cfg_"+publicID+"_"+secret)
	request.Header.Set("If-None-Match", repository.config.ETag)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{Raw: certificateDER}},
	}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304; body = %s", response.Code, response.Body)
	}
	if got := response.Header().Get("ETag"); got != repository.config.ETag {
		t.Fatalf("ETag = %q, want %q", got, repository.config.ETag)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
	select {
	case event := <-publisher.events:
		t.Fatalf("unexpected access event: %#v", event)
	default:
	}
}

func TestHandlerReturnsAuthorizedFileBytesWithoutLeakingThemIntoAccessEvent(t *testing.T) {
	const publicID = "token1"
	secretBytes := []byte("0123456789abcdef0123456789abcdef")
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	certificateDER := []byte("registered-client-certificate")
	repository := fakeRepository{
		token: machine.Token{
			PublicID:            publicID,
			SecretDigest:        sha256.Sum256(secretBytes),
			AllowedEnvironments: []string{"a"},
			ExpiresAt:           time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		certificateFingerprint: sha256.Sum256(certificateDER),
		file: machine.FileContent{
			Bytes:         []byte("sentinel-private-file-bytes"),
			Filename:      "server.pem",
			ContentType:   "application/x-pem-file",
			VaultRevision: 9,
			ETag:          `"vault-redis-9"`,
		},
	}
	publisher := &recordingAccessPublisher{events: make(chan machine.AccessEvent, 1)}
	handler := machine.NewHandler(repository, publisher)
	request := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/v1/environments/a/vault-items/platform/redis/fields/tls_cert/content",
		nil,
	)
	request.Header.Set("Authorization", "Bearer cfg_"+publicID+"_"+secret)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{Raw: certificateDER}},
	}
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	result := response.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	if result.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(result.Body)
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, body)
	}
	if got := response.Body.String(); got != string(repository.file.Bytes) {
		t.Fatalf("body = %q, want exact file bytes", got)
	}
	if got := result.Header.Get("Content-Type"); got != repository.file.ContentType {
		t.Fatalf("Content-Type = %q, want %q", got, repository.file.ContentType)
	}
	_, parameters, err := mime.ParseMediaType(result.Header.Get("Content-Disposition"))
	if err != nil || parameters["filename"] != repository.file.Filename {
		t.Fatalf("Content-Disposition = %q, parse error = %v", result.Header.Get("Content-Disposition"), err)
	}
	event := <-publisher.events
	if event.ResourceType != "vault_file" || event.Namespace != "platform" || event.Resource != "redis.tls_cert" {
		t.Fatalf("access event resource = %#v", event)
	}
	if event.VaultRevisions["platform.redis"] != 9 {
		t.Fatalf("access event version = %#v", event)
	}
}

func TestHandlerEnforcesTokenEnvironmentAndConditionalMTLS(t *testing.T) {
	secretBytes := []byte("0123456789abcdef0123456789abcdef")
	certificateDER := []byte("registered-client-certificate")
	base := fakeRepository{
		token: machine.Token{
			PublicID:            "token1",
			SecretDigest:        sha256.Sum256(secretBytes),
			AllowedEnvironments: []string{"a"},
			ExpiresAt:           time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		certificateFingerprint: sha256.Sum256(certificateDER),
	}
	tokenOnly := base
	tokenOnly.token.AllowWithoutMTLS = true
	expired := base
	expired.token.ExpiresAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	revoked := base
	revoked.token.Revoked = true
	tokenLookupFailure := base
	tokenLookupFailure.tokenErr = errors.New("MySQL unavailable")
	certificateLookupFailure := base
	certificateLookupFailure.certificateErr = errors.New("MySQL unavailable")
	authorization := "Bearer cfg_token1_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	wrongSecret := "Bearer cfg_token1_" + base64.RawURLEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
	validTLS := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: certificateDER}}}

	tests := []struct {
		name          string
		repository    fakeRepository
		url           string
		authorization string
		tlsState      *tls.ConnectionState
		wantStatus    int
		wantCode      string
	}{
		{"token only explicitly allowed", tokenOnly, "https://configra.test/v1/environments/a/configs/payment", authorization, &tls.ConnectionState{}, http.StatusOK, ""},
		{"presented unknown certificate is never ignored", tokenOnly, "https://configra.test/v1/environments/a/configs/payment", authorization, &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: []byte("unknown")}}}, http.StatusUnauthorized, "unauthorized"},
		{"certificate required by default", base, "https://configra.test/v1/environments/a/configs/payment", authorization, &tls.ConnectionState{}, http.StatusUnauthorized, "unauthorized"},
		{"environment must be granted", base, "https://configra.test/v1/environments/b/configs/payment", authorization, validTLS, http.StatusForbidden, "environment_forbidden"},
		{"plain HTTP is rejected", tokenOnly, "http://configra.test/v1/environments/a/configs/payment", authorization, nil, http.StatusUnauthorized, "unauthorized"},
		{"expired token is rejected", expired, "https://configra.test/v1/environments/a/configs/payment", authorization, validTLS, http.StatusUnauthorized, "unauthorized"},
		{"revoked token is rejected", revoked, "https://configra.test/v1/environments/a/configs/payment", authorization, validTLS, http.StatusUnauthorized, "unauthorized"},
		{"wrong token secret is rejected", base, "https://configra.test/v1/environments/a/configs/payment", wrongSecret, validTLS, http.StatusUnauthorized, "unauthorized"},
		{"missing token is rejected", base, "https://configra.test/v1/environments/a/configs/payment", "", validTLS, http.StatusUnauthorized, "unauthorized"},
		{"token lookup failure is unavailable", tokenLookupFailure, "https://configra.test/v1/environments/a/configs/payment", authorization, validTLS, http.StatusServiceUnavailable, "service_unavailable"},
		{"certificate lookup failure is unavailable", certificateLookupFailure, "https://configra.test/v1/environments/a/configs/payment", authorization, validTLS, http.StatusServiceUnavailable, "service_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.url, nil)
			request.Header.Set("Authorization", test.authorization)
			request.TLS = test.tlsState
			response := httptest.NewRecorder()
			machine.NewHandler(test.repository, nil).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body)
			}
			if test.wantCode != "" && !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("body = %q, want error code %q", response.Body.String(), test.wantCode)
			}
			if strings.Contains(response.Body.String(), string(secretBytes)) {
				t.Fatalf("response leaked token material: %q", response.Body.String())
			}
		})
	}
}

type fakeRepository struct {
	token                  machine.Token
	certificateFingerprint [32]byte
	config                 machine.ResolvedConfig
	readErr                error
	file                   machine.FileContent
	tokenErr               error
	certificateErr         error
}

func (repository fakeRepository) TokenByPublicID(context.Context, string) (machine.Token, error) {
	return repository.token, repository.tokenErr
}

func (repository fakeRepository) IsCertificateActive(_ context.Context, fingerprint [32]byte) (bool, error) {
	return fingerprint == repository.certificateFingerprint, repository.certificateErr
}

func (repository fakeRepository) ReadResolvedConfig(context.Context, string, string, string) (machine.ResolvedConfig, error) {
	return repository.config, repository.readErr
}

func (repository fakeRepository) ReadFile(context.Context, string, string, string, string, string) (machine.FileContent, error) {
	return repository.file, nil
}

type recordingAccessPublisher struct {
	events chan machine.AccessEvent
}

func (publisher *recordingAccessPublisher) TryPublish(event machine.AccessEvent) bool {
	publisher.events <- event
	return true
}
