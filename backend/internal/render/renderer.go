package render

import (
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/storage"
)

// Renderer holds each value that a page compile needs. The App makes one
// for each call, with a copy of the settings of that moment. See renderer in
// backend/render_app.go.
type Renderer struct {
	Layout storage.Layout
	Config config.Config
	// Version is APP_VERSION of the build, for the runtime values of a page.
	Version string
	// Generator is the value of the generator meta tag, "OMN-Go " and the
	// version. The App gives a constant, thus a compile makes no new string.
	Generator string
	Facts     Facts
	// OnPageWritten is the hook of RenderAndCache. A nil hook does nothing.
	OnPageWritten func(name string)
}
