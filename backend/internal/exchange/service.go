package exchange

import (
	"net/http"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// Service is the note exchange for one call. The App makes one with its
// storage layout, two settings and its loggers. See exchange in
// backend/exchange_app.go.
type Service struct {
	Layout         storage.Layout
	MimeTypes      map[string]string // the mime_types setting
	MaxUploadBytes int64             // the upload limit of the settings
	Log            func(tag logx.Tag) logx.Logger
}

// writeJSON answers with one JSON value. See render.WriteJSON.
func (svc Service) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, svc.Log(logx.Server))
}

// writeJSONError answers {"status": "error", "message": msg}.
func (svc Service) writeJSONError(w http.ResponseWriter, code int, msg string) {
	svc.writeJSON(w, code, render.JSONStatus{Status: "error", Message: msg})
}
