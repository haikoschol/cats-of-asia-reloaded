package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	cloudflareAccountPattern    = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	cloudflareImagePattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	errImageUploadConfiguration = errors.New("Cloudflare Images upload credentials are missing or invalid")
	errImageUploadTimeout       = errors.New("Cloudflare Images request timed out")
	errImageUploadRequest       = errors.New("Cloudflare Images request failed")
	errImageUploadResponse      = errors.New("Cloudflare Images returned an invalid response")
	errImageUploadSession       = errors.New("Upload requires an existing authenticated session")
	errImageUploadOwnership     = errors.New("Image does not belong to this upload session")
	errImageUploadIncomplete    = errors.New("Image upload has not completed")
)

type imageUploadURL struct {
	ID        string    `json:"id"`
	UploadURL string    `json:"uploadURL"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type cloudflareImageResult struct {
	ID        string            `json:"id"`
	UploadURL string            `json:"uploadURL"`
	Uploaded  time.Time         `json:"uploaded"`
	Draft     bool              `json:"draft"`
	Meta      map[string]string `json:"meta"`
	Metadata  map[string]string `json:"metadata"`
}

// Bind uploads to a login without giving Cloudflare the session cookie or
// adding user-management tables. This is stable across parallel requests and
// app restarts, but changes when login renews the session token.
func (app *application) imageUploadSession(ctx context.Context) (string, error) {
	token := app.sessionManager.Token(ctx)
	userID := app.sessionManager.GetInt(ctx, "authenticatedUserID")
	if token == "" || userID <= 0 {
		return "", errImageUploadSession
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("cats-of-asia-image-upload:%d:%s", userID, token)))
	return fmt.Sprintf("%x", digest), nil
}

func (app *application) requestImageUploadURL(ctx context.Context) (imageUploadURL, error) {
	binding, err := app.imageUploadSession(ctx)
	if err != nil {
		return imageUploadURL{}, err
	}
	expires := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second)
	metadata, _ := json.Marshal(map[string]string{"uploadSession": binding})
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"expiry":            expires.Format(time.RFC3339),
		"requireSignedURLs": "false",
		"metadata":          string(metadata),
	} {
		if err := form.WriteField(name, value); err != nil {
			return imageUploadURL{}, errImageUploadRequest
		}
	}
	if err := form.Close(); err != nil {
		return imageUploadURL{}, errImageUploadRequest
	}
	result, err := app.requestCloudflareImage(ctx, http.MethodPost, "v2/direct_upload", &body, form.FormDataContentType())
	if err != nil {
		return imageUploadURL{}, err
	}
	u, err := url.Parse(result.UploadURL)
	if err != nil || !cloudflareImagePattern.MatchString(result.ID) || u.Scheme != "https" ||
		u.Host != "upload.imagedelivery.net" || u.User != nil || u.Fragment != "" || !strings.HasSuffix(u.Path, "/"+result.ID) {
		return imageUploadURL{}, errImageUploadResponse
	}
	return imageUploadURL{ID: result.ID, UploadURL: result.UploadURL, ExpiresAt: expires}, nil
}

// The metadata-saving action must call this after authenticating the request.
// The client's image ID alone never proves ownership or a completed upload.
func (app *application) verifyImageUpload(ctx context.Context, imageID string) error {
	binding, err := app.imageUploadSession(ctx)
	if err != nil {
		return err
	}
	if !cloudflareImagePattern.MatchString(imageID) {
		return errImageUploadOwnership
	}
	result, err := app.requestCloudflareImage(ctx, http.MethodGet, "v1/"+imageID, nil, "")
	if err != nil {
		return err
	}
	// Cloudflare documents the field as meta in its API reference and as
	// metadata in the Direct Creator Upload guide. Accept either, failing
	// closed if both are present but disagree.
	owner := result.Meta["uploadSession"]
	if owner == "" {
		owner = result.Metadata["uploadSession"]
	} else if other := result.Metadata["uploadSession"]; other != "" && other != owner {
		return errImageUploadOwnership
	}
	if result.ID != imageID || subtle.ConstantTimeCompare([]byte(owner), []byte(binding)) != 1 {
		return errImageUploadOwnership
	}
	if result.Draft || result.Uploaded.IsZero() {
		return errImageUploadIncomplete
	}
	return nil
}

func (app *application) requestCloudflareImage(ctx context.Context, method, path string, body io.Reader, contentType string) (cloudflareImageResult, error) {
	if !cloudflareAccountPattern.MatchString(app.config.ImageAccountID) || app.config.ImageAPIKey == "" {
		return cloudflareImageResult{}, errImageUploadConfiguration
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return cloudflareImageResult{}, imageUploadRequestError(err)
	}
	r, err := http.NewRequestWithContext(ctx, method,
		"https://api.cloudflare.com/client/v4/accounts/"+app.config.ImageAccountID+"/images/"+path, body)
	if err != nil {
		return cloudflareImageResult{}, errImageUploadRequest
	}
	r.Header.Set("Authorization", "Bearer "+app.config.ImageAPIKey)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	client := http.Client{}
	if app.cloudflareImagesClient != nil {
		client = *app.cloudflareImagesClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(r)
	if err != nil {
		return cloudflareImageResult{}, imageUploadRequestError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusGatewayTimeout {
		return cloudflareImageResult{}, errImageUploadTimeout
	}
	if response.StatusCode != http.StatusOK {
		return cloudflareImageResult{}, errImageUploadRequest
	}
	const maxResponseBytes = 64 << 10
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return cloudflareImageResult{}, imageUploadRequestError(err)
	}
	if len(data) > maxResponseBytes {
		return cloudflareImageResult{}, errImageUploadResponse
	}
	var envelope struct {
		Success bool                  `json:"success"`
		Result  cloudflareImageResult `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.Success {
		// Never return provider bodies or transport errors: they may contain
		// credentials, upload URLs, or private provider metadata.
		return cloudflareImageResult{}, errImageUploadResponse
	}
	return envelope.Result, nil
}

func imageUploadRequestError(err error) error {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return errImageUploadTimeout
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return errImageUploadRequest
}
