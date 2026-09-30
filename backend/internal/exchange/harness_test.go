package exchange

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
	"net.basov.omngo/backend/internal/storage"
)

// testApp stands in for the App of package backend. It holds the storage
// directory and the settings, the fields that the App gives to a Service.
// The tests of this package read them by the names of the App.
type testApp struct {
	StorageDir string
	config     config.Store
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

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.hub) }

// exchange answers the Service of the stand-in, the same as exchange of
// package backend.
func (a *testApp) exchange() Service {
	cfg := a.config.Get()
	return Service{
		Layout:         a.layout(),
		MimeTypes:      cfg.MimeTypes,
		MaxUploadBytes: config.MaxUploadBytes(cfg),
		Log:            a.log,
	}
}

// renderer answers the page compile of the stand-in.
func (a *testApp) renderer() render.Renderer {
	return render.Renderer{Layout: a.layout(), Config: a.config.Get(), Version: "test", Generator: "OMN-Go test"}
}

func (a *testApp) exportNoteSource(name string) ([]byte, string, error) {
	return a.exchange().ExportNoteSource(name)
}

func (a *testApp) importNote(content []byte, displayName string, now time.Time) (importResult, error) {
	return a.exchange().ImportNote(content, displayName, now)
}

func (a *testApp) addIncomingIndexLine(res importResult, now time.Time) error {
	return a.exchange().addIncomingIndexLine(res, now)
}

func (a *testApp) ensureIncomingIndex(now time.Time) error {
	return a.exchange().EnsureIncomingIndex(now)
}

func (a *testApp) handleExportNote(w http.ResponseWriter, r *http.Request) {
	a.exchange().HandleExportNote(w, r)
}

func (a *testApp) handleImportNote(w http.ResponseWriter, r *http.Request) {
	a.exchange().HandleImportNote(w, r)
}
