package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alexedwards/scs/v2"
)

type unavailableSessionStore struct {
	scs.Store
	reads atomic.Int32
}

func (store *unavailableSessionStore) Find(string) ([]byte, bool, error) {
	store.reads.Add(1)
	return nil, false, errors.New("session database unavailable")
}

func TestStaticAssetsIgnoreUnavailableSessions(t *testing.T) {
	sessions := scs.New()
	store := &unavailableSessionStore{Store: sessions.Store}
	sessions.Store = store
	handler := (&application{sessionManager: sessions}).routes()

	// A signed-in browser sends its session cookie on parallel asset requests.
	// A failing session database must never replace JavaScript with a text error.
	for _, path := range []string{
		"/static/js/places.js", "/static/js/map.js", "/static/js/photo-links.js",
		"/static/js/favorites-store.js", "/static/css/style.css",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				request := httptest.NewRequest(method, path, nil)
				request.AddCookie(&http.Cookie{Name: sessions.Cookie.Name, Value: "test-session-token"})
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				wantType := "javascript"
				if strings.HasSuffix(path, ".css") {
					wantType = "text/css"
				}
				if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), wantType) {
					t.Fatalf("%s asset returned %d (%s)", method, response.Code, response.Header().Get("Content-Type"))
				}
				if len(response.Result().Cookies()) != 0 {
					t.Fatal("static request changed the login session")
				}
			}
		})
	}
	t.Cleanup(func() {
		if store.reads.Load() != 0 {
			t.Error("static requests accessed the session store")
		}
	})
}

func TestSessionFailureStillBlocksDynamicRoutes(t *testing.T) {
	sessions := scs.New()
	sessions.Store = &unavailableSessionStore{Store: sessions.Store}
	handler := (&application{sessionManager: sessions}).routes()
	for _, path := range []string{"/upload", "/api/photos", "/api/photos/save"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: sessions.Cookie.Name, Value: "test-session-token"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("dynamic route %s bypassed a failed session lookup", path)
		}
	}
}

func TestStaticAssets(t *testing.T) {
	t.Chdir(t.TempDir())
	app := &application{sessionManager: scs.New()}
	handler := app.routes()
	for _, tt := range []struct {
		path, contentType, content string
		status                     int
	}{
		{path: "/static/css/style.css", contentType: "text/css", content: ".site-header", status: http.StatusOK},
		{path: "/static/css/photo-popup.css", contentType: "text/css", content: ".photo-preview", status: http.StatusOK},
		{path: "/static/css/favorites.css", contentType: "text/css", content: ".favorites-grid", status: http.StatusOK},
		{path: "/static/css/nearme.css", contentType: "text/css", content: ".nearme-layout", status: http.StatusOK},
		{path: "/static/css/upload.css", contentType: "text/css", content: ".upload-layout", status: http.StatusOK},
		{path: "/static/js/places.js", contentType: "javascript", content: "updateSelectedPlace", status: http.StatusOK},
		{path: "/static/js/toast.js", contentType: "javascript", content: "showToast", status: http.StatusOK},
		{path: "/static/js/favorites-store.js", contentType: "javascript", content: "FavoriteStore", status: http.StatusOK},
		{path: "/static/js/favorites.js", contentType: "javascript", content: "loadFavorites", status: http.StatusOK},
		{path: "/static/js/nearme.js", contentType: "javascript", content: "searchNearby", status: http.StatusOK},
		{path: "/static/js/upload.js", contentType: "javascript", content: "preparePreview", status: http.StatusOK},
		{path: "/static/js/upload-metadata.js", contentType: "javascript", content: "UploadMetadata", status: http.StatusOK},
		{path: "/static/js/upload-transfer.js", contentType: "javascript", content: "UploadTransfer", status: http.StatusOK},
		{path: "/static/vendor/exifr/exifr-lite-7.1.3.umd.js", contentType: "javascript", content: "exifr", status: http.StatusOK},
		{path: "/static/js/map.js", contentType: "javascript", content: "worldCopyJump: true", status: http.StatusOK},
		{path: "/static/js/photo-popup.js", contentType: "javascript", content: "createPhotoPopup", status: http.StatusOK},
		{path: "/static/js/photo-viewer.js", contentType: "javascript", content: "openPhotoViewer", status: http.StatusOK},
		{path: "/static/js/photo-links.js", contentType: "javascript", content: "readPhotoLink", status: http.StatusOK},
		{path: "/static/vendor/mapbox/mapbox-logo.svg", contentType: "image/svg+xml", content: "<svg", status: http.StatusOK},
		{path: "/static/icons/cat.svg", contentType: "image/svg+xml", content: "<svg", status: http.StatusOK},
		{path: "/static/vendor/leaflet/leaflet.css", contentType: "text/css", content: ".leaflet-container", status: http.StatusOK},
		{path: "/static/vendor/leaflet/leaflet.js", contentType: "javascript", content: "Leaflet 1.9.4", status: http.StatusOK},
		{path: "/static/vendor/leaflet/images/marker-icon.png", contentType: "image/png", status: http.StatusOK},
		{path: "/static/missing.css", status: http.StatusNotFound},
		{path: "/static/.env", status: http.StatusNotFound},
		{path: "/static/%2e%2e/.env", status: http.StatusNotFound},
		{path: "/static/template/base.tmpl", status: http.StatusNotFound},
		{path: "/static/scripts/schema.sql", status: http.StatusNotFound},
		{path: "/static/pgconsole.toml", status: http.StatusNotFound},
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
