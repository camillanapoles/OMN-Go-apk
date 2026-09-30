package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderAndCacheWritesCompiledHTML pins the single cache pipeline that
// Phase 2 introduced. renderAndCache compiles a page and writes it to
// html/<name>.html. The on-disk bytes equal the output of
// render.Renderer.CompilePage, and they equal the bytes that the function
// returns.
func TestRenderAndCacheWritesCompiledHTML(t *testing.T) {
	a := newTestApp(t)
	content := []byte("Title: Doc\n\nHello **bold**")

	compiled, err := a.renderAndCache("Doc", content)
	if err != nil {
		t.Fatalf("renderAndCache: %v", err)
	}

	// Returned bytes must equal a direct render.Renderer.CompilePage of the same
	// input.
	if want := a.testRenderer().CompilePage("Doc", content); !bytes.Equal(compiled, want) {
		t.Error("returned bytes differ from render.Renderer.CompilePage output")
	}

	// The on-disk cache must equal the returned bytes exactly.
	onDisk, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "Doc.html"))
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	if !bytes.Equal(onDisk, compiled) {
		t.Error("on-disk cache differs from returned bytes")
	}

	// A sanity check. It is a real compiled page, and it carries the raw
	// runtime marker. The cache is deliberately an incomplete template. See
	// the contract in internal/render/cache.go.
	s := string(onDisk)
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Error("cache missing rendered markdown body")
	}
	if !strings.Contains(s, `<meta id="omn-go-runtime-vars-marker">`) {
		t.Error("cache missing the runtime-vars marker")
	}
}

// TestRenderAndCacheCreatesNestedDirs pins that renderAndCache materializes
// the parent directory tree for a nested page name (e.g. "local/deep/Note")
// rather than failing when html/local/deep does not exist yet.
func TestRenderAndCacheCreatesNestedDirs(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.renderAndCache("local/deep/Note", []byte("Title: N\n\nx")); err != nil {
		t.Fatalf("renderAndCache nested: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "local", "deep", "Note.html")); err != nil {
		t.Fatalf("nested cache file not created: %v", err)
	}
}

// TestPageHTMLPath holds the one path formula of a compiled page.
// storage.Layout.PageHTML and resolvePageName must give the same path for
// each page. A second formula fails this test.
func TestPageHTMLPath(t *testing.T) {
	a := &App{StorageDir: "/store"}

	for _, name := range []string{"Note", "dir/Note", "a/b/c"} {
		want := filepath.Join("/store", "html", filepath.Clean(name+".html"))
		if got := a.layout().PageHTML(name); got != want {
			t.Errorf("storage.Layout.PageHTML(%q) = %q, want %q", name, got, want)
		}
		_, htmlPath, _, isPage := a.resolvePageName(name)
		if !isPage {
			t.Fatalf("resolvePageName(%q) unexpectedly not a page", name)
		}
		if got := a.layout().PageHTML(name); got != htmlPath {
			t.Errorf("storage.Layout.PageHTML(%q)=%q disagrees with resolvePageName htmlPath %q", name, got, htmlPath)
		}
	}
}
