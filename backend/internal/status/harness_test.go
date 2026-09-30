package status

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/search"
	"net.basov.omngo/backend/internal/storage"
)

// testVersion is the APP_VERSION of the stand-in.
const testVersion = "test"

// testApp stands in for the App of package app. It holds the storage
// directory, the settings, the start time, the Android facts and the index,
// the fields that the App gives to a Service. The tests of this package read
// them by the names of the App.
type testApp struct {
	StorageDir string
	config     config.Store
	startedAt  time.Time
	android    Android
	search     *search.Index
	filter     atomic.Value
	hub        logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/
// and loads the settings, the same as newTestApp of package app.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir(), startedAt: time.Now()}
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

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.hub) }

// renderPage writes one page in the page shell, the same as renderPage of
// package app.
func (a *testApp) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := render.Renderer{Layout: a.layout(), Config: a.config.Get(), Version: "test", Generator: "OMN-Go test"}
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}

// statusService answers the Service of the stand-in, the same as
// statusService of package app.
func (a *testApp) statusService() Service {
	return Service{
		Layout:       a.layout(),
		Config:       a.config.Get(),
		Version:      testVersion,
		StartedAt:    a.startedAt,
		FallbackPort: 8080,
		Android:      &a.android,
		Search:       a.search,
		Log:          a.log,
		RenderPage:   a.renderPage,
	}
}

func (a *testApp) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.statusService().HandleStatus(w, r)
}

func (a *testApp) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	a.statusService().ServeStatusPage(w, r)
}

// enabledSearchApp answers the stand-in with global search on and an empty
// index.
func enabledSearchApp(t *testing.T) *testApp {
	t.Helper()
	a := newTestApp(t)
	a.search = &search.Index{}
	a.config.Update(func(c *config.Config) {
		c.SearchEnabled = true
		c.SearchKinds = []string{config.SearchKindMD, config.SearchKindBookmarks}
	})
	return a
}

// writeSearchNote writes md/rel with content.
func writeSearchNote(t *testing.T, a *testApp, rel, content string) {
	t.Helper()
	p := filepath.Join(a.StorageDir, "md", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// rebuildSearchIndex builds the index of the stand-in.
func (a *testApp) rebuildSearchIndex() {
	search.Service{Index: a.search, Layout: a.layout(), Config: a.config.Get(), Log: a.log}.RebuildIndex()
}
