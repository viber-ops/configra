//go:build integration

package humanauth_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/go-sql-driver/mysql"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestAdministratorRevocationInvalidatesExistingSessionsAcrossReplicas(t *testing.T) {
	rootDSN := os.Getenv("CONFIGRA_TEST_MYSQL_ROOT_DSN")
	if rootDSN == "" {
		t.Skip("real MySQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := mysql.ParseDSN(rootDSN)
	if err != nil {
		t.Fatal("invalid fixture DSN")
	}
	root, err := sql.Open("mysql", rootDSN)
	if err != nil {
		t.Fatal("open fixture")
	}
	defer root.Close()
	name := fmt.Sprintf("configra_session_%d", time.Now().UnixNano())
	if _, err := root.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer root.ExecContext(context.Background(), "DROP DATABASE "+name)
	config.DBName = name
	config.ParseTime = true
	providerKey, err := vaultcrypto.NewLocalKeyProvider(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store, err := mysqlstore.OpenManagement(ctx, config.FormatDSN(), providerKey)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sessionStore := store.NewManagementSessionStore(time.Minute)
	defer sessionStore.StopCleanup()
	provider := newSigningOIDCProvider(t)
	provider.amr = []string{"mfa"}
	provider.sid = "first-browser-session"
	var handlers []http.Handler
	for range 2 {
		auth, err := humanauth.NewOIDC(ctx, humanauth.OIDCConfig{Issuer: provider.server.URL, ClientID: "configra-client", ClientSecret: "test-client-secret", RedirectURL: "https://configra.test/auth/callback", AllowInsecureLoopback: true, RoleSource: humanauth.RoleSourceIDToken, RoleClaim: "groups", ViewerValues: []string{"developers"}, AdminValues: []string{"configra-admins"}, SessionAuthority: store}, humanauth.NewSessionManager(sessionStore))
		if err != nil {
			t.Fatal(err)
		}
		handlers = append(handlers, auth.Handler(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if _, ok := humanauth.PrincipalFromContext(request.Context()); !ok {
				response.WriteHeader(401)
				return
			}
			response.WriteHeader(204)
		})))
	}
	login := func() *http.Cookie {
		location, cookie := beginOIDCLogin(t, handlers[0])
		provider.nonce = location.Query().Get("nonce")
		provider.challenge = location.Query().Get("code_challenge")
		request := httptest.NewRequest("GET", "https://configra.test/auth/callback?code=valid-code&state="+url.QueryEscape(location.Query().Get("state")), nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handlers[0].ServeHTTP(response, request)
		if response.Code != 303 {
			t.Fatalf("login HTTP %d", response.Code)
		}
		return response.Result().Cookies()[0]
	}
	cookie := login()
	spareCookie := login() // Remains unused while the identity is blocked.
	check := func(want int) {
		t.Helper()
		for _, handler := range handlers {
			request := httptest.NewRequest("GET", "https://configra.test/v1/me", nil)
			request.AddCookie(cookie)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("replica returned %d, want %d", response.Code, want)
			}
		}
	}
	check(204)
	var logoutExtras map[string]any
	logoutToken := func(audience string) string {
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: provider.key, KeyID: "test-key"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		claims := map[string]any{"iss": provider.server.URL, "aud": []string{audience}, "iat": time.Now().Unix(), "jti": "logout-first-browser", "sid": provider.sid, "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}, "private": "private-logout-sentinel"}
		for key, value := range logoutExtras {
			claims[key] = value
		}
		token, err := jwt.Signed(signer).Claims(claims).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	postLogout := func(token string, want int) {
		t.Helper()
		request := httptest.NewRequest("POST", "https://configra.test/auth/backchannel-logout", strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handlers[1].ServeHTTP(response, request)
		if response.Code != want || strings.Contains(response.Body.String(), "private-logout-sentinel") {
			t.Fatalf("logout HTTP %d, want %d", response.Code, want)
		}
	}
	postLogout(logoutToken("different-client"), 400)
	check(204)
	for _, extras := range []map[string]any{{"nonce": "private-nonce"}, {"iss": provider.server.URL + "/different"}, {"iat": time.Now().Add(-time.Hour).Unix()}, {"exp": time.Now().Add(-time.Minute).Unix()}, {"nbf": time.Now().Add(time.Hour).Unix()}, {"events": map[string]any{}}, {"jti": ""}} {
		logoutExtras = extras
		postLogout(logoutToken("configra-client"), 400)
		check(204)
	}
	logoutExtras = nil
	validLogout := logoutToken("configra-client")
	separator := strings.LastIndex(validLogout, ".") + 1
	byteValue := "A"
	if validLogout[separator] == 'A' {
		byteValue = "B"
	}
	postLogout(validLogout[:separator]+byteValue+validLogout[separator+1:], 400)
	check(204)
	postLogout(validLogout, 200)
	check(401)
	// Same OP session can authenticate afresh after the policy timestamp; a replay
	// of the old signed notice must not kill this later authenticated session.
	time.Sleep(1100 * time.Millisecond)
	cookie = login()
	spareCookie = login()
	check(204)
	postLogout(validLogout, 200)
	check(204)
	change := mysqlstore.SessionChange{OperationID: "session-emergency-block", Actor: mysqlstore.Actor{Type: "user", ID: "security-admin"}, Issuer: provider.server.URL, Subject: "user-1", Action: "block"}
	if _, err := store.ChangeSessions(ctx, change); err != nil {
		t.Fatal(err)
	}
	check(401)
	change.OperationID = "session-unblock"
	change.Action = "unblock"
	if _, err := store.ChangeSessions(ctx, change); err != nil {
		t.Fatal(err)
	}
	cookie = spareCookie
	check(401) // Unblocking must not resurrect even a cookie never used while blocked.
}
