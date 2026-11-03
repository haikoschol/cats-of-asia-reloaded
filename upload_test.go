package main

import (
	"io"
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

func TestUploadPage(t *testing.T) {
	app, userID := newAuthTestApplication(t)
	t.Chdir(t.TempDir())
	handler := app.routes()
	for _, path := range []string{"/", "/favorites", "/nearme", "/login"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `href="/upload"`) {
			t.Fatalf("anonymous page %s exposed the upload navigation", path)
		}
	}
	cookies := authTestLogin(t, app)
	request := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		return r
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request(http.MethodGet, "/upload"))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("authenticated upload page failed: status %d", w.Code)
	}
	for _, content := range []string{
		`<title>Upload | Cats of Asia</title>`, `class="upload-layout"`,
		`href="/upload" aria-current="page"`, `class="logout-form"`,
		`id="upload-files" type="file" accept="image/jpeg,.jpg,.jpeg" multiple`,
		`id="upload-status" role="status"`, `id="upload-list"`, `id="upload-clear"`,
		`/static/css/upload.css`, `/static/js/upload.js`, `10 MB`, `<noscript>`,
		`/static/js/upload-metadata.js`, `/static/vendor/exifr/exifr-lite-7.1.3.umd.js`,
	} {
		if !strings.Contains(w.Body.String(), content) {
			t.Errorf("upload page is missing %q", content)
		}
	}
	if strings.Contains(w.Body.String(), "/static/js/map.js") {
		t.Error("upload page loaded the map")
	}
	for _, path := range []string{"/", "/favorites", "/nearme"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, request(http.MethodGet, path))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `href="/upload">Upload</a>`) {
			t.Fatalf("authenticated page %s did not show the upload navigation", path)
		}
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	r, err := http.NewRequest(http.MethodHead, server.URL+"/upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatal("upload HEAD request did not return success without a body")
	}
	// Check method restrictions separately from the outer CSRF middleware.
	w = httptest.NewRecorder()
	app.upload(w, httptest.NewRequest(http.MethodPost, "/upload", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Error("upload page accepted a write method")
	}
	if _, err := app.usersDB.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request(http.MethodGet, "/upload"))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Error("a deleted user retained access to upload")
	}
}
