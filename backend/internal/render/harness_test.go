package render

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// version and generator stand in for the build version of package app. A
// test of package app sees the same two values.
const (
	version   = "dev"
	generator = "OMN-Go dev"
)

// testApp stands in for the App of package app. It holds the storage
// directory, the settings and the log hub, the same fields that the App
// gives to a Renderer. The tests of this package read these fields by the
// names of the App, thus a test reads the same here and in package app.
type testApp struct {
	StorageDir string
	config     config.Store
	filter     atomic.Value
	logs       logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/
// and loads the settings, the same as newTestApp of package app.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a.config.Update(func(c *config.Config) {
		config.Load(c, a.layout().Config(), 8080, a.log(logx.Config))
	})
	return a
}

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.logs) }

// renderer answers the Renderer of the stand-in, the same as renderer of
// package app. The stand-in has no page facts and no OnPageWritten hook.
func (a *testApp) renderer() Renderer {
	return Renderer{
		Layout:    a.layout(),
		Config:    a.config.Get(),
		Version:   version,
		Generator: generator,
	}
}

func (a *testApp) testRenderer() *Renderer {
	rd := a.renderer()
	return &rd
}

func (a *testApp) injectRuntimeVars(page []byte) []byte {
	rd := a.renderer()
	return rd.InjectRuntimeVars(page)
}

func (a *testApp) renderAndCache(name string, content []byte) ([]byte, error) {
	rd := a.renderer()
	return rd.RenderAndCache(name, content)
}

func (a *testApp) ensureHeaderModified(content string, defaultTitle string) string {
	return EnsureHeaderModified(content, defaultTitle, a.config.Get().Author)
}

func (a *testApp) generateTagsPage() error {
	rd := a.renderer()
	return rd.GenerateTagsPage(a.log(logx.Tags))
}

func (a *testApp) resolvePageName(name string) (mdPath, htmlPath, baseName string, isPage bool) {
	return storage.ResolvePageName(a.layout(), a.config.Get().MimeTypes, name)
}
