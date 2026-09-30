package db

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

// testApp stands in for the App of package app. It holds the storage
// directory, the settings, the database store and the log hub, the same
// fields that the App gives to a Service. The tests of this package read
// these fields by the names of the App, thus a test reads the same here and
// in package app.
type testApp struct {
	StorageDir string
	config     config.Store
	dbs        Store
	filter     atomic.Value
	logs       logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/
// and loads the settings, the same as newTestApp of package app.
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

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.logs) }

// renderPage writes one page in the page shell, the same as renderPage of
// package app.
func (a *testApp) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := &render.Renderer{
		Layout:    a.layout(),
		Config:    a.config.Get(),
		Version:   "test",
		Generator: "OMN-Go test",
	}
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}

// databases answers the Service of the stand-in, the same as databases of
// package app.
func (a *testApp) databases() Service {
	cfg := a.config.Get()
	return Service{
		Store:      &a.dbs,
		Layout:     a.layout(),
		Hostname:   cfg.Hostname,
		PruneDepth: cfg.BackupPruneDepth,
		Log:        a.log,
		RenderPage: a.renderPage,
	}
}

func (a *testApp) handleSQL(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleSQL(w, r)
}

func (a *testApp) handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleBackupCreate(w, r)
}

func (a *testApp) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleRestore(w, r)
}

func (a *testApp) handleDBBackupList(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleBackupList(w, r)
}
