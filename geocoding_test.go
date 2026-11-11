package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func newGeocodingTestApp(t *testing.T) *application {
	t.Helper()
	app, _ := newAuthTestApplication(t)
	contentDB, err := sql.Open("pgx", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { contentDB.Close() })
	contentDB.SetMaxOpenConns(1)
	// Empty tables on the users connection catch lookups using the wrong role.
	// All fixtures live in connection-local tables in the isolated test database.
	for _, db := range []*sql.DB{app.usersDB, contentDB} {
		if _, err := db.Exec(`
			CREATE TEMP TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
			CREATE TEMP TABLE photos_coordinates (LIKE public.photos_coordinates INCLUDING ALL);`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := contentDB.Exec(`
		INSERT INTO photos_location (id, city, country, tzoffset) VALUES
			(1, 'Bangkok', 'Thailand', 420),
			(2, 'Zero point', 'Test country', 0),
			(3, 'Coordinate test', 'Test country', 0),
			(4, 'Other place', 'Test country', 0);
		INSERT INTO photos_coordinates (id, location_id, point, altitude) VALUES
			(1, 1, ST_SetSRID(ST_MakePoint(100.5, 13.75), 4326)::geography, 0),
			(2, 2, ST_SetSRID(ST_MakePoint(0, 0), 4326)::geography, 0),
			(3, 2, ST_SetSRID(ST_MakePoint(100.5, 0), 4326)::geography, 0),
			(4, 2, ST_SetSRID(ST_MakePoint(0, 13.75), 4326)::geography, 0),
			(5, 3, ST_SetSRID(ST_MakePoint(40.25, 20.5), 4326)::geography, 0),
			(6, 4, ST_SetSRID(ST_MakePoint(40.25, 20.5), 4326)::geography, 10);`); err != nil {
		t.Fatal(err)
	}
	app.contentDB = contentDB
	return app
}

func TestParseGeocodingCoordinates(t *testing.T) {
	for _, tt := range []struct {
		name, latitude, longitude string
		want                      geocodingCoordinates
	}{
		{"Bangkok", "13.75", "100.5", geocodingCoordinates{13.75, 100.5}},
		{"zero latitude", "0", "100.5", geocodingCoordinates{0, 100.5}},
		{"zero longitude", "13.75", "0", geocodingCoordinates{13.75, 0}},
		{"both zero", "0", "0", geocodingCoordinates{0, 0}},
		{"lower boundaries", "-90", "-180", geocodingCoordinates{-90, -180}},
		{"upper boundaries", "90", "180", geocodingCoordinates{90, 180}},
		{"negative coordinates", "-33.9", "-70.6", geocodingCoordinates{-33.9, -70.6}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGeocodingCoordinates(url.Values{
				"latitude": {tt.latitude}, "longitude": {tt.longitude},
			})
			if err != nil || got != tt.want {
				t.Fatalf("wanted %+v, got %+v, error %v", tt.want, got, err)
			}
		})
	}
}

func TestReverseGeocodingInvalidRequests(t *testing.T) {
	app, _ := newAuthTestApplication(t)
	cookies := authTestLogin(t, app)
	// Invalid input must be rejected before querying location data.
	app.contentDB = nil
	for _, tt := range []struct {
		query  string
		status int
	}{
		{"", http.StatusBadRequest},
		{"latitude=13.75", http.StatusBadRequest},
		{"longitude=100.5", http.StatusBadRequest},
		{"latitude=&longitude=100.5", http.StatusBadRequest},
		{"latitude=13.75&longitude=", http.StatusBadRequest},
		{"latitude=word&longitude=100.5", http.StatusBadRequest},
		{"latitude=13.75&longitude=word", http.StatusBadRequest},
		{"latitude=NaN&longitude=100.5", http.StatusBadRequest},
		{"latitude=Inf&longitude=100.5", http.StatusBadRequest},
		{"latitude=-Inf&longitude=100.5", http.StatusBadRequest},
		{"latitude=13.75&longitude=NaN", http.StatusBadRequest},
		{"latitude=13.75&longitude=Inf", http.StatusBadRequest},
		{"latitude=13.75&longitude=-Inf", http.StatusBadRequest},
		{"latitude=1e999&longitude=100.5", http.StatusBadRequest},
		{"latitude=90.01&longitude=100.5", http.StatusBadRequest},
		{"latitude=-90.01&longitude=100.5", http.StatusBadRequest},
		{"latitude=13.75&longitude=180.01", http.StatusBadRequest},
		{"latitude=13.75&longitude=-180.01", http.StatusBadRequest},
		{"latitude=13.75&longitude=100.5&latitude=0", http.StatusBadRequest},
		{"latitude=13.75&longitude=100.5&longitude=0", http.StatusBadRequest},
		{"latitude=13.75&longitude=100.5&radius_km=10", http.StatusBadRequest},
		{"latitude=%ZZ&longitude=100.5", http.StatusBadRequest},
		{"latitude=13.75&longitude=100.5&broken=%ZZ", http.StatusBadRequest},
	} {
		t.Run(tt.query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?"+tt.query, nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("wanted status %d, got %d: %s", tt.status, w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("expected a non-cacheable JSON response")
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Fatalf("expected a JSON error message, got %s", w.Body.String())
			}
		})
	}
}

func TestReverseGeocodingLookup(t *testing.T) {
	app := newGeocodingTestApp(t)
	cookies := authTestLogin(t, app)
	for _, tt := range []struct {
		name, query   string
		status        int
		city, country string
	}{
		{"known point", "latitude=13.75&longitude=100.5", http.StatusOK, "Bangkok", "Thailand"},
		{"zero point", "latitude=0&longitude=0", http.StatusOK, "Zero point", "Test country"},
		{"zero latitude", "latitude=0&longitude=100.5", http.StatusOK, "Zero point", "Test country"},
		{"zero longitude", "latitude=13.75&longitude=0", http.StatusOK, "Zero point", "Test country"},
		{"same point at different altitudes", "latitude=20.5&longitude=40.25", http.StatusOK, "Coordinate test", "Test country"},
		{"swapped coordinates", "latitude=40.25&longitude=20.5", http.StatusServiceUnavailable, "", ""},
		{"nearby is not exact", "latitude=13.7500001&longitude=100.5", http.StatusServiceUnavailable, "", ""},
		{"unknown point", "latitude=-33.9&longitude=-70.6", http.StatusServiceUnavailable, "", ""},
		{"lower boundaries", "latitude=-90&longitude=-180", http.StatusServiceUnavailable, "", ""},
		{"upper boundaries", "latitude=90&longitude=180", http.StatusServiceUnavailable, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?"+tt.query, nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != tt.status || w.Header().Get("Content-Type") != "application/json" ||
				w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("expected non-cacheable JSON with status %d, got %d: %s", tt.status, w.Code, w.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tt.status == http.StatusOK {
				if len(body) != 2 || body["city"] != tt.city || body["country"] != tt.country {
					t.Fatalf("unexpected stored location: %s", w.Body.String())
				}
			} else if len(body) != 1 || body["error"] != "Google geocoding is disabled" {
				t.Fatalf("unexpected response for unknown coordinates: %s", w.Body.String())
			}
		})
	}
	var places, coordinates int
	if err := app.contentDB.QueryRow(`SELECT
		(SELECT count(*) FROM photos_location),
		(SELECT count(*) FROM photos_coordinates)`).Scan(&places, &coordinates); err != nil {
		t.Fatal(err)
	}
	if places != 4 || coordinates != 6 {
		t.Fatal("location lookup changed the number of stored places or coordinates")
	}
}

func TestReverseGeocodingGoogleSuggestions(t *testing.T) {
	app := newGeocodingTestApp(t)
	cookies := authTestLogin(t, app)
	app.config.GoogleGeocodingEnabled = true
	app.config.GoogleMapsAPIKey = "fake-private-google-key"
	for _, tt := range []struct {
		name, query, providerBody, want string
		wantCalls                       int
	}{
		{
			name: "database hit bypasses the configured provider", query: "latitude=13.75&longitude=100.5",
			want: `{"city":"Bangkok","country":"Thailand"}`, wantCalls: 0,
		},
		{
			name: "one distinct city", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"Santiago","types":["locality","political"]},
				{"long_name":"Chile","types":["country","political"]},
				{"long_name":"Santiago","types":["colloquial_area"]}
			]}]}`,
			want: `{"country":"Chile","cityCandidates":["Santiago"],"city":"Santiago"}`,
		},
		{
			name: "sorted unique candidates across results", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"Zurich","types":["locality"]},
				{"long_name":"Switzerland","types":["country"]},
				{"long_name":"Alpha","types":["colloquial_area"]},
				{"long_name":"Street name","types":["route"]}
			]},{"address_components":[
				{"long_name":"Zurich","types":["administrative_area_level_1"]},
				{"long_name":"Region","types":["administrative_area_level_2"]},
				{"long_name":"Another country","types":["country"]}
			]}]}`,
			want: `{"country":"Switzerland","cityCandidates":["Alpha","Region","Zurich"]}`,
		},
		{
			name: "all original city types", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"Locality","types":["locality"]},
				{"long_name":"Nickname","types":["colloquial_area"]},
				{"long_name":"Level5","types":["administrative_area_level_5"]},
				{"long_name":"Level4","types":["administrative_area_level_4"]},
				{"long_name":"Level3","types":["administrative_area_level_3"]},
				{"long_name":"Level2","types":["administrative_area_level_2"]},
				{"long_name":"Level1","types":["administrative_area_level_1"]},
				{"long_name":"Unsupported region","types":["administrative_area_level_6"]}
			]}]}`,
			want: `{"country":null,"cityCandidates":["Level1","Level2","Level3","Level4","Level5","Locality","Nickname"]}`,
		},
		{
			name: "no match allows manual entry", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"ZERO_RESULTS","results":[]}`,
			want:         `{"country":null,"cityCandidates":[]}`,
		},
		{
			name: "country without city", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"Chile","types":["country"]}
			]}]}`,
			want: `{"country":"Chile","cityCandidates":[]}`,
		},
		{
			name: "city without country", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"Santiago","types":["locality"]}
			]}]}`,
			want: `{"country":null,"cityCandidates":["Santiago"],"city":"Santiago"}`,
		},
		{
			name: "empty or unrelated names are ignored", query: "latitude=-33.9&longitude=-70.6", wantCalls: 1,
			providerBody: `{"status":"OK","results":[{"address_components":[
				{"long_name":"","types":["country","locality"]},
				{"long_name":"  ","types":["colloquial_area"]},
				{"long_name":"12345","types":["postal_code"]},
				{"long_name":"Unknown type"}
			]}]}`,
			want: `{"country":null,"cityCandidates":[]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			app.googleGeocodingClient = &http.Client{Transport: googleGeocodingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if tt.wantCalls == 0 {
					t.Fatal("database hit made a provider request")
				}
				if r.URL.Query().Get("latlng") != "-33.9,-70.6" {
					t.Fatal("unknown coordinates were not passed to the provider correctly")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.providerBody))}, nil
			})}
			r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?"+tt.query, nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" ||
				w.Header().Get("Cache-Control") != "no-store" || calls != tt.wantCalls {
				t.Fatalf("unexpected response: status %d, calls %d, body %s", w.Code, calls, w.Body.String())
			}
			var got, want any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("wanted %s, got %s", tt.want, w.Body.String())
			}
		})
	}
	var places, coordinates int
	if err := app.contentDB.QueryRow(`SELECT
		(SELECT count(*) FROM photos_location),
		(SELECT count(*) FROM photos_coordinates)`).Scan(&places, &coordinates); err != nil {
		t.Fatal(err)
	}
	if places != 4 || coordinates != 6 {
		t.Fatal("provider suggestions created or deleted stored places or coordinates")
	}
}

func TestReverseGeocodingDatabaseError(t *testing.T) {
	app := newGeocodingTestApp(t)
	cookies := authTestLogin(t, app)
	app.config.GoogleGeocodingEnabled = true
	app.config.GoogleMapsAPIKey = "fake-private-google-key"
	app.googleGeocodingClient = &http.Client{Transport: googleGeocodingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("a database error must not fall back to the provider")
		return nil, nil
	})}
	if err := app.contentDB.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?latitude=13.75&longitude=100.5", nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	var body map[string]string
	if w.Code != http.StatusInternalServerError || w.Header().Get("Content-Type") != "application/json" ||
		json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 1 || body["error"] != "Internal Server Error" {
		t.Fatalf("database failure exposed details or was reported as a missing location: %d %s", w.Code, w.Body.String())
	}
}

func TestReverseGeocodingProviderConfiguration(t *testing.T) {
	app := newGeocodingTestApp(t)
	cookies := authTestLogin(t, app)
	app.googleGeocodingClient = &http.Client{Transport: googleGeocodingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("disabled or unconfigured geocoding attempted a provider request")
		return nil, nil
	})}
	for _, tt := range []struct {
		name         string
		enabled      bool
		key, message string
	}{
		{"disabled without key", false, "", "Google geocoding is disabled"},
		{"disabled with existing key", false, "fake-private-google-key", "Google geocoding is disabled"},
		{"enabled without key", true, "", "Google geocoding API key is not configured"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app.config.GoogleGeocodingEnabled = tt.enabled
			app.config.GoogleMapsAPIKey = tt.key
			r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?latitude=-33.9&longitude=-70.6", nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			var body map[string]string
			if w.Code != http.StatusServiceUnavailable || w.Header().Get("Content-Type") != "application/json" ||
				w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &body) != nil ||
				len(body) != 1 || body["error"] != tt.message {
				t.Fatalf("expected a JSON configuration error with status 503, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestReverseGeocodingProviderFailures(t *testing.T) {
	app := newGeocodingTestApp(t)
	cookies := authTestLogin(t, app)
	app.config.GoogleGeocodingEnabled = true
	// Include a character that is escaped in the request URL so the checks cover
	// both raw and URL-encoded credentials. This is only a dummy test key.
	const key = "fake-private-google-key+secret"
	app.config.GoogleMapsAPIKey = key
	for _, tt := range []struct {
		name                  string
		httpStatus            int
		providerBody, failure string
		wantStatus            int
		wantMessage           string
	}{
		{"HTTP unauthorized", 401, key, "", 502, "Google geocoding rejected the request"},
		{"HTTP forbidden", 403, key, "", 502, "Google geocoding rejected the request"},
		{"HTTP quota", 429, key, "", 503, "Google geocoding quota or billing limit reached"},
		{"HTTP request timeout", 408, key, "", 504, "Google geocoding request timed out"},
		{"HTTP gateway timeout", 504, key, "", 504, "Google geocoding request timed out"},
		{"HTTP server failure", 500, key, "", 502, "Google geocoding returned an unsuccessful HTTP response"},
		{"redirect", 302, key, "", 502, "Google geocoding returned an unsuccessful HTTP response"},
		{"provider denied", 200, `{"status":"REQUEST_DENIED","error_message":"` + key + `"}`, "", 502, "Google geocoding rejected the request"},
		{"provider query limit", 200, `{"status":"OVER_QUERY_LIMIT"}`, "", 503, "Google geocoding quota or billing limit reached"},
		{"provider daily limit", 200, `{"status":"OVER_DAILY_LIMIT"}`, "", 503, "Google geocoding quota or billing limit reached"},
		{"provider failure", 200, `{"status":"UNKNOWN_ERROR","error_message":"` + key + `"}`, "", 502, "Google geocoding could not process the request"},
		{"malformed response", 200, `{"status":"OK","results":"` + key + `"}`, "", 502, "Google geocoding returned an invalid response"},
		{"empty success", 200, `{"status":"OK","results":[]}`, "", 502, "Google geocoding returned an invalid response"},
		{"oversized response", 200, strings.Repeat(" ", (1<<20)+1), "", 502, "Google geocoding response is too large"},
		{"transport failure", 0, "", "request", 502, "Google geocoding request failed"},
		{"response read failure", 200, "", "body", 502, "Google geocoding request failed"},
		{"transport timeout", 0, "", "timeout", 504, "Google geocoding request timed out"},
		{"response read timeout", 200, "", "body timeout", 504, "Google geocoding request timed out"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(previous) })
			calls := 0
			app.googleGeocodingClient = &http.Client{Transport: googleGeocodingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				privateError := errors.New("failed request: " + r.URL.String())
				switch tt.failure {
				case "request":
					return nil, privateError
				case "timeout":
					return nil, fmt.Errorf("%s: %w", r.URL.String(), context.DeadlineExceeded)
				case "body":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(iotest.ErrReader(privateError))}, nil
				case "body timeout":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(iotest.ErrReader(
						fmt.Errorf("%s: %w", r.URL.String(), context.DeadlineExceeded)))}, nil
				}
				return &http.Response{StatusCode: tt.httpStatus,
					Header: http.Header{"Location": {"https://untrusted.example/?key=" + url.QueryEscape(key)}},
					Body:   io.NopCloser(strings.NewReader(tt.providerBody))}, nil
			})}
			r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?latitude=-33.9&longitude=-70.6", nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			var body map[string]string
			if w.Code != tt.wantStatus || w.Header().Get("Content-Type") != "application/json" ||
				w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Location") != "" ||
				json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 1 || body["error"] != tt.wantMessage {
				t.Fatalf("expected JSON error %d (%s), got status %d", tt.wantStatus, tt.wantMessage, w.Code)
			}
			if calls != 1 {
				t.Fatalf("provider failure caused a retry or redirect: %d calls", calls)
			}
			if !strings.Contains(logs.String(), tt.wantMessage) {
				t.Error("provider failure was not logged with its safe message")
			}
			for _, private := range []string{key, url.QueryEscape(key), "https://maps.googleapis.com/", "error_message"} {
				if strings.Contains(w.Body.String(), private) || strings.Contains(logs.String(), private) {
					t.Error("provider response or logs exposed private request details")
				}
			}
		})
	}
}

func TestReverseGeocodingPoolWaitCancellation(t *testing.T) {
	app := newGeocodingTestApp(t)
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer setupCancel()
	conn, err := app.contentDB.Conn(setupCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Hold the only content connection; the request must honor its deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?latitude=0&longitude=0", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	start := time.Now()
	app.reverseGeocode(w, r)
	var body map[string]string
	if ctx.Err() != context.DeadlineExceeded || time.Since(start) > time.Second ||
		w.Code != http.StatusInternalServerError || json.Unmarshal(w.Body.Bytes(), &body) != nil ||
		len(body) != 1 || body["error"] != "Internal Server Error" {
		t.Fatalf("canceled location query did not fail promptly and safely: %d %s", w.Code, w.Body.String())
	}
}

func TestReverseGeocodingAnonymous(t *testing.T) {
	app := &application{sessionManager: newSessionManager(config{})}
	for _, query := range []string{"", "latitude=0&longitude=0", "latitude=NaN&longitude=100.5"} {
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/locations/reverse?"+query, nil))
		var body map[string]string
		if w.Code != http.StatusUnauthorized || w.Header().Get("Content-Type") != "application/json" ||
			w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Location") != "" ||
			json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"] != "Unauthorized" {
			t.Fatalf("expected a JSON 401 before database access, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestReverseGeocodingDeletedUser(t *testing.T) {
	app, userID := newAuthTestApplication(t)
	cookies := authTestLogin(t, app)
	if _, err := app.usersDB.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
	app.contentDB = nil
	r := httptest.NewRequest(http.MethodGet, "/api/locations/reverse?latitude=0&longitude=0", nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a deleted user's session retained access: status %d", w.Code)
	}
}

func TestReverseGeocodingMethods(t *testing.T) {
	app, _ := newAuthTestApplication(t)
	cookies := authTestLogin(t, app)
	token, cookies := csrfTestForm(t, app.routes(), "/", cookies)
	app.contentDB = nil
	for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/api/locations/reverse?latitude=0&longitude=0", nil)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			// Supply a valid token so write methods reach the method check.
			r.Header.Set("X-CSRF-Token", token)
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			var body map[string]string
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" ||
				w.Header().Get("Content-Type") != "application/json" ||
				json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"] != "Method Not Allowed" {
				t.Fatalf("expected a JSON 405 with Allow: GET, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
