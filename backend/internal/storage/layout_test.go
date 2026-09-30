package storage

import (
	"path/filepath"
	"testing"
)

func TestLayoutNamesEachPlace(t *testing.T) {
	l := Layout(filepath.FromSlash("/data"))
	for got, want := range map[string]string{
		l.MD():                       "/data/md",
		l.MD("a", "b.md"):            "/data/md/a/b.md",
		l.HTML("images"):             "/data/html/images",
		l.DB("notes.sqlite"):         "/data/db/notes.sqlite",
		l.Git("OMNGO_MERGE_PARENT"):  "/data/.git/OMNGO_MERGE_PARENT",
		l.AssetBackups("26.09.1"):    "/data/asset_backups/26.09.1",
		l.File(ConfigFilename):       "/data/config.json",
		l.File("html", "js", "a.js"): "/data/html/js/a.js",
	} {
		if got != filepath.FromSlash(want) {
			t.Errorf("got %q, want %q", got, filepath.FromSlash(want))
		}
	}
}

func TestRelInside(t *testing.T) {
	root := t.TempDir()
	for p, want := range map[string]string{
		root:                                    ".",
		filepath.Join(root, "md", "a.md"):       filepath.Join("md", "a.md"),
		filepath.Join(root, "md", "..", "b.md"): "b.md",
		filepath.Join(root, "..", "x"):          "",
		filepath.Join(root, "..", filepath.Base(root)+"-other", "x"): "",
		filepath.Dir(root): "",
	} {
		rel, ok := RelInside(root, p)
		if (want != "") != ok || rel != want {
			t.Errorf("RelInside(%q) = %q, %v, want %q", p, rel, ok, want)
		}
	}
}
