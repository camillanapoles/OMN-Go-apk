package app

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// foreignRequest answers why a browser sent this request for another site,
// or "" for a request that may pass. A local connection is always admin,
// thus a page of another site must not reach the server through the browser
// of the device. See
// doc/decisions/0017-refuse-a-request-that-another-site-sends.md.
func (a *App) foreignRequest(r *http.Request) string {
	if !a.isKnownHost(r.Host) {
		return "unknown host name"
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ""
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return "request from another origin"
		}
		return ""
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return "request from another site"
	}
	return ""
}

// isKnownHost tells if the Host header names this device. An IP address
// always passes, because a rebound DNS name is the attack, and an address is
// not a name. A name must be localhost or the name of this device.
func (a *App) isKnownHost(hostport string) bool {
	if hostport == "" {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	if net.ParseIP(host) != nil || host == "localhost" {
		return true
	}
	names := []string{a.config.Get().Hostname}
	if h, err := os.Hostname(); err == nil {
		names = append(names, h)
	}
	for _, n := range names {
		n = strings.ToLower(n)
		if n != "" && (host == n || host == n+".local") {
			return true
		}
	}
	return false
}
