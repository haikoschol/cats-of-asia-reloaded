package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticatedUserIDAnonymousAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "anonymous"},
		{name: "zero", value: 0},
		{name: "negative", value: -1},
		{name: "string", value: "1"},
		{name: "wrong numeric type", value: int64(1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sessions := scs.New()
			ctx, err := sessions.Load(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if tt.value != nil {
				sessions.Put(ctx, "authenticatedUserID", tt.value)
			}
			// No database is needed when there is no valid stored user ID.
			app := &application{sessionManager: sessions}
			userID, err := app.authenticatedUserID(ctx)
			if err != nil || userID != 0 || sessions.Exists(ctx, "authenticatedUserID") {
				t.Fatalf("invalid session was not cleared: ID %d, error %v", userID, err)
			}
		})
	}
}

func TestAuthenticatedUserIDDeletedUser(t *testing.T) {
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	var storedID int
	if err := db.QueryRow("INSERT INTO users (username, hashed_password) VALUES ($1, $2) RETURNING id",
		"session-test-user", strings.Repeat("x", 60)).Scan(&storedID); err != nil {
		t.Fatal(err)
	}

	sessions := scs.New()
	ctx, err := sessions.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(ctx, "authenticatedUserID", storedID)
	app := &application{usersDB: db, sessionManager: sessions}
	userID, err := app.authenticatedUserID(ctx)
	if err != nil || userID != storedID || !sessions.Exists(ctx, "authenticatedUserID") {
		t.Fatalf("existing user was not authenticated: ID %d, error %v", userID, err)
	}

	if _, err := db.Exec("DELETE FROM users WHERE id = $1", storedID); err != nil {
		t.Fatal(err)
	}
	userID, err = app.authenticatedUserID(ctx)
	if err != nil || userID != 0 || sessions.Exists(ctx, "authenticatedUserID") {
		t.Fatalf("deleted user remained authenticated: ID %d, error %v", userID, err)
	}
}

func TestAuthenticatedUserIDDatabaseError(t *testing.T) {
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	sessions := scs.New()
	ctx, err := sessions.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(ctx, "authenticatedUserID", 1)
	app := &application{usersDB: db, sessionManager: sessions}
	userID, err := app.authenticatedUserID(ctx)
	if err == nil || userID != 0 {
		t.Fatalf("database failure must not authenticate: ID %d, error %v", userID, err)
	}
	if sessions.GetInt(ctx, "authenticatedUserID") != 1 {
		t.Fatal("database failure discarded the session")
	}
}

func newAuthTestApplication(t *testing.T) (*application, int) {
	t.Helper()
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var userID int
	if err := db.QueryRow("INSERT INTO users (username, hashed_password) VALUES ($1, $2) RETURNING id",
		"protected-action-user", string(hash)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	return &application{usersDB: db, contentDB: db, sessionManager: newSessionManager(config{})}, userID
}

func authTestLogin(t *testing.T, app *application) []*http.Cookie {
	t.Helper()
	token, cookies := csrfTestForm(t, app.routes(), "/login", nil)
	r := csrfTestPost("/login", url.Values{
		"username": {"protected-action-user"}, "password": {"test-password"}, "csrf_token": {token},
	}, cookies)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || len(w.Result().Cookies()) == 0 {
		t.Fatalf("test login failed: status %d", w.Code)
	}
	return w.Result().Cookies()
}

func TestProtectedActions(t *testing.T) {
	for _, format := range []string{"HTML", "JSON"} {
		for _, state := range []string{"anonymous", "unknown session", "authenticated", "deleted user", "database error"} {
			t.Run(format+"/"+state, func(t *testing.T) {
				app, userID := newAuthTestApplication(t)
				var cookies []*http.Cookie
				if state != "anonymous" && state != "unknown session" {
					cookies = authTestLogin(t, app)
				}
				if state == "unknown session" {
					cookies = []*http.Cookie{{Name: app.sessionManager.Cookie.Name, Value: "invalid-session-token"}}
				}
				if state == "deleted user" {
					if _, err := app.usersDB.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
						t.Fatal(err)
					}
				}
				if state == "database error" {
					if err := app.usersDB.Close(); err != nil {
						t.Fatal(err)
					}
				}

				called := false
				action := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				})
				guard := app.requireLogin(action)
				method := http.MethodGet
				if format == "JSON" {
					guard = app.requireLoginJSON(action)
					method = http.MethodPost
				}
				r := httptest.NewRequest(method, "/protected", nil)
				for _, cookie := range cookies {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				app.sessionManager.LoadAndSave(guard).ServeHTTP(w, r)

				wantStatus := http.StatusSeeOther
				if format == "JSON" {
					wantStatus = http.StatusUnauthorized
				}
				if state == "authenticated" {
					wantStatus = http.StatusNoContent
				} else if state == "database error" {
					wantStatus = http.StatusInternalServerError
				}
				if w.Code != wantStatus || called != (state == "authenticated") {
					t.Fatalf("status %d, action called %v; wanted status %d", w.Code, called, wantStatus)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Error("protected response could be cached")
				}
				if wantStatus == http.StatusSeeOther {
					if w.Header().Get("Location") != "/login" {
						t.Error("anonymous browser request did not redirect to login")
					}
				} else if w.Header().Get("Location") != "" {
					t.Error("unexpected redirect")
				}
				if format == "JSON" && state != "authenticated" {
					var body map[string]string
					if w.Header().Get("Content-Type") != "application/json" {
						t.Error("API error did not use JSON")
					}
					if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if body["error"] != http.StatusText(wantStatus) {
						t.Fatalf("unexpected API error: %v", body)
					}
				}
			})
		}
	}
}

func TestProtectedLogout(t *testing.T) {
	app, _ := newAuthTestApplication(t)
	usePostgresTestSessions(t, app)
	handler := app.routes()
	token, anonymousCookies := csrfTestForm(t, handler, "/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/logout", url.Values{"csrf_token": {token}}, anonymousCookies))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatal("anonymous logout was not guarded")
	}
	cookies := authTestLogin(t, app)
	token, cookies = csrfTestForm(t, handler, "/", cookies)
	request := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		r.Header.Set("X-CSRF-Token", token)
		return r
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request(http.MethodGet, "/logout"))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
		t.Fatal("logout accepted GET")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request(http.MethodPost, "/logout"))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatal("authenticated logout failed")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request(http.MethodGet, "/login"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `class="login-form"`) {
		t.Fatal("logout did not invalidate the old session")
	}
}
