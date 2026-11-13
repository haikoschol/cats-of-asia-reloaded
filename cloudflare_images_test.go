package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

const testUploadImageID = "21000000-0000-4000-8000-000000000001"
const testImageUploadURL = "https://upload.imagedelivery.net/test-account/" + testUploadImageID

type cloudflareRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cloudflareRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func cloudflareTestApp(transport cloudflareRoundTripFunc) *application {
	return &application{
		config:                 config{ImageAccountID: strings.Repeat("a", 32), ImageAPIKey: "fake-private-cloudflare-key"},
		sessionManager:         newSessionManager(config{}),
		cloudflareImagesClient: &http.Client{Transport: transport},
	}
}

func imageTestSession(t *testing.T, app *application) context.Context {
	t.Helper()
	ctx, err := app.sessionManager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	app.sessionManager.Put(ctx, "authenticatedUserID", 1)
	if _, _, err := app.sessionManager.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func imageTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func imageTestEnvelope(result any) string {
	body, _ := json.Marshal(map[string]any{"success": true, "result": result})
	return string(body)
}

func TestImageUploadURLRequest(t *testing.T) {
	var binding string
	app := cloudflareTestApp(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.Host != "api.cloudflare.com" ||
			r.URL.Path != "/client/v4/accounts/"+strings.Repeat("a", 32)+"/images/v2/direct_upload" ||
			r.Header.Get("Authorization") != "Bearer fake-private-cloudflare-key" {
			t.Fatal("incorrect Cloudflare upload request")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second {
			t.Fatal("missing request deadline")
		}
		if err := r.ParseMultipartForm(4096); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		if len(r.MultipartForm.Value) != 3 || len(r.MultipartForm.File) != 0 || r.FormValue("requireSignedURLs") != "false" {
			t.Fatal("request must create a public-image draft without uploading bytes")
		}
		expires, err := time.Parse(time.RFC3339, r.FormValue("expiry"))
		if err != nil || time.Until(expires) < 29*time.Minute || time.Until(expires) > 30*time.Minute {
			t.Fatal("incorrect upload expiry")
		}
		var meta map[string]string
		if err := json.Unmarshal([]byte(r.FormValue("metadata")), &meta); err != nil || len(meta) != 1 || meta["uploadSession"] != binding {
			t.Fatal("missing private session binding")
		}
		return imageTestResponse(200, imageTestEnvelope(map[string]string{"id": testUploadImageID, "uploadURL": testImageUploadURL})), nil
	})
	ctx := imageTestSession(t, app)
	var err error
	binding, err = app.imageUploadSession(ctx)
	if err != nil || len(binding) != 64 || binding == app.sessionManager.Token(ctx) {
		t.Fatal("invalid session binding")
	}
	result, err := app.requestImageUploadURL(ctx)
	if err != nil || result.ID != testUploadImageID || result.UploadURL != testImageUploadURL || result.ExpiresAt.IsZero() {
		t.Fatalf("upload URL request failed: %v", err)
	}
	// Multiple requests in the same login need no mutable pending-upload list.
	if _, err := app.requestImageUploadURL(ctx); err != nil {
		t.Fatal(err)
	}
	other := imageTestSession(t, app)
	otherBinding, err := app.imageUploadSession(other)
	if err != nil || otherBinding == binding {
		t.Fatal("two logins share an upload binding")
	}
}

func TestImageUploadProviderFailures(t *testing.T) {
	valid := imageTestEnvelope(map[string]string{"id": testUploadImageID, "uploadURL": testImageUploadURL})
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", 401, "fake-private-cloudflare-key", errImageUploadRequest},
		{"forbidden", 403, "fake-private-cloudflare-key", errImageUploadRequest},
		{"quota", 429, "fake-private-cloudflare-key", errImageUploadRequest},
		{"unavailable", 503, "fake-private-cloudflare-key", errImageUploadRequest},
		{"timeout", 504, "fake-private-cloudflare-key", errImageUploadTimeout},
		{"bad JSON", 200, `{"fake-private-cloudflare-key":`, errImageUploadResponse},
		{"trailing JSON", 200, valid + `{}`, errImageUploadResponse},
		{"unsuccessful", 200, `{"success":false,"errors":[{"message":"fake-private-cloudflare-key"}]}`, errImageUploadResponse},
		{"empty result", 200, `{"success":true,"result":{}}`, errImageUploadResponse},
		{"oversized", 200, strings.Repeat("x", (64<<10)+1), errImageUploadResponse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := cloudflareTestApp(func(*http.Request) (*http.Response, error) { return imageTestResponse(tt.status, tt.body), nil })
			_, err := app.requestImageUploadURL(imageTestSession(t, app))
			if !errors.Is(err, tt.want) {
				t.Fatalf("wanted %v, got %v", tt.want, err)
			}
			if strings.Contains(err.Error(), "fake-private-cloudflare-key") {
				t.Fatal("provider error leaked credentials")
			}
		})
	}
	for _, address := range []string{"http://upload.imagedelivery.net/test/" + testUploadImageID, "https://foreign.example/" + testUploadImageID,
		"https://fake-private-cloudflare-key@upload.imagedelivery.net/test/" + testUploadImageID, "https://upload.imagedelivery.net/test/other-id"} {
		t.Run(address, func(t *testing.T) {
			app := cloudflareTestApp(func(*http.Request) (*http.Response, error) {
				return imageTestResponse(200, imageTestEnvelope(map[string]string{"id": testUploadImageID, "uploadURL": address})), nil
			})
			if _, err := app.requestImageUploadURL(imageTestSession(t, app)); !errors.Is(err, errImageUploadResponse) {
				t.Fatal("unsafe upload URL accepted")
			}
		})
	}
}

func TestImageUploadTransportAndConfiguration(t *testing.T) {
	for _, mode := range []string{"missing account", "invalid account", "missing key", "canceled", "deadline", "transport", "body read", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			app := cloudflareTestApp(func(r *http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "deadline":
					<-r.Context().Done()
					return nil, r.Context().Err()
				case "transport":
					return nil, errors.New("fake-private-cloudflare-key")
				case "body read":
					response := imageTestResponse(200, "")
					response.Body = io.NopCloser(iotest.ErrReader(errors.New("fake-private-cloudflare-key")))
					return response, nil
				case "redirect":
					response := imageTestResponse(302, "")
					response.Header.Set("Location", "https://foreign.example/secret")
					return response, nil
				}
				t.Fatal("unconfigured or canceled request reached provider")
				return nil, nil
			})
			ctx := imageTestSession(t, app)
			want := errImageUploadRequest
			switch mode {
			case "missing account":
				app.config.ImageAccountID = ""
				want = errImageUploadConfiguration
			case "invalid account":
				app.config.ImageAccountID = "../private"
				want = errImageUploadConfiguration
			case "missing key":
				app.config.ImageAPIKey = ""
				want = errImageUploadConfiguration
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			case "deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
				want = errImageUploadTimeout
			}
			_, err := app.requestImageUploadURL(ctx)
			if !errors.Is(err, want) || strings.Contains(err.Error(), "fake-private-cloudflare-key") {
				t.Fatalf("unsafe or incorrect failure: %v", err)
			}
			wantCalls := 1
			if mode == "missing account" || mode == "invalid account" || mode == "missing key" || mode == "canceled" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatal("unexpected provider call or followed redirect")
			}
		})
	}
}

func TestVerifyImageUpload(t *testing.T) {
	for _, mode := range []string{"complete meta", "complete metadata", "draft", "missing upload time", "other session", "old session", "missing owner", "wrong image", "conflicting metadata", "invalid ID"} {
		t.Run(mode, func(t *testing.T) {
			var result map[string]any
			calls := 0
			app := cloudflareTestApp(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/client/v4/accounts/"+strings.Repeat("a", 32)+"/images/v1/"+testUploadImageID {
					t.Fatal("incorrect image details request")
				}
				return imageTestResponse(200, imageTestEnvelope(result)), nil
			})
			ctx := imageTestSession(t, app)
			binding, err := app.imageUploadSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			result = map[string]any{"id": testUploadImageID, "uploaded": "2026-06-15T07:30:45Z", "meta": map[string]string{"uploadSession": binding}}
			var want error
			id := testUploadImageID
			switch mode {
			case "complete metadata":
				delete(result, "meta")
				result["metadata"] = map[string]string{"uploadSession": binding}
			case "draft":
				result["draft"] = true
				want = errImageUploadIncomplete
			case "missing upload time":
				delete(result, "uploaded")
				want = errImageUploadIncomplete
			case "other session":
				ctx = imageTestSession(t, app)
				want = errImageUploadOwnership
			case "old session":
				if err := app.sessionManager.RenewToken(ctx); err != nil {
					t.Fatal(err)
				}
				want = errImageUploadOwnership
			case "missing owner":
				delete(result, "meta")
				want = errImageUploadOwnership
			case "wrong image":
				result["id"] = "21000000-0000-4000-8000-000000000002"
				want = errImageUploadOwnership
			case "conflicting metadata":
				result["metadata"] = map[string]string{"uploadSession": "other-login"}
				want = errImageUploadOwnership
			case "invalid ID":
				id = "../another-image"
				want = errImageUploadOwnership
			}
			if err := app.verifyImageUpload(ctx, id); !errors.Is(err, want) {
				t.Fatalf("wanted %v, got %v", want, err)
			}
			if mode == "invalid ID" && calls != 0 {
				t.Fatal("invalid image ID reached provider")
			}
		})
	}
}
