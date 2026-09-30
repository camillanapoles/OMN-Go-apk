package backend

import (
	"net/http"

	"net.basov.omngo/backend/internal/files"
)

// The App side of the files package.

// filesService answers the Files page of the App for one request.
func (a *App) filesService() files.Service {
	return files.Service{
		Layout:     a.layout(),
		MimeTypes:  a.config.Get().MimeTypes,
		RenderPage: a.renderPage,
	}
}

// serveFilesPage answers GET /OMNGoFiles.html. See files.Service.ServePage.
func (a *App) serveFilesPage(w http.ResponseWriter, r *http.Request) {
	a.filesService().ServePage(w, r)
}
