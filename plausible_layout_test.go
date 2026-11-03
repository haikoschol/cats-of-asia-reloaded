package main

import (
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestPlausibleLayout(t *testing.T) {
	// Rendering HTML must never fetch the tracker or send a visitor event. Use
	// an HTTPS server that is already closed to represent unavailable analytics.
	analytics := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("rendering contacted the analytics service")
	}))
	analytics.Close()
	script := regexp.MustCompile(`<script\b[^>]*\bdata-domain="[^"]*"[^>]*>`)
	for _, settings := range []struct {
		name string
		cfg  config
	}{
		{name: "default off"},
		{name: "explicitly off", cfg: config{PlausibleDomain: "reloaded.example.test", PlausibleScriptURL: analytics.URL + "/js/script.js"}},
		{name: "enabled unavailable service", cfg: config{PlausibleEnabled: true, PlausibleDomain: "reloaded.example.test", PlausibleScriptURL: analytics.URL + "/js/script.js"}},
		{name: "custom domain and script", cfg: config{PlausibleEnabled: true, PlausibleDomain: "other.example.test", PlausibleScriptURL: "https://custom.example.test/tracker.js?version=1&feature=2"}},
	} {
		t.Run(settings.name, func(t *testing.T) {
			app := &application{config: settings.cfg}
			for _, page := range []struct {
				name   string
				status int
			}{
				{name: "home.tmpl", status: http.StatusOK},
				{name: "login.tmpl", status: http.StatusOK},
				{name: "favorites.tmpl", status: http.StatusOK},
				{name: "nearme.tmpl", status: http.StatusOK},
				{name: "upload.tmpl", status: http.StatusOK},
				{name: "login.tmpl", status: http.StatusUnprocessableEntity},
			} {
				w := httptest.NewRecorder()
				app.renderPage(w, page.status, page.name, templateData{FormError: "Example login error"})
				if w.Code != page.status || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
					t.Fatalf("%s did not render with tracking settings", page.name)
				}
				body := w.Body.String()
				scripts := script.FindAllString(body, -1)
				if !settings.cfg.PlausibleEnabled {
					if len(scripts) != 0 || strings.Contains(body, settings.cfg.PlausibleScriptURL) && settings.cfg.PlausibleScriptURL != "" {
						t.Errorf("%s included disabled tracking", page.name)
					}
					continue
				}
				if len(scripts) != 1 {
					t.Fatalf("%s: expected exactly one tracking script, got %d", page.name, len(scripts))
				}
				for _, attribute := range []string{
					` async `,
					`data-domain="` + html.EscapeString(settings.cfg.PlausibleDomain) + `"`,
					`src="` + html.EscapeString(settings.cfg.PlausibleScriptURL) + `"`,
				} {
					if !strings.Contains(scripts[0], attribute) {
						t.Errorf("%s: tracking script missing %s", page.name, attribute)
					}
				}
				if strings.Count(body, `src="`+html.EscapeString(settings.cfg.PlausibleScriptURL)+`"`) != 1 {
					t.Errorf("%s loaded the tracking URL more than once", page.name)
				}
			}
		})
	}
}
