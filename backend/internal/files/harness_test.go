package files

import (
	"net/http"
	"testing"

	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub.
type testApp struct {
	*testkit.App
}

func newTestApp(t *testing.T) *testApp { return &testApp{App: testkit.New(t)} }

// filesService answers the Service of the stand-in, the same as filesService
// of package app.
func (a *testApp) filesService() Service {
	return Service{Layout: a.Layout(), MimeTypes: a.Config.Get().MimeTypes, RenderPage: a.RenderPage}
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
