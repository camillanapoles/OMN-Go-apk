package repocheck

// ----------------------------------------------------------------------
// The endpoint reference, against the tree
// ----------------------------------------------------------------------
//
// doc/API.md names a function and the file that holds it in nine places.
// It writes the claim as a fixed shape:
//
//	`config-reveal` in `omn-go-config.js`
//
// A claim of that shape goes stale each time a function moves. A person
// moves code, runs the gate, and the gate says nothing about a document.
// This test makes the gate say it.
//
// WHAT THIS TEST PROVES, AND WHAT IT DOES NOT. It proves that the named
// file HOLDS the name. It does not prove that the file DEFINES it, and a
// caller counts the same as a definition. That is a weak rule, and it
// still catches each fault above. A file that never mentions a name is
// the shape that a move leaves behind.
//
// A stronger rule needs a parser for two languages. The weak rule costs
// one regular expression and it runs in a millisecond.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The claim shape of doc/API.md. The document wraps its lines, thus the
// scan collapses each run of whitespace before it matches.
var apiDocClaimRe = regexp.MustCompile(
	"`([A-Za-z_][A-Za-z0-9_]*)\\(?\\)?` in `([A-Za-z0-9_.\\-/]+\\.(?:js|go))`")

var apiDocSpaceRunRe = regexp.MustCompile(`\s+`)

// apiDocResolve answers the path of a file that a claim names.
//
// The document writes a Go file as a repository path or as a bare name.
// It writes a script as a bare name. Each form resolves here, and an
// unknown name gives the empty string.
func apiDocResolve(name string) string {
	for _, candidate := range []string{
		name,
		filepath.Join("backend", name),
		filepath.Join("backend", "frontend", "html", "js", "OMN-Go", name),
		filepath.Join("backend", "frontend", "html", "js", name),
	} {
		p := filepath.Join(repoRoot, filepath.FromSlash(candidate))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// Each function that doc/API.md places in a file must be in that file.
//
// A failure means one of two things. Somebody moved the function and the
// reference stayed behind, thus repair doc/API.md. Or somebody renamed
// the function, thus repair both.
func TestApiDocNamesTheRightFile(t *testing.T) {
	raw, err := readRepoFile("doc/API.md")
	if err != nil {
		t.Skipf("doc/API.md is not in this tree: %v", err)
	}
	flat := apiDocSpaceRunRe.ReplaceAllString(raw, " ")

	claims := apiDocClaimRe.FindAllStringSubmatch(flat, -1)
	// The count is a floor and not a golden value. A new claim is
	// welcome, and a claim that vanishes without a word is not.
	if len(claims) < 9 {
		t.Fatalf("doc/API.md holds %d claims of the shape \"`name` in `file`\". "+
			"It held 9 at 26.09.35. A claim that vanished needs a word in the "+
			"commit message.", len(claims))
	}

	for _, c := range claims {
		name, file := c[1], c[2]
		path := apiDocResolve(file)
		if path == "" {
			t.Errorf("doc/API.md places %s in %s, and no such file is in the tree",
				name, file)
			continue
		}
		src, err := readRepoFile(path)
		if err != nil {
			t.Errorf("cannot read %s: %v", path, err)
			continue
		}
		if !strings.Contains(src, name) {
			t.Errorf("doc/API.md places %s in %s, and that file never names it.\n"+
				"  Either the function moved, or it has a new name.\n"+
				"  Repair doc/API.md, and the code as well when the name changed.",
				name, file)
		}
	}
}
