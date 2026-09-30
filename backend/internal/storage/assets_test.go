package storage

import (
	"testing"

	"net.basov.omngo/backend/frontend"
)

// Each version-dependent file must be in the embed. When a file is not there,
// RefreshEmbeddedAssets only logs "not embedded", and the file reaches no
// install. A new note, for example md/AndroidIntents.md, had that fault: the
// list named it, and the build did not ship it. This test reads the whole
// list, thus the test run finds the fault before a release.
func TestVersionDependentAssetsAllEmbedded(t *testing.T) {
	for _, rel := range VersionDependentAssets {
		if _, err := frontend.Static.ReadFile(rel); err != nil {
			t.Errorf("version-dependent asset %q is not embedded in frontend.Static: %v", rel, err)
		}
	}
}
