package gitsync

import (
	"net/http"
	"sync"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// State holds the state of the sync that lives as long as the App. The
// zero value is ready for use.
type State struct {
	// mu serializes each operation on the repository on disk.
	mu       sync.Mutex
	hostKeys hostKeyState
}

// Service is the sync feature for one call. The App makes one with its
// state, its storage layout, a copy of the settings and its loggers. See
// gitSync in backend/internal/app/gitsync_app.go.
type Service struct {
	State  *State
	Layout storage.Layout
	Config config.Config
	Log    func(tag logx.Tag) logx.Logger
}

// writeJSON answers with one JSON value. See render.WriteJSON.
func (svc Service) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, svc.Log(logx.Server))
}

// writeJSONError answers {"status": "error", "message": msg}.
func (svc Service) writeJSONError(w http.ResponseWriter, code int, msg string) {
	svc.writeJSON(w, code, render.JSONStatus{Status: "error", Message: msg})
}
