package repocheck

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
)

// ---------------------------------------------------------------------
// The clipboard and the documented API
// ---------------------------------------------------------------------

// TestClipboardHasOneAuthority exists because a second clipboard path
// gives a second chance to get the Android WebView wrong. Two paths can
// then behave differently: one copy works on Android 6 and the other does
// not.
//
// Only omn-go-core.js can call execCommand('copy'). That file holds
// omnGoCopyText, which each other caller uses. It also holds
// copyQuickNote, which stays direct. The text of copyQuickNote is already
// in a textarea, and the focus must stay there for the typing that
// follows.
func TestClipboardHasOneAuthority(t *testing.T) {
	const authority = "html/js/OMN-Go/omn-go-core.js"
	for _, tree := range []struct {
		name string
		fs   fs.FS
		root string
	}{
		{"frontend.Static", frontend.Static, "html"},
		{"frontend.Templates", frontend.Templates, "templates"},
	} {
		err := fs.WalkDir(tree.fs, tree.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(p, ".js") && !strings.HasSuffix(p, ".html") {
				return nil
			}
			src, rerr := fs.ReadFile(tree.fs, p)
			if rerr != nil {
				return rerr
			}
			if !strings.Contains(string(src), `execCommand('copy')`) {
				return nil
			}
			if p != authority {
				t.Errorf("%s calls execCommand('copy'). Call "+
					"window.omnGoCopyText in its place. That function is the "+
					"only clipboard writer. It holds what this project knows "+
					"about the Android WebView.", p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree.name, err)
		}
	}
}

// documentedCoreAPI names each frontend function that the "Useful
// functions" section of the User Manual gives to a note script. A
// documented name is a promise. Each name thus has an explicit window
// export, and not a bare declaration that becomes global by accident.
var documentedCoreAPI = []string{
	"OMNProgress",
	"omnClearHighlights",
	"omnGoCopyText",
	"omnGoOnServerLog",
	"omnGoOpenDatabase",
	"omnGoPageLink",
	"omnGoPageTitle",
	"omnGoRenderMath",
	"omnHighlightTerms",
	"omnSearchOpen",
}

// TestDocumentedCoreAPIIsExported exists because the manual sends a reader
// to these names. A rename takes a documented name away, and an IIFE
// around one file does the same. There is no other sign of the loss. The
// note that used the name then fails in the browser of the reader, and
// nowhere else.
func TestDocumentedCoreAPIIsExported(t *testing.T) {
	// It reads EVERY shipped script and not a named list. A script can
	// split into several files, and a fixed list would have to grow with
	// each such move. A name that no file exports is the fault
	// this test looks for, and the file that holds it does not matter.
	entries, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	var read int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		src, rErr := frontend.Static.ReadFile("html/js/OMN-Go/" + e.Name())
		if rErr != nil {
			t.Fatalf("%s is not embedded: %v", e.Name(), rErr)
		}
		all.Write(src)
		read++
	}
	if read == 0 {
		t.Fatal("no script was read, thus this test proves nothing")
	}
	src := all.String()
	for _, name := range documentedCoreAPI {
		if !strings.Contains(src, "window."+name+" =") {
			t.Errorf("the User Manual documents window.%s. No frontend file "+
				"exports that name. Keep the name, or change the manual in "+
				"the same commit.", name)
		}
	}
}

// ----------------------------------------------------------------------
// A modal that hides by class must have a rule that shows it again
// ----------------------------------------------------------------------
//
// modals.html holds four overlays. Two of them start with the .hidden
// class, and JS shows each one with classList.remove('hidden'). The other
// two carry no class, and JS shows each one with style.display.
//
// .overlay in omn-go-core.css sets display:none. A modal of the first kind
// is thus invisible until a rule gives it a display value again.
//
// A modal with no such rule never appears. runSync finds the element, thus
// the confirm() fallback beside it does not run either. A rejected push
// then gives the reader nothing, and the only report is one line in the
// log.
//
// This test reads the two files. It is a source test, because a browser is
// what applies a CSS rule, and the test suite holds no CSS engine.
func TestAnOverlayHiddenByClassCanBeShownAgain(t *testing.T) {
	markup, err := frontend.Templates.ReadFile("templates/modals.html")
	if err != nil {
		t.Fatalf("modals.html is not embedded: %v", err)
	}
	css, err := frontend.Static.ReadFile("html/css/OMN-Go/omn-go-core.css")
	if err != nil {
		t.Fatalf("omn-go-core.css is not embedded: %v", err)
	}

	divRe := regexp.MustCompile(`<div\s+id="([^"]+)"\s+class="([^"]*)"`)
	checked := 0
	for _, m := range divRe.FindAllStringSubmatch(string(markup), -1) {
		overlay, hidden := false, false
		for _, c := range strings.Fields(m[2]) {
			switch c {
			case "overlay":
				overlay = true
			case "hidden":
				hidden = true
			}
		}
		if !overlay || !hidden {
			continue
		}
		checked++
		rule := "#" + m[1] + ":not(.hidden)"
		if !strings.Contains(string(css), rule) {
			t.Errorf("%s starts hidden by class, and omn-go-core.css holds no "+
				"%s rule. The overlay stays at display:none, thus the modal "+
				"never appears.", m[1], rule)
		}
	}
	if checked < 2 {
		t.Fatalf("the scan found %d overlay that hides by class, and the file "+
			"holds two. The pattern no longer matches the markup.", checked)
	}
}
