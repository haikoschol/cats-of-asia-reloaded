package main

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestLocations(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := &application{db: db}
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
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

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
		db:             db,
		sessionManager: sessionManager,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	server := httptest.NewServer(sessionManager.LoadAndSave(mux))
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
	for _, content := range []string{
		`<header class="site-header">`,
		`Cats of Asia`,
		`<summary id="places-toggle">Places</summary>`,
		`<h1 id="selected-place" class="selected-place" role="status">No place selected</h1>`,
		`Bali, Indonesia`,
		`Vientiane, Laos`,
		`Kuala Lumpur, Malaysia`,
	} {
		if !strings.Contains(string(body), content) {
			t.Errorf("homepage did not contain %q", content)
		}
	}
}
