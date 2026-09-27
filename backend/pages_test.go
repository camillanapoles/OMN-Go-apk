package backend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Each row of the page-access table gets its role check from the router. A
// guest on another machine gets the refusal page for an admin row, and the
// page itself for each other row. The device itself gets each page.
func TestEachSystemPageFollowsItsRow(t *testing.T) {
	a := newTestApp(t)
	const refusal = "for the admin of this device"

	for _, p := range a.systemPages() {
		req := httptest.NewRequest(http.MethodGet, p.path, nil)
		req.RemoteAddr = "192.168.1.44:51000"
		req.AddCookie(sessionCookie(t, a, roleGuest))
		rec := routeServe(a, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: a guest got %d, want a page", p.path, rec.Code)
			continue
		}
		refused := strings.Contains(rec.Body.String(), refusal)
		if refused != p.admin {
			t.Errorf("%s: a guest refused=%v, want %v", p.path, refused, p.admin)
		}
		if refused && !strings.Contains(rec.Body.String(), p.title) {
			t.Errorf("%s: the refusal page does not name %q", p.path, p.title)
		}

		local := routeReq(a, http.MethodGet, p.path)
		if strings.Contains(local.Body.String(), refusal) {
			t.Errorf("%s: the device itself got the refusal page", p.path)
		}
	}
}
