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

func TestSessionLimitsComeFromConfigurationAndLongLifetimesPersist(t *testing.T) {
	defaults := humanauth.NewSessionManagerWithLimits(memstore.New(), humanauth.SessionLimits{})
	if defaults.IdleTimeout != 30*time.Minute || defaults.Lifetime != 8*time.Hour || defaults.Cookie.Persist {
		t.Fatalf("zero limits = idle %s, lifetime %s, persist %v", defaults.IdleTimeout, defaults.Lifetime, defaults.Cookie.Persist)
	}
	shorter := humanauth.NewSessionManagerWithLimits(memstore.New(), humanauth.SessionLimits{IdleTimeout: 10 * time.Minute, Lifetime: 4 * time.Hour})
	if shorter.IdleTimeout != 10*time.Minute || shorter.Lifetime != 4*time.Hour || shorter.Cookie.Persist {
		t.Fatalf("shorter limits = idle %s, lifetime %s, persist %v", shorter.IdleTimeout, shorter.Lifetime, shorter.Cookie.Persist)
	}
	longer := humanauth.NewSessionManagerWithLimits(memstore.New(), humanauth.SessionLimits{IdleTimeout: 2 * time.Hour, Lifetime: 72 * time.Hour})
	if longer.IdleTimeout != 2*time.Hour || longer.Lifetime != 72*time.Hour || !longer.Cookie.Persist {
		t.Fatalf("longer limits = idle %s, lifetime %s, persist %v", longer.IdleTimeout, longer.Lifetime, longer.Cookie.Persist)
	}
	if !longer.Cookie.Secure || !longer.Cookie.HttpOnly || longer.Cookie.Name != "__Host-configra_session" {
		t.Fatalf("longer limits weakened the Cookie: %#v", longer.Cookie)
	}
}
