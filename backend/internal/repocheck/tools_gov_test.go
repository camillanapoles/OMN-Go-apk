package repocheck

import (
	"os"
	"strings"
	"testing"
)

// The code intelligence tool writes a section of its own into AGENTS.md and
// CLAUDE.md at analysis time. The contract rules protect CLAUDE.md. A commit
// of that section would break the contract of each session. Run the tool
// with --skip-agents-md. This test breaks the gate when the marker returns.
func TestGitNexusStaysOutOfTheContract(t *testing.T) {
	const marker = "<!-- gitnexus:start -->"
	for _, rel := range []string{"CLAUDE.md", "AGENTS.md"} {
		src, err := readRepoFile(rel)
		if err != nil {
			if os.IsNotExist(err) {
				// AGENTS.md does not exist at the root. That is the
				// clean state of the tree.
				continue
			}
			t.Fatal(err)
		}
		if strings.Contains(src, marker) {
			t.Errorf("%s carries the gitnexus section. Run gitnexus analyze with --skip-agents-md and drop the section.", rel)
		}
	}
}
