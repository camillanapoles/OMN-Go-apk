package render

import "testing"

func TestRelPrefix(t *testing.T) {
	cases := map[string]string{
		"Welcome":                     "",
		"QuickNotes":                  "",
		"local/Note":                  "../",
		"AI/GeminiSvgComponentEditor": "../",
		"a/b/c":                       "../../",
	}
	for name, want := range cases {
		if got := RelPrefix(name); got != want {
			t.Errorf("RelPrefix(%q) = %q, want %q", name, got, want)
		}
	}
}
