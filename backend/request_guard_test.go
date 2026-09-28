package backend

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

// guardReq sends one request from the device itself through the whole
// server: connectionMiddleware and the real router.
func guardReq(a *App, method, target, host string, header map[string]string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:40000"
	req.Host = host
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.connectionMiddleware(mux).ServeHTTP(rec, req)
	return rec
}

// The probe of finding X1: a page of another site makes the browser of the
// device send a write. The server must refuse it, and the note must not
// change. See doc/decisions/0017-refuse-a-request-that-another-site-sends.md.
func TestAnotherSiteCannotWriteANote(t *testing.T) {
	a := newTestApp(t)
	mdPath, _, _, _ := a.resolvePageName("GuardProbe")
	target := "/api/save?" + url.Values{"name": {"GuardProbe"}, "content": {"# Probe"}}.Encode()

	for _, tc := range []struct {
		why    string
		host   string
		header map[string]string
	}{
		{"a rebound name", "attacker.example:8080", map[string]string{"Origin": "https://evil.example"}},
		{"another origin", "localhost:8080", map[string]string{"Origin": "https://evil.example"}},
		{"another port of this device", "localhost:8080", map[string]string{"Origin": "http://localhost:3000"}},
		{"an opaque origin", "localhost:8080", map[string]string{"Origin": "null"}},
		{"a cross-site fetch", "localhost:8080", map[string]string{"Sec-Fetch-Site": "cross-site"}},
	} {
		if w := guardReq(a, http.MethodPost, target, tc.host, tc.header); w.Code != http.StatusForbidden {
			t.Errorf("%s: answered %d, want 403", tc.why, w.Code)
		}
	}
	if _, err := os.Stat(mdPath); err == nil {
		t.Fatal("a request of another site wrote the note")
	}

	// The page of the server itself, and Java with no Origin, may write.
	for _, header := range []map[string]string{
		{"Origin": "http://localhost:8080", "Sec-Fetch-Site": "same-origin"},
		nil,
	} {
		if w := guardReq(a, http.MethodPost, target, "localhost:8080", header); w.Code != http.StatusOK {
			t.Errorf("Origin %q: answered %d %q, want 200", header["Origin"], w.Code, w.Body.String())
		}
	}
}

// A rebound name must not read the passwords that GET /api/config answers
// to the device itself.
func TestAReboundNameCannotReadTheConfig(t *testing.T) {
	a := newTestApp(t)
	if w := guardReq(a, http.MethodGet, "/api/config", "attacker.example:8080", nil); w.Code != http.StatusForbidden {
		t.Errorf("a foreign Host answered %d, want 403", w.Code)
	}
	if w := guardReq(a, http.MethodGet, "/api/config", "127.0.0.1:8080", nil); w.Code != http.StatusOK {
		t.Errorf("127.0.0.1 answered %d, want 200", w.Code)
	}
}

func TestIsKnownHost(t *testing.T) {
	a := newTestApp(t)
	a.config.update(func(c *Config) { c.Hostname = "Pixel7" })
	for host, want := range map[string]bool{
		"":                      true,
		"localhost":             true,
		"LOCALHOST:8080":        true,
		"127.0.0.1:8080":        true,
		"[::1]:8080":            true,
		"192.168.1.5:8080":      true,
		"[fe80::1%25wlan0]:80":  true,
		"pixel7:8080":           true,
		"pixel7.local.:8080":    true,
		"attacker.example:8080": false,
		"localhost.evil.com":    false,
		"pixel7.evil.com":       false,
	} {
		if got := a.isKnownHost(host); got != want {
			t.Errorf("isKnownHost(%q) = %v, want %v", host, got, want)
		}
	}
}
