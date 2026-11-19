package main

import (
	"net/http"
)

type googleGeocodingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f googleGeocodingRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
