package backend

import (
	"fmt"
	"os"
	"path/filepath"
)

// ----------------------------------------------------------------------
// The one pipeline for the compiled HTML cache
// ----------------------------------------------------------------------
//
// THE CACHE RULES, written here and nowhere else:
//
//	- md/<name>.md is the SOURCE. Only a save or an edit writes it,
//	  and the Tags page of tags.go is the one exception.
//	- html/<name>.html is a CACHE. renderAndCache is its ONLY writer.
//	- The mtime test of serveHTMLPage is its ONLY invalidator: md newer
//	  than html, html missing, or ?refresh.
//
// The cached HTML is an INCOMPLETE page on purpose. It holds
// runtimeVarsMarker, and injectRuntimeVars fills the marker for each request
// with the values of now, for example APP_VERSION and the theme. A change of
// these values thus needs no new cache. Do not put the values into the cache
// when it compiles.

// pageHTMLPath is the one formula for the compiled HTML path of a page.
// resolvePageName answers the same path for a page, and TestPageHTMLPath
// compares the two.
func (a *App) pageHTMLPath(name string) string {
	return a.layout().html(filepath.FromSlash(containedName(name) + ".html"))
}

// renderAndCache compiles a page and writes html/<name>.html. It is the ONLY
// writer of that file. name has no extension, and content is the markdown
// source. The function makes each parent directory. It answers the compiled
// bytes for a caller that also sends them.
func (a *App) renderAndCache(name string, content []byte) ([]byte, error) {
	compiled := a.compilePage(name, content)
	htmlPath := a.pageHTMLPath(name)
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
		return compiled, fmt.Errorf("cache %q: mkdir: %w", name, err)
	}
	if err := os.WriteFile(htmlPath, compiled, 0644); err != nil {
		return compiled, fmt.Errorf("cache %q: write: %w", name, err)
	}
	// Each change of a note inside the process comes here, thus this is the
	// one place that tells the search index about it. It only skips the wait
	// for the next stat walk.
	a.markSearchIndexDirty()
	return compiled, nil
}
