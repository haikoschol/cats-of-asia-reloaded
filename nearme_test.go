package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestNearMePage(t *testing.T) {
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The public page and embedded assets must work without map/image settings
	// and without relying on the source checkout as the working directory.
	t.Chdir(t.TempDir())
	app := &application{contentDB: db, usersDB: db, sessionManager: scs.New()}
	server := httptest.NewServer(app.routes())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/nearme")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("expected a public HTML page, got %d", response.StatusCode)
	}
	for _, content := range []string{
		`<title>Near Me | Cats of Asia</title>`, `href="/nearme" aria-current="page"`,
		`id="nearme-status" role="status"`, `id="nearme-retry" type="button" hidden`,
		`id="nearme-grid" class="favorites-grid" aria-busy="true" hidden`,
		`/static/css/nearme.css`, `/static/css/favorites.css`,
		`/static/js/photo-links.js`, `/static/js/photo-viewer.js`, `/static/js/nearme.js`,
		`Bali, Indonesia`, `<noscript>`,
	} {
		if !strings.Contains(string(body), content) {
			t.Errorf("near me page is missing %q", content)
		}
	}
	for _, content := range []string{`/static/js/map.js`, `/static/vendor/leaflet/`, `class="logout-form"`, `coming soon`} {
		if strings.Contains(string(body), content) {
			t.Errorf("near me page unexpectedly contains %q", content)
		}
	}
	for _, dependency := range []string{"/static/js/photo-links.js", "/static/js/photo-viewer.js"} {
		if strings.Index(string(body), dependency) > strings.Index(string(body), "/static/js/nearme.js") {
			t.Errorf("near me script loaded before %s", dependency)
		}
	}

	response, err = server.Client().Head(server.URL + "/nearme")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatal("HEAD did not return a successful response without a body")
	}

	response, err = server.Client().Post(server.URL+"/nearme", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != "GET, HEAD" {
		t.Fatal("near me page did not reject a write method")
	}
}
