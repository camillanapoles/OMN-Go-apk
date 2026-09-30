package exchange

import (
	"net/http"
	"testing"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub.
type testApp struct {
	*testkit.App
}

func newTestApp(t *testing.T) *testApp { return &testApp{App: testkit.New(t)} }

// exchange answers the Service of the stand-in, the same as exchange of
// package app.
func (a *testApp) exchange() Service {
	cfg := a.Config.Get()
	return Service{
		Layout:         a.Layout(),
		MimeTypes:      cfg.MimeTypes,
		MaxUploadBytes: config.MaxUploadBytes(cfg),
		Log:            a.Log,
	}
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
