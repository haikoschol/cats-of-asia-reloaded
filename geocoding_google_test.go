package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

type googleGeocodingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f googleGeocodingRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func googleGeocodingTestApp(transport googleGeocodingRoundTripFunc) *application {
	return &application{
		config:                config{GoogleGeocodingEnabled: true, GoogleMapsAPIKey: "fake-private-google-key"},
		googleGeocodingClient: &http.Client{Transport: transport},
	}
}

func TestGoogleGeocodingRequest(t *testing.T) {
	for _, tt := range []struct {
		name        string
		coordinates geocodingCoordinates
		latlng      string
	}{
		{"Bangkok", geocodingCoordinates{13.75, 100.5}, "13.75,100.5"},
		{"zero point", geocodingCoordinates{0, 0}, "0,0"},
		{"negative coordinates", geocodingCoordinates{-33.9, -70.6}, "-33.9,-70.6"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			app := googleGeocodingTestApp(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.URL.Scheme != "https" ||
					r.URL.Host != "maps.googleapis.com" || r.URL.Path != "/maps/api/geocode/json" {
					t.Fatal("unexpected provider address or method")
				}
				query := r.URL.Query()
				if len(query) != 4 || query.Get("latlng") != tt.latlng || query.Get("language") != "en" ||
					query.Get("key") != "fake-private-google-key" ||
					query.Get("result_type") != "country|locality|colloquial_area|administrative_area_level_1|administrative_area_level_2|administrative_area_level_3|administrative_area_level_4|administrative_area_level_5" {
					t.Fatal("provider parameters do not match the original location lookup")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
					t.Fatal("provider request has no bounded deadline")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
					"status":"OK", "results":[{"address_components":[
						{"long_name":"Bangkok", "short_name":"BKK", "types":["locality","political"]},
						{"long_name":"Thailand", "types":["country","political"]}
					]}]
				}`))}, nil
			})
			result, err := app.requestGoogleGeocoding(context.Background(), tt.coordinates)
			if err != nil || calls != 1 || result.Status != "OK" || len(result.Results) != 1 ||
				len(result.Results[0].AddressComponents) != 2 {
				t.Fatalf("provider response was not decoded: calls %d, error %v", calls, err)
			}
			components := result.Results[0].AddressComponents
			if components[0].LongName != "Bangkok" || components[1].LongName != "Thailand" ||
				len(components[0].Types) != 2 || components[0].Types[0] != "locality" {
				t.Fatal("provider address names or types were lost")
			}
		})
	}
}

func TestGoogleGeocodingConfigurationRequired(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		key     string
		want    error
	}{
		{"disabled", false, "", errGoogleGeocodingDisabled},
		{"existing key does not enable requests", false, "fake-private-google-key", errGoogleGeocodingDisabled},
		{"enabled without key", true, "", errGoogleGeocodingMissingKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := googleGeocodingTestApp(func(*http.Request) (*http.Response, error) {
				t.Fatal("unconfigured provider attempted a request")
				return nil, nil
			})
			app.config.GoogleGeocodingEnabled = tt.enabled
			app.config.GoogleMapsAPIKey = tt.key
			if _, err := app.requestGoogleGeocoding(context.Background(), geocodingCoordinates{}); !errors.Is(err, tt.want) {
				t.Fatalf("wanted %v, got %v", tt.want, err)
			}
		})
	}
}

func TestGoogleGeocodingResponses(t *testing.T) {
	for _, tt := range []struct {
		name            string
		status          int
		body, wantError string
	}{
		{"no match", 200, `{"status":"ZERO_RESULTS","results":[]}`, ""},
		{"HTTP unauthorized", 401, "fake-private-google-key", "Google geocoding rejected the request"},
		{"HTTP rejected", 403, "fake-private-google-key", "Google geocoding rejected the request"},
		{"HTTP quota", 429, "fake-private-google-key", "Google geocoding quota or billing limit reached"},
		{"HTTP request timeout", 408, "fake-private-google-key", "Google geocoding request timed out"},
		{"HTTP gateway timeout", 504, "fake-private-google-key", "Google geocoding request timed out"},
		{"HTTP failure", 503, "fake-private-google-key", "Google geocoding returned an unsuccessful HTTP response"},
		{"redirect", 302, "", "Google geocoding returned an unsuccessful HTTP response"},
		{"invalid JSON", 200, `{"fake-private-google-key":`, "Google geocoding returned an invalid response"},
		{"trailing JSON", 200, `{"status":"ZERO_RESULTS"}{}`, "Google geocoding returned an invalid response"},
		{"empty success", 200, `{"status":"OK","results":[]}`, "Google geocoding returned an invalid response"},
		{"missing status", 200, `{}`, "Google geocoding could not process the request"},
		{"denied", 200, `{"status":"REQUEST_DENIED","error_message":"fake-private-google-key"}`, "Google geocoding rejected the request"},
		{"query limit", 200, `{"status":"OVER_QUERY_LIMIT"}`, "Google geocoding quota or billing limit reached"},
		{"daily limit", 200, `{"status":"OVER_DAILY_LIMIT"}`, "Google geocoding quota or billing limit reached"},
		{"invalid request", 200, `{"status":"INVALID_REQUEST"}`, "Google geocoding could not process the request"},
		{"provider failure", 200, `{"status":"UNKNOWN_ERROR"}`, "Google geocoding could not process the request"},
		{"unrecognized status", 200, `{"status":"fake-private-google-key"}`, "Google geocoding could not process the request"},
		{"oversized response", 200, strings.Repeat(" ", (1<<20)+1), "Google geocoding response is too large"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			app := googleGeocodingTestApp(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tt.status,
					Header: http.Header{"Location": {"https://untrusted.example/?key=fake-private-google-key"}},
					Body:   io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			result, err := app.requestGoogleGeocoding(context.Background(), geocodingCoordinates{})
			if calls != 1 {
				t.Fatalf("provider request was retried or redirected: %d calls", calls)
			}
			if tt.wantError == "" {
				if err != nil || result.Status != "ZERO_RESULTS" || result.Results == nil || len(result.Results) != 0 {
					t.Fatalf("no-match response was not preserved: error %v", err)
				}
			} else if err == nil || err.Error() != tt.wantError || result.Status != "" || len(result.Results) != 0 {
				t.Fatalf("provider failure was not reported safely: error %v", err)
			}
		})
	}
}

func TestGoogleGeocodingTransportFailures(t *testing.T) {
	for _, bodyFailure := range []bool{false, true} {
		name := "request failure"
		if bodyFailure {
			name = "response body failure"
		}
		t.Run(name, func(t *testing.T) {
			app := googleGeocodingTestApp(func(r *http.Request) (*http.Response, error) {
				privateError := errors.New("failed request: " + r.URL.String())
				if bodyFailure {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(iotest.ErrReader(privateError))}, nil
				}
				return nil, privateError
			})
			if _, err := app.requestGoogleGeocoding(context.Background(), geocodingCoordinates{}); err == nil ||
				err.Error() != "Google geocoding request failed" {
				t.Fatal("transport failure exposed private request details")
			}
		})
	}
}

func TestGoogleGeocodingCancellation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "caller deadline"
		if expired {
			name = "already canceled"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			want := errGoogleGeocodingTimeout
			if expired {
				cancel()
				want = context.Canceled
			}
			app := googleGeocodingTestApp(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			start := time.Now()
			if _, err := app.requestGoogleGeocoding(ctx, geocodingCoordinates{}); !errors.Is(err, want) || time.Since(start) > time.Second {
				t.Fatalf("provider request did not honor cancellation: error %v", err)
			}
		})
	}
}
