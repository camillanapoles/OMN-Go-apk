package backend

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
)

// routeReq sends one request through the real router, from the device
// itself. The local bypass of authMiddleware thus applies.
func routeReq(a *App, method, target string) *httptest.ResponseRecorder {
	return routeServe(a, httptest.NewRequest(method, target, nil))
}

// routeServe sends req through the real router. httptest gives each request
// the address 192.0.2.1. routeServe changes that address to the device
// itself, and it keeps each other address.
func routeServe(a *App, req *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	if req.RemoteAddr == "192.0.2.1:1234" {
		req.RemoteAddr = "127.0.0.1:40000"
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// Each route with a method refuses each other method with 405, and the
// Allow header names the methods of the route. The test reads the routes
// from registerRoutes, thus a new route needs no new line here.
func TestEachMethodRouteRefusesAnotherMethod(t *testing.T) {
	a := newTestApp(t)
	rec := &routeRecorder{}
	a.registerRoutes(rec)

	methods := map[string][]string{}
	for _, p := range rec.patterns {
		if m, path, ok := strings.Cut(p, " "); ok {
			methods[path] = append(methods[path], m)
		}
	}
	if len(methods) == 0 {
		t.Fatal("registerRoutes registered no route with a method")
	}

	for path, ms := range methods {
		var allow []string
		for _, m := range ms {
			allow = append(allow, m)
			if m == http.MethodGet {
				allow = append(allow, http.MethodHead)
			}
		}
		sort.Strings(allow)

		for _, wrong := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			if strings.Contains(strings.Join(ms, " "), wrong) {
				continue
			}
			w := routeReq(a, wrong, path)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s answered %d, want 405", wrong, path, w.Code)
				continue
			}
			got := strings.Split(w.Header().Get("Allow"), ", ")
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(allow, ",") {
				t.Errorf("%s %s: Allow is %v, want %v", wrong, path, got, allow)
			}
		}
	}
}

// A GET must not write a note, and a POST must. See
// doc/decisions/0016-give-each-route-one-method.md.
func TestGetDoesNotSaveANote(t *testing.T) {
	a := newTestApp(t)
	mdPath, _, _, _ := a.resolvePageName("MethodProbe")
	q := url.Values{"name": {"MethodProbe"}, "content": {"# Probe"}}.Encode()

	if w := routeReq(a, http.MethodGet, "/api/save?"+q); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/save answered %d, want 405", w.Code)
	}
	if _, err := os.Stat(mdPath); err == nil {
		t.Fatal("a GET wrote the note")
	}

	if w := routeReq(a, http.MethodPost, "/api/save?"+q); w.Code != http.StatusOK {
		t.Errorf("POST /api/save answered %d %q, want 200", w.Code, w.Body.String())
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("the POST did not write the note: %v", err)
	}
}
