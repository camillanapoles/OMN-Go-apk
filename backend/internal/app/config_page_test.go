package app

import (
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
)

// configPageOf renders the Config page of c and of the git server cards, the
// same way as getConfigPageBody.
func configPageOf(c config.Config, servers ...gitServerView) string {
	return renderConfigPage(configPageView{Values: config.PageValues(c), GitServers: servers})
}

func TestRenderConfigPage(t *testing.T) {
	c := config.Config{
		ServerPort:    8080,
		Author:        "A & B",
		UseInternalEd: true,
		DesktopExtCmd: "subl",
	}
	out := configPageOf(c,
		gitServerView{Index: 0, Slot: 1, Active: true, Name: `srv "one"`, URL: "git@host:repo.git"},
		gitServerView{Index: 1, Slot: 2, Active: false, Name: "srv two"},
	)

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
	off := configPageOf(config.Config{})
	if strings.Contains(off, "%%") {
		t.Fatalf("Android toggle placeholder left unfilled:\n%s", off)
	}
	if strings.Contains(off, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri wrongly checked when EnableIntentURI is false")
	}
	if strings.Contains(off, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent wrongly checked when EnableTermuxIntent is false")
	}

	// On: both checkboxes render checked.
	on := configPageOf(config.Config{EnableIntentURI: true, EnableTermuxIntent: true})
	if !strings.Contains(on, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri checkbox not checked when EnableIntentURI is true")
	}
	if !strings.Contains(on, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent checkbox not checked when EnableTermuxIntent is true")
	}
}

// selectBlock answers the markup of the <select name="..."> element in html.
// A test can then check one dropdown, and no other dropdown changes the
// result. The page holds more than one select (theme, android_fullscreen),
// thus a count of the selected attributes of the whole page proves nothing.
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
		out := configPageOf(config.Config{Theme: tc.theme})
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
		out := configPageOf(config.Config{AndroidFullscreen: tc.mode})
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

// Each value of the table has its place in config_page.html. A new row of the
// table thus fails here until the page shows it. The log tag boxes come from
// renderLogTagBoxes and not from the template.
func TestEachTableValueHasAPlaceOnTheConfigPage(t *testing.T) {
	for _, pv := range config.PageValues(config.Config{}) {
		if strings.HasPrefix(pv.Name, config.PlaceholderName("log_tags_")) {
			continue
		}
		if !strings.Contains(configPageTmpl, "%%"+pv.Name+"%%") {
			t.Errorf("config_page.html has no %%%%%s%%%%", pv.Name)
		}
	}
}
