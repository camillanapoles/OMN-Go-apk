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

// ----------------------------------------------------------------------
// A control names its work in data-action
// ----------------------------------------------------------------------
//
// A control of a template has no inline handler such as onclick. It holds
// data-action="name", and OMN.action("name", fn) in a script gives the name
// its function. One click listener of omn-go-core.js joins the two.
//
// A name with no function is a dead button. Nothing throws, and the only
// report is one console warning at the click. The first test below finds
// such a name in the source.

// inlineHandlerRe finds an inline event handler in markup: a space, then
// "on", a name and "=". The space keeps "content=" out.
var inlineHandlerRe = regexp.MustCompile(`\son[a-z]+\s*=\s*["']`)

// dataActionRe finds the name of a data-action attribute.
var dataActionRe = regexp.MustCompile(`data-action="([^"]+)"`)

// actionCallRe finds the name in a call of OMN.action or of its local name
// action.
var actionCallRe = regexp.MustCompile(`\baction\('([a-z0-9-]+)'`)

// templatesWithInlineHandlers lists the templates that still hold an inline
// handler, with the count of each. THE LIST ONLY SHRINKS. Change a template
// to data-action, and then remove its row.
var templatesWithInlineHandlers = map[string]int{
	"templates/config_page.html":   10,
	"templates/db_backups.html":    4,
	"templates/external_edit.html": 1,
}

// Each data-action of a template has a function in a shipped script.
func TestEachDataActionHasAFunction(t *testing.T) {
	registered := map[string]bool{}
	scripts, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scripts {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "omn-go-") {
			continue
		}
		src, rerr := frontend.Static.ReadFile("html/js/OMN-Go/" + e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range actionCallRe.FindAllStringSubmatch(string(src), -1) {
			registered[m[1]] = true
		}
	}
	if len(registered) == 0 {
		t.Fatal("the scan found no OMN.action call, thus this test proves nothing")
	}

	used := map[string]bool{}
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range templates {
		src, rerr := frontend.Templates.ReadFile("templates/" + e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range dataActionRe.FindAllStringSubmatch(string(src), -1) {
			used[m[1]] = true
			if !registered[m[1]] {
				t.Errorf("templates/%s has data-action=%q, and no script calls "+
					"OMN.action('%s', ...). The control does nothing.", e.Name(), m[1], m[1])
			}
		}
	}
	if len(used) == 0 {
		t.Fatal("the scan found no data-action in the templates, thus this test proves nothing")
	}
	for name := range registered {
		if !used[name] {
			t.Errorf("a script calls OMN.action('%s', ...), and no template has a "+
				"control with that data-action. Remove the action.", name)
		}
	}
}

// A template holds no inline handler, except the templates of
// templatesWithInlineHandlers.
func TestTemplatesHoldNoInlineHandler(t *testing.T) {
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range templates {
		name := "templates/" + e.Name()
		src, rerr := frontend.Templates.ReadFile(name)
		if rerr != nil {
			t.Fatal(rerr)
		}
		got := len(inlineHandlerRe.FindAllString(string(src), -1))
		want, listed := templatesWithInlineHandlers[name]
		seen[name] = true
		switch {
		case !listed && got > 0:
			t.Errorf("%s holds %d inline handlers. Give each control a "+
				"data-action, and call OMN.action in a script.", name, got)
		case listed && got > want:
			t.Errorf("%s holds %d inline handlers, and the list allows %d. "+
				"Give the new control a data-action.", name, got, want)
		case listed && got < want:
			t.Errorf("%s holds %d inline handlers, and the list says %d. "+
				"Lower the number of templatesWithInlineHandlers, or remove the row at 0.",
				name, got, want)
		}
	}
	for name := range templatesWithInlineHandlers {
		if !seen[name] {
			t.Errorf("templatesWithInlineHandlers names %s, and no such template exists", name)
		}
	}
}
