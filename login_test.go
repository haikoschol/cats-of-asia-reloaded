package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alexedwards/scs/postgresstore"
)

func usePostgresTestSessions(t *testing.T, app *application) {
	t.Helper()
	// Connection-local tables keep these tests independent of other sessions.
	if _, err := app.usersDB.Exec("CREATE TEMP TABLE sessions (LIKE public.sessions INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	app.sessionManager.Store = postgresstore.NewWithCleanupInterval(app.usersDB, 0)
}

func TestLoginCredentials(t *testing.T) {
	for _, tt := range []struct {
		name, username, password string
		status                   int
		message                  string
	}{
		{name: "correct", username: "protected-action-user", password: "test-password", status: http.StatusSeeOther},
		{name: "wrong password", username: "protected-action-user", password: "incorrect-password", status: http.StatusUnauthorized,
			message: "Username or password is incorrect."},
		{name: "unknown user", username: "nonexistent-user", password: "test-password", status: http.StatusUnauthorized,
			message: "Username or password is incorrect."},
		{name: "missing username", password: "test-password", status: http.StatusUnprocessableEntity,
			message: "Username and password are required."},
		{name: "missing password", username: "protected-action-user", status: http.StatusUnprocessableEntity,
			message: "Username and password are required."},
		{name: "missing both", status: http.StatusUnprocessableEntity, message: "Username and password are required."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newAuthTestApplication(t)
			usePostgresTestSessions(t, app)
			handler := app.routes()
			token, cookies := csrfTestForm(t, handler, "/login", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, csrfTestPost("/login", url.Values{
				"username": {tt.username}, "password": {tt.password}, "csrf_token": {token},
			}, cookies))
			if w.Code != tt.status {
				t.Fatalf("wanted status %d, got %d", tt.status, w.Code)
			}
			if tt.status == http.StatusSeeOther {
				if w.Header().Get("Location") != "/" {
					t.Fatal("successful login did not redirect to the homepage")
				}
				cookies = w.Result().Cookies()
			} else {
				if w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), tt.message) {
					t.Fatal("incorrect login did not display the expected form error")
				}
				if tt.password != "" && strings.Contains(w.Body.String(), tt.password) {
					t.Fatal("login error reflected the submitted password")
				}
			}

			// Check actual access, not just the login response or a session cookie.
			request := httptest.NewRequest(http.MethodGet, "/api/test-protected", nil)
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			called := false
			guard := app.requireLoginJSON(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			w = httptest.NewRecorder()
			app.sessionManager.LoadAndSave(guard).ServeHTTP(w, request)
			if tt.status == http.StatusSeeOther {
				if w.Code != http.StatusNoContent || !called {
					t.Fatal("correct credentials did not grant access")
				}
				request = httptest.NewRequest(http.MethodGet, "/login", nil)
				for _, cookie := range cookies {
					request.AddCookie(cookie)
				}
				w = httptest.NewRecorder()
				handler.ServeHTTP(w, request)
				if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
					t.Fatal("visiting login after signing in did not redirect home")
				}
			} else if w.Code != http.StatusUnauthorized || called {
				t.Fatal("incorrect credentials granted access")
			}
		})
	}
}

func TestLoginRetryAfterIncorrectPassword(t *testing.T) {
	app, _ := newAuthTestApplication(t)
	usePostgresTestSessions(t, app)
	handler := app.routes()
	token, cookies := csrfTestForm(t, handler, "/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/login", url.Values{
		"username": {"protected-action-user"}, "password": {"incorrect-password"}, "csrf_token": {token},
	}, cookies))
	match := csrfInputPattern.FindStringSubmatch(w.Body.String())
	if w.Code != http.StatusUnauthorized || len(match) != 2 {
		t.Fatal("incorrect login did not return a usable form")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/login", url.Values{
		"username": {"protected-action-user"}, "password": {"test-password"}, "csrf_token": {match[1]},
	}, cookies))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatal("correcting the password did not allow login")
	}
}

func TestStoredSessionInvalidation(t *testing.T) {
	for _, tt := range []struct{ name, invalidate string }{
		{"expired", "UPDATE sessions SET expiry = CURRENT_TIMESTAMP - INTERVAL '1 second'"},
		{"revoked", "DELETE FROM sessions"},
		{"deleted user", "DELETE FROM users"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newAuthTestApplication(t)
			usePostgresTestSessions(t, app)
			cookies := authTestLogin(t, app)
			var count int
			if err := app.usersDB.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 1 {
				t.Fatalf("login did not persist its session: count %d, error %v", count, err)
			}
			if _, err := app.usersDB.Exec(tt.invalidate); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"HTML", "JSON"} {
				t.Run(format, func(t *testing.T) {
					called := false
					action := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
					guard := app.requireLogin(action)
					status := http.StatusSeeOther
					if format == "JSON" {
						guard = app.requireLoginJSON(action)
						status = http.StatusUnauthorized
					}
					r := httptest.NewRequest(http.MethodGet, "/test-protected", nil)
					for _, cookie := range cookies {
						r.AddCookie(cookie)
					}
					w := httptest.NewRecorder()
					app.sessionManager.LoadAndSave(guard).ServeHTTP(w, r)
					if w.Code != status || called {
						t.Fatal("invalidated session retained access")
					}
					if format == "HTML" && w.Header().Get("Location") != "/login" {
						t.Fatal("invalidated browser session did not redirect to login")
					}
				})
			}
			// The stale browser cookie must not prevent signing in again.
			csrfTestForm(t, app.routes(), "/login", cookies)
		})
	}
}
