package backend

import (
	"net"
	"net/http"
	"strings"
)

func (a *App) isLocalConnection(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func (a *App) connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.ActiveConns.Add(1)
		defer a.ActiveConns.Add(-1)

		if reason := a.foreignRequest(r); reason != "" {
			a.log(logServer).errf("refused %s %s from %s, Host %q: %s",
				r.Method, r.URL.Path, r.RemoteAddr, r.Host, reason)
			http.Error(w, "Forbidden: "+reason, http.StatusForbidden)
			return
		}

		// This is the one place that controls the cache of the client. See
		// doc/decisions/0004-tell-the-browser-to-ask-before-it-uses-a-copy.md.
		// "no-cache" keeps the copy and asks the server each time, and the
		// server answers 304 while the file does not change. A handler that
		// writes its own Cache-Control later wins, for example the log
		// stream.
		w.Header().Set("Cache-Control", "no-cache")

		next.ServeHTTP(&pageCacheWriter{ResponseWriter: w}, r)
	})
}

// pageCacheWriter changes "no-cache" to "no-store" for a page. For a Back or
// Forward load, Chromium reads its copy and asks nothing. A page that changed
// in the meantime thus shows in its old form. "no-store" keeps no copy. An
// asset keeps "no-cache": KaTeX, highlight.js and the fonts are too large to load
// again for each page.
//
// The change needs both conditions: the type starts with "text/html", and
// Cache-Control still holds the "no-cache" from above.
type pageCacheWriter struct {
	http.ResponseWriter
	decided bool
}

// decide runs one time, when the writer sends the header. Write calls it too,
// because a body with no WriteHeader sends the header.
func (w *pageCacheWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if w.Header().Get("Cache-Control") != "no-cache" {
		return
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
}

func (w *pageCacheWriter) WriteHeader(status int) {
	w.decide()
	w.ResponseWriter.WriteHeader(status)
}

func (w *pageCacheWriter) Write(b []byte) (int, error) {
	w.decide()
	return w.ResponseWriter.Write(b)
}

// Flush keeps the log stream alive. Without it, the wrapper fails the
// http.Flusher test, and each line waits for the end of the response.
func (w *pageCacheWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// hasRole answers "may this request do a protected thing". It is the ONE
// answer. A connection from the device itself is always the owner. Another
// machine needs a signed admin cookie. The page-access table calls it
// directly, thus it can answer a refusal with a page.
func (a *App) hasRole(r *http.Request) bool {
	if a.isLocalConnection(r) {
		return true
	}
	// readSessionRole answers "" for a cookie that this install did not sign.
	// See doc/decisions/0001-sign-the-session-cookie.md.
	return a.readSessionRole(r) == roleAdmin
}

// authMiddleware answers 401 with plain text when hasRole refuses the
// request. A system page answers a refusal with a page. See page_access.go.
func (a *App) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.hasRole(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
