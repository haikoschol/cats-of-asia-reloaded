package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// This HTTP substitute is used only by the integration test. No real provider
// address is contacted, and completed images require the exact fixture bytes.
type uploadIntegrationImages struct {
	mu                                       sync.Mutex
	scenario, filename, binding              string
	bytes                                    []byte
	creates, transfers, verifications, saves int
	complete                                 bool
}

func (images *uploadIntegrationImages) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	images.mu.Lock()
	defer images.mu.Unlock()
	if r.Header.Get("Cookie") != "" || r.Header.Get("X-CSRF-Token") != "" {
		t.Error("application credentials leaked to the substitute provider")
		http.Error(w, "unexpected application credentials", 400)
		return
	}
	respond := func(result any) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, imageTestEnvelope(result))
	}
	const apiPrefix = "/client/v4/accounts/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/images/"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/maps/api/geocode/json":
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"OK","results":[{"address_components":[{"long_name":"Thailand","types":["country"]},{"long_name":"Bangkok","types":["locality"]}]}]}`)
	case r.Method == http.MethodPost && r.URL.Path == apiPrefix+"v2/direct_upload":
		if r.Header.Get("Authorization") != "Bearer fake-private-cloudflare-key" {
			t.Error("missing provider authorization")
			http.Error(w, "missing provider authorization", 403)
			return
		}
		if err := r.ParseMultipartForm(4096); err != nil {
			t.Error("invalid draft request")
			http.Error(w, "invalid draft request", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		var metadata map[string]string
		if err := json.Unmarshal([]byte(r.FormValue("metadata")), &metadata); err != nil || len(metadata["uploadSession"]) != 64 {
			t.Error("draft request lacks its private session binding")
			http.Error(w, "missing session binding", 400)
			return
		}
		images.creates++
		images.binding = metadata["uploadSession"]
		respond(map[string]string{"id": testUploadImageID, "uploadURL": testImageUploadURL})
	case r.Method == http.MethodPost && r.URL.Path == "/direct/"+testUploadImageID:
		if r.Header.Get("Authorization") != "" || images.creates != 1 {
			t.Error("image transfer must use the issued draft without provider credentials")
			http.Error(w, "invalid image transfer", 403)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error("invalid image multipart")
			http.Error(w, "invalid image multipart", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error("image multipart lacks the file field")
			http.Error(w, "missing file", 400)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil || header.Filename != images.filename || !bytes.Equal(data, images.bytes) ||
			len(r.MultipartForm.File) != 1 || len(r.MultipartForm.Value) != 0 {
			t.Error("image bytes, filename, or multipart fields changed during transfer")
			http.Error(w, "invalid image bytes", 400)
			return
		}
		images.transfers++
		if images.scenario == "incomplete-image" && images.transfers == 1 {
			http.Error(w, "simulated image-transfer failure", 503)
			return
		}
		images.complete = true
		if images.scenario == "lost-image-response" {
			dropUploadIntegrationResponse(t, w)
			return
		}
		respond(map[string]string{"id": testUploadImageID})
	case r.Method == http.MethodGet && r.URL.Path == apiPrefix+"v1/"+testUploadImageID:
		images.verifications++
		if r.Header.Get("Authorization") != "Bearer fake-private-cloudflare-key" || images.creates != 1 {
			t.Error("verification must authenticate and use the issued draft")
			http.Error(w, "invalid verification", 403)
			return
		}
		respond(map[string]any{"id": testUploadImageID, "draft": !images.complete,
			"uploaded": "2026-06-15T07:30:45Z", "meta": map[string]string{"uploadSession": images.binding}})
	default:
		t.Error("unexpected provider request")
		http.Error(w, "unexpected provider request", 404)
	}
}

func dropUploadIntegrationResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	connection, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Error("could not simulate a lost response")
		return
	}
	connection.Close()
}

func TestUploadIntegration(t *testing.T) {
	for _, tt := range []struct {
		name, filename   string
		transfers, saves int
	}{
		{"success", "complete.jpg", 1, 1},
		{"database-failure", "numeric-key.jpg", 1, 2},
		{"lost-save-response", "below-sea-level.jpg", 1, 2},
		{"lost-image-response", "complete.jpg", 1, 1},
		{"incomplete-image", "complete.jpg", 2, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _, _ := newUploadSaveTestApp(t)
			fixture, err := os.ReadFile("testdata/upload/" + tt.filename)
			if err != nil {
				t.Fatal(err)
			}
			images := &uploadIntegrationImages{scenario: tt.name, filename: tt.filename, bytes: fixture}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { images.serve(t, w, r) }))
			defer provider.Close()
			providerURL, err := url.Parse(provider.URL)
			if err != nil {
				t.Fatal(err)
			}
			providerClient := provider.Client()
			providerClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			client := &http.Client{Transport: cloudflareRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || (r.URL.Host != "api.cloudflare.com" && r.URL.Host != "maps.googleapis.com") {
					return nil, errors.New("unexpected provider destination")
				}
				request := r.Clone(r.Context())
				request.URL.Scheme, request.URL.Host = providerURL.Scheme, providerURL.Host
				return providerClient.Do(request)
			})}
			app.cloudflareImagesClient = client
			app.googleGeocodingClient = client
			app.config.GoogleGeocodingEnabled = true
			app.config.GoogleMapsAPIKey = "fake-private-google-key"
			routes := app.routes()
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/photos/save" {
					routes.ServeHTTP(w, r)
					return
				}
				images.mu.Lock()
				images.saves++
				firstSave := images.saves == 1
				images.mu.Unlock()
				if !firstSave || (tt.name != "database-failure" && tt.name != "lost-save-response") {
					routes.ServeHTTP(w, r)
					return
				}
				if tt.name == "database-failure" {
					// Fail the final table write, after the other inserts took place.
					if _, err := app.contentDB.Exec("ALTER TABLE photos_rawmetadata ADD CONSTRAINT integration_save_failure CHECK(false)"); err != nil {
						t.Error("could not prepare transaction failure")
						http.Error(w, "test fixture failure", 500)
						return
					}
				}
				recorder := httptest.NewRecorder()
				routes.ServeHTTP(recorder, r)
				if tt.name == "lost-save-response" {
					if recorder.Code != http.StatusCreated {
						t.Error("save must commit before its response is lost")
					}
					dropUploadIntegrationResponse(t, w)
					return
				}
				if recorder.Code != http.StatusInternalServerError {
					t.Error("simulated final-write failure did not reject the save")
				}
				for _, table := range []string{"photos_location", "photos_coordinates", "photos_photo", "photos_rawmetadata"} {
					var count int
					if err := app.contentDB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
						t.Errorf("failed transaction left rows in %s", table)
					}
				}
				if _, err := app.contentDB.Exec("ALTER TABLE photos_rawmetadata DROP CONSTRAINT integration_save_failure"); err != nil {
					t.Error("could not clear simulated transaction failure")
				}
				for name, values := range recorder.Header() {
					w.Header()[name] = values
				}
				w.WriteHeader(recorder.Code)
				w.Write(recorder.Body.Bytes())
			}))
			defer backend.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "node", "scripts/upload-integration.cjs", backend.URL, provider.URL, tt.name, tt.filename)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			output, err := command.Output()
			if err != nil {
				t.Fatalf("JavaScript/Go upload integration failed: %v\n%s", err, stderr.String())
			}
			var result struct {
				ID, SHA256 string
				Altitude   float64
				Raw        json.RawMessage
			}
			if err := json.Unmarshal(output, &result); err != nil || result.ID != testUploadImageID || result.SHA256 != fmt.Sprintf("%x", sha256.Sum256(fixture)) {
				t.Fatal("integration client returned incorrect image identity")
			}
			assertUploadCounts(t, app.contentDB, 1, 1, 1, 1)
			var correct bool
			if err := app.contentDB.QueryRow(`SELECT p.filename=$1 AND p.timestamp=$2::timestamptz
				AND c.altitude=$3 AND l.tzoffset=-420 AND raw.metadata=$4::jsonb
				FROM photos_photo p JOIN photos_coordinates c ON c.id=p.coordinates_id
				JOIN photos_location l ON l.id=c.location_id JOIN photos_rawmetadata raw ON raw.photo_id=p.sha256`,
				tt.filename, "2026-06-15T07:30:45Z", result.Altitude, string(result.Raw)).Scan(&correct); err != nil || !correct {
				t.Fatal("image identity, capture details or raw EXIF changed during saving")
			}
			images.mu.Lock()
			defer images.mu.Unlock()
			if images.creates != 1 || images.transfers != tt.transfers || images.saves != tt.saves || images.verifications != tt.saves || !images.complete {
				t.Fatalf("unexpected provider work: creates=%d transfers=%d saves=%d verifications=%d", images.creates, images.transfers, images.saves, images.verifications)
			}
			var usersPhotos int
			if err := app.usersDB.QueryRow("SELECT count(*) FROM photos_photo WHERE id=$1", result.ID).Scan(&usersPhotos); err != nil || usersPhotos != 0 {
				t.Fatal("upload wrote photo data through the users connection")
			}
		})
	}
}
