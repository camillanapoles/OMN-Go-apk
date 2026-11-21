package repocheck

// ----------------------------------------------------------------------
// The global response headers and the login throttle
// ----------------------------------------------------------------------
//
// The nosniff header and the login throttle are hardening rules of the
// server. A refactor can drop either one and each behavioral test can
// still pass, one call site at a time. This test pins the source text.
//
// It reads the source and not the behavior. The behavior lives in
// TestConnectionMiddlewareSetsNoSniff and TestLoginThrottleRefusesTheSixthFailure
// of internal/app. This file is the second lock, in the style of
// TestNoDirectLogPrintf.

import (
	"strings"
	"testing"
)

// httpHardeningNeedles names the text that each file must keep. The map key
// is the path below backend/.
var httpHardeningNeedles = map[string][]string{
	"internal/app/middleware.go": {
		`w.Header().Set("X-Content-Type-Options", "nosniff")`,
	},
	"internal/app/session.go": {
		"Retry-After",
		"http.StatusTooManyRequests",
		"loginFailLimit",
	},
}

// Each needle above stays in its file. A missing needle means that a
// refactor dropped one half of the hardening. Restore it, or change the
// behavioral test first and then this list with a word in the commit.
func TestHTTPHardeningStaysInPlace(t *testing.T) {
	for rel, needles := range httpHardeningNeedles {
		src, err := readBackendFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		for _, needle := range needles {
			if !strings.Contains(string(src), needle) {
				t.Errorf("%s no longer holds %s.\n"+
					"  The nosniff header and the login throttle guard the whole server.\n"+
					"  See doc/decisions/0031-throttle-the-network-login.md.", rel, needle)
			}
		}
	}
}
