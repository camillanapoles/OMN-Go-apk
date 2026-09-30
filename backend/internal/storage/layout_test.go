package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// backendRoot is the backend/ directory. The test runs in
// backend/internal/storage.
const backendRoot = "../.."

// productionFiles answers each Go file below backend/ that is not a test,
// with a slash path from backendRoot.
func productionFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(backendRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(backendRoot, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("found no production file below backend/")
	}
	return out
}

// Layout is the one place that joins a name to StorageDir. A second join
// fails this test.
func TestOnlyLayoutJoinsStoragePaths(t *testing.T) {
	for _, f := range productionFiles(t) {
		src, err := os.ReadFile(filepath.Join(backendRoot, f))
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
		rel, ok := RelInside(root, p)
		if (want != "") != ok || rel != want {
			t.Errorf("RelInside(%q) = %q, %v, want %q", p, rel, ok, want)
		}
	}
}

// RelInside is the one containment test. A second copy of the ".." test
// fails this test.
func TestOnlyRelInsideTestsContainment(t *testing.T) {
	for _, f := range productionFiles(t) {
		if f == "internal/storage/layout.go" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(backendRoot, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `".."+string(filepath.Separator)`) {
			t.Errorf("%s tests containment itself. Call storage.RelInside.", f)
		}
	}
}
