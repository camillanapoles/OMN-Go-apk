package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The chain walks the pointers of config.json, and each rule of the walk
// holds. The peek of the test answers from a map, thus the walk reads no
// disk, and the test holds the rules alone.
func TestStorageChainWalksThePointers(t *testing.T) {
	peek := func(m map[string]string) func(string) string {
		return func(path string) string { return m[path] }
	}
	t.Run("three hops end at the last name", func(t *testing.T) {
		m := map[string]string{
			filepath.Join("A", "config.json"): "B",
			filepath.Join("B", "config.json"): "C",
		}
		if got := storageChainDir("A", peek(m)); got != "C" {
			t.Errorf("the walk ended at %s, want C", got)
		}
	})
	t.Run("a pointer back to a visited name ends the walk", func(t *testing.T) {
		m := map[string]string{
			filepath.Join("A", "config.json"): "B",
			filepath.Join("B", "config.json"): "A",
		}
		if got := storageChainDir("A", peek(m)); got != "B" {
			t.Errorf("the walk ended at %s, want B with no loop", got)
		}
	})
	t.Run("a child of the current directory ends the walk", func(t *testing.T) {
		m := map[string]string{
			filepath.Join("A", "config.json"): filepath.Join("A", "sub"),
		}
		if got := storageChainDir("A", peek(m)); got != "A" {
			t.Errorf("the walk ended at %s, want A with no nested storage", got)
		}
	})
	t.Run("an empty answer ends the walk", func(t *testing.T) {
		if got := storageChainDir("A", func(string) string { return "" }); got != "A" {
			t.Errorf("the walk ended at %s, want A", got)
		}
	})
}

// peekStorageDir reads the field, and it answers an empty string for an
// absent file, a broken file and a field that is absent.
func TestPeekStorageDirReadsTheField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if got := peekStorageDir(filepath.Join(dir, "absent.json")); got != "" {
		t.Errorf("an absent file answered %q, want empty", got)
	}
	os.WriteFile(path, []byte("not json"), 0644)
	if got := peekStorageDir(path); got != "" {
		t.Errorf("a broken file answered %q, want empty", got)
	}
	os.WriteFile(path, []byte(`{"theme":"dark"}`), 0644)
	if got := peekStorageDir(path); got != "" {
		t.Errorf("a config without the field answered %q, want empty", got)
	}
	os.WriteFile(path, []byte(`{"storage_dir":"  /tmp/notes  "}`), 0644)
	if got := peekStorageDir(path); got != "/tmp/notes" {
		t.Errorf("the field answered %q, want the trimmed value", got)
	}
}

// followStorageChain moves the App to the directory of the config, and the
// directory exists after the move.
func TestFollowStorageChainMovesTheApp(t *testing.T) {
	a := newUnconfiguredApp(t)
	next := filepath.Join(a.StorageDir, "..", "chosen-notes")
	os.WriteFile(filepath.Join(a.StorageDir, "config.json"),
		[]byte(`{"storage_dir":"`+next+`"}`), 0644)

	a.followStorageChain()

	if a.StorageDir != next {
		t.Fatalf("the storage stayed at %s, want %s", a.StorageDir, next)
	}
	if _, err := os.Stat(next); err != nil {
		t.Errorf("the chosen directory does not exist: %v", err)
	}
}

// handleTemplates answers the drop-in files of the storage and the
// built-ins, with the mark that tells them apart.
func TestHandleTemplatesAnswersEachSource(t *testing.T) {
	a := newTestApp(t)
	tplDir := a.layout().Templates()
	if err := os.MkdirAll(tplDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tplDir, "From Obsidian.md"), []byte("Title: Mine\n"), 0644)

	rec := httptest.NewRecorder()
	a.handleTemplates(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("handleTemplates answered %d, want 200", rec.Code)
	}
	var out struct {
		Dir       string `json:"dir"`
		DirExists bool   `json:"dir_exists"`
		Templates []struct {
			Name    string `json:"name"`
			Builtin bool   `json:"builtin"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the answer is no JSON: %v", err)
	}
	if !out.DirExists {
		t.Error("the answer says the templates directory is absent")
	}
	var mine, meeting *struct {
		Name    string `json:"name"`
		Builtin bool   `json:"builtin"`
	}
	for i := range out.Templates {
		if out.Templates[i].Name == "From Obsidian" {
			mine = &out.Templates[i]
		}
		if out.Templates[i].Name == "Meeting notes" {
			meeting = &out.Templates[i]
		}
	}
	if mine == nil || mine.Builtin {
		t.Error("the drop-in template is absent or carries a wrong mark")
	}
	if meeting == nil || !meeting.Builtin {
		t.Error("the built-in template is absent or lost its mark")
	}
}
