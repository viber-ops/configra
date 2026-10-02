package machine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"
)

const maxResolvedConfigBytes = 5 << 20

var (
	ErrNotFound    = errors.New("not found")
	ErrNotModified = errors.New("not modified")
	ErrUnresolved  = errors.New("unresolved vault reference")
	ErrIntegrity   = errors.New("cryptographic integrity failure")
	ErrForbidden   = errors.New("scope forbidden")
)

const (
	TokenReadOnly    = "read-only"
	TokenWriteScoped = "write-scoped"
)

type Token struct {
	PublicID               string
	ParentPublicID         string
	SecretDigest           [sha256.Size]byte
	EnvironmentGranted     bool // For the Environment passed to TokenForEnvironment.
	AllowWithoutMTLS       bool
	ExpiresAt              time.Time
	Revoked                bool
	Kind                   string
	ConfigKeys             []string // nil permits all; an empty list permits none.
	NamespaceKeys          []string
	CertificateFingerprint []byte // Deployment credentials are bound to their issued certificate.
}

func (token Token) AllowsConfig(key string) bool {
	return token.ConfigKeys == nil || slices.Contains(token.ConfigKeys, key)
}
func (token Token) AllowsNamespace(key string) bool {
	return token.NamespaceKeys == nil || slices.Contains(token.NamespaceKeys, key)
}

type tokenContextKey struct{}

func WithToken(ctx context.Context, token Token) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}
func TokenFromContext(ctx context.Context) (Token, bool) {
	token, ok := ctx.Value(tokenContextKey{}).(Token)
	return token, ok
}

type ResolvedConfig struct {
	Format         string            `json:"format"`
	Content        string            `json:"content"`
	ConfigRevision uint64            `json:"config_revision"`
	VaultRevisions map[string]uint64 `json:"vault_revisions"`
	ETag           string            `json:"-"`
}

type FileContent struct {
	Bytes         []byte
	Filename      string
	ContentType   string
	VaultRevision uint64
	ETag          string
}

type Repository interface {
	TokenForEnvironment(context.Context, string, string) (Token, error)
	IsCertificateActive(context.Context, [sha256.Size]byte) (bool, error)
	ReadResolvedConfig(context.Context, string, string, string) (ResolvedConfig, error)
	ReadFile(context.Context, string, string, string, string, string) (FileContent, error)
	ReadRelease(context.Context, string, string, string, string) (ReleaseBundle, error)
	ReadReleaseState(context.Context, string, string) (ReleaseState, error)
}

type Authentication string

const (
	AuthenticationMTLS      Authentication = "mtls"
	AuthenticationTokenOnly Authentication = "token_only"
	AuthenticationOIDC      Authentication = "oidc"
)

type AccessEvent struct {
	Time           time.Time
	Principal      string
	Authentication Authentication
	Environment    string
	Namespace      string
	ResourceType   string
	Resource       string
	ConfigRevision uint64
	VaultRevisions map[string]uint64
}

// AccessPublisher must return immediately; a false result means the event was dropped.
type AccessPublisher interface {
	TryPublish(AccessEvent) bool
}

func NewHandler(repository Repository, publisher AccessPublisher) http.Handler {
	if publisher == nil {
		publisher = discardAccessPublisher{}
	}
	server := &server{repository: repository, publisher: publisher, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/environments/{environment}/identity", server.readIdentity)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}", server.readResolvedConfig)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/release", server.readRelease)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/release-state", server.readRelease)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/releases/{release}", server.readRelease)
	mux.HandleFunc("GET /v1/environments/{environment}/vault-items/{namespace}/{item}/fields/{field}/content", server.readFile)
	return mux
}

func (server *server) readIdentity(response http.ResponseWriter, request *http.Request) {
	environment := request.PathValue("environment")
	if !validResourceKey(environment) {
		writeError(response, 400, "invalid_resource_key")
		return
	}
	token, authentication, status := server.authenticate(request, environment)
	if status != 0 {
		writeError(response, status, statusCode(status))
		return
	}
	var expires, certificateExpiry *time.Time
	if !token.ExpiresAt.IsZero() {
		value := token.ExpiresAt
		expires = &value
	}
	fingerprint := ""
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate := request.TLS.PeerCertificates[0]
		value := certificate.NotAfter.UTC()
		certificateExpiry = &value
		digest := sha256.Sum256(certificate.Raw)
		fingerprint = hex.EncodeToString(digest[:])
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(struct {
		PublicID               string         `json:"public_id"`
		ParentPublicID         string         `json:"parent_public_id,omitempty"`
		Kind                   string         `json:"kind"`
		Environment            string         `json:"environment"`
		ConfigKeys             []string       `json:"config_keys"`
		NamespaceKeys          []string       `json:"namespace_keys"`
		ExpiresAt              *time.Time     `json:"expires_at"`
		Authentication         Authentication `json:"authentication"`
		CertificateFingerprint string         `json:"certificate_fingerprint,omitempty"`
		CertificateExpiresAt   *time.Time     `json:"certificate_expires_at,omitempty"`
	}{token.PublicID, token.ParentPublicID, token.Kind, environment, token.ConfigKeys, token.NamespaceKeys, expires, authentication, fingerprint, certificateExpiry})
}

func (server *server) readFile(response http.ResponseWriter, request *http.Request) {
	environment := request.PathValue("environment")
	namespace := request.PathValue("namespace")
	item := request.PathValue("item")
	field := request.PathValue("field")
	if !validResourceKey(environment) || !validResourceKey(namespace) || !validResourceKey(item) || !validResourceKey(field) {
		writeError(response, http.StatusBadRequest, "invalid_resource_key")
		return
	}

	token, authentication, status := server.authenticate(request, environment)
	if status != 0 {
		writeError(response, status, statusCode(status))
		return
	}
	if !token.AllowsNamespace(namespace) {
		writeError(response, http.StatusForbidden, "scope_forbidden")
		return
	}

	file, err := server.repository.ReadFile(
		request.Context(), environment, namespace, item, field, request.Header.Get("If-None-Match"),
	)
	switch {
	case errors.Is(err, ErrNotModified):
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("ETag", file.ETag)
		response.WriteHeader(http.StatusNotModified)
		return
	case errors.Is(err, ErrNotFound):
		writeError(response, http.StatusNotFound, "not_found")
		return
	case errors.Is(err, ErrIntegrity):
		writeError(response, http.StatusInternalServerError, "crypto_integrity_failure")
		return
	case err != nil:
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	case len(file.Bytes) > maxResolvedConfigBytes:
		writeError(response, http.StatusInternalServerError, "file_too_large")
		return
	}

	contentType, _, err := mime.ParseMediaType(file.ContentType)
	if err != nil {
		contentType = "application/octet-stream"
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
	response.Header().Set("ETag", file.ETag)
	response.WriteHeader(http.StatusOK)
	written, writeErr := response.Write(file.Bytes)
	if writeErr == nil && written == len(file.Bytes) {
		server.publisher.TryPublish(AccessEvent{
			Time:           server.now().UTC(),
			Principal:      tokenPublicID(request.Header.Get("Authorization")),
			Authentication: authentication,
			Environment:    environment,
			Namespace:      namespace,
			ResourceType:   "vault_file",
			Resource:       item + "." + field,
			VaultRevisions: map[string]uint64{namespace + "." + item: file.VaultRevision},
		})
	}
}

type server struct {
	repository Repository
	publisher  AccessPublisher
	now        func() time.Time
}

func (server *server) readResolvedConfig(response http.ResponseWriter, request *http.Request) {
	environment := request.PathValue("environment")
	config := request.PathValue("config")
	if !validResourceKey(environment) || !validResourceKey(config) {
		writeError(response, http.StatusBadRequest, "invalid_resource_key")
		return
	}

	token, authentication, status := server.authenticate(request, environment)
	if status != 0 {
		writeError(response, status, statusCode(status))
		return
	}
	if !token.AllowsConfig(config) {
		writeError(response, http.StatusForbidden, "scope_forbidden")
		return
	}

	resolved, err := server.repository.ReadResolvedConfig(
		WithToken(request.Context(), token), environment, config, request.Header.Get("If-None-Match"),
	)
	switch {
	case errors.Is(err, ErrForbidden):
		writeError(response, http.StatusForbidden, "scope_forbidden")
		return
	case errors.Is(err, ErrNotModified):
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("ETag", resolved.ETag)
		response.WriteHeader(http.StatusNotModified)
		return
	case errors.Is(err, ErrNotFound):
		writeError(response, http.StatusNotFound, "not_found")
		return
	case errors.Is(err, ErrUnresolved):
		writeError(response, http.StatusUnprocessableEntity, "unresolved_vault_reference")
		return
	case errors.Is(err, ErrIntegrity):
		writeError(response, http.StatusInternalServerError, "crypto_integrity_failure")
		return
	case err != nil:
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	case len(resolved.Content) > maxResolvedConfigBytes:
		writeError(response, http.StatusInternalServerError, "resolved_config_too_large")
		return
	}

	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("ETag", resolved.ETag)
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(resolved); err == nil {
		server.publisher.TryPublish(AccessEvent{
			Time:           server.now().UTC(),
			Principal:      tokenPublicID(request.Header.Get("Authorization")),
			Authentication: authentication,
			Environment:    environment,
			ResourceType:   "config",
			Resource:       config,
			ConfigRevision: resolved.ConfigRevision,
			VaultRevisions: maps.Clone(resolved.VaultRevisions),
		})
	}
}

// Authenticate validates both the bearer credential and the presented, registered mTLS identity.
func Authenticate(repository Repository, request *http.Request, environment string) (Token, Authentication, int) {
	return (&server{repository: repository, now: time.Now}).authenticate(request, environment)
}

func (server *server) authenticate(request *http.Request, environment string) (Token, Authentication, int) {
	publicID, secret, ok := bearerToken(request.Header.Get("Authorization"))
	if !ok {
		return Token{}, "", http.StatusUnauthorized
	}
	token, err := server.repository.TokenForEnvironment(request.Context(), publicID, environment)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Token{}, "", http.StatusUnauthorized
		}
		return Token{}, "", http.StatusServiceUnavailable
	}
	presentedDigest := sha256.Sum256(secret)
	if token.PublicID != publicID ||
		subtle.ConstantTimeCompare(presentedDigest[:], token.SecretDigest[:]) != 1 ||
		token.Revoked ||
		(!token.ExpiresAt.IsZero() && !server.now().Before(token.ExpiresAt)) {
		return Token{}, "", http.StatusUnauthorized
	}
	if request.TLS == nil {
		return Token{}, "", http.StatusUnauthorized
	}
	if len(request.TLS.PeerCertificates) == 0 {
		if token.AllowWithoutMTLS && token.Kind != TokenWriteScoped && len(token.CertificateFingerprint) == 0 {
			if !token.EnvironmentGranted {
				return token, "", http.StatusForbidden
			}
			return token, AuthenticationTokenOnly, 0
		}
		return Token{}, "", http.StatusUnauthorized
	}
	fingerprint := sha256.Sum256(request.TLS.PeerCertificates[0].Raw)
	if len(token.CertificateFingerprint) > 0 && subtle.ConstantTimeCompare(fingerprint[:], token.CertificateFingerprint) != 1 {
		return Token{}, "", http.StatusUnauthorized
	}
	active, err := server.repository.IsCertificateActive(request.Context(), fingerprint)
	if err != nil {
		return Token{}, "", http.StatusServiceUnavailable
	}
	if !active {
		return Token{}, "", http.StatusUnauthorized
	}
	if !token.EnvironmentGranted {
		return token, "", http.StatusForbidden
	}
	return token, AuthenticationMTLS, 0
}

func bearerToken(header string) (string, []byte, bool) {
	const prefix = "Bearer cfg_"
	if !strings.HasPrefix(header, prefix) || strings.ContainsAny(header, "\r\n") {
		return "", nil, false
	}
	publicID, encodedSecret, found := strings.Cut(strings.TrimPrefix(header, prefix), "_")
	if !found || !validPublicID(publicID) || encodedSecret == "" || strings.Contains(encodedSecret, "=") {
		return "", nil, false
	}
	secret, err := base64.RawURLEncoding.DecodeString(encodedSecret)
	if err != nil || len(secret) != 32 || base64.RawURLEncoding.EncodeToString(secret) != encodedSecret {
		return "", nil, false
	}
	return publicID, secret, true
}

func tokenPublicID(header string) string {
	publicID, _, _ := bearerToken(header)
	return publicID
}

func validPublicID(value string) bool {
	if len(value) < 6 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validResourceKey(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func statusCode(status int) string {
	if status == http.StatusForbidden {
		return "environment_forbidden"
	}
	if status == http.StatusServiceUnavailable {
		return "service_unavailable"
	}
	return "unauthorized"
}

func writeError(response http.ResponseWriter, status int, code string) {
	requestIDBytes := make([]byte, 12)
	_, _ = rand.Read(requestIDBytes)
	body := struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}{}
	body.Error.Code = code
	body.Error.RequestID = hex.EncodeToString(requestIDBytes)
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

type discardAccessPublisher struct{}

func (discardAccessPublisher) TryPublish(AccessEvent) bool { return false }
