package gitsync

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
// directory, the settings, the sync state and the log hub, the same fields
// that the App gives to a Service. The tests of this package read these
// fields by the names of the App, thus a test reads the same here and in
// package app.
type testApp struct {
	StorageDir string
	config     config.Store
	git        State
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

// gitSync answers the Service of the stand-in, the same as gitSync of
// package app.
func (a *testApp) gitSync() Service {
	return Service{
		State:  &a.git,
		Layout: a.layout(),
		Config: a.config.Get(),
		Log:    a.log,
	}
}

func (a *testApp) handleSync(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleSync(w, r)
}

func (a *testApp) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleSyncPreview(w, r)
}

func (a *testApp) handleTrustHostKey(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleTrustHostKey(w, r)
}

// syncNoteFilesToHTML copies the plain files of md/ to html/, the same as
// syncNoteFilesToHTML of package app.
func (a *testApp) syncNoteFilesToHTML() {
	storage.SyncNoteFilesToHTML(a.layout(), a.log(logx.NoteFiles))
}

// writeJSON writes one JSON answer, the same as writeJSON of package app.
func (a *testApp) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, a.log(logx.Server))
}

// runtimeSupportsFileMode answers whether Perm() reports what os.WriteFile
// asked for. Windows does not carry a Unix mode, thus a test skips that one
// check there. Package app holds the same helper.
func runtimeSupportsFileMode() bool {
	return os.PathSeparator == '/'
}
