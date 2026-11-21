package app

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/render"
)

// connectionMiddleware wraps each route (see server.go), thus each
// response must carry Cache-Control. Without the header http.ServeFile
// sends Last-Modified only, and a browser or the Android WebView then
// keeps a script for days. An update of the application then shows new
// pages that operate with the old scripts.
//
// An asset keeps "no-cache". Only a page becomes "no-store", and the
// three tests below that one pin the rule.
func TestConnectionMiddlewareSetsCacheControl(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/js/OMN-Go/omn-go-core.js", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

// A page must say "no-store". Chromium does not ask the server on a Back
// load. "no-cache" thus gave the old copy of a page that OMN-Go changed
// while that page waited in the history. The + button showed this. The link
// that it writes into the page you started from was absent after Back.
func TestConnectionMiddlewareUsesNoStoreForAPage(t *testing.T) {
	// render.HTMLContentType is the value that render.WriteHTMLHeader writes for
	// each page. A change of it that loses the prefix "text/html" makes each page
	// cacheable again, and Back then shows an old copy.
	for _, contentType := range []string{"text/html", "text/html; charset=utf-8", render.HTMLContentType} {
		a := &App{}
		h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.Write([]byte("<html></html>"))
		}))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/Welcome.html", nil))

		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Content-Type %q: Cache-Control = %q, want %q", contentType, got, "no-store")
		}
	}
}

// The handler still wins. handleLogsSSE writes its own Cache-Control for the
// log stream, and the page rule must not take that decision back.
func TestConnectionMiddlewareKeepsTheWordsOfTheHandler(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-cache, private")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/Welcome.html", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-cache, private" {
		t.Errorf("Cache-Control = %q, want the value of the handler", got)
	}
}

// The log stream sends one line at a time. handleLogsSSE asks the writer for
// http.Flusher, thus the wrapper of connectionMiddleware must answer that
// question. Without this the lines wait until the response ends.
func TestConnectionMiddlewareWriterIsAFlusher(t *testing.T) {
	a := &App{}
	var isFlusher bool
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, isFlusher = w.(http.Flusher)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://localhost/api/logs", nil))

	if !isFlusher {
		t.Error("the writer of connectionMiddleware is not an http.Flusher")
	}
}

// The middleware sets the header before the handler operates, thus a
// handler that needs other words can write them. The log stream does
// this (see registerRoutes in server.go).
func TestConnectionMiddlewareLetsAHandlerReplaceCacheControl(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/api/logs", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want the value of the handler %q", got, "no-store")
	}
}

// App.ActiveConns must align itself on a 32-bit build. See
// TestNoBare64BitAtomics in internal/repocheck for the reason.
func TestActiveConnsIsAnAtomicInt64(t *testing.T) {
	field, ok := reflect.TypeOf(App{}).FieldByName("ActiveConns")
	if !ok {
		t.Fatal("App has no ActiveConns field")
	}
	if field.Type != reflect.TypeOf(atomic.Int64{}) {
		t.Errorf("App.ActiveConns is %s, want atomic.Int64 - a bare int64 with "+
			"atomic.AddInt64 panics on every 32-bit build", field.Type)
	}
}

// Each response carries X-Content-Type-Options: nosniff. The header stops
// a browser from guessing a type that the server did not announce. The
// pageCacheWriter changes the cache header only, and the nosniff header
// survives it. See doc/decisions/0031-throttle-the-network-login.md.
func TestConnectionMiddlewareSetsNoSniff(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		render.WriteHTMLHeader(w)
		w.Write([]byte("<html></html>"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/Welcome.html", nil))

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	// The same answer went through the pageCacheWriter.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
}

// A TCP connection carries an IP and a port in RemoteAddr. The string
// "localhost" never appears there, and a client that writes it into a
// header controls that header. The comparison is dead code, and it
// invites a future check on a name that the client owns.
func TestAHostNameIsNotLocal(t *testing.T) {
	a := &App{}
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.RemoteAddr = "localhost:1"
	if a.isLocalConnection(r) {
		t.Error(`a RemoteAddr of "localhost:1" counted as local`)
	}
}
