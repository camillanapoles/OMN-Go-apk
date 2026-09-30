package db

import (
	"database/sql"
	"net/http"
	"sync"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// Store holds the open handles of the user databases. The zero value is
// ready for use.
type Store struct {
	mu  sync.Mutex         // guards dbs
	dbs map[string]*sql.DB // the handles that Open made, by name
	// restoreMu serializes each restore and each swap. Never take it while
	// you hold mu.
	restoreMu sync.Mutex
}

// Service is the db feature for one call. The App makes one with its
// store, its storage layout, two settings, its loggers and its page shell.
// See databases in backend/db_app.go.
type Service struct {
	Store      *Store
	Layout     storage.Layout
	Hostname   string // the Hostname setting, for the name of a new backup
	PruneDepth int    // the backup_prune_depth setting
	Log        func(tag logx.Tag) logx.Logger
	RenderPage func(w http.ResponseWriter, code int, name string, header []byte, body string)
}

// writeJSON answers with one JSON value. See render.WriteJSON.
func (svc Service) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, svc.Log(logx.Server))
}

// writeJSONError answers {"status": "error", "message": msg}.
func (svc Service) writeJSONError(w http.ResponseWriter, code int, msg string) {
	svc.writeJSON(w, code, render.JSONStatus{Status: "error", Message: msg})
}
