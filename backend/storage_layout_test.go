package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageLayoutNamesEachPlace(t *testing.T) {
	l := storageLayout(filepath.FromSlash("/data"))
	for got, want := range map[string]string{
		l.md():                       "/data/md",
		l.md("a", "b.md"):            "/data/md/a/b.md",
		l.html("images"):             "/data/html/images",
		l.db("notes.sqlite"):         "/data/db/notes.sqlite",
		l.git("OMNGO_MERGE_PARENT"):  "/data/.git/OMNGO_MERGE_PARENT",
		l.assetBackups("26.09.1"):    "/data/asset_backups/26.09.1",
		l.file(configFilename):       "/data/config.json",
		l.file("html", "js", "a.js"): "/data/html/js/a.js",
	} {
		if got != filepath.FromSlash(want) {
			t.Errorf("got %q, want %q", got, filepath.FromSlash(want))
		}
	}
}

// storageLayout is the one place that joins a name to StorageDir. A second
// join fails this test.
func TestOnlyStorageLayoutJoinsStoragePaths(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "filepath.Join(a.StorageDir") {
			t.Errorf("%s joins a path to StorageDir. Call a method of a.layout().", f)
		}
	}
}
