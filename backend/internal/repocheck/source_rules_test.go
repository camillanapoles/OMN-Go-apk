package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Each test of this file reads each production Go file below backend/. It
// holds a rule that the compiler cannot hold, for example one authority for
// a job, or a call that a 32-bit build cannot make.

// render.WriteHTMLHeader is the one place that names the content type of a
// page. A handler that writes the header by hand can lose the charset.
//
// This test scans the source, the same as TestNoDirectLogPrintf.
func TestNoBareHTMLContentType(t *testing.T) {
	// A test file ships to no device, and this file names the banned text
	// as a string. productionGoFiles skips each test file.
	for _, name := range productionGoFiles(t) {
		// internal/render/pages.go declares the value, thus it holds the text
		// once.
		if name == "internal/render/pages.go" {
			continue
		}
		src, err := readBackendFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `Set("Content-Type", "text/html`) {
				continue
			}
			t.Errorf("%s:%d writes the HTML content type by hand. Call "+
				"render.WriteHTMLHeader(w) instead. A header with no charset lets the "+
				"browser guess the encoding of a page that the server renders.",
				name, i+1)
		}
	}
}

// writeJSON is the one JSON writer. A second encoder on a ResponseWriter
// fails this test.
func TestOnlyWriteJSONEncodesAnAnswer(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		if f == "internal/render/json_response.go" {
			continue
		}
		src, err := readBackendFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "json.NewEncoder(w)") {
			t.Errorf("%s encodes an answer. Call a.writeJSON.", f)
		}
	}
}

// logPrintfAllowed names the only two files that may call log.Printf. No *App
// can reach either call site. render.LoadTemplate in
// internal/render/templates.go runs at package init. logAnchorsOff and
// addBookmarks in internal/search/sections.go run from a package-level function
// inside a sync.Once, and from a method on searchDocument, which has no
// application.
//
// Each of those lines is a fault, and a fault always prints, so the missing
// level costs the reader nothing. They write "(error)" in the text by hand,
// which the second half of this test checks.
var logPrintfAllowed = map[string]bool{
	"internal/render/templates.go": true,
	"internal/search/sections.go":  true,
}

// handWrittenLevelRe matches the shape those two files must produce:
// a bracketed tag, then "(error)", then the message.
var handWrittenLevelRe = regexp.MustCompile(`^log\.Printf\("\[[a-z0-9-]+\] \(error\) `)

// TestNoDirectLogPrintf exists because a log.Printf line reaches stdout and
// the browser with no tag and no level. The Config page can then never
// switch it off, and the person who asked for less noise still gets it.
//
// The scan reads each production file below backend/, thus a package of the
// split cannot hide a call.
func TestNoDirectLogPrintf(t *testing.T) {
	// A test file does not ship to a device, and productionGoFiles skips
	// it. This file names the banned call as a string.
	for _, name := range productionGoFiles(t) {
		src, err := readBackendFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "log.Printf(") {
				continue
			}
			if !logPrintfAllowed[name] {
				t.Errorf("%s:%d calls log.Printf. Use a.log(tag).Debugf, Infof "+
					"or Errf with a tag from internal/logx/levels.go. A line with no "+
					"level cannot be filtered, and the reader has no way to "+
					"switch it off.", name, i+1)
				continue
			}
			if !handWrittenLevelRe.MatchString(trimmed) {
				t.Errorf("%s:%d is an allowed log.Printf, but its text does not "+
					"start with \"[tag] (error) \". The browser reads that shape "+
					"to decide what to print.", name, i+1)
			}
		}
	}
}

// A 64-bit atomic needs an 8-byte-aligned address. A 32-bit build aligns a
// struct to 4. The rule that usually saves you is this: "the first word in
// an allocated struct can be relied upon to be 64-bit aligned". It covers
// the FIRST word alone. App.ActiveConns sits after Config and a RWMutex, on
// GOARCH=386 and armeabi-v7a puts it at an offset of 164: a multiple of 4
// and not of 8. connectionMiddleware wraps every route, so the first request
// on such a build panicked with "unaligned 64-bit atomic operation" and the
// browser saw an empty response. arm64 aligns to 8 naturally and hid it, so
// the whole 32-bit half of the ABI split was broken and nothing said so.
//
// atomic.Int64 carries its own alignment guarantee. This test keeps it that
// way. It also stops a person who introduces the pattern again somewhere
// else. The architecture that catches the fault is not the one that this
// test runs on.
func TestNoBare64BitAtomics(t *testing.T) {
	banned := []string{
		"atomic.AddInt64(", "atomic.LoadInt64(", "atomic.StoreInt64(",
		"atomic.SwapInt64(", "atomic.CompareAndSwapInt64(",
		"atomic.AddUint64(", "atomic.LoadUint64(", "atomic.StoreUint64(",
		"atomic.SwapUint64(", "atomic.CompareAndSwapUint64(",
	}
	// Test files are not shipped to a device, and this one names every
	// banned call as a string. productionGoFiles skips each test file.
	for _, name := range productionGoFiles(t) {
		src, err := readBackendFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range banned {
			if strings.Contains(string(src), call) {
				t.Errorf("%s uses %s. On a 32-bit build that panics unless the "+
					"address happens to be 8-byte aligned, which a struct field "+
					"cannot promise. Use the atomic.Int64 / atomic.Uint64 types, "+
					"which align themselves.", name, call)
			}
		}
	}
}

// The F-Droid build fetches each vendor asset with this script, and it
// writes the files by path. A script that writes to the old directory
// gives an APK with no KaTeX and no icon font.
//
// The Docker build does not run the script. A local build would thus look
// correct and hide the fault until the release.
func TestFdroidFetchScriptWritesUnderOMNGo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "android", "fdroid_fetch_assets.sh"))
	if err != nil {
		t.Skipf("the script is not in this tree: %v", err)
	}
	script := string(raw)

	for _, want := range []string{
		`JS_DIR="$REPO_ROOT/backend/frontend/html/js/OMN-Go"`,
		`CSS_DIR="$REPO_ROOT/backend/frontend/html/css/OMN-Go"`,
		`FONT_DIR="$REPO_ROOT/backend/frontend/html/css/OMN-Go/fonts"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the F-Droid script does not hold %s", want)
		}
	}
	if strings.Contains(script, "github-markdown") {
		t.Error("the F-Droid script still fetches markdown.css, which this build dropped")
	}
}

// Layout is the one place that joins a name to StorageDir. A second join
// fails this test.
func TestOnlyLayoutJoinsStoragePaths(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		src, err := readBackendFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "filepath.Join(a.StorageDir") {
			t.Errorf("%s joins a path to StorageDir. Call a method of a.layout().", f)
		}
	}
}

// RelInside is the one containment test. A second copy of the ".." test
// fails this test.
func TestOnlyRelInsideTestsContainment(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		if f == "internal/storage/layout.go" {
			continue
		}
		src, err := readBackendFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `".."+string(filepath.Separator)`) {
			t.Errorf("%s tests containment itself. Call storage.RelInside.", f)
		}
	}
}
