package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const googleGeocodingResultTypes = "country|locality|colloquial_area|administrative_area_level_1|administrative_area_level_2|administrative_area_level_3|administrative_area_level_4|administrative_area_level_5"

var (
	errGoogleGeocodingDisabled   = errors.New("Google geocoding is disabled")
	errGoogleGeocodingMissingKey = errors.New("Google geocoding API key is not configured")
	errGoogleGeocodingTimeout    = errors.New("Google geocoding request timed out")
	errGoogleGeocodingRejected   = errors.New("Google geocoding rejected the request")
	errGoogleGeocodingQuota      = errors.New("Google geocoding quota or billing limit reached")
)

type googleGeocodingResponse struct {
	Status  string                  `json:"status"`
	Results []googleGeocodingResult `json:"results"`
}

type googleGeocodingResult struct {
	AddressComponents []googleAddressComponent `json:"address_components"`
}

type googleAddressComponent struct {
	LongName string   `json:"long_name"`
	Types    []string `json:"types"`
}

type geocodingSuggestions struct {
	Country        *string  `json:"country"`
	CityCandidates []string `json:"cityCandidates"`
	City           string   `json:"city,omitempty"`
}

func extractGoogleGeocodingSuggestions(response googleGeocodingResponse) geocodingSuggestions {
	suggestions := geocodingSuggestions{CityCandidates: []string{}}
	cities := make(map[string]bool)
	for _, result := range response.Results {
		for _, component := range result.AddressComponents {
			// Missing names must not become selectable suggestions.
			if strings.TrimSpace(component.LongName) == "" {
				continue
			}
			for _, kind := range component.Types {
				switch kind {
				case "country":
					if suggestions.Country == nil {
						name := component.LongName
						suggestions.Country = &name
					}
				case "locality", "colloquial_area", "administrative_area_level_1", "administrative_area_level_2",
					"administrative_area_level_3", "administrative_area_level_4", "administrative_area_level_5":
					cities[component.LongName] = true
				}
			}
		}
	}
	for name := range cities {
		suggestions.CityCandidates = append(suggestions.CityCandidates, name)
	}
	slices.Sort(suggestions.CityCandidates)
	if len(suggestions.CityCandidates) == 1 {
		suggestions.City = suggestions.CityCandidates[0]
	}
	return suggestions
}

func (app *application) requestGoogleGeocoding(ctx context.Context, coordinates geocodingCoordinates) (googleGeocodingResponse, error) {
	if !app.config.GoogleGeocodingEnabled {
		return googleGeocodingResponse{}, errGoogleGeocodingDisabled
	}
	if app.config.GoogleMapsAPIKey == "" {
		return googleGeocodingResponse{}, errGoogleGeocodingMissingKey
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return googleGeocodingResponse{}, googleGeocodingRequestError(err)
	}
	query := url.Values{
		"key":         {app.config.GoogleMapsAPIKey},
		"language":    {"en"},
		"latlng":      {strconv.FormatFloat(coordinates.latitude, 'f', -1, 64) + "," + strconv.FormatFloat(coordinates.longitude, 'f', -1, 64)},
		"result_type": {googleGeocodingResultTypes},
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://maps.googleapis.com/maps/api/geocode/json?"+query.Encode(), nil)
	if err != nil {
		return googleGeocodingResponse{}, errors.New("Cannot create Google geocoding request")
	}
	client := http.Client{}
	if app.googleGeocodingClient != nil {
		client = *app.googleGeocodingClient
	}
	// Never follow a redirect with a credential-bearing provider request.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(r)
	if err != nil {
		return googleGeocodingResponse{}, googleGeocodingRequestError(err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return googleGeocodingResponse{}, errGoogleGeocodingRejected
	case http.StatusTooManyRequests:
		return googleGeocodingResponse{}, errGoogleGeocodingQuota
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return googleGeocodingResponse{}, errGoogleGeocodingTimeout
	default:
		return googleGeocodingResponse{}, errors.New("Google geocoding returned an unsuccessful HTTP response")
	}
	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return googleGeocodingResponse{}, googleGeocodingRequestError(err)
	}
	if len(body) > maxResponseBytes {
		return googleGeocodingResponse{}, errors.New("Google geocoding response is too large")
	}
	var result googleGeocodingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return googleGeocodingResponse{}, errors.New("Google geocoding returned an invalid response")
	}
	switch result.Status {
	case "OK":
		if len(result.Results) == 0 {
			return googleGeocodingResponse{}, errors.New("Google geocoding returned an invalid response")
		}
		return result, nil
	case "ZERO_RESULTS":
		return googleGeocodingResponse{Status: "ZERO_RESULTS", Results: []googleGeocodingResult{}}, nil
	case "REQUEST_DENIED":
		return googleGeocodingResponse{}, errGoogleGeocodingRejected
	case "OVER_QUERY_LIMIT", "OVER_DAILY_LIMIT":
		return googleGeocodingResponse{}, errGoogleGeocodingQuota
	default:
		// Provider error messages and transport errors may contain the API key
		// or complete request URL. Return only fixed messages to callers/logs.
		return googleGeocodingResponse{}, errors.New("Google geocoding could not process the request")
	}
}

func googleGeocodingRequestError(err error) error {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return errGoogleGeocodingTimeout
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return errors.New("Google geocoding request failed")
}
