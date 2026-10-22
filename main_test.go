package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestLocations(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := &application{contentDB: db}
	locations, err := app.locations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []location{
		{City: "Bali", Country: "Indonesia"},
		{City: "Vientiane", Country: "Laos"},
		{City: "Kuala Lumpur", Country: "Malaysia"},
	}
	if len(locations) != len(want) {
		t.Fatalf("wanted %d places, got %d", len(want), len(locations))
	}
	for i, place := range locations {
		if place.ID <= 0 || place.City != want[i].City || place.Country != want[i].Country {
			t.Errorf("place %d: wanted %s, %s with a database ID, got %+v",
				i, want[i].City, want[i].Country, place)
		}
	}
}

func TestHome(t *testing.T) {
	privateSettings := setPrivateTestCredentials(t)
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "images.example.com")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "example-account-hash")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "pk.example-public-map-token")
	cfg, err := loadConfigFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	databaseURL := testDatabaseURL(t)

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}

	sessionManager := scs.New()
	app := &application{
		config:         cfg,
		contentDB:      db,
		usersDB:        db,
		sessionManager: sessionManager,
	}

	server := httptest.NewServer(app.routes())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("wanted status %d, got %d", http.StatusOK, response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(body), "anonymous coward") {
		t.Fatal("homepage still contained the old greeting")
	}
	for name, value := range privateSettings {
		if strings.Contains(string(body), value) {
			t.Errorf("homepage exposed the value of %s", name)
		}
	}
	if strings.Contains(string(body), databaseURL) {
		t.Error("homepage exposed the database connection URL")
	}
	for _, content := range []string{
		`<title>Cats of Asia</title>`,
		`<link rel="stylesheet" href="/static/css/style.css">`,
		`<script src="/static/js/places.js" defer></script>`,
		`<link rel="stylesheet" href="/static/vendor/leaflet/leaflet.css">`,
		`<link rel="stylesheet" href="/static/css/photo-popup.css">`,
		`<script src="/static/vendor/leaflet/leaflet.js" defer></script>`,
		`<script src="/static/js/map.js" defer></script>`,
		`<script src="/static/js/photo-popup.js" defer></script>`,
		`<script src="/static/js/photo-links.js" defer></script>`,
		`id="photo-map"`,
		`id="photo-status" role="status"`,
		`data-mapbox-token="pk.example-public-map-token"`,
		`<header class="site-header">`,
		`Cats of Asia`,
		`<summary id="places-toggle">Places</summary>`,
		`<body class="map-layout">`,
		`<h1 id="selected-place" class="selected-place visually-hidden" role="status">No place selected</h1>`,
		`id="map-status" role="status" hidden`,
		`id="photo-status" role="status" hidden`,
		`Bali, Indonesia`,
		`Vientiane, Laos`,
		`Kuala Lumpur, Malaysia`,
	} {
		if !strings.Contains(string(body), content) {
			t.Errorf("homepage did not contain %q", content)
		}
	}
	locations, err := app.locations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range locations {
		link := fmt.Sprintf(`href="/?place=%d" data-place-id="%d"`, place.ID, place.ID)
		if !strings.Contains(string(body), link) {
			t.Errorf("homepage did not link place ID %d", place.ID)
		}
	}

	for _, token := range []string{"", "sk.fake-secret-token"} {
		app.config.MapboxAccessToken = token
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Map unavailable:") {
			t.Error("missing or private map token did not produce a helpful homepage message")
		}
		if !strings.Contains(w.Body.String(), `data-mapbox-token=""`) || strings.Contains(w.Body.String(), "sk.fake-secret-token") {
			t.Error("homepage exposed an invalid or private map token")
		}
		if strings.Contains(w.Body.String(), `id="map-status" role="status" hidden`) {
			t.Error("homepage hid the missing/invalid map token message")
		}
	}
}

func TestLoginLayout(t *testing.T) {
	t.Chdir(t.TempDir())
	databaseURL := testDatabaseURL(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sessionManager := scs.New()
	app := &application{contentDB: db, usersDB: db, sessionManager: sessionManager}
	handler := app.routes()

	for _, tt := range []struct {
		name, method, form string
		status             int
	}{
		{name: "form", method: http.MethodGet, status: http.StatusOK},
		{name: "required fields", method: http.MethodPost,
			form: "username=%3Cscript%3E", status: http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/login", strings.NewReader(tt.form))
			if tt.method == http.MethodPost {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("wanted status %d, got %d: %s", tt.status, w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, content := range []string{
				`<title>Log in | Cats of Asia</title>`,
				`<meta name="viewport" content="width=device-width, initial-scale=1">`,
				`<header class="site-header">`,
				`<link rel="stylesheet" href="/static/css/style.css">`,
				`<script src="/static/js/places.js" defer></script>`,
				`Bali, Indonesia`,
				`<a class="place-option" href="/?place=`,
				`<form class="login-form" action="/login" method="post">`,
				`type="password"`,
			} {
				if !strings.Contains(body, content) {
					t.Errorf("login page did not contain %q", content)
				}
			}
			if strings.Count(body, "<!doctype html>") != 1 || strings.Contains(body, `id="selected-place"`) || strings.Contains(body, "const selectedPlace") {
				t.Error("login layout contains duplicate markup or homepage content/scripts")
			}
			for _, asset := range []string{
				"/static/vendor/leaflet/", "/static/js/map.js", "/static/js/photo-popup.js",
				"/static/js/photo-links.js", "/static/css/photo-popup.css",
			} {
				if strings.Contains(body, asset) {
					t.Errorf("login page loaded homepage-only asset %s", asset)
				}
			}
			if tt.method == http.MethodPost {
				if !strings.Contains(body, "Username and password are required.") || !strings.Contains(body, `value="&lt;script&gt;"`) {
					t.Error("form error or safely escaped username missing")
				}
			}
		})
	}
}
