package backend

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// importLayers gives the layer of each package of the module. A package
// imports only packages of a lower layer. Each new package under
// backend/internal gets a row here. The layers are the same as groupLayers
// in group_links_test.go: 0 for the leaves, up to 6 for app. backend is the
// facade on top.
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
	"net.basov.omngo/backend/internal/noteheader": 0,
	"net.basov.omngo/backend/internal/textmatch":  0,
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
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		pkg := module + "backend"
		if dir := filepath.ToSlash(filepath.Dir(p)); dir != "." {
			pkg += "/" + dir
		}
		found[pkg] = true
		layer, ok := importLayers[pkg]
		if !ok {
			t.Errorf("%s has no row in importLayers. Give the package a layer.", pkg)
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(path, module) {
				continue
			}
			to, ok := importLayers[path]
			if !ok || to >= layer {
				t.Errorf("%s imports %s. A package in layer %d can import only a package of a lower layer.",
					p, path, layer)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg := range importLayers {
		if !found[pkg] {
			t.Errorf("importLayers names %s, and the package has no production file", pkg)
		}
	}
	if _, err := os.Stat("frontend/embed.go"); err != nil {
		t.Errorf("backend/frontend has no embed.go: %v", err)
	}
}

// productionGoFiles answers each Go file below backend/ that is not a test,
// as a slash path from backend/. A source scan uses it, thus a package of the
// split cannot hide a file from the scan.
func productionGoFiles(t *testing.T) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		names = append(names, filepath.ToSlash(p))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "internal/render/templates.go") {
		t.Fatal("the scan did not find internal/render/templates.go. The scan is broken.")
	}
	return names
}
