package gitsync

import (
	"net/http"
	"os"
	"testing"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub. It adds the sync state.
type testApp struct {
	*testkit.App
	git State
}

func newTestApp(t *testing.T) *testApp { return &testApp{App: testkit.New(t)} }

// gitSync answers the Service of the stand-in, the same as gitSync of
// package app.
func (a *testApp) gitSync() Service {
	return Service{
		State:  &a.git,
		Layout: a.Layout(),
		Config: a.Config.Get(),
		Log:    a.Log,
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
	storage.SyncNoteFilesToHTML(a.Layout(), a.Log(logx.NoteFiles))
}

// writeJSON writes one JSON answer, the same as writeJSON of package app.
func (a *testApp) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, a.Log(logx.Server))
}

// runtimeSupportsFileMode answers whether Perm() reports what os.WriteFile
// asked for. Windows does not carry a Unix mode, thus a test skips that one
// check there. Package app holds the same helper.
func runtimeSupportsFileMode() bool {
	return os.PathSeparator == '/'
}
