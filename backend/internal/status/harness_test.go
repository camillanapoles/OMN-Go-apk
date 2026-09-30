package status

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/search"
	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub. It adds the start time, the Android facts and the index.
type testApp struct {
	*testkit.App
	startedAt time.Time
	android   Android
	search    *search.Index
}

func newTestApp(t *testing.T) *testApp {
	return &testApp{App: testkit.New(t), startedAt: time.Now()}
}

// statusService answers the Service of the stand-in, the same as
// statusService of package app.
func (a *testApp) statusService() Service {
	return Service{
		Layout:       a.Layout(),
		Config:       a.Config.Get(),
		Version:      testkit.Version,
		StartedAt:    a.startedAt,
		FallbackPort: 8080,
		Android:      &a.android,
		Search:       a.search,
		Log:          a.Log,
		RenderPage:   a.RenderPage,
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
	a.Config.Update(func(c *config.Config) {
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
	search.Service{Index: a.search, Layout: a.Layout(), Config: a.Config.Get(), Log: a.Log}.RebuildIndex()
}
