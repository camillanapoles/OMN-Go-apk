// Package repocheck holds only tests. Each test reads the files of the
// repository, for example the Go source, the scripts, the Android files, the
// build files and the documents. It checks a rule that spans more than one
// package or more than one language. It does not import package app, thus a
// rule needs no App and no test harness.
package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tests of this package run in backend/internal/repocheck. repoRoot is
// the root of the repository, and backendDir is its backend/ directory.
const (
	repoRoot   = "../../.."
	backendDir = "../.."
)

// productionGoFiles answers each Go file below backend/ that is not a test,
// as a slash path from backend/. A source scan uses it, thus a package of the
// split cannot hide a file from the scan. readBackendFile reads one of them.
func productionGoFiles(t *testing.T) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(backendDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(backendDir, p)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "internal/render/templates.go") {
		t.Fatal("the scan did not find internal/render/templates.go. The scan is broken.")
	}
	return names
}

// backendPath answers the path of rel, a slash path from backend/.
func backendPath(rel string) string {
	return filepath.Join(backendDir, filepath.FromSlash(rel))
}

// readBackendFile reads the file rel, a slash path from backend/.
func readBackendFile(rel string) ([]byte, error) {
	return os.ReadFile(backendPath(rel))
}

// readRepoFile reads the file rel, a slash path from the root of the
// repository.
func readRepoFile(rel string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	return string(raw), err
}
