package humanauth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

const (
	DefaultSessionIdleTimeout = 30 * time.Minute
	DefaultSessionLifetime    = 8 * time.Hour
)

// SessionLimits come from Cold-start Configuration and are fixed for the life
// of the process (ADR-0035). Zero values keep the defaults.
type SessionLimits struct {
	IdleTimeout time.Duration
	Lifetime    time.Duration
}

func NewSessionManager(store scs.Store) *scs.SessionManager {
	return NewSessionManagerWithLimits(store, SessionLimits{})
}

func NewSessionManagerWithLimits(store scs.Store, limits SessionLimits) *scs.SessionManager {
	if store == nil {
		panic("human authentication Session Store is required")
	}
	sessions := scs.New()
	sessions.Store = store
	sessions.Lifetime = DefaultSessionLifetime
	if limits.Lifetime > 0 {
		sessions.Lifetime = limits.Lifetime
	}
	sessions.IdleTimeout = DefaultSessionIdleTimeout
	if limits.IdleTimeout > 0 {
		sessions.IdleTimeout = limits.IdleTimeout
	}
	sessions.HashTokenInStore = true
	sessions.Cookie.Name = "__Host-configra_session"
	sessions.Cookie.Domain = ""
	sessions.Cookie.Path = "/"
	sessions.Cookie.Secure = true
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	// The default 8-hour session ends with the browser. A longer configured
	// lifetime keeps the cookie across browser restarts until the session expires.
	sessions.Cookie.Persist = sessions.Lifetime > DefaultSessionLifetime
	return sessions
}
