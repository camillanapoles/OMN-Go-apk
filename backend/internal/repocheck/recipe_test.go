package repocheck

import (
	"os"
	"path"
	"strings"
	"testing"
)

// Each feature package has two parts of the recipe in section 3 of
// CLAUDE.md. internal/app/<name>_app.go holds the App side. The package holds
// a testApp in its own harness_test.go. A feature is a package of layer 4 or
// 5. testkit is in layer 4 and holds only test helpers, thus it is no
// feature.
func TestEachFeatureHasItsParts(t *testing.T) {
	const prefix = "net.basov.omngo/backend/internal/"
	features := 0
	for pkg, layer := range importLayers {
		name := strings.TrimPrefix(pkg, prefix)
		if layer < 4 || layer > 5 || name == pkg || name == "testkit" {
			continue
		}
		features++
		for _, rel := range []string{
			path.Join("internal/app", name+"_app.go"),
			path.Join("internal", name, "harness_test.go"),
		} {
			if _, err := os.Stat(backendPath(rel)); err != nil {
				t.Errorf("the feature %s has no %s. See the recipe in section 3 of CLAUDE.md.", name, rel)
			}
		}
	}
	if features < 6 {
		t.Errorf("the scan found %d features. The repository holds more.", features)
	}
}
