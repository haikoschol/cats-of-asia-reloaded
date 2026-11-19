//go:build integration

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestNearbyDatabaseError(t *testing.T) {
	db := stdlib.OpenDB(pgx.ConnConfig{})
	db.Close()
	app := &application{contentDB: db, sessionManager: scs.New()}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/nearby?latitude=0&longitude=0", nil))
	if w.Code != http.StatusInternalServerError || w.Body.String() != "Internal Server Error\n" {
		t.Fatal("database failure did not produce a generic 500 response")
	}
}
