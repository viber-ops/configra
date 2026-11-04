package humanauth_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/viber-ops/configra/internal/humanauth"
)

func TestNewSessionManagerUsesProductionSecurityDefaults(t *testing.T) {
	store := memstore.New()
	sessions := humanauth.NewSessionManager(store)
	if sessions.Store != store {
		t.Fatal("Session Manager did not use the supplied persistent Store")
	}
	if sessions.Lifetime != 8*time.Hour || sessions.IdleTimeout != 30*time.Minute || !sessions.HashTokenInStore {
		t.Fatalf("Session expiry settings = lifetime %s, idle %s, hash %v", sessions.Lifetime, sessions.IdleTimeout, sessions.HashTokenInStore)
	}
	if sessions.Cookie.Name != "__Host-configra_session" || sessions.Cookie.Domain != "" || sessions.Cookie.Path != "/" {
		t.Fatalf("Session Cookie scope = %#v", sessions.Cookie)
	}
	if !sessions.Cookie.Secure || !sessions.Cookie.HttpOnly || sessions.Cookie.Persist || sessions.Cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("Session Cookie security = %#v", sessions.Cookie)
	}
}

func TestApplySessionSettingsSupportsConfiguredPresets(t *testing.T) {
	store := memstore.New()
	sessions := humanauth.NewSessionManager(store)
	if !humanauth.ApplySessionSettings(sessions, humanauth.SessionSettings{IdleTimeout: "24h", Lifetime: "7d"}) {
		t.Fatal("expected 24h / 7d session settings to be accepted")
	}
	if sessions.IdleTimeout != 24*time.Hour || sessions.Lifetime != 7*24*time.Hour || !sessions.Cookie.Persist {
		t.Fatalf("Session settings = idle %s, lifetime %s, persist %v", sessions.IdleTimeout, sessions.Lifetime, sessions.Cookie.Persist)
	}
	if !humanauth.ApplySessionSettings(sessions, humanauth.SessionSettings{IdleTimeout: "7d", Lifetime: "30d"}) {
		t.Fatal("expected 7d / 30d session settings to be accepted")
	}
	if sessions.IdleTimeout != 7*24*time.Hour || sessions.Lifetime != 30*24*time.Hour || !sessions.Cookie.Persist {
		t.Fatalf("Session settings = idle %s, lifetime %s, persist %v", sessions.IdleTimeout, sessions.Lifetime, sessions.Cookie.Persist)
	}
	if humanauth.ApplySessionSettings(sessions, humanauth.SessionSettings{IdleTimeout: "invalid", Lifetime: "7d"}) {
		t.Fatal("expected invalid idle timeout to be rejected")
	}
}

