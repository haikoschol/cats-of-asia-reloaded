package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

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

	if !strings.Contains(string(body), "anonymous coward") {
		t.Fatal("homepage did not contain anonymous coward")
	}
}
