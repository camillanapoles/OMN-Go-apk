package config

import (
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/logx"
)

func TestNormalizeFullscreen(t *testing.T) {
	cases := map[string]string{
		"off":        FullscreenOff,
		"fullscreen": FullscreenOn,
		"immersive":  FullscreenImmersive,
		// Empty is the important one: it is what a config.json with no
		// android_fullscreen key gives.
		"":          FullscreenOn,
		"sideways":  FullscreenOn,
		"OFF":       FullscreenOn, // case-sensitive whitelist, as NormalizeTheme
		"Immersive": FullscreenOn,
	}
	for in, want := range cases {
		if got := NormalizeFullscreen(in); got != want {
			t.Errorf("NormalizeFullscreen(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"auto":   ThemeAuto,
		"light":  ThemeLight,
		"dark":   ThemeDark,
		"":       ThemeAuto,
		"purple": ThemeAuto,
		"DARK":   ThemeAuto, // case-sensitive whitelist by design
	}
	for in, want := range cases {
		if got := NormalizeTheme(in); got != want {
			t.Errorf("NormalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormalizeLogTags pins the nil rule. An install that upgrades to this
// version has no log_tags key in config.json, and it must get every tag. A
// person who unticks every box gets an empty list, which is a different
// thing and must survive a save.
func TestNormalizeLogTags(t *testing.T) {
	if got := NormalizeLogTags(nil); len(got) != len(logx.AllTags) {
		t.Errorf("nil gave %d tags, want every one of the %d", len(got), len(logx.AllTags))
	}
	if got := NormalizeLogTags([]string{}); len(got) != 0 {
		t.Errorf("an empty list gave %v, want an empty list - unticking every box "+
			"is not the same as an upgrade with no key", got)
	}
	got := NormalizeLogTags([]string{"SYNC", " sync ", "not-a-tag", "assets"})
	want := []string{"assets", "sync"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("NormalizeLogTags gave %v, want %v - it lowercases, trims, "+
			"drops an unknown tag, and keeps the order of logx.AllTags", got, want)
	}
}

func TestNormalizeSearchKinds(t *testing.T) {
	// Absent (nil) and empty are NOT the same thing. The difference is what
	// stands between "this config predates the feature" and "the user
	// unticked everything on purpose".
	if got := NormalizeSearchKinds(nil); strings.Join(got, ",") != "md,bookmarks" {
		t.Errorf("nil -> %v, want the default md,bookmarks", got)
	}
	if got := NormalizeSearchKinds([]string{}); len(got) != 0 {
		t.Errorf("explicitly empty -> %v, want it to stay empty", got)
	}

	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"md"}, "md"},
		{[]string{"MD", " js "}, "md,js"},                           // folded and trimmed
		{[]string{"md", "md", "js"}, "md,js"},                       // de-duplicated
		{[]string{"md", "nonsense", "js"}, "md,js"},                 // unknown dropped
		{[]string{"nonsense"}, ""},                                  // ... even to nothing
		{[]string{"user_json", "bookmarks"}, "user_json,bookmarks"}, // order kept
	}
	for _, c := range cases {
		if got := strings.Join(NormalizeSearchKinds(c.in), ","); got != c.want {
			t.Errorf("NormalizeSearchKinds(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSearchScope(t *testing.T) {
	for in, want := range map[string]string{
		"page": SearchScopePage,
		"PAGE": SearchScopePage,
		"all":  SearchScopeAll,
		"":     SearchScopeAll, // every config written before this field
		"junk": SearchScopeAll,
	} {
		if got := NormalizeSearchScope(in); got != want {
			t.Errorf("NormalizeSearchScope(%q) = %q, want %q", in, got, want)
		}
	}
}
