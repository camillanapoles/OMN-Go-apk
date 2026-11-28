package repocheck

import (
	"strings"
	"testing"
)

// The hardening headers and the login throttle are a contract of the fork.
// The middleware sets nosniff on every answer. The system pages alone get
// the CSP, and the login answers 429 with a wait. The local check knows no
// localhost name. See doc/decisions/0035.
func TestTheHTTPGuardStands(t *testing.T) {
	middleware, err := readBackendFile("internal/app/middleware.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`"X-Content-Type-Options", "nosniff"`,
		`"Content-Security-Policy", contentSecurityPolicy`,
		"a.systemPages()",
	} {
		if !strings.Contains(string(middleware), marker) {
			t.Errorf("middleware.go lost %q", marker)
		}
	}
	if strings.Contains(string(middleware), `"localhost"`) {
		t.Error("middleware.go still compares the address with the word localhost")
	}
	session, err := readBackendFile("internal/app/session.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"StatusTooManyRequests", `"Retry-After"`, "loginTryLimit", "loginBackoffCap",
	} {
		if !strings.Contains(string(session), marker) {
			t.Errorf("session.go lost %q", marker)
		}
	}
}
