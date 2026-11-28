package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fresh install answers the built-ins, and a file in the directory
// shadows the built-in of the same name. The list stays sorted by name.
func TestListTemplatesAnswersTheBuiltins(t *testing.T) {
	got := ListTemplates(t.TempDir())
	if len(got) != 4 {
		t.Fatalf("ListTemplates answered %d templates, want the 4 built-ins", len(got))
	}
	for _, w := range []string{"Book review", "Daily log", "Meeting notes", "Task list"} {
		found := false
		for _, g := range got {
			if g.Name == w {
				found = true
				if !g.Builtin {
					t.Errorf("template %s lost the Builtin mark", w)
				}
			}
		}
		if !found {
			t.Errorf("ListTemplates answered no %s template", w)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Name > got[i].Name {
			t.Fatalf("the list is not sorted by name: %s before %s", got[i-1].Name, got[i].Name)
		}
	}
}

// A .md file in the directory joins the list, and a file of the name of a
// built-in replaces its text. A subdirectory and a broken name stay out.
func TestListTemplatesShadowsABuiltin(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "nested"), 0755)
	os.WriteFile(filepath.Join(dir, "Meeting notes.md"), []byte("Title: My meeting\n"), 0644)
	os.WriteFile(filepath.Join(dir, "From Obsidian.md"), []byte("Title: From Obsidian\n"), 0644)
	os.WriteFile(filepath.Join(dir, "draft.md.bak"), []byte("not a template"), 0644)

	got := ListTemplates(dir)
	var mine, meeting *Template
	for i := range got {
		if got[i].Name == "From Obsidian" {
			mine = &got[i]
		}
		if got[i].Name == "Meeting notes" {
			meeting = &got[i]
		}
	}
	if mine == nil {
		t.Fatal("the drop-in file did not join the list")
	}
	if mine.Builtin {
		t.Error("a drop-in file must not carry the Builtin mark")
	}
	if meeting == nil {
		t.Fatal("the Meeting notes template vanished")
	}
	if meeting.Builtin {
		t.Error("the shadowing file must replace the built-in, with no Builtin mark")
	}
	if !strings.Contains(meeting.Content, "My meeting") {
		t.Errorf("the shadowed content did not win: %q", meeting.Content)
	}
	if len(got) != 5 {
		t.Errorf("the list holds %d templates, want 5 with the one drop-in", len(got))
	}
}

// An absent directory answers the built-ins alone, and no error leaves the
// call broken.
func TestListTemplatesToleratesAnAbsentDirectory(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "no-such-dir")
	got := ListTemplates(absent)
	if len(got) != 4 {
		t.Fatalf("an absent directory answered %d templates, want the 4 built-ins", len(got))
	}
}
