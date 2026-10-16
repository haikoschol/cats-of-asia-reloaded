package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestStaticAssets(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	handler := app.routes()
	for _, tt := range []struct {
		path, contentType, content string
		status                     int
	}{
		{path: "/static/css/style.css", contentType: "text/css", content: ".site-header", status: http.StatusOK},
		{path: "/static/js/places.js", contentType: "javascript", content: "updateSelectedPlace", status: http.StatusOK},
		{path: "/static/icons/cat.svg", contentType: "image/svg+xml", content: "<svg", status: http.StatusOK},
		{path: "/static/missing.css", status: http.StatusNotFound},
		{path: "/static/.env", status: http.StatusNotFound},
		{path: "/static/%2e%2e/.env", status: http.StatusNotFound},
	} {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if w.Code != tt.status {
				t.Fatalf("wanted status %d, got %d", tt.status, w.Code)
			}
			if tt.status == http.StatusOK && (!strings.Contains(w.Header().Get("Content-Type"), tt.contentType) || !strings.Contains(w.Body.String(), tt.content)) {
				t.Fatalf("asset has an unexpected content type or body: %s", w.Header().Get("Content-Type"))
			}
		})
	}
}
