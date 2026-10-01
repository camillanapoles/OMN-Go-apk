package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The route table of doc/API.md section 3 comes from registerRoutes. This
// test reads a file outside the package, because only package app can run
// registerRoutes. It skips when doc/ is absent, as in the Docker context.
//
// After a change of a route, write the table again with
//
//	OMN_WRITE_API_TABLE=1 go test -run TestAPIRouteTable ./backend/internal/app/

const (
	apiDocPath       = "../../../doc/API.md"
	apiTableBegin    = "<!-- The rows below come from registerRoutes. See TestAPIRouteTable. -->\n"
	apiTableEnd      = "<!-- The end of the rows from registerRoutes. -->\n"
	apiTableHeader   = "| Method(s) | URL | Auth | Response |\n| --- | --- | --- | --- |\n"
	apiRefusalAccess = "admin (a page for a remote caller, not a 401)"
)

// routeDocRecorder is a routeTable that keeps each routeDoc.
type routeDocRecorder struct {
	routeRecorder
	docs []routeDoc
}

func (r *routeDocRecorder) document(d routeDoc) { r.docs = append(r.docs, d) }

// apiRouteTable writes the rows of the table from the routes of a.
func apiRouteTable(a *App) string {
	rec := &routeDocRecorder{}
	a.registerRoutes(rec)
	var b strings.Builder
	b.WriteString(apiTableHeader)
	for _, d := range rec.docs {
		who := map[access]string{open: "none", admin: "admin", adminPage: apiRefusalAccess}[d.who]
		b.WriteString("| " + strings.Join(d.methods, ", ") + " | `" + d.path + "` | " + who + " | " + d.answer + " |\n")
	}
	return b.String()
}

func TestAPIRouteTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash(apiDocPath))
	if err != nil {
		t.Skipf("doc/API.md is not in this tree: %v", err)
	}
	doc := string(raw)
	i := strings.Index(doc, apiTableBegin)
	j := strings.Index(doc, apiTableEnd)
	if i < 0 || j < i {
		t.Fatal("doc/API.md has no markers of the route table")
	}
	want := apiRouteTable(newTestApp(t))
	got := doc[i+len(apiTableBegin) : j]
	if got == want {
		return
	}
	if os.Getenv("OMN_WRITE_API_TABLE") == "1" {
		doc = doc[:i+len(apiTableBegin)] + want + doc[j:]
		if err := os.WriteFile(filepath.FromSlash(apiDocPath), []byte(doc), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("the route table of doc/API.md differs from registerRoutes. Run "+
		"OMN_WRITE_API_TABLE=1 go test -run TestAPIRouteTable ./backend/internal/app/\nwant:\n%s", want)
}
