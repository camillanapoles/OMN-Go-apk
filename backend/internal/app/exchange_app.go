package app

import (
	"net/http"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/exchange"
)

// The App side of the exchange package.

// exchange answers the note exchange of the App for one call.
func (a *App) exchange() exchange.Service {
	cfg := a.config.Get()
	return exchange.Service{
		Layout:         a.layout(),
		MimeTypes:      cfg.MimeTypes,
		MaxUploadBytes: config.MaxUploadBytes(cfg),
		Log:            a.log,
	}
}

// ensureIncomingIndex makes the incoming index note when it is absent. See
// exchange.Service.EnsureIncomingIndex.
func (a *App) ensureIncomingIndex(now time.Time) error {
	return a.exchange().EnsureIncomingIndex(now)
}

// addIncomingFile puts a line for an uploaded file on the incoming index. See
// exchange.Service.AddIncomingFile.
func (a *App) addIncomingFile(rel string, now time.Time) error {
	return a.exchange().AddIncomingFile(rel, now)
}

// handleExportNote answers GET /api/export/note. See
// exchange.Service.HandleExportNote.
func (a *App) handleExportNote(w http.ResponseWriter, r *http.Request) {
	a.exchange().HandleExportNote(w, r)
}

// handleImportNote answers POST /api/import/note. See
// exchange.Service.HandleImportNote.
func (a *App) handleImportNote(w http.ResponseWriter, r *http.Request) {
	a.exchange().HandleImportNote(w, r)
}
