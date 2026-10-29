package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestFavoritesPage(t *testing.T) {
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sessions := scs.New()
	app := &application{contentDB: db, usersDB: db, sessionManager: sessions}
	handler := app.routes()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/favorites", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("favorites should be public and work without map configuration: %d", w.Code)
	}
	body := w.Body.String()
	for _, content := range []string{
		`<title>Favorites | Cats of Asia</title>`,
		`href="/favorites" aria-current="page"`,
		`id="favorites-grid"`, `id="favorites-status" role="status"`,
		`id="favorites-empty" hidden`, `href="/">map</a>`,
		`/static/css/favorites.css`, `/static/js/favorites-store.js`,
		`/static/js/favorites.js`, `/static/js/photo-viewer.js`,
		`Bali, Indonesia`,
	} {
		if !strings.Contains(body, content) {
			t.Errorf("favorites page did not contain %q", content)
		}
	}
	for _, content := range []string{"/static/vendor/leaflet/", "/static/js/map.js", "/static/js/photo-popup.js", `class="logout-form"`} {
		if strings.Contains(body, content) {
			t.Errorf("anonymous favorites page unexpectedly contained %q", content)
		}
	}
	if strings.Index(body, "/static/js/favorites-store.js") > strings.Index(body, "/static/js/favorites.js") ||
		strings.Index(body, "/static/js/photo-viewer.js") > strings.Index(body, "/static/js/favorites.js") {
		t.Error("favorites script loaded before its dependencies")
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/favorites", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Error("favorites page accepted a write method")
	}

	var userID int
	err = db.QueryRow("INSERT INTO users (username, hashed_password) VALUES ($1, $2) RETURNING id",
		"favorites-page-test-user", strings.Repeat("x", 60)).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = $1", userID)
	ctx, err := sessions.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(ctx, "authenticatedUserID", userID)
	w = httptest.NewRecorder()
	app.favorites(w, httptest.NewRequest(http.MethodGet, "/favorites", nil).WithContext(ctx))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `class="logout-form"`) {
		t.Error("favorites page did not retain the logged-in header")
	}
	sessions.Put(ctx, "authenticatedUserID", -1)
	w = httptest.NewRecorder()
	app.favorites(w, httptest.NewRequest(http.MethodGet, "/favorites", nil).WithContext(ctx))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `class="logout-form"`) || sessions.Exists(ctx, "authenticatedUserID") {
		t.Error("favorites page retained a deleted user's authentication")
	}
}
