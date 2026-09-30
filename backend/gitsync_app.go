package backend

import (
	"net/http"

	"net.basov.omngo/backend/internal/gitsync"
)

// The App side of the gitsync package. gitSync gives the package the sync
// state of the App, the storage layout, a copy of the settings and the
// loggers. Each handler makes one gitsync.Service for its request.

// gitSync answers the sync feature of the App for one call.
func (a *App) gitSync() gitsync.Service {
	return gitsync.Service{
		State:  &a.git,
		Layout: a.layout(),
		Config: a.config.Get(),
		Log:    a.log,
	}
}

// handleSync answers POST /api/sync. See gitsync.Service.HandleSync.
func (a *App) handleSync(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleSync(w, r)
}

// handleSyncPreview answers GET /api/sync/preview. See
// gitsync.Service.HandleSyncPreview.
func (a *App) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleSyncPreview(w, r)
}

// handleTrustHostKey answers POST /api/sync/trust-host-key. See
// gitsync.Service.HandleTrustHostKey.
func (a *App) handleTrustHostKey(w http.ResponseWriter, r *http.Request) {
	a.gitSync().HandleTrustHostKey(w, r)
}
