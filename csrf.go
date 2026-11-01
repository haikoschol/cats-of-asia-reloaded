package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func (app *application) csrfToken(ctx context.Context) string {
	token, _ := app.sessionManager.Get(ctx, "csrfToken").(string)
	if token == "" {
		token = rand.Text()
		app.sessionManager.Put(ctx, "csrfToken", token)
	}
	return token
}

// protectCSRF runs inside LoadAndSave and protects every non-safe request,
// including future write routes. Reading pages or API data needs no token.
func (app *application) protectCSRF(next http.Handler) http.Handler {
	origins := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")

		if err := origins.Check(r); err != nil {
			rejectCSRF(w, r)
			return
		}
		expected, _ := app.sessionManager.Get(r.Context(), "csrfToken").(string)
		provided := r.Header.Get("X-CSRF-Token")
		if provided == "" {
			// ParseForm reads URL-encoded forms, not multipart uploads. Uploads
			// and JSON clients send the token in the header instead. Never take
			// a token from the URL, where it could end up in logs or history.
			if err := r.ParseForm(); err != nil {
				rejectCSRF(w, r)
				return
			}
			provided = r.PostForm.Get("csrf_token")
		}
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			rejectCSRF(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rejectCSRF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden"}); err != nil {
			log.Printf("CSRF JSON error: %v", err)
		}
		return
	}
	http.Error(w, "Forbidden: reload the page and try again.", http.StatusForbidden)
}
