package humanauth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

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
