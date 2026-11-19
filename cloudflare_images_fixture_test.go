package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const testUploadImageID = "21000000-0000-4000-8000-000000000001"

const testImageUploadURL = "https://upload.imagedelivery.net/test-account/" + testUploadImageID

type cloudflareRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cloudflareRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func imageTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func imageTestEnvelope(result any) string {
	body, _ := json.Marshal(map[string]any{"success": true, "result": result})
	return string(body)
}
