package logx

import (
	"os"
	"regexp"
	"testing"
)

// The Config page makes its checkboxes from AllTags. A tag that is not in
// AllTags cannot be switched off, and config.NormalizeLogTags of package
// backend drops it from config.json at the next save.
func TestAllTagsIsComplete(t *testing.T) {
	src, err := os.ReadFile("levels.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?m)^\t[A-Z][A-Za-z0-9]*\s+Tag = "([a-z0-9-]+)"`).
		FindAllStringSubmatch(string(src), -1)
	if len(declared) == 0 {
		t.Fatal("levels.go has no Tag constant. The shape of the constant block changed.")
	}
	listed := map[Tag]bool{}
	for _, tag := range AllTags {
		listed[tag] = true
	}
	for _, m := range declared {
		if !listed[Tag(m[1])] {
			t.Errorf("the tag %q has a constant and no row in AllTags. Add a new tag to both.", m[1])
		}
	}
	if len(declared) != len(AllTags) {
		t.Errorf("levels.go has %d tag constants and %d rows in AllTags", len(declared), len(AllTags))
	}
}
