package db

import (
	"net/http"
	"testing"

	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub. It adds the database store.
type testApp struct {
	*testkit.App
	dbs Store
}

func newTestApp(t *testing.T) *testApp { return &testApp{App: testkit.New(t)} }

// databases answers the Service of the stand-in, the same as databases of
// package app.
func (a *testApp) databases() Service {
	cfg := a.Config.Get()
	return Service{
		Store:      &a.dbs,
		Layout:     a.Layout(),
		Hostname:   cfg.Hostname,
		PruneDepth: cfg.BackupPruneDepth,
		Log:        a.Log,
		RenderPage: a.RenderPage,
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
