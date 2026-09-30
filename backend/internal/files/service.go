package files

import (
	"net/http"

	"net.basov.omngo/backend/internal/storage"
)

// Service is the Files page for one request. The App makes one with its
// storage layout, its mime_types setting and its page shell. See
// filesService in backend/internal/app/files_app.go.
type Service struct {
	Layout     storage.Layout
	MimeTypes  map[string]string // the mime_types setting
	RenderPage func(w http.ResponseWriter, code int, name string, header []byte, body string)
}
