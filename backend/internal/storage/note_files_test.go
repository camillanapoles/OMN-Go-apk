package storage

import "testing"

func TestIsSyncedNoteFile(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"log.txt", true},
		{"SHOUT.TXT", true}, // the extension is matched case-insensitively
		{"project/data.txt", true},
		// A markdown file in md/ is a NOTE. Copying it into html/ would put
		// the source of a page next to that page's compiled cache.
		{"Note.md", false},
		{"Note.html", false},
		{"photo.png", false},
		{"app.js", false},
		{"README", false},
	} {
		if got := IsSyncedNoteFile(c.name); got != c.want {
			t.Errorf("IsSyncedNoteFile(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
