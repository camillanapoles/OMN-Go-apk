package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/render"
)

// ----------------------------------------------------------------------
// The content type of an install
// ----------------------------------------------------------------------
//
// config.BuiltinMIME in internal/config/content_types.go is the one authority
// for a content type, and Config.MimeTypes is an override that
// config.ResolveContentType reads first.
//
// A fresh install must write no override. Each row of an override hides
// the table. newTestApp builds a Config with an empty map, thus it cannot
// show that fault. See doc/decisions/0003-use-one-table-for-each-content-type.md.
//
// These tests measure the state of a REAL INSTALL. Each one runs
// loadConfig against an empty directory, which is what a first start
// does.

// freshInstall runs loadConfig against an empty storage directory and
// returns the application and the path of config.json.
func freshInstall(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	a := &App{StorageDir: dir}
	a.loadConfig(a.layout().Config())
	return a, filepath.Join(dir, "config.json")
}

// A new install must write no mime_types key. The field is an override,
// and a new install overrides nothing.
func TestFreshInstallWritesNoMimeSeed(t *testing.T) {
	_, path := freshInstall(t)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the fresh install wrote no config.json: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("config.json does not parse: %v", err)
	}
	seed, ok := onDisk["mime_types"]
	if !ok {
		return // the key is absent, which is the state this test wants
	}
	if m, isMap := seed.(map[string]any); isMap && len(m) > 0 {
		t.Errorf("the fresh install seeded mime_types with %d rows. Each row "+
			"shadows config.BuiltinMIME, and the seed carried no charset.", len(m))
	}
}

// A fresh install answers each content type with its charset. The test
// fails when a seed answers in place of the table.
func TestFreshInstallServesTheCharset(t *testing.T) {
	a, _ := freshInstall(t)

	for _, c := range []struct{ name, want string }{
		{"x.css", "text/css; charset=utf-8"},
		{"x.js", "text/javascript; charset=utf-8"},
		{"x.html", "text/html; charset=utf-8"},
		{"x.md", "text/markdown; charset=utf-8"},
		{"x.txt", "text/plain; charset=utf-8"},
		{"x.jsonl", "text/plain; charset=utf-8"},
	} {
		if got := a.resolveContentType(c.name); got != c.want {
			t.Errorf("a fresh install answers %q for %s, want %q", got, c.name, c.want)
		}
	}

	// A type that carries no text needs no charset, and it must not gain
	// one.
	for _, c := range []struct{ name, want string }{
		{"x.png", "image/png"},
		{"x.woff2", "font/woff2"},
		{"x.json", "application/json"},
	} {
		if got := a.resolveContentType(c.name); got != c.want {
			t.Errorf("a fresh install answers %q for %s, want %q", got, c.name, c.want)
		}
	}
}

// An install that ran an older version holds the seed. The repair drops
// it, and the table answers again.
func TestLegacyMimeSeedIsDropped(t *testing.T) {
	for i, seed := range config.LegacyMimeSeeds {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		writeConfigWithMime(t, path, seed)

		a := &App{StorageDir: dir}
		a.loadConfig(a.layout().Config())

		if got := a.config.Get().MimeTypes; got != nil {
			t.Errorf("seed %d survived the load as %v", i, got)
		}
		if got := a.resolveContentType("x.css"); got != "text/css; charset=utf-8" {
			t.Errorf("seed %d: .css answers %q after the repair", i, got)
		}

		// The repair writes config.json, thus a second start finds
		// nothing to do.
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("seed %d: reading config.json: %v", i, err)
		}
		if strings.Contains(string(raw), `"mime_types": {`) {
			t.Errorf("seed %d: the map is still on disk after the repair", i)
		}
	}
}

// A map that a person wrote by hand is a choice. The repair must keep
// each row of it, including a row that equals a row of the seed.
func TestHandWrittenMimeMapSurvives(t *testing.T) {
	// The ten-row seed with ONE row changed. A rule that dropped a row at
	// a time would take the other nine and leave a map that the person
	// never wrote.
	mine := map[string]string{}
	for k, v := range config.LegacyMimeSeeds[0] {
		mine[k] = v
	}
	mine[".css"] = "text/css; charset=windows-1251"

	dir := t.TempDir()
	writeConfigWithMime(t, filepath.Join(dir, "config.json"), mine)

	a := &App{StorageDir: dir}
	a.loadConfig(a.layout().Config())

	got := a.config.Get().MimeTypes
	if len(got) != len(mine) {
		t.Fatalf("the map holds %d rows after the load, want %d", len(got), len(mine))
	}
	if got[".css"] != "text/css; charset=windows-1251" {
		t.Errorf("the changed row is %q", got[".css"])
	}
	if got[".png"] != "image/png" {
		t.Errorf("a row of the person went away: .png is %q", got[".png"])
	}
	if ct := a.resolveContentType("x.css"); ct != "text/css; charset=windows-1251" {
		t.Errorf("the override no longer answers: .css is %q", ct)
	}
}

// A map with one row alone is a choice as well, and it must survive.
func TestSmallMimeMapSurvives(t *testing.T) {
	dir := t.TempDir()
	writeConfigWithMime(t, filepath.Join(dir, "config.json"),
		map[string]string{".rst": "text/x-rst"})

	a := &App{StorageDir: dir}
	a.loadConfig(a.layout().Config())

	if got := a.resolveContentType("x.rst"); got != "text/x-rst" {
		t.Errorf(".rst answers %q, want the value of the person", got)
	}
	if got := a.resolveContentType("x.css"); got != "text/css; charset=utf-8" {
		t.Errorf(".css answers %q, and the map of the person does not name it", got)
	}
}

// hasKnownAssetExtension asks the same two sources, and it decides
// whether a name is a note or a file. The removal of the seed must NOT
// change that answer for any name, or a note of a person becomes a file.
//
// Each row of each seed is a row of config.BuiltinMIME as well, thus the union
// does not change. This test holds that fact.
func TestDroppingTheSeedChangesNoNoteName(t *testing.T) {
	for i, seed := range config.LegacyMimeSeeds {
		for ext := range seed {
			if _, ok := config.BuiltinMIME[ext]; !ok {
				t.Errorf("seed %d names %s and config.BuiltinMIME does not. A name that "+
					"ends in %s was a file before the repair and is a note after it.",
					i, ext, ext)
			}
		}
	}

	withSeed := &App{}
	withSeed.config.Update(func(c *config.Config) { c.MimeTypes = config.LegacyMimeSeeds[0] })
	clean := &App{}

	for _, name := range []string{
		"Report.2026", "Draft.txt", "Note.md", "page.html", "style.css",
		"script.js", "photo.png", "font.woff2", "data.json", "Plan.doc",
	} {
		if a, b := withSeed.hasKnownAssetExtension(name), clean.hasKnownAssetExtension(name); a != b {
			t.Errorf("%q is a file=%v with the seed and a file=%v without it", name, a, b)
		}
	}
}

// writeConfigWithMime writes a config.json that carries one mime_types
// map and nothing else that matters.
func writeConfigWithMime(t *testing.T, path string, mime map[string]string) {
	t.Helper()
	cfg := config.Config{
		ServerPort:    8080,
		AdminPassword: "x",
		MimeTypes:     mime,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ----------------------------------------------------------------------
// The content type of a page
// ----------------------------------------------------------------------

// render.WriteHTMLHeader is the one place that names the content type of a
// page. A handler that writes the header by hand can lose the charset.
//
// This test scans the source, the same as TestNoDirectLogPrintf.
func TestNoBareHTMLContentType(t *testing.T) {
	// A test file ships to no device, and this file names the banned text
	// as a string. productionGoFiles skips each test file.
	for _, name := range productionGoFiles(t) {
		// internal/render/pages.go declares the value, thus it holds the text
		// once.
		if name == "internal/render/pages.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `Set("Content-Type", "text/html`) {
				continue
			}
			t.Errorf("%s:%d writes the HTML content type by hand. Call "+
				"render.WriteHTMLHeader(w) instead. A header with no charset lets the "+
				"browser guess the encoding of a page that the server renders.",
				name, i+1)
		}
	}
}

// The one value that render.WriteHTMLHeader writes. It carries the charset, and
// it keeps the prefix that pageCacheWriter reads.
func TestPageContentTypeCarriesTheCharset(t *testing.T) {
	if render.HTMLContentType != "text/html; charset=utf-8" {
		t.Fatalf("the page content type is %q", render.HTMLContentType)
	}
	// The value must start with the prefix that pageCacheWriter reads.
	if !strings.HasPrefix(render.HTMLContentType, "text/html") {
		t.Error("pageCacheWriter reads the prefix text/html to make a page no-store")
	}
}
