package humanauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionLoginState      = "oidc.login.state"
	sessionLoginNonce      = "oidc.login.nonce"
	sessionLoginVerifier   = "oidc.login.verifier"
	sessionLoginCreatedAt  = "oidc.login.created_at"
	sessionLoginReturnTo   = "oidc.login.return_to"
	sessionIdentityIssuer  = "oidc.identity.issuer"
	sessionIdentitySubject = "oidc.identity.subject"
	sessionIdentityEmail   = "oidc.identity.email"
	sessionIdentityRole    = "oidc.identity.role"
)

type RoleSource string

const (
	RoleSourceIDToken  RoleSource = "id_token"
	RoleSourceUserInfo RoleSource = "userinfo"
)

type OIDCConfig struct {
	Issuer                string
	ClientID              string
	ClientSecret          string
	RedirectURL           string
	Scopes                []string
	RoleSource            RoleSource
	RoleClaim             string
	ViewerValues          []string
	AdminValues           []string
	AllowInsecureLoopback bool
}

type OIDC struct {
	sessions   *scs.SessionManager
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	oauth      oauth2.Config
	roles      RoleMapper
	roleSource RoleSource
	clientID   string
	publicHost string
	httpClient *http.Client
	now        func() time.Time
}

func NewOIDC(ctx context.Context, config OIDCConfig, sessions *scs.SessionManager) (*OIDC, error) {
	if sessions == nil || config.ClientID == "" || config.ClientSecret == "" {
		return nil, errors.New("OIDC Session Manager, Client ID, and Client Secret are required")
	}
	if err := validateOIDCURL(config.Issuer, config.AllowInsecureLoopback); err != nil {
		return nil, errors.New("invalid OIDC Issuer URL")
	}
	if err := validateOIDCURL(config.RedirectURL, config.AllowInsecureLoopback); err != nil {
		return nil, errors.New("invalid OIDC Redirect URL")
	}
	redirect, _ := url.Parse(config.RedirectURL)
	if redirect.Path != "/auth/callback" || redirect.RawQuery != "" || redirect.Fragment != "" {
		return nil, errors.New("OIDC Redirect URL must use the fixed /auth/callback path")
	}
	roles, err := NewRoleMapper(config.RoleClaim, config.ViewerValues, config.AdminValues)
	if err != nil {
		return nil, err
	}
	if config.RoleSource != RoleSourceIDToken && config.RoleSource != RoleSourceUserInfo {
		return nil, errors.New("OIDC Role Source must be id_token or userinfo")
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, httpClient), config.Issuer)
	if err != nil {
		return nil, errors.New("discover OIDC Provider")
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	for _, scope := range config.Scopes {
		if scope != "" && !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	return &OIDC{
		sessions: sessions,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		oauth: oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  config.RedirectURL,
			Scopes:       scopes,
		},
		roles:      roles,
		roleSource: config.RoleSource,
		clientID:   config.ClientID,
		publicHost: redirect.Host,
		httpClient: httpClient,
		now:        time.Now,
	}, nil
}

func (authentication *OIDC) Handler(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/login", authentication.login)
	mux.HandleFunc("GET /auth/callback", authentication.callback)
	csrf := http.NewCrossOriginProtection()
	mux.Handle("POST /auth/logout", csrf.Handler(http.HandlerFunc(authentication.logout)))
	mux.HandleFunc("/auth/logout", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", http.MethodPost)
		response.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.Handle("/", authentication.attachPrincipal(next))
	loaded := authentication.sessions.LoadAndSave(mux)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !strings.EqualFold(request.Host, authentication.publicHost) {
			writeOIDCError(response, http.StatusBadRequest, "invalid_host")
			return
		}
		loaded.ServeHTTP(response, request)
	})
}

func (authentication *OIDC) logout(response http.ResponseWriter, request *http.Request) {
	if err := authentication.sessions.Destroy(request.Context()); err != nil {
		writeOIDCError(response, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	http.Redirect(response, request, "/", http.StatusSeeOther)
}

func (authentication *OIDC) callback(response http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	state := authentication.sessions.PopString(ctx, sessionLoginState)
	nonce := authentication.sessions.PopString(ctx, sessionLoginNonce)
	verifier := authentication.sessions.PopString(ctx, sessionLoginVerifier)
	createdAt := authentication.sessions.GetInt64(ctx, sessionLoginCreatedAt)
	authentication.sessions.Remove(ctx, sessionLoginCreatedAt)
	returnTo := authentication.sessions.PopString(ctx, sessionLoginReturnTo)
	presentedState := request.URL.Query().Get("state")
	now := authentication.now().UTC()
	if state == "" || nonce == "" || verifier == "" || createdAt == 0 ||
		subtle.ConstantTimeCompare([]byte(state), []byte(presentedState)) != 1 ||
		now.Unix()-createdAt > int64(10*time.Minute/time.Second) || createdAt > now.Add(time.Minute).Unix() ||
		request.URL.Query().Get("error") != "" || request.URL.Query().Get("code") == "" {
		writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
		return
	}
	exchangeContext := oidc.ClientContext(ctx, authentication.httpClient)
	token, err := authentication.oauth.Exchange(
		exchangeContext,
		request.URL.Query().Get("code"),
		oauth2.VerifierOption(verifier),
	)
	if err != nil {
		writeOIDCError(response, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
		return
	}
	idToken, err := authentication.verifier.Verify(exchangeContext, rawIDToken)
	if err != nil || idToken.Subject == "" ||
		subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
		return
	}
	var idClaims map[string]json.RawMessage
	if err := idToken.Claims(&idClaims); err != nil || !validAuthorizedParty(idToken.Audience, idClaims, authentication.clientID) {
		writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
		return
	}
	roleClaims := idClaims
	if authentication.roleSource == RoleSourceUserInfo {
		userInfo, err := authentication.provider.UserInfo(exchangeContext, oauth2.StaticTokenSource(token))
		if err != nil || userInfo.Subject != idToken.Subject {
			writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
			return
		}
		roleClaims = nil
		if err := userInfo.Claims(&roleClaims); err != nil {
			writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
			return
		}
	}
	role, err := authentication.roles.Role(roleClaims)
	if err != nil {
		writeOIDCError(response, http.StatusForbidden, "role_denied")
		return
	}
	principal := Principal{
		Issuer:  idToken.Issuer,
		Subject: idToken.Subject,
		Email:   claimString(idClaims, "email"),
		Role:    role,
	}
	if principal.ActorID() == "" || len(principal.ActorID()) > 255 {
		writeOIDCError(response, http.StatusUnauthorized, "authentication_failed")
		return
	}
	if err := authentication.sessions.RenewToken(ctx); err != nil {
		writeOIDCError(response, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	authentication.sessions.Put(ctx, sessionIdentityIssuer, principal.Issuer)
	authentication.sessions.Put(ctx, sessionIdentitySubject, principal.Subject)
	authentication.sessions.Put(ctx, sessionIdentityEmail, principal.Email)
	authentication.sessions.Put(ctx, sessionIdentityRole, string(principal.Role))
	response.Header().Set("Cache-Control", "no-store")
	http.Redirect(response, request, safeReturnTo(returnTo), http.StatusSeeOther)
}

func (authentication *OIDC) attachPrincipal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal := Principal{
			Issuer:  authentication.sessions.GetString(request.Context(), sessionIdentityIssuer),
			Subject: authentication.sessions.GetString(request.Context(), sessionIdentitySubject),
			Email:   authentication.sessions.GetString(request.Context(), sessionIdentityEmail),
			Role:    Role(authentication.sessions.GetString(request.Context(), sessionIdentityRole)),
		}
		if principal.Subject != "" && len(principal.ActorID()) <= 255 &&
			(principal.Role == RoleViewer || principal.Role == RoleAdmin) {
			request = request.WithContext(WithPrincipal(request.Context(), principal))
		}
		next.ServeHTTP(response, request)
	})
}

func validAuthorizedParty(audience []string, claims map[string]json.RawMessage, clientID string) bool {
	if len(audience) <= 1 {
		return true
	}
	return claimString(claims, "azp") == clientID
}

func claimString(claims map[string]json.RawMessage, key string) string {
	var value string
	if err := json.Unmarshal(claims[key], &value); err != nil {
		return ""
	}
	return value
}

func writeOIDCError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{"error": map[string]string{"code": code}})
}

func (authentication *OIDC) login(response http.ResponseWriter, request *http.Request) {
	state, err := randomLoginValue()
	if err != nil {
		http.Error(response, "authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	nonce, err := randomLoginValue()
	if err != nil {
		http.Error(response, "authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	verifier := oauth2.GenerateVerifier()
	authentication.sessions.Put(request.Context(), sessionLoginState, state)
	authentication.sessions.Put(request.Context(), sessionLoginNonce, nonce)
	authentication.sessions.Put(request.Context(), sessionLoginVerifier, verifier)
	authentication.sessions.Put(request.Context(), sessionLoginCreatedAt, authentication.now().UTC().Unix())
	authentication.sessions.Put(request.Context(), sessionLoginReturnTo, safeReturnTo(request.URL.Query().Get("return_to")))
	location := authentication.oauth.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	response.Header().Set("Cache-Control", "no-store")
	http.Redirect(response, request, location, http.StatusSeeOther)
}

func randomLoginValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func safeReturnTo(value string) string {
	if value == "" {
		return "/"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
		return "/"
	}
	return parsed.RequestURI()
}

func validateOIDCURL(value string, allowInsecureLoopback bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("invalid URL")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" || !allowInsecureLoopback {
		return errors.New("HTTPS is required")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return errors.New("insecure URL is not loopback")
	}
	return nil
}
