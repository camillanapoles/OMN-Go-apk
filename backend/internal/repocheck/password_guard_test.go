package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The default admin password is a public constant of the fork. It lives in
// config.go alone, and each other file reaches it through the exported
// constant of the config package. A second literal would be a second
// authority on the credential. See doc/decisions/0034.
func TestTheDefaultPasswordLiteralLivesInConfigAlone(t *testing.T) {
	literal := "admin_secret" + "_changeme"
	allowed := filepath.Join(backendDir, "internal", "config", "config.go")
	err := filepath.WalkDir(backendDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || p == allowed {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), literal) {
			t.Errorf("%s holds the literal of the default admin password. Use config.DefaultAdminPassword.", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
