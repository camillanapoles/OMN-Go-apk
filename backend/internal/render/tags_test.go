package render

import (
	"strings"
	"testing"
)

func TestTagSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hydroponics", "Hydroponics"},
		{"3D Print", "3D-Print"},
		{"QR code", "QR-code"},
		{"OMN documentation", "OMN-documentation"},
		{"R&D", "R-D"},
		{"  spaced  ", "spaced"},
		{"a--b__c", "a-b-c"},
		{"Гидропоника", "Гидропоника"}, // unicode letters kept
		{"", ""},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := TagSlug(c.in); got != c.want {
			t.Errorf("TagSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractTitleTags(t *testing.T) {
	title, tags := ExtractTitleTags("Title: My Page\nTags: Red, Blue ,, Green\n\nbody")
	if title != "My Page" {
		t.Errorf("title = %q, want %q", title, "My Page")
	}
	if strings.Join(tags, "|") != "Red|Blue|Green" {
		t.Errorf("tags = %v, want [Red Blue Green] (empties dropped, trimmed)", tags)
	}

	// No header -> empty title, nil tags.
	if ti, tg := ExtractTitleTags("no header here"); ti != "" || tg != nil {
		t.Errorf("no-header = (%q,%v), want (\"\",nil)", ti, tg)
	}

	// Header without Title/Tags.
	if ti, tg := ExtractTitleTags("Date: 2026-01-01\n\nbody"); ti != "" || len(tg) != 0 {
		t.Errorf("no title/tags = (%q,%v), want empty", ti, tg)
	}
}
