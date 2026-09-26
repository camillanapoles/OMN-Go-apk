package backend

// ----------------------------------------------------------------------
// The decision records, against the tree
// ----------------------------------------------------------------------
//
// doc/decisions holds a record for each rule of the code that is not
// obvious. A comment keeps one sentence of the reason and names the
// record by its path. See doc/decisions/README.md.
//
// A record that nothing lists is hard to find. A path in a comment that
// names no file sends the reader nowhere. The two tests below find both.
//
// THE DOCKER BUILD HAS NO doc/ DIRECTORY. .dockerignore excludes it, thus
// the two tests skip in the gate of the Docker build. They run in each
// go test of a clone. TestApiDocNamesTheRightFile does the same.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// decRecordNameRe is the file name of a record: four digits, a dash, and a
// short title in lower case.
var decRecordNameRe = regexp.MustCompile(`^([0-9]{4})-[a-z0-9-]+\.md$`)

// decRecordPathRe finds the path of a record in a comment.
var decRecordPathRe = regexp.MustCompile(`doc/decisions/[0-9]{4}-[a-z0-9-]+\.md`)

// decRecords answers the file name of each record, in the order of the
// numbers. It does not answer README.md. It skips the test when the tree
// has no doc/decisions directory. See the banner above.
func decRecords(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "doc", "decisions"))
	if os.IsNotExist(err) {
		t.Skipf("doc/decisions is not in this tree: %v", err)
	}
	if err != nil {
		t.Fatalf("cannot read doc/decisions: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "README.md" {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// Each record has a correct name and a correct first line. The numbers
// start at 0001 and have no gap. The index of README.md links each record.
func TestEachDecisionRecordIsListed(t *testing.T) {
	names := decRecords(t)
	if len(names) == 0 {
		t.Fatal("doc/decisions holds no record, thus this test proves nothing")
	}
	index, err := readRepoFile("doc/decisions/README.md")
	if err != nil {
		t.Fatalf("cannot read the index: %v", err)
	}
	for i, name := range names {
		m := decRecordNameRe.FindStringSubmatch(name)
		if m == nil {
			t.Errorf("doc/decisions/%s: the name must be NNNN-short-title.md in lower case", name)
			continue
		}
		if want := fmt.Sprintf("%04d", i+1); m[1] != want {
			t.Errorf("doc/decisions/%s: the number must be %s. The numbers have no gap and no double.",
				name, want)
		}
		src, err := readRepoFile("doc/decisions/" + name)
		if err != nil {
			t.Fatalf("cannot read doc/decisions/%s: %v", name, err)
		}
		if first, _, _ := strings.Cut(src, "\n"); !strings.HasPrefix(first, "# "+m[1]+". ") {
			t.Errorf("doc/decisions/%s: the first line must start with %q", name, "# "+m[1]+". ")
		}
		if !strings.Contains(index, "("+name+")") {
			t.Errorf("the index in doc/decisions/README.md has no link to %s", name)
		}
	}
}

// Each path of a record in a comment names a file that exists.
func TestEachCommentNamesARealDecisionRecord(t *testing.T) {
	have := map[string]bool{}
	for _, name := range decRecords(t) {
		have["doc/decisions/"+name] = true
	}
	for _, rel := range commentStyleFiles(t) {
		src, err := readRepoFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		for _, line := range strings.Split(src, "\n") {
			m := styleCommentLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			for _, p := range decRecordPathRe.FindAllString(m[1], -1) {
				if !have[p] {
					t.Errorf("%s names %s, and that record does not exist", rel, p)
				}
			}
		}
	}
}
