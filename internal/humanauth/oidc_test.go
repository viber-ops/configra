package humanauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"

	"github.com/viber-ops/configra/internal/humanauth"
)

func TestOIDCLoginUsesDiscoveryStateNoncePKCEAndServerSession(t *testing.T) {
	provider := newDiscoveryServer(t)
	sessions := humanauth.NewSessionManager(memstore.New())
	authentication, err := humanauth.NewOIDC(context.Background(), humanauth.OIDCConfig{
		Issuer:                provider.URL,
		ClientID:              "configra-client",
		ClientSecret:          "oidc-client-secret-sentinel",
		RedirectURL:           "https://configra.test/auth/callback",
		AllowInsecureLoopback: true,
		RoleSource:            humanauth.RoleSourceIDToken,
		RoleClaim:             "groups",
		ViewerValues:          []string{"developers"},
		AdminValues:           []string{"configra-admins"},
	}, sessions)
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}
	handler := authentication.Handler(http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/login?return_to=/configs", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body = %s", response.Code, response.Body)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if location.Scheme+"://"+location.Host != provider.URL || location.Path != "/authorize" {
		t.Fatalf("authorization endpoint = %s", location)
	}
	query := location.Query()
	for key, want := range map[string]string{
		"client_id":             "configra-client",
		"redirect_uri":          "https://configra.test/auth/callback",
		"response_type":         "code",
		"code_challenge_method": "S256",
	} {
		if query.Get(key) != want {
			t.Fatalf("%s = %q, want %q", key, query.Get(key), want)
		}
	}
	if query.Get("state") == "" || query.Get("nonce") == "" || query.Get("state") == query.Get("nonce") || query.Get("code_challenge") == "" {
		t.Fatalf("state/nonce/PKCE query = %v", query)
	}
	if query.Get("code_verifier") != "" {
		t.Fatal("authorization redirect leaked PKCE verifier")
	}
	if !strings.Contains(" "+query.Get("scope")+" ", " openid ") {
		t.Fatalf("scope = %q, want openid", query.Get("scope"))
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v, want one Session Cookie", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-configra_session" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Value == "" {
		t.Fatalf("Session Cookie = %#v", cookie)
	}
	if strings.Contains(response.Header().Get("Location"), "oidc-client-secret-sentinel") {
		t.Fatal("authorization redirect leaked Client Secret")
	}
}

func TestOIDCCallbackVerifiesTokenMapsRoleAndRenewsSession(t *testing.T) {
	provider := newSigningOIDCProvider(t)
	sessions := humanauth.NewSessionManager(memstore.New())
	authentication, err := humanauth.NewOIDC(context.Background(), humanauth.OIDCConfig{
		Issuer:                provider.server.URL,
		ClientID:              "configra-client",
		ClientSecret:          "oidc-client-secret-sentinel",
		RedirectURL:           "https://configra.test/auth/callback",
		AllowInsecureLoopback: true,
		RoleSource:            humanauth.RoleSourceIDToken,
		RoleClaim:             "groups",
		ViewerValues:          []string{"developers"},
		AdminValues:           []string{"configra-admins"},
	}, sessions)
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}
	next := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal, ok := humanauth.PrincipalFromContext(request.Context())
		if !ok {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(response, "%s|%s|%s", principal.Issuer, principal.Subject, principal.Role)
	})
	handler := authentication.Handler(next)

	loginRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/login?return_to=/configs", nil)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	location, err := url.Parse(loginResponse.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse login Location: %v", err)
	}
	provider.nonce = location.Query().Get("nonce")
	provider.challenge = location.Query().Get("code_challenge")
	oldCookie := loginResponse.Result().Cookies()[0]

	callbackRequest := httptest.NewRequest(
		http.MethodGet,
		"https://configra.test/auth/callback?code=valid-code&state="+url.QueryEscape(location.Query().Get("state")),
		nil,
	)
	callbackRequest.AddCookie(oldCookie)
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusSeeOther || callbackResponse.Header().Get("Location") != "/configs" {
		t.Fatalf("callback = %d %q; body = %s", callbackResponse.Code, callbackResponse.Header().Get("Location"), callbackResponse.Body)
	}
	newCookies := callbackResponse.Result().Cookies()
	if len(newCookies) != 1 || newCookies[0].Value == "" || newCookies[0].Value == oldCookie.Value {
		t.Fatalf("Session was not renewed: old %#v, new %#v", oldCookie, newCookies)
	}
	newCookie := newCookies[0]

	principalRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/me", nil)
	principalRequest.AddCookie(newCookie)
	principalResponse := httptest.NewRecorder()
	handler.ServeHTTP(principalResponse, principalRequest)
	if principalResponse.Code != http.StatusOK || principalResponse.Body.String() != provider.server.URL+"|user-1|admin" {
		t.Fatalf("authenticated principal = %d %q", principalResponse.Code, principalResponse.Body.String())
	}

	oldRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/me", nil)
	oldRequest.AddCookie(oldCookie)
	oldResponse := httptest.NewRecorder()
	handler.ServeHTTP(oldResponse, oldRequest)
	if oldResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old pre-login Session status = %d, want 401", oldResponse.Code)
	}
}

func TestOIDCRejectsRequestsForAnUnexpectedPublicHost(t *testing.T) {
	provider := newDiscoveryServer(t)
	authentication, err := humanauth.NewOIDC(context.Background(), humanauth.OIDCConfig{
		Issuer:                provider.URL,
		ClientID:              "configra-client",
		ClientSecret:          "oidc-client-secret-sentinel",
		RedirectURL:           "https://configra.test/auth/callback",
		AllowInsecureLoopback: true,
		RoleSource:            humanauth.RoleSourceIDToken,
		RoleClaim:             "groups",
		ViewerValues:          []string{"developers"},
		AdminValues:           []string{"configra-admins"},
	}, humanauth.NewSessionManager(memstore.New()))
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "https://unexpected.test/auth/login", nil)
	response := httptest.NewRecorder()
	authentication.Handler(http.NotFoundHandler()).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || response.Header().Get("Location") != "" || len(response.Result().Cookies()) != 0 {
		t.Fatalf("unexpected Host response = %d, Location %q, Cookies %#v", response.Code, response.Header().Get("Location"), response.Result().Cookies())
	}
}

func TestOIDCLogoutIsSameOriginPOSTAndDestroysSession(t *testing.T) {
	provider := newDiscoveryServer(t)
	sessions := humanauth.NewSessionManager(memstore.New())
	authentication, err := humanauth.NewOIDC(context.Background(), humanauth.OIDCConfig{
		Issuer:                provider.URL,
		ClientID:              "configra-client",
		ClientSecret:          "oidc-client-secret-sentinel",
		RedirectURL:           "https://configra.test/auth/callback",
		AllowInsecureLoopback: true,
		RoleSource:            humanauth.RoleSourceIDToken,
		RoleClaim:             "groups",
		ViewerValues:          []string{"developers"},
		AdminValues:           []string{"configra-admins"},
	}, sessions)
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}

	seed := sessions.LoadAndSave(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		sessions.Put(request.Context(), "probe", "present")
	}))
	seedRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/seed", nil)
	seedResponse := httptest.NewRecorder()
	seed.ServeHTTP(seedResponse, seedRequest)
	cookie := seedResponse.Result().Cookies()[0]
	handler := authentication.Handler(http.NotFoundHandler())

	crossOriginRequest := httptest.NewRequest(http.MethodPost, "https://configra.test/auth/logout", nil)
	crossOriginRequest.Header.Set("Origin", "https://attacker.test")
	crossOriginRequest.AddCookie(cookie)
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOriginRequest)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin logout status = %d, want 403", crossOriginResponse.Code)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "https://configra.test/auth/logout", nil)
	logoutRequest.Header.Set("Origin", "https://configra.test")
	logoutRequest.AddCookie(cookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusSeeOther || logoutResponse.Header().Get("Location") != "/" {
		t.Fatalf("logout = %d %q; body = %s", logoutResponse.Code, logoutResponse.Header().Get("Location"), logoutResponse.Body)
	}
	logoutCookies := logoutResponse.Result().Cookies()
	if len(logoutCookies) != 1 || logoutCookies[0].MaxAge >= 0 {
		t.Fatalf("logout Cookie = %#v, want expired Cookie", logoutCookies)
	}

	probe := ""
	inspect := sessions.LoadAndSave(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		probe = sessions.GetString(request.Context(), "probe")
	}))
	inspectRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/inspect", nil)
	inspectRequest.AddCookie(cookie)
	inspect.ServeHTTP(httptest.NewRecorder(), inspectRequest)
	if probe != "" {
		t.Fatalf("destroyed Session probe = %q", probe)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/logout", nil)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET logout status = %d, want 405", getResponse.Code)
	}
}

func TestOIDCCallbackConsumesStateBeforeCallingTheProvider(t *testing.T) {
	provider := newSigningOIDCProvider(t)
	authentication := newTestOIDC(t, provider, humanauth.RoleSourceIDToken)
	handler := authentication.Handler(http.NotFoundHandler())
	location, cookie := beginOIDCLogin(t, handler)
	provider.nonce = location.Query().Get("nonce")
	provider.challenge = location.Query().Get("code_challenge")

	wrong := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/callback?code=valid-code&state=state-sentinel", nil)
	wrong.AddCookie(cookie)
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusUnauthorized || provider.tokenRequests != 0 {
		t.Fatalf("wrong state = %d with %d token requests", wrongResponse.Code, provider.tokenRequests)
	}
	if strings.Contains(wrongResponse.Body.String(), "state-sentinel") || strings.Contains(wrongResponse.Body.String(), "valid-code") {
		t.Fatalf("wrong-state response leaked callback values: %s", wrongResponse.Body)
	}

	retry := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/callback?code=valid-code&state="+url.QueryEscape(location.Query().Get("state")), nil)
	retry.AddCookie(cookie)
	retryResponse := httptest.NewRecorder()
	handler.ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusUnauthorized || provider.tokenRequests != 0 {
		t.Fatalf("consumed state retry = %d with %d token requests", retryResponse.Code, provider.tokenRequests)
	}
}

func TestOIDCCallbackFailsClosedAtIdentityBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		roleSource humanauth.RoleSource
		configure  func(*signingOIDCProvider)
		wantStatus int
	}{
		{
			name:       "missing ID Token",
			roleSource: humanauth.RoleSourceIDToken,
			configure:  func(provider *signingOIDCProvider) { provider.omitIDToken = true },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Nonce mismatch",
			roleSource: humanauth.RoleSourceIDToken,
			configure:  func(provider *signingOIDCProvider) { provider.nonce = "wrong-nonce-sentinel" },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong Audience",
			roleSource: humanauth.RoleSourceIDToken,
			configure:  func(provider *signingOIDCProvider) { provider.audience = jwt.Audience{"another-client"} },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "multiple Audiences without authorized party",
			roleSource: humanauth.RoleSourceIDToken,
			configure: func(provider *signingOIDCProvider) {
				provider.audience = jwt.Audience{"configra-client", "another-client"}
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Role claim denied",
			roleSource: humanauth.RoleSourceIDToken,
			configure:  func(provider *signingOIDCProvider) { provider.groups = []string{"unmapped-role-sentinel"} },
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "UserInfo subject mismatch",
			roleSource: humanauth.RoleSourceUserInfo,
			configure:  func(provider *signingOIDCProvider) { provider.userInfoSubject = "different-user-sentinel" },
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newSigningOIDCProvider(t)
			authentication := newTestOIDC(t, provider, test.roleSource)
			handler := authentication.Handler(http.NotFoundHandler())
			location, cookie := beginOIDCLogin(t, handler)
			provider.nonce = location.Query().Get("nonce")
			provider.challenge = location.Query().Get("code_challenge")
			test.configure(provider)

			request := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/callback?code=valid-code&state="+url.QueryEscape(location.Query().Get("state")), nil)
			request.AddCookie(cookie)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus || provider.tokenRequests != 1 {
				t.Fatalf("callback = %d with %d token requests, want %d and one", response.Code, provider.tokenRequests, test.wantStatus)
			}
			for _, sentinel := range []string{"valid-code", "access-token-sentinel", "wrong-nonce-sentinel", "unmapped-role-sentinel", "different-user-sentinel"} {
				if strings.Contains(response.Body.String(), sentinel) {
					t.Fatalf("callback response leaked %q: %s", sentinel, response.Body)
				}
			}
		})
	}
}

func TestOIDCCallbackCanMapRoleFromMatchingUserInfo(t *testing.T) {
	provider := newSigningOIDCProvider(t)
	provider.groups = []string{"not-an-id-token-role"}
	authentication := newTestOIDC(t, provider, humanauth.RoleSourceUserInfo)
	handler := authentication.Handler(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal, ok := humanauth.PrincipalFromContext(request.Context())
		if !ok {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = response.Write([]byte(principal.Role))
	}))
	location, cookie := beginOIDCLogin(t, handler)
	provider.nonce = location.Query().Get("nonce")
	provider.challenge = location.Query().Get("code_challenge")

	callback := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/callback?code=valid-code&state="+url.QueryEscape(location.Query().Get("state")), nil)
	callback.AddCookie(cookie)
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusSeeOther || provider.userInfoRequests != 1 {
		t.Fatalf("UserInfo callback = %d with %d UserInfo requests; body = %s", callbackResponse.Code, provider.userInfoRequests, callbackResponse.Body)
	}

	request := httptest.NewRequest(http.MethodGet, "https://configra.test/v1/me", nil)
	request.AddCookie(callbackResponse.Result().Cookies()[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "admin" {
		t.Fatalf("UserInfo role response = %d %q", response.Code, response.Body.String())
	}
}

func newTestOIDC(t *testing.T, provider *signingOIDCProvider, roleSource humanauth.RoleSource) *humanauth.OIDC {
	t.Helper()
	authentication, err := humanauth.NewOIDC(context.Background(), humanauth.OIDCConfig{
		Issuer:                provider.server.URL,
		ClientID:              "configra-client",
		ClientSecret:          "oidc-client-secret-sentinel",
		RedirectURL:           "https://configra.test/auth/callback",
		AllowInsecureLoopback: true,
		RoleSource:            roleSource,
		RoleClaim:             "groups",
		ViewerValues:          []string{"developers"},
		AdminValues:           []string{"configra-admins"},
	}, humanauth.NewSessionManager(memstore.New()))
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}
	return authentication
}

func beginOIDCLogin(t *testing.T, handler http.Handler) (*url.URL, *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "https://configra.test/auth/login", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d; body = %s", response.Code, response.Body)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse login Location: %v", err)
	}
	return location, response.Result().Cookies()[0]
}

func newDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"issuer":                                server.URL,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"jwks_uri":                              server.URL + "/keys",
			"userinfo_endpoint":                     server.URL + "/userinfo",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

type signingOIDCProvider struct {
	server           *httptest.Server
	key              *rsa.PrivateKey
	nonce            string
	challenge        string
	audience         jwt.Audience
	groups           []string
	userInfoSubject  string
	userInfoGroups   []string
	omitIDToken      bool
	tokenRequests    int
	userInfoRequests int
}

func newSigningOIDCProvider(t *testing.T) *signingOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate OIDC signing key: %v", err)
	}
	provider := &signingOIDCProvider{
		key:             key,
		audience:        jwt.Audience{"configra-client"},
		groups:          []string{"configra-admins"},
		userInfoSubject: "user-1",
		userInfoGroups:  []string{"configra-admins"},
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (provider *signingOIDCProvider) serveHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(response).Encode(map[string]any{
			"issuer":                                provider.server.URL,
			"authorization_endpoint":                provider.server.URL + "/authorize",
			"token_endpoint":                        provider.server.URL + "/token",
			"jwks_uri":                              provider.server.URL + "/keys",
			"userinfo_endpoint":                     provider.server.URL + "/userinfo",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/keys":
		_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &provider.key.PublicKey,
			KeyID:     "test-key",
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}}})
	case "/token":
		provider.tokenRequests++
		_ = request.ParseForm()
		if request.Form.Get("code") != "valid-code" ||
			oauth2.S256ChallengeFromVerifier(request.Form.Get("code_verifier")) != provider.challenge {
			response.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(response).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		signer, err := jose.NewSigner(
			jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: provider.key, KeyID: "test-key"}},
			(&jose.SignerOptions{}).WithType("JWT"),
		)
		if err != nil {
			http.Error(response, "signer failure", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		idToken, err := jwt.Signed(signer).Claims(jwt.Claims{
			Issuer:   provider.server.URL,
			Subject:  "user-1",
			Audience: provider.audience,
			Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt: jwt.NewNumericDate(now),
		}).Claims(map[string]any{
			"nonce":  provider.nonce,
			"groups": provider.groups,
			"email":  "admin@example.com",
			"name":   "Admin",
		}).Serialize()
		if err != nil {
			http.Error(response, "token failure", http.StatusInternalServerError)
			return
		}
		tokenResponse := map[string]any{
			"access_token": "access-token-sentinel",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		if !provider.omitIDToken {
			tokenResponse["id_token"] = idToken
		}
		_ = json.NewEncoder(response).Encode(tokenResponse)
	case "/userinfo":
		provider.userInfoRequests++
		if request.Header.Get("Authorization") != "Bearer access-token-sentinel" {
			response.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(response).Encode(map[string]string{"error": "invalid_token"})
			return
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"sub":    provider.userInfoSubject,
			"groups": provider.userInfoGroups,
			"email":  "admin@example.com",
		})
	default:
		http.NotFound(response, request)
	}
}
