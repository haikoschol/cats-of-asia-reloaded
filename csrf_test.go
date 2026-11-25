//go:build integration

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var csrfInputPattern = regexp.MustCompile(`<input type="hidden" name="csrf_token" value="([^"]+)">`)

func csrfTestForm(t *testing.T, handler http.Handler, path string, cookies []*http.Cookie) (string, []*http.Cookie) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	byName := make(map[string]*http.Cookie)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
		byName[cookie.Name] = cookie
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	match := csrfInputPattern.FindStringSubmatch(w.Body.String())
	if w.Code != http.StatusOK || len(match) != 2 {
		t.Fatalf("form %s did not supply a CSRF token: status %d", path, w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("a page containing a CSRF token could be cached")
	}
	for _, cookie := range w.Result().Cookies() {
		byName[cookie.Name] = cookie
	}
	cookies = nil
	for _, cookie := range byName {
		cookies = append(cookies, cookie)
	}
	return match[1], cookies
}

func csrfTestPost(path string, form url.Values, cookies []*http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	return r
}

func TestCSRFLogin(t *testing.T) {
	for _, name := range []string{
		"valid form", "valid header", "same origin", "missing", "incorrect",
		"query only", "other session", "no session", "expired session",
		"foreign origin", "null origin", "cross-site", "same-site",
	} {
		t.Run(name, func(t *testing.T) {
			app, _ := newAuthTestApplication(t)
			if name == "expired session" {
				app.sessionManager.Lifetime = -time.Second
			}
			handler := app.routes()
			token, cookies := csrfTestForm(t, handler, "/login", nil)
			form := url.Values{
				"username": {"protected-action-user"}, "password": {"test-password"},
				"csrf_token": {token},
			}
			switch name {
			case "valid header", "missing", "query only":
				form.Del("csrf_token")
			case "incorrect":
				form.Set("csrf_token", "incorrect-token")
			case "other session":
				otherToken, _ := csrfTestForm(t, handler, "/login", nil)
				form.Set("csrf_token", otherToken)
			}
			requestCookies := cookies
			if name == "no session" {
				requestCookies = nil
			}
			r := csrfTestPost("/login", form, requestCookies)
			switch name {
			case "valid header":
				r.Header.Set("X-CSRF-Token", token)
			case "same origin":
				r.Header.Set("Origin", "http://example.com")
			case "query only":
				r.URL.RawQuery = url.Values{"csrf_token": {token}}.Encode()
			case "foreign origin":
				r.Header.Set("Origin", "https://foreign.example")
			case "null origin":
				r.Header.Set("Origin", "null")
			case "cross-site", "same-site":
				r.Header.Set("Sec-Fetch-Site", name)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if name == "valid form" || name == "valid header" || name == "same origin" {
				if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
					t.Fatalf("valid login rejected: status %d", w.Code)
				}
				return
			}
			if w.Code != http.StatusForbidden || w.Header().Get("Location") != "" {
				t.Fatalf("unsafe login accepted: status %d", w.Code)
			}
			if strings.Contains(w.Body.String(), token) {
				t.Fatal("CSRF error exposed the token")
			}
			// A failed CSRF check must not authenticate even valid credentials.
			csrfTestForm(t, handler, "/login", cookies)
		})
	}
}

func TestCSRFLogoutAndRotation(t *testing.T) {
	app, _ := newAuthTestApplication(t)
	handler := app.routes()
	beforeLogin, cookies := csrfTestForm(t, handler, "/login", nil)
	unchanged, cookies := csrfTestForm(t, handler, "/login", cookies)
	if unchanged != beforeLogin {
		t.Fatal("opening a second form invalidated the first form")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/login", url.Values{
		"username": {"protected-action-user"}, "password": {"test-password"}, "csrf_token": {beforeLogin},
	}, cookies))
	if w.Code != http.StatusSeeOther {
		t.Fatal("valid login failed")
	}
	authenticatedCookies := w.Result().Cookies()
	// RenewToken must also invalidate the pre-login session cookie.
	csrfTestForm(t, handler, "/login", cookies)
	token, cookies := csrfTestForm(t, handler, "/", authenticatedCookies)
	if token == beforeLogin {
		t.Fatal("login did not rotate the CSRF token")
	}
	r := csrfTestPost("/logout", url.Values{"csrf_token": {token}}, cookies)
	r.Header.Set("Origin", "https://foreign.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("foreign-origin logout accepted a valid token")
	}
	for _, invalid := range []string{"", "incorrect-token", beforeLogin} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, csrfTestPost("/logout", url.Values{"csrf_token": {invalid}}, cookies))
		if w.Code != http.StatusForbidden {
			t.Fatalf("invalid logout token accepted: status %d", w.Code)
		}
		r := httptest.NewRequest(http.MethodGet, "/login", nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusSeeOther {
			t.Fatal("rejected logout ended the session")
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/logout", url.Values{"csrf_token": {token}}, cookies))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatal("valid logout failed")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, csrfTestPost("/logout", url.Values{"csrf_token": {token}}, cookies))
	if w.Code != http.StatusForbidden {
		t.Fatal("logged-out session and token could be replayed")
	}
	afterLogout, _ := csrfTestForm(t, handler, "/login", cookies)
	if afterLogout == token {
		t.Fatal("logout retained the old CSRF token")
	}
}
