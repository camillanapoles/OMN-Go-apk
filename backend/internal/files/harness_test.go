package files

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// testApp stands in for the App of package backend. It holds the storage
// directory and the settings, the fields that the App gives to a Service.
// The tests of this package read them by the names of the App.
type testApp struct {
	StorageDir string
	config     config.Store
	filter     atomic.Value
	hub        logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/
// and loads the settings, the same as newTestApp of package backend.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a.config.Update(func(c *config.Config) {
		config.Load(c, a.layout().Config(), 8080, logx.New(logx.Config, &a.filter, &a.hub))
	})
	return a
}

func (a *testApp) layout() storage.Layout { return storage.Layout(a.StorageDir) }

// renderPage writes one page in the page shell, the same as renderPage of
// package backend.
func (a *testApp) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := render.Renderer{Layout: a.layout(), Config: a.config.Get(), Version: "test", Generator: "OMN-Go test"}
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}

// filesService answers the Service of the stand-in, the same as filesService
// of package backend.
func (a *testApp) filesService() Service {
	return Service{Layout: a.layout(), MimeTypes: a.config.Get().MimeTypes, RenderPage: a.renderPage}
}

func (a *testApp) serveFilesPage(w http.ResponseWriter, r *http.Request) {
	a.filesService().ServePage(w, r)
}

func (a *testApp) treeEntries(tree string) []filesEntry { return a.filesService().treeEntries(tree) }

func (a *testApp) walkStorage(sub string) []indexedFile { return a.filesService().walkStorage(sub) }

func (a *testApp) filesRowFor(tree string, e filesEntry) filesFileRow {
	return a.filesService().filesRowFor(tree, e)
}

func (a *testApp) filesEditable(logical string) bool { return a.filesService().Editable(logical) }
