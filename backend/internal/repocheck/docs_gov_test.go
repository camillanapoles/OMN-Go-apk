package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fork build has one Dockerfile. The old CI name died when the original
// was merged in, and the fork docs kept it for a while. Each command that a
// reader copied from those docs failed. The docs now say Dockerfile. This
// test keeps the dead name out of the fork docs. The upstream doc/ tree
// keeps its history and stays out of the scan.
func TestNoDeadDockerfileName(t *testing.T) {
	const dead = "Dockerfile.ci"
	err := filepath.WalkDir(filepath.Join(repoRoot, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), dead) {
			rel, relErr := filepath.Rel(repoRoot, p)
			if relErr != nil {
				rel = p
			}
			t.Errorf("%s names the dead %s. The build has one Dockerfile.", filepath.ToSlash(rel), dead)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
