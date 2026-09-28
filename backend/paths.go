package backend

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// containedName makes a name from a request safe to join under a storage
// directory. It answers a clean, relative name with slashes. A ".." cannot
// climb above the root of the name: "../../x" gives "x", and "a/../../b"
// gives "b". The function removes a leading slash. On Windows, a backslash
// counts as a separator.
//
// filepath.Join resolves a ".." in a name, and the result can leave the
// storage directory. /api/save and /api/note would then write and read files
// outside it. Each path that a request names must pass through this function.
// TestContainedName holds the rule.
func containedName(name string) string {
	return strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(name)), "/")
}

// resolvePageName is the one place that answers two questions about a name
// from a user or a URL. Is it a markdown page? Where are its .md source and
// its .html file? It accepts three forms of the same page:
//
//   - a page name, for example "Welcome" or "Report.2026"
//   - a markdown file name, for example "Welcome.md"
//   - a compiled file name, for example "Welcome.html"
//
// It answers mdPath and htmlPath together, thus no caller builds a path a
// second time. A name that ends in a known file extension, for example ".js"
// or ".txt", is a file under html/. isPage is then false, and mdPath is
// empty.
//
// hasKnownAssetExtension in serving.go is the one authority for that test:
// the LAST extension decides, and an unknown extension is a page. Keep the
// decision here, and do not copy it.
func (a *App) resolvePageName(name string) (mdPath, htmlPath, baseName string, isPage bool) {
	switch {
	case strings.HasSuffix(name, ".md"):
		baseName = strings.TrimSuffix(name, ".md")
		isPage = true
	case strings.HasSuffix(name, ".html"):
		baseName = strings.TrimSuffix(name, ".html")
		isPage = true
	case !a.hasKnownAssetExtension(name):
		baseName = name
		isPage = true
	default:
		// The name ends in an extension that this install serves as a file.
		// See hasKnownAssetExtension.
		name = containedName(name)
		return "", a.layout().html(filepath.FromSlash(name)), name, false
	}

	// pageHTMLPath in render_cache.go is the one formula for the path of a
	// compiled page.
	baseName = containedName(baseName)
	mdPath = a.layout().md(filepath.FromSlash(baseName + ".md"))
	htmlPath = a.pageHTMLPath(baseName)
	return mdPath, htmlPath, baseName, true
}

// fileExists reports whether p is a file that can be read. A directory
// answers false.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
