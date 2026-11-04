package humanauth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

const (
	DefaultIdleTimeout = "30m"
	DefaultLifetime    = "8h"
)

type SessionSettings struct {
	IdleTimeout string `json:"idle_timeout"`
	Lifetime    string `json:"lifetime"`
}

func DefaultSessionSettings() SessionSettings {
	return SessionSettings{
		IdleTimeout: DefaultIdleTimeout,
		Lifetime:    DefaultLifetime,
	}
}

func ParseSessionSettings(settings SessionSettings) (idle time.Duration, lifetime time.Duration, ok bool) {
	switch settings.IdleTimeout {
	case "", DefaultIdleTimeout:
		idle = 30 * time.Minute
	case "24h":
		idle = 24 * time.Hour
	case "7d":
		idle = 7 * 24 * time.Hour
	default:
		return 0, 0, false
	}
	switch settings.Lifetime {
	case "", DefaultLifetime:
		lifetime = 8 * time.Hour
	case "7d":
		lifetime = 7 * 24 * time.Hour
	case "30d":
		lifetime = 30 * 24 * time.Hour
	default:
		return 0, 0, false
	}
	return idle, lifetime, true
}

func NormalizeSessionSettings(settings SessionSettings) (SessionSettings, bool) {
	if settings.IdleTimeout == "" {
		settings.IdleTimeout = DefaultIdleTimeout
	}
	if settings.Lifetime == "" {
		settings.Lifetime = DefaultLifetime
	}
	_, _, ok := ParseSessionSettings(settings)
	return settings, ok
}

func ApplySessionSettings(sessions *scs.SessionManager, settings SessionSettings) bool {
	if sessions == nil {
		return false
	}
	idle, lifetime, ok := ParseSessionSettings(settings)
	if !ok {
		return false
	}
	if idle > lifetime {
		lifetime = idle
	}
	sessions.IdleTimeout = idle
	sessions.Lifetime = lifetime
	sessions.Cookie.Persist = idle > 30*time.Minute || lifetime > 8*time.Hour
	return true
}

func NewSessionManager(store scs.Store) *scs.SessionManager {
	if store == nil {
		panic("human authentication Session Store is required")
	}
	sessions := scs.New()
	sessions.Store = store
	sessions.Lifetime = 8 * time.Hour
	sessions.IdleTimeout = 30 * time.Minute
	sessions.HashTokenInStore = true
	sessions.Cookie.Name = "__Host-configra_session"
	sessions.Cookie.Domain = ""
	sessions.Cookie.Path = "/"
	sessions.Cookie.Secure = true
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Persist = false
	return sessions
}
