package app

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/render"
)

func TestRenderConfigPage(t *testing.T) {
	v := configPageView{
		ServerPort:    8080,
		Author:        "A & B",
		UseInternalEd: true,
		DesktopExtCmd: "subl",
		GitServers: []gitServerView{
			{Index: 0, Slot: 1, Active: true, Name: `srv "one"`, URL: "git@host:repo.git"},
			{Index: 1, Slot: 2, Active: false, Name: "srv two"},
		},
	}
	out := renderConfigPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	// Stored-XSS check: hostile values must arrive escaped. The view
	// holds no password, thus the git server name carries this check. See
	// TestConfigPageCarriesNoSecret.
	if !strings.Contains(out, "srv &quot;one&quot;") {
		t.Error("git server name not HTML-escaped")
	}
	if !strings.Contains(out, `value="A &amp; B"`) {
		t.Error("author not HTML-escaped")
	}
	// Exactly one card is the active radio.
	if strings.Count(out, `value="0" checked`) != 1 {
		t.Error("active git server slot 0 not marked checked exactly once")
	}
	if strings.Contains(out, `value="1" checked`) {
		t.Error("inactive slot wrongly marked checked")
	}
	// Both cards rendered, indices intact.
	for _, want := range []string{"git_name_0", "git_name_1", "Slot 1", "Slot 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in config page output", want)
		}
	}
	// Internal editor checkbox honored.
	if !strings.Contains(out, `name="use_internal_editor" value="true" checked`) {
		t.Error("use_internal_editor checkbox not checked")
	}
}

func TestRenderConfigPageAndroidToggles(t *testing.T) {
	// Off (zero value): neither Android checkbox is checked, but both
	// placeholders are still filled (no leftover %%...%%).
	off := renderConfigPage(configPageView{})
	if strings.Contains(off, "%%INTENT_URI_CHECKED%%") || strings.Contains(off, "%%TERMUX_INTENT_CHECKED%%") {
		t.Fatalf("Android toggle placeholder left unfilled:\n%s", off)
	}
	if strings.Contains(off, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri wrongly checked when EnableIntentURI is false")
	}
	if strings.Contains(off, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent wrongly checked when EnableTermuxIntent is false")
	}

	// On: both checkboxes render checked.
	on := renderConfigPage(configPageView{EnableIntentURI: true, EnableTermuxIntent: true})
	if !strings.Contains(on, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri checkbox not checked when EnableIntentURI is true")
	}
	if !strings.Contains(on, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent checkbox not checked when EnableTermuxIntent is true")
	}
}

func TestInjectRuntimeVars(t *testing.T) {
	a := &App{}
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

	page := []byte("<head>" + render.RuntimeVarsMarker + "</head>")
	out := string(a.injectRuntimeVars(page))

	if strings.Contains(out, render.RuntimeVarsMarker) {
		t.Error("marker not replaced")
	}
	if !strings.Contains(out, `var APP_VERSION = "`+version+`";`) {
		t.Error("APP_VERSION not injected")
	}
	if !strings.Contains(out, "var USE_INTERNAL_ED = true;") {
		t.Error("USE_INTERNAL_ED not injected")
	}

	// A page without the marker passes through unchanged.
	plain := []byte("<head>no marker</head>")
	if got := string(a.injectRuntimeVars(plain)); got != string(plain) {
		t.Errorf("page without marker was modified: %q", got)
	}
}

// End-to-end guard. A page rendered through render.RenderIndexPage carries the
// marker, and injectRuntimeVars finds it. That is the exact pair that broke
// when the marker was an HTML comment.
func TestRenderedPageAcceptsRuntimeVars(t *testing.T) {
	a := &App{}
	out := a.injectRuntimeVars([]byte(render.RenderIndexPage(render.IndexPageView{Title: "T", PageName: "T"})))
	if !strings.Contains(string(out), "var APP_VERSION") {
		t.Error("rendered index page did not accept runtime vars injection")
	}
}

// selectBlock returns the markup of the <select name="..."> element in html,
// so a test can assert about one dropdown without being perturbed by any
// other. The page carries more than one select (theme, android_fullscreen),
// which is why "count the selected attributes in the whole page" is not a
// safe assertion.
func selectBlock(t *testing.T, html, name string) string {
	t.Helper()
	i := strings.Index(html, `name="`+name+`"`)
	if i == -1 {
		t.Fatalf("no <select name=%q> in rendered page", name)
	}
	j := strings.Index(html[i:], "</select>")
	if j == -1 {
		t.Fatalf("unterminated <select name=%q> in rendered page", name)
	}
	return html[i : i+j]
}

func TestRenderConfigPageThemeSelection(t *testing.T) {
	cases := []struct {
		theme        string
		wantSelected string
	}{
		{"dark", `value="dark" selected`},
		{"light", `value="light" selected`},
		{"auto", `value="auto" selected`},
		// pre-theme configs (empty) and garbage both normalize to auto
		{"", `value="auto" selected`},
		{"purple", `value="auto" selected`},
	}
	for _, tc := range cases {
		out := renderConfigPage(configPageView{Theme: tc.theme})
		if !strings.Contains(out, tc.wantSelected) {
			t.Errorf("theme=%q: expected %q in output", tc.theme, tc.wantSelected)
		}
		// Exactly one option may be selected - within the theme select.
		if n := strings.Count(selectBlock(t, out, "theme"), " selected"); n != 1 {
			t.Errorf("theme=%q: %d theme options selected, want exactly 1", tc.theme, n)
		}
		if strings.Contains(out, "%%") {
			t.Fatalf("theme=%q: unfilled placeholder left in output", tc.theme)
		}
	}
}

func TestRenderConfigPageFullscreenSelection(t *testing.T) {
	cases := []struct {
		mode         string
		wantSelected string
	}{
		{"off", `value="off" selected`},
		{"fullscreen", `value="fullscreen" selected`},
		{"immersive", `value="immersive" selected`},
		// A config.json written before android_fullscreen existed carries
		// "". It MUST land on fullscreen, and not on off. That is what
		// keeps an upgraded install looking the way it always has. Garbage
		// lands there too.
		{"", `value="fullscreen" selected`},
		{"sideways", `value="fullscreen" selected`},
	}
	for _, tc := range cases {
		out := renderConfigPage(configPageView{AndroidFullscreen: tc.mode})
		if !strings.Contains(out, tc.wantSelected) {
			t.Errorf("fullscreen=%q: expected %q in output", tc.mode, tc.wantSelected)
		}
		if n := strings.Count(selectBlock(t, out, "android_fullscreen"), " selected"); n != 1 {
			t.Errorf("fullscreen=%q: %d fullscreen options selected, want exactly 1", tc.mode, n)
		}
		if strings.Contains(out, "%%") {
			t.Fatalf("fullscreen=%q: unfilled placeholder left in output", tc.mode)
		}
	}
}

func TestNormalizeFullscreen(t *testing.T) {
	cases := map[string]string{
		"off":        config.FullscreenOff,
		"fullscreen": config.FullscreenOn,
		"immersive":  config.FullscreenImmersive,
		// Empty is the important one: it is what a config.json with no
		// android_fullscreen key gives.
		"":          config.FullscreenOn,
		"sideways":  config.FullscreenOn,
		"OFF":       config.FullscreenOn, // case-sensitive whitelist, as config.NormalizeTheme
		"Immersive": config.FullscreenOn,
	}
	for in, want := range cases {
		if got := config.NormalizeFullscreen(in); got != want {
			t.Errorf("config.NormalizeFullscreen(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInjectRuntimeVarsTheme(t *testing.T) {
	page := []byte("<head>" + render.RuntimeVarsMarker + "</head>")

	// Explicit theme delivered verbatim, and applied to <html> from the
	// injected head script (before first paint).
	a := &App{}
	a.config.Update(func(c *config.Config) { c.Theme = config.ThemeDark })
	out := string(a.injectRuntimeVars(page))
	if !strings.Contains(out, `var OMN_THEME = "dark";`) {
		t.Error("dark theme not injected")
	}
	if !strings.Contains(out, `document.documentElement.setAttribute('data-theme', OMN_THEME);`) {
		t.Error("data-theme application script missing")
	}

	// Unset / invalid themes normalize to auto at the injection point too
	// (belt and braces on top of loadConfig's normalization).
	for _, raw := range []string{"", "purple"} {
		b := &App{}
		b.config.Update(func(c *config.Config) { c.Theme = raw })
		got := string(b.injectRuntimeVars(page))
		if !strings.Contains(got, `var OMN_THEME = "auto";`) {
			t.Errorf("theme=%q: expected auto in injection, got:\n%s", raw, got)
		}
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"auto":   config.ThemeAuto,
		"light":  config.ThemeLight,
		"dark":   config.ThemeDark,
		"":       config.ThemeAuto,
		"purple": config.ThemeAuto,
		"DARK":   config.ThemeAuto, // case-sensitive whitelist by design
	}
	for in, want := range cases {
		if got := config.NormalizeTheme(in); got != want {
			t.Errorf("config.NormalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------
// The compat script
//
// omn-go-compat.js tells a person with an old WebView why the page is
// blank. Every other script uses async/await and arrow functions, and a
// parser that cannot read those drops the WHOLE file. A notice written in
// the style of its neighbors would be the one thing that does not run when
// it is needed.
//
// The notice is a file of its own, and not an inline block of index.html.
// An inline copy would go into the compiled page of every note. A <script
// src> element is its own parse unit, thus a SyntaxError in omn-go-core.js
// cannot stop it. That holds while two rules
// hold, and this test is the whole guarantee of both:
//
//   - the file itself is ES5, thus the old parser accepts it.
//   - index.html loads it FIRST, thus nothing throws before it runs.
//
// See the banner of omn-go-compat.js for the version number and where that
// number comes from.
// ---------------------------------------------------------------------

// compatCommentRe removes a block comment and a line comment. The prose of
// the banner names "async/await" and "arrow functions". It thus cannot look
// like code to the scan below.
var compatCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)

// compatBannedES6 are tokens that an ES5 parser rejects. Each one is
// written so that it cannot match ordinary prose.
var compatBannedES6 = []string{"=>", "`", "const ", "let ", "async ", "await ", "class ", "?.", "??", "..."}

func TestCompatScriptIsFirstAndES5(t *testing.T) {
	// 1. index.html loads it before every other script, and before the
	//    stylesheet links, which would delay it for no reason.
	scripts := regexp.MustCompile(`<script[^>]*>`).FindAllString(render.IndexPageTmpl, -1)
	if len(scripts) == 0 {
		t.Fatal("index.html loads no script at all")
	}
	if !strings.Contains(scripts[0], "js/OMN-Go/omn-go-compat.js") {
		t.Errorf("the first script of index.html is %q, want omn-go-compat.js. "+
			"A script above it that a WebView cannot parse throws before the "+
			"notice runs, and the reader sees a blank page with no reason.",
			scripts[0])
	}
	if strings.Contains(scripts[0], "defer") || strings.Contains(scripts[0], " async") {
		t.Errorf("the compat script carries defer or async: %q. Either one "+
			"delays it past the modern scripts, which is the order this test "+
			"exists to protect.", scripts[0])
	}
	at := strings.Index(render.IndexPageTmpl, "omn-go-compat.js")
	if css := strings.Index(render.IndexPageTmpl, `<link rel="stylesheet"`); css >= 0 && at > css {
		t.Error("the compat script is after the stylesheet link, which delays it for no reason")
	}

	raw, err := frontend.Static.ReadFile("html/js/OMN-Go/omn-go-compat.js")
	if err != nil {
		t.Fatalf("omn-go-compat.js is not embedded: %v", err)
	}
	code := compatCommentRe.ReplaceAllString(string(raw), "")

	// 2. The file parses on the oldest WebView this build supports.
	for _, es6 := range compatBannedES6 {
		if strings.Contains(code, es6) {
			t.Errorf("omn-go-compat.js uses %q, which an old WebView cannot parse - "+
				"it must stay ES5, or it is the one script that fails when it is needed", es6)
		}
	}

	// 3. The number it reports has to be a number, and one this application
	//    can justify: 85 is String.replaceAll, the highest requirement the
	//    frontend really has.
	if !strings.Contains(code, "var MIN = 85;") {
		t.Error("the notice no longer names 85 as the minimum; if that changed on purpose, " +
			"change it here too and say why in the banner")
	}

	// 4. Every byte stays ASCII. The server sends this file as
	//    application/javascript with no charset, thus a literal multi-byte
	//    character can arrive misdecoded.
	for i, c := range raw {
		if c > 127 {
			t.Errorf("omn-go-compat.js byte %d is not ASCII. Write a \\uXXXX "+
				"escape instead of the character.", i)
			break
		}
	}
}

// TestCompiledPageShellStaysSmall exists because the shell of index.html is
// copied into html/<name>.html for EVERY note. A new inline block here
// costs the same bytes again, on disk and in every
// git sync, multiplied by the note count.
func TestCompiledPageShellStaysSmall(t *testing.T) {
	const maxShellBytes = 5000
	if n := len(render.IndexPageTmpl); n > maxShellBytes {
		t.Errorf("index.html is %d bytes, over the %d-byte guard. Put the new "+
			"code in an asset under frontend/html/ and load it with a src, or "+
			"raise this number on purpose.", n, maxShellBytes)
	}
}

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
