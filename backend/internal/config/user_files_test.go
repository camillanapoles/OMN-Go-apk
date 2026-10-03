package config

import (
	"strings"
	"testing"
)

// Each tree of UserFileTrees needs its own directory, route and search kind.
// Two rows with one directory would mix the files of two uploads. This test
// fails for a row that a person copied and did not change.
func TestUserFileTreesAreDistinct(t *testing.T) {
	dirs := map[string]bool{}
	routes := map[string]bool{}
	exts := map[string]string{}
	for _, tree := range UserFileTrees {
		if tree.Dir == "" || tree.Label == "" || len(tree.Exts) == 0 {
			t.Errorf("the row %+v has an empty field", tree)
		}
		if dirs[tree.Dir] || routes[tree.Upload] {
			t.Errorf("%s: the directory or the route is in two rows", tree.Dir)
		}
		dirs[tree.Dir] = true
		routes[tree.Upload] = true
		if !strings.HasPrefix(tree.Upload, "/api/upload_") {
			t.Errorf("%s: the route %q does not start with /api/upload_", tree.Dir, tree.Upload)
		}
		for _, ext := range tree.Exts {
			if ext != strings.ToLower(ext) || !strings.HasPrefix(ext, ".") {
				t.Errorf("%s: the extension %q is not a lower-case extension", tree.Dir, ext)
			}
			if other, ok := exts[ext]; ok {
				t.Errorf("%s is in %s and in %s. One extension has one tree.", ext, other, tree.Dir)
			}
			exts[ext] = tree.Dir
		}
	}
}

// The index can cover each tree, and the Config page has one checkbox for
// each kind of SearchKindsAll. A tree is off by default, the same as JSON.
func TestEachUserFileTreeIsASearchKind(t *testing.T) {
	for _, tree := range UserFileTrees {
		got := NormalizeSearchKinds([]string{tree.Dir})
		if len(got) != 1 || got[0] != tree.Dir {
			t.Errorf("NormalizeSearchKinds drops the kind %q: %v", tree.Dir, got)
		}
		for _, kind := range NormalizeSearchKinds(nil) {
			if kind == tree.Dir {
				t.Errorf("the default index covers %s. A tree costs memory, thus it is optional.", tree.Dir)
			}
		}
	}
}

// A file of a tree must be a file and not a note, on each device. The
// builtin table thus needs a row for each extension. The editor must open
// it, because ?edit=true is the one way to change a contact in the
// application.
//
// A contact and a calendar are text/plain. See
// doc/decisions/0025-serve-a-contact-and-a-calendar-as-plain-text.md.
func TestEachUserFileExtensionIsAKnownTextFile(t *testing.T) {
	for _, tree := range UserFileTrees {
		for _, ext := range tree.Exts {
			name := tree.Dir + "/data" + ext
			if !HasKnownAssetExtension(nil, name) {
				t.Errorf("%s is a note. BuiltinMIME needs a row for %s.", name, ext)
			}
			if !EditableFileType(nil, name) {
				t.Errorf("%s gets no editor", name)
			}
			if tree.Dir == "user_json" {
				continue
			}
			if ct := BuiltinMIME[ext]; ct != "text/plain; charset=utf-8" {
				t.Errorf("%s has the type %q. The Android WebView shows only text/plain.", ext, ct)
			}
		}
	}
}
