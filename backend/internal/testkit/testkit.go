// Package testkit holds the stand-in of the App for the tests of the feature
// packages. Only test files import it, thus no binary holds it. Each feature
// package embeds App in its own testApp and adds the fields and the Service
// of that package.
//
// The tests of storage and render cannot use it, because it imports both.
package testkit

import (
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// Version is the APP_VERSION of the stand-in.
const Version = "test"

// App holds the storage directory, the settings and the log hub, the fields
// that the App of package app gives to each Service.
type App struct {
	StorageDir string
	Config     config.Store
	Logs       logx.Hub
	filter     atomic.Value
}

// NewUnconfigured answers a stand-in with md/, html/ and the zero Config,
// the same as newUnconfiguredApp of package app.
func NewUnconfigured(t testing.TB) *App {
	t.Helper()
	a := &App{StorageDir: t.TempDir()}
	for _, d := range []string{a.Layout().MD(), a.Layout().HTML()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// New answers the stand-in of a fresh install. It also loads the settings,
// the same as newTestApp of package app.
func New(t testing.TB) *App {
	t.Helper()
	a := NewUnconfigured(t)
	a.Config.Update(func(c *config.Config) {
		config.Load(c, a.Layout().Config(), 8080, a.Log(logx.Config))
	})
	return a
}

func (a *App) Layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *App) Log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.Logs) }

// Renderer answers the page compile of the stand-in.
func (a *App) Renderer() *render.Renderer {
	return &render.Renderer{
		Layout:    a.Layout(),
		Config:    a.Config.Get(),
		Version:   Version,
		Generator: "OMN-Go " + Version,
	}
}

// RenderPage writes one page in the page shell, the same as renderPage of
// package app.
func (a *App) RenderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := a.Renderer()
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}
