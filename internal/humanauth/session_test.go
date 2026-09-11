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
