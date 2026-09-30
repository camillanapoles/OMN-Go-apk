package app

// Tests for the md/ <-> html/ mirror of plain files kept beside a note
// (internal/storage/note_files.go).
//
// The property under test is not "a copy happened". It is that the pair
// SETTLES. Each direction runs on its own event, and a copy carries the time
// of its source. A start that follows a save must find nothing to do.
//
// A mirror that copies a file back and forth on every start would pass a
// naive "the content matches" test. It would rewrite the notes tree of the
// user forever.

import (
	"testing"
)

// The mirror is useful only when the copy is then SERVED, and served as text.
// ".txt" is not in the own MIME table of Go. Without the config.BuiltinMIME
// row, the file has no content type on a device with no /etc/mime.types.
// editableFileType reads that same table, and it then calls the file "not
// text".
func TestTxtIsServedAndEditableText(t *testing.T) {
	a := newTestApp(t)
	if ct := a.resolveContentType("log.txt"); ct != "text/plain; charset=utf-8" {
		t.Errorf("content type of .txt = %q, want text/plain; charset=utf-8", ct)
	}
	if !a.editableFileType("log.txt") {
		t.Error("a .txt file is text and must open in the editor")
	}
	if !a.filesService().Editable("log.txt") {
		t.Error("a .txt file must offer an edit link in the file index")
	}
}
