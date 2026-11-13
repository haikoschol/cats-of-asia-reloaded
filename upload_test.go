package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPhotoUploadURLRoute(t *testing.T) {
	for _, mode := range []string{"success", "anonymous", "deleted user", "missing CSRF", "foreign origin", "GET", "missing configuration", "provider error", "provider timeout"} {
		t.Run(mode, func(t *testing.T) {
			app, userID := newAuthTestApplication(t)
			app.config.ImageAccountID = strings.Repeat("a", 32)
			app.config.ImageAPIKey = "fake-private-cloudflare-key"
			calls := 0
			binding := ""
			app.cloudflareImagesClient = &http.Client{Transport: cloudflareRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if mode == "provider error" {
					return nil, errors.New("fake-private-cloudflare-key")
				}
				if mode == "provider timeout" {
					return imageTestResponse(504, "fake-private-cloudflare-key"), nil
				}
				if err := r.ParseMultipartForm(4096); err != nil {
					t.Fatal(err)
				}
				defer r.MultipartForm.RemoveAll()
				var meta map[string]string
				if err := json.Unmarshal([]byte(r.FormValue("metadata")), &meta); err != nil {
					t.Fatal(err)
				}
				binding = meta["uploadSession"]
				return imageTestResponse(200, imageTestEnvelope(map[string]string{"id": testUploadImageID, "uploadURL": testImageUploadURL})), nil
			})}
			handler := app.routes()
			var cookies []*http.Cookie
			var csrf string
			if mode == "anonymous" {
				csrf, cookies = csrfTestForm(t, handler, "/login", nil)
			} else {
				cookies = authTestLogin(t, app)
				csrf, cookies = csrfTestForm(t, handler, "/", cookies)
			}
			method := http.MethodPost
			want := http.StatusCreated
			switch mode {
			case "anonymous":
				want = http.StatusUnauthorized
			case "deleted user":
				if _, err := app.usersDB.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
					t.Fatal(err)
				}
				want = http.StatusUnauthorized
			case "missing CSRF", "foreign origin":
				want = http.StatusForbidden
			case "GET":
				method = http.MethodGet
				want = http.StatusMethodNotAllowed
			case "missing configuration":
				app.config.ImageAPIKey = ""
				want = http.StatusServiceUnavailable
			case "provider error":
				want = http.StatusBadGateway
			case "provider timeout":
				want = http.StatusGatewayTimeout
			}
			r := httptest.NewRequest(method, "/api/photos/upload-url", nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			if mode != "missing CSRF" {
				r.Header.Set("X-CSRF-Token", csrf)
			}
			if mode == "foreign origin" {
				r.Header.Set("Origin", "https://foreign.example")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected upload response: status %d", w.Code)
			}
			wantCalls := 0
			if mode == "success" || mode == "provider error" || mode == "provider timeout" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatal("protected upload unexpectedly reached image service")
			}
			if mode == "GET" && w.Header().Get("Allow") != http.MethodPost {
				t.Fatal("missing allowed method")
			}
			if mode == "success" {
				var result imageUploadURL
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.ID != testUploadImageID || result.UploadURL != testImageUploadURL || result.ExpiresAt.IsZero() || len(binding) != 64 {
					t.Fatal("upload response is incomplete")
				}
			}
			for _, secret := range []string{"fake-private-cloudflare-key", strings.Repeat("a", 32), binding} {
				if secret != "" && strings.Contains(w.Body.String(), secret) {
					t.Fatal("upload response exposed private provider data")
				}
			}
			for _, cookie := range cookies {
				if strings.Contains(w.Body.String(), cookie.Value) {
					t.Fatal("upload response exposed a session cookie")
				}
			}
		})
	}
}

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
		`id="upload-location-form"`, `name="city"`, `name="country"`, `name="cityCandidate"`,
		`id="upload-help-save"`, `aria-describedby="upload-help-save" disabled>Upload</button>`,
		`/static/js/upload-transfer.js`, `class="upload-transfer-status upload-file-status"`, `class="upload-map-link" hidden`,
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
