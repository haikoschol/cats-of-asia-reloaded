package main

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

func newSessionManager(cfg config) *scs.SessionManager {
	sessions := scs.New()
	sessions.Lifetime = 12 * time.Hour
	sessions.Cookie.Secure = cfg.SessionCookieSecure
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Path = "/"
	sessions.Cookie.Domain = "" // Host-only; do not share sessions with sibling apps.
	return sessions
}
