package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func sessionTestCookie(t *testing.T, response *http.Response, name string, secure bool) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name != name {
			continue
		}
		if cookie.Secure != secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode ||
			cookie.Path != "/" || cookie.Domain != "" {
			t.Fatal("session cookie has incorrect security attributes")
		}
		return cookie
	}
	t.Fatal("response did not set the session cookie")
	return nil
}

func sessionTestForm(t *testing.T, client *http.Client, endpoint string) (*http.Response, string) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	match := csrfInputPattern.FindSubmatch(body)
	if response.StatusCode != http.StatusOK || len(match) != 2 {
		t.Fatalf("expected an HTML form, got status %d", response.StatusCode)
	}
	return response, string(match[1])
}

func TestSessionCookiesHTTPAndHTTPS(t *testing.T) {
	for _, tt := range []struct {
		name   string
		secure bool
	}{
		{name: "local HTTP"},
		{name: "production HTTPS", secure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newAuthTestApplication(t)
			app.config.SessionCookieSecure = tt.secure
			app.sessionManager = newSessionManager(app.config)
			server := httptest.NewUnstartedServer(app.routes())
			if tt.secure {
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			client := server.Client()
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client.Jar = jar
			client.CheckRedirect = func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

			response, token := sessionTestForm(t, client, server.URL+"/login")
			cookie := sessionTestCookie(t, response, app.sessionManager.Cookie.Name, tt.secure)
			if cookie.MaxAge <= 0 || cookie.MaxAge > int((12*time.Hour).Seconds())+1 {
				t.Fatal("session cookie did not retain its 12-hour lifetime")
			}
			endpoint, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if len(jar.Cookies(endpoint)) == 0 {
				t.Fatal("client did not retain the session cookie")
			}
			if tt.secure {
				// Browsers and Go treat loopback hosts as secure even on HTTP.
				// Check transport and host scope using a normal production host.
				productionJar, err := cookiejar.New(nil)
				if err != nil {
					t.Fatal(err)
				}
				production := &url.URL{Scheme: "https", Host: "reloaded.example"}
				productionJar.SetCookies(production, []*http.Cookie{cookie})
				if len(productionJar.Cookies(production)) != 1 {
					t.Fatal("Secure cookie was not available over HTTPS")
				}
				production.Scheme = "http"
				if len(productionJar.Cookies(production)) != 0 {
					t.Fatal("Secure cookie would be sent over plain HTTP")
				}
				production.Scheme = "https"
				production.Host = "analytics.example"
				if len(productionJar.Cookies(production)) != 0 {
					t.Fatal("session cookie was shared with a sibling app")
				}
			}

			response, err = client.PostForm(server.URL+"/login", url.Values{
				"username": {"protected-action-user"}, "password": {"test-password"}, "csrf_token": {token},
			})
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/" {
				t.Fatal("cookie-based login failed")
			}
			loggedIn := sessionTestCookie(t, response, app.sessionManager.Cookie.Name, tt.secure)
			if loggedIn.Value == cookie.Value {
				t.Fatal("login did not replace the anonymous session cookie")
			}

			_, token = sessionTestForm(t, client, server.URL+"/")
			response, err = client.PostForm(server.URL+"/logout", url.Values{"csrf_token": {token}})
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/" {
				t.Fatal("cookie-based logout failed")
			}
			expired := sessionTestCookie(t, response, app.sessionManager.Cookie.Name, tt.secure)
			if expired.MaxAge >= 0 || expired.Value != "" || !expired.Expires.Before(time.Now()) || len(jar.Cookies(endpoint)) != 0 {
				t.Fatal("logout did not remove the cookie from the client")
			}
			sessionTestForm(t, client, server.URL+"/login")
		})
	}
}

func TestSessionCookieBehindProxy(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app, _ := newAuthTestApplication(t)
		app.sessionManager = newSessionManager(config{SessionCookieSecure: secure})
		// Caddy's connection to Go is HTTP even when the browser used HTTPS.
		r := httptest.NewRequest(http.MethodGet, "http://reloaded.example/login", nil)
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("proxy request failed: status %d", w.Code)
		}
		sessionTestCookie(t, w.Result(), app.sessionManager.Cookie.Name, secure)
	}
}
