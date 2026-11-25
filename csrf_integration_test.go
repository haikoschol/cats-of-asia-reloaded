//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestCSRFWriteMethodsAndJSON(t *testing.T) {
	app := &application{sessionManager: scs.New()}
	ctx, err := app.sessionManager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	token := app.csrfToken(ctx)
	sessionToken, _, err := app.sessionManager.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, valid := range []bool{false, true} {
			name := "missing"
			if valid {
				name = "valid"
			}
			t.Run(method+"/"+name, func(t *testing.T) {
				body := `{"example":"unchanged request body"}`
				r := httptest.NewRequest(method, "/api/test-write", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.AddCookie(&http.Cookie{Name: app.sessionManager.Cookie.Name, Value: sessionToken})
				if valid {
					r.Header.Set("X-CSRF-Token", token)
				}
				called := false
				action := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					data, err := io.ReadAll(r.Body)
					if err != nil || string(data) != body {
						t.Error("CSRF protection consumed the JSON body")
					}
					w.WriteHeader(http.StatusNoContent)
				})
				w := httptest.NewRecorder()
				app.sessionManager.LoadAndSave(app.protectCSRF(action)).ServeHTTP(w, r)
				if valid {
					if !called || w.Code != http.StatusNoContent {
						t.Fatal("valid write did not execute")
					}
					return
				}
				if called || w.Code != http.StatusForbidden || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("invalid API write was not rejected as JSON")
				}
				var bodyJSON map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &bodyJSON); err != nil || bodyJSON["error"] != "Forbidden" {
					t.Fatal("invalid CSRF error body")
				}
			})
		}
	}
	t.Run("multipart upload", func(t *testing.T) {
		body := "--test-boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"cat.jpg\"\r\n" +
			"Content-Type: image/jpeg\r\n\r\nexample-photo-data\r\n--test-boundary--\r\n"
		r := httptest.NewRequest(http.MethodPost, "/api/test-upload", strings.NewReader(body))
		r.Header.Set("Content-Type", "multipart/form-data; boundary=test-boundary")
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(&http.Cookie{Name: app.sessionManager.Cookie.Name, Value: sessionToken})
		w := httptest.NewRecorder()
		action := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil || string(data) != body || r.MultipartForm != nil {
				t.Error("CSRF protection consumed or parsed the upload")
			}
			w.WriteHeader(http.StatusNoContent)
		})
		app.sessionManager.LoadAndSave(app.protectCSRF(action)).ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatal("valid multipart header token was rejected")
		}
	})
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			w := httptest.NewRecorder()
			app.sessionManager.LoadAndSave(app.protectCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))).ServeHTTP(w, httptest.NewRequest(method, "/api/test-read", nil))
			if w.Code != http.StatusNoContent || len(w.Result().Cookies()) != 0 {
				t.Fatal("safe read required a token or created a session cookie")
			}
		})
	}
}
