package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/storage"
)

// TestHasKnownAssetExtension pins the rule that decides a note from a file.
// The LAST extension decides, and only the two tables of this install answer.
//
// The stdlib table takes no part on purpose. mime.TypeByExtension reads
// /etc/mime.types, thus a desktop with mime-support knows ".doc" and a phone
// does not. A note named "Plan.doc" would then be a file on one device and a
// note on the other. Git sync carries that name to both.
func TestHasKnownAssetExtension(t *testing.T) {
	a := &App{StorageDir: "/store"}
	cases := map[string]bool{
		"Welcome":       false,
		"Report.2026":   false,
		"a.b.c":         false,
		"Plan.doc":      false, // the stdlib knows this one. This function must not.
		"Draft.txt":     true,
		"app.js":        true,
		"js/app.min.js": true,
		"images/x.png":  true,
		"page.html":     true,
		"note.md":       true,
	}
	for name, want := range cases {
		if got := a.hasKnownAssetExtension(name); got != want {
			t.Errorf("hasKnownAssetExtension(%q) = %v, want %v", name, got, want)
		}
	}

	// A per-install override in config.json counts, because that install
	// really does serve the extension.
	b := &App{StorageDir: "/store"}
	b.config.Update(func(c *config.Config) { c.MimeTypes = map[string]string{".2026": "text/plain"} })
	if !b.hasKnownAssetExtension("Report.2026") {
		t.Error("a mime_types override in config.json did not count")
	}
}

// ---------------------------------------------------------------------
// A note name that holds a dot, end to end
//
// The unit tests above pin the classifier. These two pin what a person
// meets: the page comes back, and a save reaches the markdown source.
// ---------------------------------------------------------------------

// TestDottedNoteNameServesAndSaves holds two rules. /Report.2026.html
// serves the note. A save for a note named "Draft.txt" writes the markdown
// source, and not the file html/Draft.txt.
func TestDottedNoteNameServesAndSaves(t *testing.T) {
	a := newTestApp(t)

	notes := map[string]string{
		"Report.2026.md": "Title: Report\n\nthe body of the report",
		"a.b.c.md":       "Title: Deep\n\nthe deep body",
		"Draft.txt.md":   "Title: Draft\n\nthe draft body",
	}
	for rel, body := range notes {
		baseWriteMD(t, a, rel, body)
	}

	// 1. Each one comes back as a page, with its body in it.
	for _, tc := range []struct{ url, want string }{
		{"/Report.2026.html", "the body of the report"},
		{"/a.b.c.html", "the deep body"},
		{"/Draft.txt.html", "the draft body"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		rec := httptest.NewRecorder()
		a.serveFrontend(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200. A note name may hold a dot.", tc.url, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("GET %s does not carry %q", tc.url, tc.want)
		}
	}

	// 2. A save reaches the markdown source, and writes nothing under html/
	//    beside the compiled page. "Draft.txt" is the dangerous name: ".txt"
	//    is an extension this install serves, thus the bare form resolves to
	//    a file. The editor sends "Draft.txt.md", which cannot.
	postForm(t, a.handleSaveNote, "/api/save", url.Values{
		"name":    {"Draft.txt.md"},
		"content": {"Title: Draft\n\nthe new draft body"},
	})
	src, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Draft.txt.md"))
	if err != nil {
		t.Fatalf("reading the note source: %v", err)
	}
	if !strings.Contains(string(src), "the new draft body") {
		t.Error("the save did not reach md/Draft.txt.md")
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "Draft.txt")); err == nil {
		t.Error("the save wrote html/Draft.txt. A save for a note must never " +
			"land beside the files, where nothing reads it and the note keeps " +
			"the old text.")
	}
}

// TestDottedNoteNameReachesTheEditor holds the path of the editor. The
// editor gets its text from /api/note. For a note named "Draft.txt" it
// must ask for the note, and not for the file html/Draft.txt.
func TestDottedNoteNameReachesTheEditor(t *testing.T) {
	a := newTestApp(t)
	baseWriteMD(t, a, "Draft.txt.md", "Title: Draft\n\nthe draft body")

	// The editor page names what it will ask for.
	rec := httptest.NewRecorder()
	a.renderInternalEditor(rec, "Draft.txt.html")
	page := rec.Body.String()
	if !strings.Contains(page, `OMN_EDIT_NAME = 'Draft.txt.md'`) {
		t.Errorf("the editor page does not send the unambiguous name. It must "+
			"ask for %q, or /api/note answers with the file of that name.",
			"Draft.txt.md")
	}

	// And that name really does bring the note back.
	req := httptest.NewRequest(http.MethodGet, "/api/note?name=Draft.txt.md", nil)
	got := httptest.NewRecorder()
	a.handleGetNote(got, req)
	if got.Code != http.StatusOK {
		t.Fatalf("/api/note?name=Draft.txt.md = %d, want 200", got.Code)
	}
	if !strings.Contains(got.Body.String(), "the draft body") {
		t.Error("/api/note did not answer with the note text")
	}
}

// TestDottedNoteCompilesAsMarkdown exists because the compiled page carries
// IS_MARKDOWN, and the frontend reads it to decide which controls a page gets.
// render.Renderer.CompilePageWithBody cannot answer from the name alone: a note
// named "Draft.txt" and the file html/Draft.txt look the same there. It reads
// the caller instead. An empty customBody means a note.
func TestDottedNoteCompilesAsMarkdown(t *testing.T) {
	a := newTestApp(t)
	for _, name := range []string{"Welcome", "Report.2026", "Draft.txt", "a.b.c"} {
		page := string(a.testRenderer().CompilePage(name, []byte("Title: X\n\nbody")))
		if !strings.Contains(page, "var IS_MARKDOWN = true;") {
			t.Errorf("the compiled page of note %q is not markdown. The page "+
				"then loses each control that belongs to a note.", name)
		}
		if !strings.Contains(page, "var PAGE_EXT = '';") {
			t.Errorf("the compiled page of note %q carries a file extension "+
				"in PAGE_EXT", name)
		}
	}

	// A server-built view of a file keeps the extension of that file.
	wait := string(a.testRenderer().CompilePageWithBody("js/app.min.js",
		[]byte("Title: Wait\n\n"), "<p>waiting</p>"))
	if !strings.Contains(wait, "var PAGE_EXT = '.js';") {
		t.Error("a server-built view of a .js file lost its extension")
	}
}

// pathFilesOutside answers each regular file in the parent of the storage
// directory that is not inside the storage directory. Each test here has
// its own parent, thus a file there came from the test.
func pathFilesOutside(t *testing.T, a *App) []string {
	t.Helper()
	var out []string
	parent := filepath.Dir(a.StorageDir)
	filepath.Walk(parent, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasPrefix(p, a.StorageDir+string(os.PathSeparator)) {
			rel, _ := filepath.Rel(parent, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// A name with ".." must not write outside the storage directory. Before
// storage.ContainedName, /api/save wrote each of these names to a file outside
// it. A name with a known file extension could write any .js or .json
// file on the device. The route is admin only, but a loopback caller is
// admin.
func TestSaveStaysInTheStorageDirectory(t *testing.T) {
	for _, name := range []string{"../../escape", "../../escape.js", "sub/../../../escape"} {
		// A subtest has its own temporary parent directory.
		t.Run(name, func(t *testing.T) {
			a := newTestApp(t)
			rec := postForm(t, a.handleSaveNote, "/api/save", url.Values{"name": {name}, "content": {"x"}})
			if rec.Code != http.StatusOK {
				t.Errorf("status %d, want 200", rec.Code)
			}
			if out := pathFilesOutside(t, a); len(out) != 0 {
				t.Errorf("files outside the storage directory: %v", out)
			}
		})
	}
}

// /api/note needs no login. A name with ".." must not read a file outside the
// storage directory, and it must not create one. Before storage.ContainedName
// it did both. It read an outside .txt, .json or .md file. For a missing page,
// it wrote a new .md file outside.
func TestGetNoteStaysInTheStorageDirectory(t *testing.T) {
	a := newTestApp(t)
	secret := filepath.Join(filepath.Dir(a.StorageDir), "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret.txt", "../../secret.txt", "../new-page"} {
		req := httptest.NewRequest(http.MethodGet, "/api/note?name="+url.QueryEscape(name), nil)
		rec := httptest.NewRecorder()
		a.handleGetNote(rec, req)
		if strings.Contains(rec.Body.String(), "SECRET") {
			t.Errorf("%q: the answer holds a file from outside the storage directory", name)
		}
	}
	if out := pathFilesOutside(t, a); len(out) != 1 || out[0] != "secret.txt" {
		t.Errorf("files outside the storage directory: %v, want only secret.txt", out)
	}
}

// /api/newpage writes the target and rewrites the source. A ".." in
// either one must stay in the md directory. Before storage.ContainedName, a
// source outside the storage directory got a new link line written into
// it.
func TestNewPageStaysInTheStorageDirectory(t *testing.T) {
	a := newTestApp(t)
	victim := filepath.Join(filepath.Dir(a.StorageDir), "victim.md")
	if err := os.WriteFile(victim, []byte("Title: Victim\n\nbody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rec := postForm(t, a.handleNewPage, "/api/newpage", url.Values{
		"source": {"../victim"}, "target": {"../../escape"}, "title": {"T"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if got, _ := os.ReadFile(victim); string(got) != "Title: Victim\n\nbody\n" {
		t.Errorf("the file outside the storage directory changed:\n%s", got)
	}
	if out := pathFilesOutside(t, a); len(out) != 1 || out[0] != "victim.md" {
		t.Errorf("files outside the storage directory: %v, want only victim.md", out)
	}
	if !storage.FileExists(filepath.Join(a.StorageDir, "md", "escape.md")) {
		t.Error("the target is not at md/escape.md")
	}
}
