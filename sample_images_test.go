package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestSampleImages(t *testing.T) {
	// Empty service configuration demonstrates that local delivery needs neither
	// Cloudflare nor Mapbox credentials. Image requests do not need DB queries.
	app := &application{sessionManager: scs.New()}
	server := httptest.NewServer(app.routes())
	defer server.Close()
	for _, photo := range samplePhotos {
		t.Run(photo.Filename, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("sample-data", "images", photo.Filename))
			if err != nil {
				t.Fatal(err)
			}
			url := server.URL + "/sample-images/" + photo.Filename
			response, err := server.Client().Get(url)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" {
				t.Fatalf("wanted a 200 JPEG response, got %d, %s", response.StatusCode, response.Header.Get("Content-Type"))
			}
			if !bytes.Equal(body, want) {
				t.Fatal("HTTP response did not match the repository image")
			}
			if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
				t.Fatalf("response was not a valid JPEG: %v", err)
			}
			if response.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing nosniff header")
			}
			response, err = server.Client().Head(url)
			if err != nil {
				t.Fatal(err)
			}
			body, err = io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || len(body) != 0 || response.Header.Get("Content-Length") != strconv.Itoa(len(want)) {
				t.Fatal("HEAD did not return the image size without a body")
			}
		})
	}
}

func TestSampleImageRejectsOtherMethods(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/sample-images/schilln.jpeg", nil)
		app.routes().ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("%s: wanted 405 and Allow: GET, HEAD, got %d", method, response.Code)
		}
	}
}

func TestSampleImageRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	images := filepath.Join(dir, "sample-data", "images")
	if err := os.MkdirAll(images, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := []byte("private-fixture-not-for-http")
	for _, path := range []string{filepath.Join(dir, ".env"), filepath.Join(images, ".env"), filepath.Join(images, "unexpected.jpeg")} {
		if err := os.WriteFile(path, secret, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(images, "kuscheln.jpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Even a whitelisted name must not be able to escape through a symlink.
	if err := os.Symlink(filepath.Join(dir, ".env"), filepath.Join(images, "noDogorCat.jpeg")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	app := &application{sessionManager: scs.New()}
	handler := app.routes()
	for _, path := range []string{
		"/sample-images/",
		"/sample-images/missing.jpeg",
		"/sample-images/knuddeln.jpeg",   // Whitelisted but missing.
		"/sample-images/kuscheln.jpeg",   // A directory, not a regular file.
		"/sample-images/noDogorCat.jpeg", // Symlink outside the image directory.
		"/sample-images/unexpected.jpeg", // Existing but not whitelisted.
		"/sample-images/.env",
		"/sample-images/subdir/schilln.jpeg",
		"/sample-images/%2e%2e/.env",
		"/sample-images/%2e%2e%2f.env",
		"/sample-images/%252e%252e/.env",
		"/sample-images/..%5c.env",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("wanted 404, got %d", response.Code)
			}
			if bytes.Contains(response.Body.Bytes(), secret) {
				t.Fatal("response exposed a private file")
			}
		})
	}
}

func TestSampleImageMissingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	app := &application{sessionManager: scs.New()}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sample-images/schilln.jpeg", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("wanted 404 for missing sample assets, got %d", response.Code)
	}
}

func TestSeededPhotosHaveLocalImages(t *testing.T) {
	db, _ := sampleTestDatabase(t)
	if inserted, err := seedSampleData(context.Background(), db); err != nil || inserted != 4 {
		t.Fatalf("wanted four sample rows, got %d, error %v", inserted, err)
	}
	app := &application{db: db, sessionManager: scs.New()}
	server := httptest.NewServer(app.routes())
	defer server.Close()
	rows, err := db.Query("SELECT filename, sha256 FROM photos_photo ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var filename, hash string
		if err := rows.Scan(&filename, &hash); err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Get(server.URL + "/sample-images/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || fmt.Sprintf("%x", sha256.Sum256(body)) != hash {
			t.Fatalf("local image does not match the seeded database row for %s", filename)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("wanted four served sample images, got %d", count)
	}
}
