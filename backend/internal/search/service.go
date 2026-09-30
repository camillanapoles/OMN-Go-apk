package search

import (
	"net/http"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// Service is the search feature for one call. The App makes one with its
// index, its storage layout, a copy of the settings, its loggers and its page
// shell. See searchService in backend/internal/app/search_app.go.
type Service struct {
	Index      *Index // nil before initStorage makes the index
	Layout     storage.Layout
	Config     config.Config
	Log        func(tag logx.Tag) logx.Logger
	RenderPage func(w http.ResponseWriter, code int, name string, header []byte, body string)
}

// writeJSON answers with one JSON value. See render.WriteJSON.
func (svc Service) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, svc.Log(logx.Server))
}
