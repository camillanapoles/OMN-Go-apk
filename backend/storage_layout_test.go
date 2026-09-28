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
		rel, ok := relInside(root, p)
		if (want != "") != ok || rel != want {
			t.Errorf("relInside(%q) = %q, %v, want %q", p, rel, ok, want)
		}
	}
}

// relInside is the one containment test. A second copy of the ".." test
// fails this test.
func TestOnlyRelInsideTestsContainment(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "storage_layout.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `".."+string(filepath.Separator)`) {
			t.Errorf("%s tests containment itself. Call relInside.", f)
		}
	}
}
