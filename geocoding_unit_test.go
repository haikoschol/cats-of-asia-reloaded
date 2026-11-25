//go:build !integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

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
