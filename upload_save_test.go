package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func uploadSaveMetadata() photoUploadMetadata {
	latitude, longitude := 13.75, 100.5
	return photoUploadMetadata{
		ID: testUploadImageID, Filename: "cat.jpg", SHA256: strings.Repeat("a", 64),
		Latitude: &latitude, Longitude: &longitude, City: "Bangkok", Country: "Thailand",
		CaptureTime: "2026-06-15 14:30:45", TimeSource: "DateTimeOriginal", UTCOffset: "+07:00", Timestamp: "2026-06-15T07:30:45.000Z",
		Raw: json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:30:45","OffsetTimeOriginal":"+07:00","GPSLatitude":[13,45,0],"GPSLatitudeRef":"N","GPSLongitude":[100,30,0],"GPSLongitudeRef":"E"}`),
	}
}

func TestValidatePhotoUpload(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*photoUploadMetadata)
	}{
		{"image ID", func(m *photoUploadMetadata) { m.ID = "../other-image" }},
		{"filename path", func(m *photoUploadMetadata) { m.Filename = "../cat.jpg" }},
		{"filename type", func(m *photoUploadMetadata) { m.Filename = "cat.heic" }},
		{"filename length", func(m *photoUploadMetadata) { m.Filename = strings.Repeat("a", 252) + ".jpg" }},
		{"filename control", func(m *photoUploadMetadata) { m.Filename = "cat\x00.jpg" }},
		{"hash", func(m *photoUploadMetadata) { m.SHA256 = strings.Repeat("x", 64) }},
		{"missing latitude", func(m *photoUploadMetadata) { m.Latitude = nil }},
		{"missing longitude", func(m *photoUploadMetadata) { m.Longitude = nil }},
		{"latitude range", func(m *photoUploadMetadata) { v := 90.01; m.Latitude = &v }},
		{"longitude range", func(m *photoUploadMetadata) { v := -180.01; m.Longitude = &v }},
		{"coordinate NaN", func(m *photoUploadMetadata) { v := math.NaN(); m.Latitude = &v }},
		{"altitude infinity", func(m *photoUploadMetadata) { v := math.Inf(1); m.Altitude = &v }},
		{"missing city", func(m *photoUploadMetadata) { m.City = " \t " }},
		{"country control", func(m *photoUploadMetadata) { m.Country = "Thai\nland" }},
		{"digitization time", func(m *photoUploadMetadata) { m.TimeSource = "CreateDate" }},
		{"modification time", func(m *photoUploadMetadata) { m.TimeSource = "ModifyDate" }},
		{"missing time", func(m *photoUploadMetadata) { m.CaptureTime = "" }},
		{"calendar rollover", func(m *photoUploadMetadata) { m.CaptureTime = "2026-02-30 14:30:45" }},
		{"missing offset", func(m *photoUploadMetadata) { m.UTCOffset = "" }},
		{"offset format", func(m *photoUploadMetadata) { m.UTCOffset = "Europe/Berlin" }},
		{"offset range", func(m *photoUploadMetadata) { m.UTCOffset = "+14:01" }},
		{"offset minutes", func(m *photoUploadMetadata) { m.UTCOffset = "+07:60" }},
		{"missing timestamp", func(m *photoUploadMetadata) { m.Timestamp = "" }},
		{"timestamp without zone", func(m *photoUploadMetadata) { m.Timestamp = "2026-06-15T14:30:45" }},
		{"double offset conversion", func(m *photoUploadMetadata) { m.Timestamp = "2026-06-15T00:30:45Z" }},
		{"raw null", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`null`) }},
		{"raw array", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`[]`) }},
		{"raw malformed", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`{`) }},
		{"raw missing capture", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`{"OffsetTimeOriginal":"+07:00"}`) }},
		{"raw wrong capture", func(m *photoUploadMetadata) {
			m.Raw = json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:31:45","OffsetTimeOriginal":"+07:00"}`)
		}},
		{"raw wrong offset", func(m *photoUploadMetadata) {
			m.Raw = json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:30:45","OffsetTimeOriginal":"+08:00"}`)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := uploadSaveMetadata()
			tt.change(&m)
			if _, err := validatePhotoUpload(m); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	for _, tt := range []struct {
		name, capture, offset, timestamp string
		numeric                          bool
	}{
		{"east", "2026-06-15 14:30:45", "+07:00", "2026-06-15T07:30:45Z", false},
		{"west half-hour", "2026-01-01 00:15:00", "-03:30", "2026-01-01T03:45:00Z", false},
		{"quarter-hour leap day", "2024-03-01 00:15:00", "+05:45", "2024-02-29T18:30:00Z", false},
		{"numeric EXIF keys", "2026-06-15 14:30:45", "+07:00", "2026-06-15T07:30:45Z", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := uploadSaveMetadata()
			m.CaptureTime, m.UTCOffset, m.Timestamp = tt.capture, tt.offset, tt.timestamp
			wall, err := time.Parse("2006-01-02 15:04:05", tt.capture)
			if err != nil {
				t.Fatal(err)
			}
			timeKey, offsetKey := "DateTimeOriginal", "OffsetTimeOriginal"
			if tt.numeric {
				timeKey, offsetKey = "36867", "36881"
			}
			m.Raw, _ = json.Marshal(map[string]string{timeKey: wall.Format("2006:01:02 15:04:05"), offsetKey: tt.offset})
			m.SHA256 = strings.ToUpper(m.SHA256)
			m.City = " Bangkok "
			m.Country = " Thailand "
			valid, err := validatePhotoUpload(m)
			if err != nil || valid.takenAt.Format(time.RFC3339) != tt.timestamp || valid.SHA256 != strings.Repeat("a", 64) || valid.City != "Bangkok" || valid.Country != "Thailand" {
				t.Fatalf("valid metadata rejected or altered: %v", err)
			}
		})
	}
}

type saveTestImages struct {
	binding         string
	draft, fail     bool
	creates, checks int
}

func newUploadSaveTestApp(t *testing.T) (*application, *saveTestImages, string, []*http.Cookie) {
	t.Helper()
	app, _ := newAuthTestApplication(t)
	db, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	// Independent connection-local photo tables ensure that writes use contentDB,
	// and that no test rows can leak into usersDB or another test's fixtures.
	if _, err := db.Exec(`
		CREATE TEMP TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
		CREATE TEMP TABLE photos_coordinates (LIKE public.photos_coordinates INCLUDING ALL);
		CREATE TEMP TABLE photos_photo (LIKE public.photos_photo INCLUDING ALL);
		CREATE TEMP TABLE photos_rawmetadata (LIKE public.photos_rawmetadata INCLUDING ALL);
		ALTER TABLE photos_coordinates ADD FOREIGN KEY (location_id) REFERENCES photos_location(id);
		ALTER TABLE photos_photo ADD FOREIGN KEY (coordinates_id) REFERENCES photos_coordinates(id);
		ALTER TABLE photos_rawmetadata ADD FOREIGN KEY (photo_id) REFERENCES photos_photo(sha256);
	`); err != nil {
		t.Fatal(err)
	}
	app.contentDB = db
	app.config.ImageAccountID = strings.Repeat("a", 32)
	app.config.ImageAPIKey = "fake-private-cloudflare-key"
	app.config.ImageDomain = "images.example.com"
	app.config.ImageAccountHash = "test-image-hash"
	cookies := authTestLogin(t, app)
	csrf, cookies := csrfTestForm(t, app.routes(), "/upload", cookies)
	ctx, err := app.sessionManager.Load(context.Background(), cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := app.imageUploadSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service := &saveTestImages{binding: binding}
	app.cloudflareImagesClient = &http.Client{Transport: cloudflareRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			service.creates++
			if err := r.ParseMultipartForm(4096); err != nil {
				t.Fatal(err)
			}
			defer r.MultipartForm.RemoveAll()
			var meta map[string]string
			if err := json.Unmarshal([]byte(r.FormValue("metadata")), &meta); err != nil {
				t.Fatal(err)
			}
			service.binding = meta["uploadSession"]
			return imageTestResponse(200, imageTestEnvelope(map[string]string{"id": testUploadImageID, "uploadURL": testImageUploadURL})), nil
		}
		if r.Method != http.MethodGet {
			t.Fatal("unexpected image-service mutation")
		}
		service.checks++
		if service.fail {
			return imageTestResponse(503, "fake-private-cloudflare-key"), nil
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		return imageTestResponse(200, imageTestEnvelope(map[string]any{"id": id, "uploaded": "2026-06-15T07:30:45Z", "draft": service.draft, "meta": map[string]string{"uploadSession": service.binding}})), nil
	})}
	return app, service, csrf, cookies
}

func uploadSaveRequest(t *testing.T, app *application, m photoUploadMetadata, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/photos/save", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}

func assertUploadCounts(t *testing.T, db *sql.DB, places, coordinates, photos, raw int) {
	t.Helper()
	for _, tt := range []struct {
		table string
		want  int
	}{{"photos_location", places}, {"photos_coordinates", coordinates}, {"photos_photo", photos}, {"photos_rawmetadata", raw}} {
		var got int
		if err := db.QueryRow("SELECT count(*) FROM " + tt.table).Scan(&got); err != nil || got != tt.want {
			t.Fatalf("%s: wanted %d, got %d, error %v", tt.table, tt.want, got, err)
		}
	}
}

func TestSavePhotoRollbackRetryAndAPI(t *testing.T) {
	app, service, csrf, cookies := newUploadSaveTestApp(t)
	m := uploadSaveMetadata()
	r := httptest.NewRequest(http.MethodPost, "/api/photos/upload-url", nil)
	r.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusCreated || service.creates != 1 {
		t.Fatal("could not issue upload URL")
	}
	// Force the last write to fail after place, coordinates and photo insertion.
	if _, err := app.contentDB.Exec("ALTER TABLE photos_rawmetadata ADD CONSTRAINT simulate_save_failure CHECK(false)"); err != nil {
		t.Fatal(err)
	}
	w = uploadSaveRequest(t, app, m, csrf, cookies)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "retry with the same image ID") {
		t.Fatal("save failure was not retryable")
	}
	assertUploadCounts(t, app.contentDB, 0, 0, 0, 0)
	if _, err := app.contentDB.Exec("ALTER TABLE photos_rawmetadata DROP CONSTRAINT simulate_save_failure"); err != nil {
		t.Fatal(err)
	}
	w = uploadSaveRequest(t, app, m, csrf, cookies)
	if w.Code != http.StatusCreated {
		t.Fatalf("retry failed: status %d", w.Code)
	}
	assertUploadCounts(t, app.contentDB, 1, 1, 1, 1)
	var timestamp time.Time
	var altitude sql.NullFloat64
	var offset int
	var rawMatches bool
	if err := app.contentDB.QueryRow(`SELECT p.timestamp,c.altitude,l.tzoffset,r.metadata=$1::jsonb
		FROM photos_photo p JOIN photos_coordinates c ON c.id=p.coordinates_id
		JOIN photos_location l ON l.id=c.location_id JOIN photos_rawmetadata r ON r.photo_id=p.sha256`, string(m.Raw)).Scan(&timestamp, &altitude, &offset, &rawMatches); err != nil {
		t.Fatal(err)
	}
	if timestamp.UTC().Format(time.RFC3339) != "2026-06-15T07:30:45Z" || altitude.Valid || offset != -420 || !rawMatches {
		t.Fatal("saved capture details were changed or invented")
	}
	w = uploadSaveRequest(t, app, m, csrf, cookies)
	var replay savedPhotoUpload
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil || w.Code != http.StatusOK || !replay.Success || !replay.AlreadySaved {
		t.Fatal("lost-response retry did not return the saved photo")
	}
	assertUploadCounts(t, app.contentDB, 1, 1, 1, 1)
	if service.creates != 1 || service.checks != 3 {
		t.Fatal("retry issued another image upload instead of reusing its ID")
	}
	var usersPhotos int
	if err := app.usersDB.QueryRow("SELECT count(*) FROM photos_photo WHERE id=$1", m.ID).Scan(&usersPhotos); err != nil || usersPhotos != 0 {
		t.Fatal("photo was written through usersDB")
	}
	w = httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos", nil))
	var photos []photo
	if err := json.Unmarshal(w.Body.Bytes(), &photos); err != nil || w.Code != 200 || len(photos) != 1 || photos[0].ID != m.ID || photos[0].Latitude == nil || *photos[0].Latitude != 13.75 || photos[0].Longitude == nil || *photos[0].Longitude != 100.5 || photos[0].City == nil || *photos[0].City != "Bangkok" || photos[0].URLs.Mobile == "" {
		t.Fatal("saved photo is not available to the existing map API")
	}
}

func TestSavePhotoReuseAndConflicts(t *testing.T) {
	app, _, csrf, cookies := newUploadSaveTestApp(t)
	m := uploadSaveMetadata()
	for i, altitude := range []*float64{nil, nil, new(float64), func() *float64 { v := -14.0; return &v }()} {
		m.ID = fmt.Sprintf("21000000-0000-4000-8000-%012d", i+1)
		m.SHA256 = strings.Repeat(fmt.Sprintf("%x", i+1), 64)
		m.Altitude = altitude
		if w := uploadSaveRequest(t, app, m, csrf, cookies); w.Code != http.StatusCreated {
			t.Fatalf("reuse scenario %d failed: %d", i, w.Code)
		}
	}
	assertUploadCounts(t, app.contentDB, 1, 3, 4, 4)
	// A different per-photo offset must not overwrite the shared place's offset.
	m = uploadSaveMetadata()
	m.ID = "21000000-0000-4000-8000-000000000005"
	m.SHA256 = strings.Repeat("5", 64)
	m.UTCOffset = "+08:00"
	m.Timestamp = "2026-06-15T06:30:45Z"
	m.Raw = json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:30:45","OffsetTimeOriginal":"+08:00"}`)
	if w := uploadSaveRequest(t, app, m, csrf, cookies); w.Code != http.StatusCreated {
		t.Fatal("photo offset was confused with shared place offset")
	}
	assertUploadCounts(t, app.contentDB, 1, 3, 5, 5)
	var offset int
	if err := app.contentDB.QueryRow("SELECT tzoffset FROM photos_location").Scan(&offset); err != nil || offset != -420 {
		t.Fatal("existing place offset was overwritten")
	}
	for _, mode := range []string{"same hash other image", "same image other hash", "changed saved metadata", "different place"} {
		t.Run(mode, func(t *testing.T) {
			conflict := m
			switch mode {
			case "same hash other image":
				conflict.ID = "21000000-0000-4000-8000-000000000006"
			case "same image other hash":
				conflict.SHA256 = strings.Repeat("6", 64)
			case "changed saved metadata":
				conflict.City = "Another city"
			case "different place":
				conflict.ID = "21000000-0000-4000-8000-000000000006"
				conflict.SHA256 = strings.Repeat("6", 64)
				conflict.City = "Another city"
			}
			if w := uploadSaveRequest(t, app, conflict, csrf, cookies); w.Code != http.StatusConflict {
				t.Fatalf("conflicting upload accepted: %d", w.Code)
			}
			assertUploadCounts(t, app.contentDB, 1, 3, 5, 5)
		})
	}
}

func TestSavePhotoGuards(t *testing.T) {
	for _, mode := range []string{"anonymous", "deleted user", "missing CSRF", "foreign origin", "GET", "content type", "bad JSON", "trailing JSON", "unknown field", "oversized", "missing coordinates", "missing offset", "foreign image", "draft", "provider failure", "missing configuration", "database failure"} {
		t.Run(mode, func(t *testing.T) {
			app, service, csrf, cookies := newUploadSaveTestApp(t)
			m := uploadSaveMetadata()
			want := http.StatusBadRequest
			method, contentType := http.MethodPost, "application/json"
			switch mode {
			case "anonymous":
				csrf, cookies = csrfTestForm(t, app.routes(), "/login", nil)
				want = http.StatusUnauthorized
			case "deleted user":
				if _, err := app.usersDB.Exec("DELETE FROM users"); err != nil {
					t.Fatal(err)
				}
				want = http.StatusUnauthorized
			case "missing CSRF":
				csrf = ""
				want = http.StatusForbidden
			case "foreign origin":
				want = http.StatusForbidden
			case "GET":
				method = http.MethodGet
				want = http.StatusMethodNotAllowed
			case "content type":
				contentType = "text/plain"
				want = http.StatusUnsupportedMediaType
			case "oversized":
				m.Raw = json.RawMessage(`{"comment":"` + strings.Repeat("x", 256<<10) + `"}`)
				want = http.StatusRequestEntityTooLarge
			case "missing coordinates":
				m.Latitude = nil
				want = http.StatusUnprocessableEntity
			case "missing offset":
				m.UTCOffset = ""
				want = http.StatusUnprocessableEntity
			case "foreign image":
				service.binding = "another-login"
				want = http.StatusForbidden
			case "draft":
				service.draft = true
				want = http.StatusConflict
			case "provider failure":
				service.fail = true
				want = http.StatusBadGateway
			case "missing configuration":
				app.config.ImageAPIKey = ""
				want = http.StatusServiceUnavailable
			case "database failure":
				if err := app.contentDB.Close(); err != nil {
					t.Fatal(err)
				}
				want = http.StatusInternalServerError
			}
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			body := string(data)
			switch mode {
			case "bad JSON":
				body = `{"raw":"private-test-marker",`
			case "trailing JSON":
				body += `{}`
			case "unknown field":
				body = strings.TrimSuffix(body, "}") + `,"unexpected":"private-test-marker"}`
			}
			r := httptest.NewRequest(method, "/api/photos/save", strings.NewReader(body))
			r.Header.Set("Content-Type", contentType)
			r.Header.Set("X-CSRF-Token", csrf)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			if mode == "foreign origin" {
				r.Header.Set("Origin", "https://foreign.example")
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("wanted %d, got %d", want, w.Code)
			}
			if mode == "GET" && w.Header().Get("Allow") != http.MethodPost {
				t.Fatal("missing allowed method")
			}
			if mode == "draft" {
				var failure map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil || failure["code"] != "image_incomplete" {
					t.Fatal("incomplete upload lacks a machine-readable retry code")
				}
			}
			wantChecks := 0
			if mode == "foreign image" || mode == "draft" || mode == "provider failure" || mode == "database failure" {
				wantChecks = 1
			}
			if service.checks != wantChecks || service.creates != 0 {
				t.Fatal("rejected save reached an unexpected image-service action")
			}
			if mode != "database failure" {
				assertUploadCounts(t, app.contentDB, 0, 0, 0, 0)
			}
			for _, secret := range []string{"fake-private-cloudflare-key", "private-test-marker", service.binding} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("save error exposed private data")
				}
			}
		})
	}
}

func TestConcurrentPhotoSaves(t *testing.T) {
	for index, mode := range []string{"same submission", "same hash other image"} {
		t.Run(mode, func(t *testing.T) {
			connection := testDatabaseURL(t)
			admin, err := sql.Open("pgx", connection)
			if err != nil {
				t.Fatal(err)
			}
			schema := fmt.Sprintf("upload_save_race_%d", index)
			if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
				admin.Close()
				t.Fatal(err)
			}
			cfg, err := pgx.ParseConfig(connection)
			if err != nil {
				t.Fatal("cannot parse isolated connection")
			}
			cfg.RuntimeParams["search_path"] = schema + ",public"
			db := stdlib.OpenDB(*cfg)
			db.SetMaxOpenConns(2)
			t.Cleanup(func() {
				db.Close()
				if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
					t.Error(err)
				}
				admin.Close()
			})
			if _, err := db.Exec(`
				CREATE TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
				CREATE TABLE photos_coordinates (LIKE public.photos_coordinates INCLUDING ALL);
				CREATE TABLE photos_photo (LIKE public.photos_photo INCLUDING ALL);
				CREATE TABLE photos_rawmetadata (LIKE public.photos_rawmetadata INCLUDING ALL);
				ALTER TABLE photos_coordinates ADD FOREIGN KEY (location_id) REFERENCES photos_location(id);
				ALTER TABLE photos_photo ADD FOREIGN KEY (coordinates_id) REFERENCES photos_coordinates(id);
				ALTER TABLE photos_rawmetadata ADD FOREIGN KEY (photo_id) REFERENCES photos_photo(sha256);
			`); err != nil {
				t.Fatal(err)
			}
			first := uploadSaveMetadata()
			second := uploadSaveMetadata()
			if mode == "same hash other image" {
				second.ID = "21000000-0000-4000-8000-000000000002"
				second.City = "Other city"
				longitude := 100.6
				second.Longitude = &longitude
			}
			one, err := validatePhotoUpload(first)
			if err != nil {
				t.Fatal(err)
			}
			two, err := validatePhotoUpload(second)
			if err != nil {
				t.Fatal(err)
			}
			app := &application{contentDB: db}
			type outcome struct {
				result savedPhotoUpload
				err    error
			}
			outcomes := make(chan outcome, 2)
			start := make(chan struct{})
			for _, metadata := range []validatedPhotoUpload{one, two} {
				go func() {
					<-start
					result, err := app.storeUploadedPhoto(context.Background(), metadata)
					outcomes <- outcome{result, err}
				}()
			}
			close(start)
			created, replayed, conflicts := 0, 0, 0
			for range 2 {
				outcome := <-outcomes
				if errors.Is(outcome.err, errDuplicateUpload) {
					conflicts++
				} else if outcome.err != nil {
					t.Fatal(outcome.err)
				} else if outcome.result.AlreadySaved {
					replayed++
				} else if outcome.result.Success {
					created++
				}
			}
			if created != 1 || (mode == "same submission" && replayed != 1) || (mode == "same hash other image" && conflicts != 1) {
				t.Fatal("concurrent saves were not duplicate-safe")
			}
			assertUploadCounts(t, db, 1, 1, 1, 1)
			// A write must also honor its caller's deadline while waiting for a pool slot.
			db.SetMaxOpenConns(1)
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			_, err = app.storeUploadedPhoto(ctx, one)
			cancel()
			conn.Close()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("save did not honor deadline while waiting for a connection")
			}
		})
	}
}
