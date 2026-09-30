package search

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// testApp stands in for the App of package backend. It holds the storage
// directory, the settings and the index, the same fields that the App gives
// to a Service. The tests of this package read these fields by the names of
// the App, thus a test reads the same here and in package backend.
type testApp struct {
	StorageDir string
	config     config.Store
	search     *Index
	filter     atomic.Value
	hub        logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/
// and loads the settings, the same as newTestApp of package backend.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a.config.Update(func(c *config.Config) {
		config.Load(c, a.layout().Config(), 8080, a.log(logx.Config))
	})
	return a
}

// newUnconfiguredApp answers the stand-in with md/ and html/ and the zero
// Config, the same as newUnconfiguredApp of package backend.
func newUnconfiguredApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// testRenderer answers the page compile of the stand-in.
func (a *testApp) testRenderer() *render.Renderer {
	return &render.Renderer{
		Layout:    a.layout(),
		Config:    a.config.Get(),
		Version:   "test",
		Generator: "OMN-Go test",
	}
}

// renderPage writes one page in the page shell, the same as renderPage of
// package backend.
func (a *testApp) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := a.testRenderer()
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.hub) }

// searchService answers the Service of the stand-in, the same as
// searchService of package backend.
func (a *testApp) searchService() Service {
	return Service{
		Index:      a.search,
		Layout:     a.layout(),
		Config:     a.config.Get(),
		Log:        a.log,
		RenderPage: a.renderPage,
	}
}

func (a *testApp) rebuildSearchIndex() { a.searchService().RebuildIndex() }

func (a *testApp) dropSearchIndex() { a.searchService().DropIndex() }

func (a *testApp) searchIndexStatus() string { return a.searchService().IndexStatus() }

func (a *testApp) globalSearchAvailable() bool { return a.searchService().GlobalAvailable() }

func (a *testApp) markSearchIndexDirty() { a.search.MarkDirty() }

func (a *testApp) handleSearch(w http.ResponseWriter, r *http.Request) {
	a.searchService().HandleSearch(w, r)
}

func (a *testApp) serveSearchPage(w http.ResponseWriter, r *http.Request) {
	a.searchService().ServeSearchPage(w, r)
}
