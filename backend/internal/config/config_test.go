package config

import "testing"

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
