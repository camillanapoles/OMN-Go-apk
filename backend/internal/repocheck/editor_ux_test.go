package repocheck

import (
	"strings"
	"testing"
)

// The editor toolbar holds the text design tools, and each one edits the
// selection in place. A tool that loses its entry leaves the registry, and
// this test then fails with the name of the tool.
//
// The design tools sit at the front of the TOOLS registry, and the house
// tools follow them. See omn-go-editor.js.
func TestEditorToolbarHoldsTheTextDesignTools(t *testing.T) {
	src, err := readRepoFile("backend/frontend/html/js/OMN-Go/omn-go-editor.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"function wrapSelection",
		"function toggleLinePrefix",
		"function wrapLink",
		"icon: 'format_bold'",
		"icon: 'format_italic'",
		"icon: 'looks_one'",
		"icon: 'looks_two'",
		"icon: 'format_list_bulleted'",
		"icon: 'check_box'",
		"icon: 'format_quote'",
		"icon: 'data_object'",
		"icon: 'link'",
		"icon: 'note_add'",
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("omn-go-editor.js lost %q", marker)
		}
	}
	if n := strings.Count(src, "icon: '"); n < 16 {
		t.Errorf("the TOOLS registry holds %d buttons, and the house holds more", n)
	}
}

// The editor offers the note templates, and the server answers them. Each
// part of the chain carries its marker, and the four built-ins keep their
// names. See doc/decisions/0032-open-note-templates.md.
func TestEditorOffersTheNoteTemplates(t *testing.T) {
	editor, err := readRepoFile("backend/frontend/html/js/OMN-Go/omn-go-editor.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"omn-template-box",
		"function openTemplateBox",
		"function insertTemplate",
		"/api/templates",
	} {
		if !strings.Contains(editor, marker) {
			t.Errorf("omn-go-editor.js lost the template marker %q", marker)
		}
	}
	templates, err := readRepoFile("backend/internal/storage/templates.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"Meeting notes", "Daily log", "Task list", "Book review",
	} {
		if !strings.Contains(templates, `"`+name+`"`) {
			t.Errorf("the built-in template %q left the storage list", name)
		}
	}
	server, err := readRepoFile("backend/internal/app/server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(server, `"/api/templates"`) {
		t.Error("the route table of server.go lost the templates route")
	}
}

// The desktop can keep its notes in the directory that config.json names,
// and each part of the chain holds its marker. See
// doc/decisions/0033-the-config-chooses-the-storage-directory.md.
func TestTheStorageDirectoryIsASetting(t *testing.T) {
	configGo, err := readBackendFile("internal/config/config.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configGo), `json:"storage_dir"`) {
		t.Error("the Config struct lost the storage_dir field")
	}
	fieldsGo, err := readBackendFile("internal/config/fields.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fieldsGo), `Key: "storage_dir"`) {
		t.Error("the field table of config lost the storage_dir row")
	}
	chain, err := readBackendFile("internal/app/storage_dir.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"func storageChainDir", "func peekStorageDir", "func (a *App) followStorageChain",
	} {
		if !strings.Contains(string(chain), marker) {
			t.Errorf("storage_dir.go lost %q", marker)
		}
	}
	initGo, err := readBackendFile("internal/app/storage.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(initGo), "followStorageChain") {
		t.Error("initStorage does not follow the storage chain any more")
	}
	page, err := readRepoFile("backend/frontend/templates/config_page.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`data-screen="storage"`,
		`name="storage_dir"`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("config_page.html lost the marker %q", marker)
		}
	}
}
