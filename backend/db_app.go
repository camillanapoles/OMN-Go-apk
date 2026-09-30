package backend

import (
	"net/http"

	"net.basov.omngo/backend/internal/db"
)

// The App side of the db package. databases gives the package the store of
// the App, the storage layout, two settings, the loggers and the page shell.
// Each handler makes one db.Service for its request.

// databases answers the db feature of the App for one call.
func (a *App) databases() db.Service {
	cfg := a.config.Get()
	return db.Service{
		Store:      &a.dbs,
		Layout:     a.layout(),
		Hostname:   cfg.Hostname,
		PruneDepth: cfg.BackupPruneDepth,
		Log:        a.log,
		RenderPage: a.renderPage,
	}
}

// handleSQL answers POST /api/sql. See db.Service.HandleSQL.
func (a *App) handleSQL(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleSQL(w, r)
}

// serveDBBackupsPage answers GET /db_backups. See db.Service.ServeBackupsPage.
func (a *App) serveDBBackupsPage(w http.ResponseWriter, r *http.Request) {
	a.databases().ServeBackupsPage(w, r)
}

// handleDBBackupCreate answers POST /api/db/backup. See
// db.Service.HandleBackupCreate.
func (a *App) handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleBackupCreate(w, r)
}

// handleDBRestore answers POST /api/db/restore. See db.Service.HandleRestore.
func (a *App) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleRestore(w, r)
}

// handleDBBackupList answers GET /api/db/backups. See
// db.Service.HandleBackupList.
func (a *App) handleDBBackupList(w http.ResponseWriter, r *http.Request) {
	a.databases().HandleBackupList(w, r)
}
