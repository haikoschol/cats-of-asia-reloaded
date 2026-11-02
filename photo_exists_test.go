package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const existingPhotoHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newPhotoExistsTestApp(t *testing.T) (*application, int) {
	t.Helper()
	app, userID := newAuthTestApplication(t)
	contentDB, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { contentDB.Close() })
	contentDB.SetMaxOpenConns(1)
	// Only the content connection contains the known photo. An accidental
	// lookup in usersDB would report it missing.
	for _, db := range []*sql.DB{app.usersDB, contentDB} {
		if _, err := db.Exec("CREATE TEMP TABLE photos_photo (LIKE public.photos_photo INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := contentDB.Exec(`INSERT INTO photos_photo (id, filename, sha256, timestamp)
		VALUES ('41000000-0000-4000-8000-000000000001', '/private/cat.jpeg', $1, '2026-01-02T10:30:00Z')`, existingPhotoHash); err != nil {
		t.Fatal(err)
	}
	app.contentDB = contentDB
	return app, userID
}

func TestPhotoExists(t *testing.T) {
	app, _ := newPhotoExistsTestApp(t)
	cookies := authTestLogin(t, app)
	for _, tt := range []struct {
		name, hash string
		exists     bool
	}{
		{name: "existing", hash: existingPhotoHash, exists: true},
		{name: "new", hash: strings.Repeat("f", 64)},
		{name: "uppercase", hash: strings.ToUpper(existingPhotoHash), exists: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/photos/exists?"+url.Values{"sha256": {tt.hash}}.Encode(), nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("expected JSON success, got status %d", w.Code)
			}
			var got map[string]bool
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			exists, present := got["exists"]
			if len(got) != 1 || !present || exists != tt.exists {
				t.Fatalf("unexpected existence response: %s", w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("protected response could be cached")
			}
		})
	}
}

func TestPhotoExistsInvalidRequests(t *testing.T) {
	app, _ := newPhotoExistsTestApp(t)
	cookies := authTestLogin(t, app)
	// Validating bad input must not attempt to query the photo connection.
	app.contentDB = nil
	for _, query := range []string{
		"", "sha256=", "sha256=" + strings.Repeat("a", 63), "sha256=" + strings.Repeat("a", 65),
		"sha256=" + strings.Repeat("g", 64), "sha256=+" + existingPhotoHash,
		"sha256=%ZZ", "hash=" + existingPhotoHash,
		"sha256=" + existingPhotoHash + "&sha256=" + existingPhotoHash,
		"sha256=" + existingPhotoHash + "&extra=value",
	} {
		t.Run(query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/photos/exists?"+query, nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 before photo query, got %d", w.Code)
			}
		})
	}
	r := httptest.NewRequest(http.MethodHead, "/api/photos/exists?sha256="+existingPhotoHash, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
		t.Error("unsupported method did not return 405 with Allow: GET")
	}
}

func TestPhotoExistsAnonymous(t *testing.T) {
	// Neither database is available: anonymous requests must stop at the guard.
	app := &application{sessionManager: newSessionManager(config{})}
	for _, query := range []string{"", "sha256=" + existingPhotoHash} {
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/exists?"+query, nil))
		if w.Code != http.StatusUnauthorized || w.Header().Get("Content-Type") != "application/json" ||
			w.Header().Get("Location") != "" {
			t.Fatal("anonymous request did not receive a JSON 401")
		}
	}
}

func TestPhotoExistsDeletedUser(t *testing.T) {
	app, userID := newPhotoExistsTestApp(t)
	cookies := authTestLogin(t, app)
	if _, err := app.usersDB.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
	app.contentDB = nil
	r := httptest.NewRequest(http.MethodGet, "/api/photos/exists?sha256="+existingPhotoHash, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("a deleted user retained access to the hash check")
	}
}

func TestPhotoExistsDatabaseError(t *testing.T) {
	app, _ := newPhotoExistsTestApp(t)
	cookies := authTestLogin(t, app)
	if err := app.contentDB.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/photos/exists?sha256="+existingPhotoHash, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError || w.Body.String() != "Internal Server Error\n" {
		t.Fatal("database error must not be reported as a new photo or expose details")
	}
}

func TestPhotoExistsLoginLifecycle(t *testing.T) {
	app, _ := newPhotoExistsTestApp(t)
	usePostgresTestSessions(t, app)
	server := httptest.NewServer(app.routes())
	t.Cleanup(server.Close)
	client := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	checkHash := func(hash string, status int, exists bool) {
		t.Helper()
		response, err := client.Get(server.URL + "/api/photos/exists?" + url.Values{"sha256": {hash}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != status || response.Header.Get("Location") != "" ||
			response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected hash-check response: status %d", response.StatusCode)
		}
		if status == http.StatusOK {
			var got map[string]bool
			if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			value, present := got["exists"]
			if !present || len(got) != 1 || value != exists {
				t.Fatal("hash check returned the wrong existence result")
			}
		} else if status == http.StatusUnauthorized {
			var got map[string]string
			if err := json.NewDecoder(response.Body).Decode(&got); err != nil || got["error"] != "Unauthorized" {
				t.Fatal("unauthenticated hash check did not return a JSON error")
			}
		}
	}

	checkHash(existingPhotoHash, http.StatusUnauthorized, false)
	_, token := sessionTestForm(t, client, server.URL+"/login")
	response, err := client.PostForm(server.URL+"/login", url.Values{
		"username": {"protected-action-user"}, "password": {"test-password"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatal("login failed")
	}
	checkHash(existingPhotoHash, http.StatusOK, true)
	checkHash(strings.Repeat("f", 64), http.StatusOK, false)
	checkHash("invalid-hash", http.StatusBadRequest, false)

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldCookies := jar.Cookies(endpoint)
	if len(oldCookies) == 0 {
		t.Fatal("login did not retain a session cookie")
	}
	_, token = sessionTestForm(t, client, server.URL+"/")
	response, err = client.PostForm(server.URL+"/logout", url.Values{"csrf_token": {token}})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || len(jar.Cookies(endpoint)) != 0 {
		t.Fatal("logout failed to remove the session cookie")
	}
	checkHash(existingPhotoHash, http.StatusUnauthorized, false)

	// Replay the old cookie explicitly: server-side invalidation must also work
	// when a client retains a copy of its former authenticated session.
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/photos/exists?sha256="+existingPhotoHash, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range oldCookies {
		request.AddCookie(cookie)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("the old session retained access after logout")
	}
}

func TestPhotoExistsPoolWaitCancellation(t *testing.T) {
	app, _ := newPhotoExistsTestApp(t)
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer setupCancel()
	conn, err := app.contentDB.Conn(setupCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Hold the only photo connection. A canceled request must stop waiting,
	// rather than falsely reporting that the photo does not exist.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/photos/exists?sha256="+existingPhotoHash, nil).WithContext(ctx)
	w := httptest.NewRecorder()
	start := time.Now()
	app.photoExists(w, r)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || w.Code != http.StatusInternalServerError || time.Since(start) > time.Second {
		t.Fatal("hash check did not promptly honor the caller deadline")
	}
	if w.Body.String() != "Internal Server Error\n" {
		t.Fatal("canceled query was reported as a new photo or exposed internal details")
	}
}
