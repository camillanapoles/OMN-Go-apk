package repocheck

import (
	"go/parser"
	"go/token"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
)

// importLayers gives the layer of each package of the module. A package imports
// only packages of a lower layer. Each new package under backend/internal gets
// a row here. The layers are the same as groupLayers in
// internal/app/group_links_test.go: 0 for the leaves, up to 6 for app. backend
// is the facade on top.
var importLayers = map[string]int{
	"net.basov.omngo/backend/frontend":            0,
	"net.basov.omngo/backend/internal/logx":       0,
	"net.basov.omngo/backend/internal/config":     1,
	"net.basov.omngo/backend/internal/storage":    2,
	"net.basov.omngo/backend/internal/render":     3,
	"net.basov.omngo/backend/internal/db":         4,
	"net.basov.omngo/backend/internal/gitsync":    4,
	"net.basov.omngo/backend/internal/search":     4,
	"net.basov.omngo/backend/internal/files":      4,
	"net.basov.omngo/backend/internal/exchange":   4,
	"net.basov.omngo/backend/internal/status":     5,
	"net.basov.omngo/backend/internal/noteheader": 0,
	"net.basov.omngo/backend/internal/textmatch":  0,
	"net.basov.omngo/backend/internal/app":        6,
	"net.basov.omngo/backend":                     7,
}

// TestImportLayers reads the imports of each production file below
// backend/. It fails in three cases:
//
//   - a package has no row.
//   - a row names no package.
//   - a package imports a package of the same layer or a higher one.
func TestImportLayers(t *testing.T) {
	const module = "net.basov.omngo/"
	fset := token.NewFileSet()
	found := map[string]bool{}
	for _, rel := range productionGoFiles(t) {
		pkg := module + "backend"
		if dir := path.Dir(rel); dir != "." {
			pkg += "/" + dir
		}
		found[pkg] = true
		layer, ok := importLayers[pkg]
		if !ok {
			t.Errorf("%s has no row in importLayers. Give the package a layer.", pkg)
			continue
		}
		f, err := parser.ParseFile(fset, backendPath(rel), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			to, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(to, module) {
				continue
			}
			toLayer, ok := importLayers[to]
			if !ok || toLayer >= layer {
				t.Errorf("%s imports %s. A package in layer %d can import only a package of a lower layer.",
					rel, to, layer)
			}
		}
	}
	for pkg := range importLayers {
		if !found[pkg] {
			t.Errorf("importLayers names %s, and the package has no production file", pkg)
		}
	}
	if _, err := os.Stat(backendPath("frontend/embed.go")); err != nil {
		t.Errorf("backend/frontend has no embed.go: %v", err)
	}
}
