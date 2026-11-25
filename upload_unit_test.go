//go:build !integration

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadAnonymous(t *testing.T) {
	app := &application{sessionManager: newSessionManager(config{})}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/upload", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatal("anonymous upload visitor was not redirected to login")
	}
	if strings.Contains(w.Body.String(), "upload-files") {
		t.Fatal("anonymous response exposed the upload page")
	}
}
