//go:build !integration

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
