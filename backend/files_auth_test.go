package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// Authorization
// ----------------------------------------------------------------------

func remoteFilesRequest(t *testing.T, a *App, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/OMNGoFiles.html?tree=served&dir=js%2F", nil)
	req.RemoteAddr = "192.168.1.50:41234"
	// A signed cookie, and not the bare word: the server refuses an
	// unsigned value. See session.go.
	if cookie != "" {
		req.AddCookie(sessionCookie(t, a, cookie))
	}
	return routeServe(a, req)
}

func TestFilesPage_Authorization(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/secret-name.js", "// x")

	// A connection from the device itself is always the owner - that is how the
	// Android WebView and the desktop browser both arrive.
	if body := localFilesRequest(t, a).Body.String(); !strings.Contains(body, "secret-name.js") {
		t.Error("a local connection was refused")
	}

	if body := remoteFilesRequest(t, a, "admin").Body.String(); strings.Contains(body, "for the admin of this device") {
		t.Error("an admin cookie was refused")
	}

	// An old guest cookie is not an admin here, and neither is a request with
	// no cookie.
	for _, role := range []string{"", "guest"} {
		rec := remoteFilesRequest(t, a, role)
		body := rec.Body.String()
		if !strings.Contains(body, "for the admin of this device") {
			t.Errorf("role %q was not refused", role)
		}
		if !strings.Contains(body, "Log in") {
			t.Errorf("role %q got a refusal with no way to act on it", role)
		}
		// A refusal must not be a listing with the rows removed.
		if strings.Contains(body, "secret-name.js") {
			t.Errorf("role %q saw a filename in the refusal", role)
		}
		if strings.Contains(body, "%%") {
			t.Errorf("role %q: unfilled template placeholder", role)
		}
	}
}

// authMiddleware asks hasRole, the one definition of the rule. A local
// connection and an admin cookie pass. An old guest cookie and no cookie
// get 401.
func TestAuthMiddlewareStillRefusesAfterExtraction(t *testing.T) {
	a := newTestApp(t)
	h := a.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("through"))
	})

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // admin cookie
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = c.remote
		// A signed cookie, and not the bare word. See session.go.
		if c.cookie != "" {
			req.AddCookie(sessionCookie(t, a, c.cookie))
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s/%q: status %d, want %d", c.remote, c.cookie, rec.Code, c.want)
		}
	}
}

// writeDiskFile writes the file rel below html/ of the storage directory.
func writeDiskFile(t *testing.T, a *App, rel, body string) {
	t.Helper()
	full := filepath.Join(a.StorageDir, "html", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// localFilesRequest asks for html/js/ of the Files page from the device
// itself, through the routes of the App.
func localFilesRequest(t *testing.T, a *App) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/OMNGoFiles.html?tree=served&dir=js%2F", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	return routeServe(a, req)
}
