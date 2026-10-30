package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultNearbyRadiusKM = 10
	maxNearbyRadiusKM     = 100
	defaultNearbyLimit    = 10
	maxNearbyLimit        = 100
)

type nearbySearch struct {
	latitude, longitude float64
	radiusKM            float64
	limit               int
}

func parseNearbySearch(query url.Values) (nearbySearch, error) {
	search := nearbySearch{radiusKM: defaultNearbyRadiusKM, limit: defaultNearbyLimit}
	for name, values := range query {
		switch name {
		case "latitude", "longitude", "radius_km", "limit":
		default:
			return search, fmt.Errorf("unsupported search parameter")
		}
		if len(values) != 1 {
			return search, fmt.Errorf("search parameters must appear only once")
		}
	}
	for _, field := range []struct {
		name     string
		min, max float64
		target   *float64
	}{
		{"latitude", -90, 90, &search.latitude},
		{"longitude", -180, 180, &search.longitude},
	} {
		value, err := strconv.ParseFloat(query.Get(field.name), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < field.min || value > field.max {
			return search, fmt.Errorf("%s must be a finite number between %g and %g", field.name, field.min, field.max)
		}
		*field.target = value
	}
	if query.Has("radius_km") {
		value, err := strconv.ParseFloat(query.Get("radius_km"), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > maxNearbyRadiusKM {
			return search, fmt.Errorf("radius_km must be greater than 0 and at most %d", maxNearbyRadiusKM)
		}
		search.radiusKM = value
	}
	if query.Has("limit") {
		value, err := strconv.Atoi(query.Get("limit"))
		if err != nil || value < 1 || value > maxNearbyLimit {
			return search, fmt.Errorf("limit must be a whole number between 1 and %d", maxNearbyLimit)
		}
		search.limit = value
	}
	return search, nil
}

// Keep c.point as geography so the radius is measured in metres and the
// existing GiST index can filter candidates before sorting by distance.
const nearbyPhotosQuery = `
		SELECT p.id, p.sha256, p.timestamp,
		       l.id, ST_Y(c.point::geometry), ST_X(c.point::geometry), l.city, l.country
		FROM photos_coordinates c
		JOIN photos_photo p ON p.coordinates_id = c.id
		LEFT JOIN photos_location l ON l.id = c.location_id
		WHERE ST_DWithin(c.point, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		ORDER BY ST_Distance(c.point, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography), p.id
		LIMIT $4`

func (app *application) nearbyPhotos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "Invalid search parameters", http.StatusBadRequest)
		return
	}
	search, err := parseNearbySearch(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	photos, err := app.queryPhotos(r.Context(), nearbyPhotosQuery, search.longitude, search.latitude, search.radiusKM*1000, search.limit)
	if err != nil {
		log.Printf("nearby photo query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writePhotosJSON(w, photos)
}
