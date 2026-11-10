package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type geocodingCoordinates struct {
	latitude, longitude float64
}

type geocodingLocation struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

func (app *application) lookupGeocodingLocation(ctx context.Context, coordinates geocodingCoordinates) (geocodingLocation, error) {
	// Also bound the wait for a free content connection.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	var place geocodingLocation
	err := app.contentDB.QueryRowContext(ctx, `
		SELECT l.city, l.country
		FROM photos_coordinates c
		JOIN photos_location l ON l.id = c.location_id
		WHERE c.point = ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography
		ORDER BY c.id
		LIMIT 1`, coordinates.longitude, coordinates.latitude).Scan(&place.City, &place.Country)
	// Different altitudes can share a point. Like the original application's
	// first(), choose the coordinate record with the lowest ID.
	return place, err
}

func parseGeocodingCoordinates(query url.Values) (geocodingCoordinates, error) {
	var coordinates geocodingCoordinates
	for name, values := range query {
		if name != "latitude" && name != "longitude" {
			return coordinates, fmt.Errorf("unsupported location parameter")
		}
		if len(values) != 1 {
			return coordinates, fmt.Errorf("location parameters must appear only once")
		}
	}
	for _, field := range []struct {
		name     string
		min, max float64
		target   *float64
	}{
		{"latitude", -90, 90, &coordinates.latitude},
		{"longitude", -180, 180, &coordinates.longitude},
	} {
		value, err := strconv.ParseFloat(query.Get(field.name), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < field.min || value > field.max {
			return coordinates, fmt.Errorf("%s must be a finite number between %g and %g", field.name, field.min, field.max)
		}
		*field.target = value
	}
	return coordinates, nil
}

func (app *application) reverseGeocode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeGeocodingError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeGeocodingError(w, http.StatusBadRequest, "Invalid location parameters")
		return
	}
	coordinates, err := parseGeocodingCoordinates(query)
	if err != nil {
		writeGeocodingError(w, http.StatusBadRequest, err.Error())
		return
	}

	place, err := app.lookupGeocodingLocation(r.Context(), coordinates)
	var payload any = place
	if errors.Is(err, sql.ErrNoRows) {
		response, providerErr := app.requestGoogleGeocoding(r.Context(), coordinates)
		if providerErr != nil {
			status := http.StatusBadGateway
			if errors.Is(providerErr, errGoogleGeocodingDisabled) || errors.Is(providerErr, errGoogleGeocodingMissingKey) ||
				errors.Is(providerErr, errGoogleGeocodingQuota) {
				status = http.StatusServiceUnavailable
			} else if errors.Is(providerErr, errGoogleGeocodingTimeout) {
				status = http.StatusGatewayTimeout
			}
			// The provider client returns only fixed, credential-free errors.
			log.Printf("geocoding provider error: %v", providerErr)
			writeGeocodingError(w, status, providerErr.Error())
			return
		}
		payload = extractGoogleGeocodingSuggestions(response)
	} else if err != nil {
		log.Printf("geocoding location query error: %v", err)
		writeGeocodingError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("geocoding JSON error: %v", err)
	}
}

func writeGeocodingError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("geocoding JSON error: %v", err)
	}
}
