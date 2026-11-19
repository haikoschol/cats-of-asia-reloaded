//go:build !integration

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestNearbyInvalidRequests(t *testing.T) {
	// A nil content connection ensures invalid input is rejected before any query.
	app := &application{sessionManager: scs.New()}
	for _, query := range []string{
		"", "latitude=13.75", "longitude=100.5",
		"latitude=&longitude=100.5", "latitude=word&longitude=100.5",
		"latitude=NaN&longitude=100.5", "latitude=Inf&longitude=100.5",
		"latitude=90.01&longitude=100.5", "latitude=-90.01&longitude=100.5",
		"latitude=13.75&longitude=180.01", "latitude=13.75&longitude=-180.01",
		"latitude=13.75&longitude=NaN", "latitude=13.75&longitude=Inf",
		"latitude=13.75&longitude=100.5&radius_km=0",
		"latitude=13.75&longitude=100.5&radius_km=-1",
		"latitude=13.75&longitude=100.5&radius_km=100.01",
		"latitude=13.75&longitude=100.5&radius_km=NaN",
		"latitude=13.75&longitude=100.5&radius_km=Inf",
		"latitude=13.75&longitude=100.5&radius_km=",
		"latitude=13.75&longitude=100.5&limit=0",
		"latitude=13.75&longitude=100.5&limit=-1",
		"latitude=13.75&longitude=100.5&limit=101",
		"latitude=13.75&longitude=100.5&limit=1.5",
		"latitude=13.75&longitude=100.5&limit=NaN",
		"latitude=13.75&longitude=100.5&limit=",
		"latitude=13.75&latitude=0&longitude=100.5",
		"latitude=13.75&longitude=100.5&radius_km=1&radius_km=2",
		"latitude=13.75&longitude=100.5&limit=1&limit=2",
		"latitude=13.75&longitude=100.5&radius=10",
		"latitude=13.75&longitude=100.5&limit=%ZZ",
	} {
		t.Run(query, func(t *testing.T) {
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/photos/nearby?"+query, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("wanted 400 before database access, got %d", w.Code)
			}
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		// Check method restrictions independently of the CSRF middleware.
		app.nearbyPhotos(w, httptest.NewRequest(method, "/api/photos/nearby?latitude=0&longitude=0", nil))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
			t.Errorf("%s was not rejected with 405 and Allow: GET", method)
		}
	}
}

func TestNearbyParameterBounds(t *testing.T) {
	for _, query := range []string{
		"latitude=0&longitude=0", "latitude=-90&longitude=-180",
		"latitude=90&longitude=180&radius_km=100&limit=100",
		"latitude=0&longitude=0&radius_km=0.001&limit=1",
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseNearbySearch(values); err != nil {
			t.Errorf("valid boundary values were rejected: %s: %v", query, err)
		}
	}
}
