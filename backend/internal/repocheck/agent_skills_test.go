package repocheck

// ----------------------------------------------------------------------
// The agent skills manifest, against the tree
// ----------------------------------------------------------------------
//
// local/skills/manifest.yml catalogs each agent skill of the repository.
// One entry names the source, the pin, the path and the state of one
// skill, and it carries the reason of the state. The file
// local/agent-setup/install.sh reads the catalog and writes
// .claude/skills/. Nothing else installs a skill.
//
// A field that a person forgets shows no fault until a machine misses a
// skill. The tests below read the catalog and fail on each such field.
// This is the gate of the audit flow in docs/AUDIT_PROMPT.md.
//
// THE RULES OF THE CHECKLIST.
//
//   - A global entry without a reason or without a pin fails the gate.
//   - A pin that is short or absent fails the gate. A pin holds one full
//     commit hash or one tool version.
//   - A skill that the manifest lists and the tree does not hold fails
//     the gate with the path of the file.
//
// The tests also keep a third party out of local/skills/. A source of
// another repository is a reference with a pin and never a copy. A
// Docker build holds no local/ directory. The tests then skip.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillEntry is one entry of the agent skills manifest.
type skillEntry struct {
	id     string
	source string
	pin    string
	path   string
	status string
	reason string
}

// skillPinRe holds the two shapes of a pin. A commit pin is the full
// hash, and a tool pin is a three part version.
var skillPinRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9]+\.[0-9]+\.[0-9]+)$`)

// skillSourceRe holds the two shapes of a source. One names a tool of
// the workspace, and the other names one repository of GitHub.
var skillSourceRe = regexp.MustCompile(`^(tool:[A-Za-z0-9_-]+|[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)$`)

// skillIDRe holds the shape of an id. The id names the directory of the
// skill under .claude/skills/.
var skillIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// skillManifestRel is the path of the catalog from the root.
const skillManifestRel = "local/skills/manifest.yml"

// skillManifest reads the catalog and answers its entries. It skips the
// test when the tree holds no catalog. The Docker build excludes local/
// on purpose, and a tree without the catalog proves nothing.
func skillManifest(t *testing.T) []skillEntry {
	t.Helper()
	src, err := readRepoFile(skillManifestRel)
	if os.IsNotExist(err) {
		t.Skipf("the tree holds no agent skills catalog: %v", err)
	}
	if err != nil {
		t.Fatalf("cannot read %s: %v", skillManifestRel, err)
	}
	var out []skillEntry
	var cur *skillEntry
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || line == "skills:" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "- id: "); ok {
			out = append(out, skillEntry{id: strings.TrimSpace(rest)})
			cur = &out[len(out)-1]
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok || cur == nil {
			t.Fatalf("%s: the line %q is not a field of an entry", skillManifestRel, line)
		}
		value = strings.Trim(value, "\"")
		switch key {
		case "source":
			cur.source = value
		case "pin":
			cur.pin = value
		case "path":
			cur.path = value
		case "status":
			cur.status = value
		case "reason":
			cur.reason = value
		case "applies_to":
			// The catalog carries domain hints. No rule reads them.
		default:
			t.Fatalf("%s: entry %s holds the unknown field %q", skillManifestRel, cur.id, key)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: the catalog holds no entry", skillManifestRel)
	}
	return out
}

// skillFrontmatterOK reports whether src opens a frontmatter block with a
// name and a description. Each agent reads those two fields before it
// loads a skill.
func skillFrontmatterOK(src string) bool {
	lines := strings.Split(src, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return false
	}
	name, desc, closed := false, false, false
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			closed = true
			break
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok && strings.TrimSpace(v) != "" {
			name = true
		}
		if v, ok := strings.CutPrefix(line, "description:"); ok && strings.TrimSpace(v) != "" {
			desc = true
		}
	}
	return closed && name && desc
}

// Each entry of the catalog carries source, pin, path, status and reason.
// A name without a field is a hole in the catalog, and a machine can then
// install a skill without a record.
func TestSkillsManifestEntryCarriesSourcePinStatusReason(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range skillManifest(t) {
		if !skillIDRe.MatchString(e.id) {
			t.Errorf("entry %q: the id must be lower case with dashes", e.id)
		}
		if seen[e.id] {
			t.Errorf("entry %s: the id is a double", e.id)
		}
		seen[e.id] = true
		if e.source == "" || !skillSourceRe.MatchString(e.source) {
			t.Errorf("entry %s: the source %q must be tool:<name> or <owner>/<repo>", e.id, e.source)
		}
		if e.path == "" {
			t.Errorf("entry %s: the path is empty", e.id)
		}
		if e.status != "global" && e.status != "inactive" && e.status != "rejected" {
			t.Errorf("entry %s: the status %q must be global, inactive or rejected", e.id, e.status)
		}
		if e.reason == "" {
			t.Errorf("entry %s: the reason is empty", e.id)
		}
		if e.path == "-" && e.status != "rejected" {
			t.Errorf("entry %s: a path of - needs the status rejected", e.id)
		}
	}
}

// The pin of an entry is a full commit hash or a three part tool version.
// A short pin is not a pin: the content of the source can then move.
func TestSkillsManifestPinIsFull(t *testing.T) {
	for _, e := range skillManifest(t) {
		if !skillPinRe.MatchString(e.pin) {
			t.Errorf("entry %s: the pin %q must be a full commit hash or a tool version", e.id, e.pin)
		}
	}
}

// A global skill runs on each machine and each worktree. The checklist of
// the audit demands a reason and a pin for each one of them.
func TestSkillsManifestGlobalCarriesReasonAndPin(t *testing.T) {
	for _, e := range skillManifest(t) {
		if e.status != "global" {
			continue
		}
		if e.reason == "" {
			t.Errorf("entry %s: a global skill needs a reason", e.id)
		}
		if e.pin == "" {
			t.Errorf("entry %s: a global skill needs a pin", e.id)
		}
	}
}

// The catalog lists a tree skill, and the tree must hold that file. The
// install copies it, thus a missing file gives a broken install on the
// next machine. The frontmatter carries name and description for each
// agent that reads the skill.
func TestSkillsManifestListedTreeSkillIsInTheTree(t *testing.T) {
	for _, e := range skillManifest(t) {
		if !strings.HasPrefix(e.path, "local/") {
			continue
		}
		want := "local/skills/" + e.id + "/SKILL.md"
		if e.path != want {
			t.Errorf("entry %s: the path is %s, want %s", e.id, e.path, want)
			continue
		}
		src, err := readRepoFile(e.path)
		if err != nil {
			t.Errorf("entry %s: the tree holds no skill at %s", e.id, e.path)
			continue
		}
		if !skillFrontmatterOK(src) {
			t.Errorf("entry %s: the frontmatter of %s needs name and description", e.id, e.path)
		}
	}
}

// A skill of the tree that the catalog does not list is invisible to the
// install and to the review. The catalog is the one authority, thus the
// test keeps the tree and the catalog in step.
func TestSkillsTreeHoldsNoSkillOutsideTheManifest(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "local", "skills"))
	if os.IsNotExist(err) {
		t.Skipf("the tree holds no local/skills directory: %v", err)
	}
	if err != nil {
		t.Fatalf("cannot read local/skills: %v", err)
	}
	listed := map[string]bool{}
	for _, e := range skillManifest(t) {
		listed[e.id] = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, "local", "skills", e.Name(), "SKILL.md")); err != nil {
			continue
		}
		if !listed[e.Name()] {
			t.Errorf("local/skills/%s/SKILL.md is not in the manifest", e.Name())
		}
	}
}

// A source of another repository is never a copy under local/skills/. The
// manifest carries the reference and the pin, and the install downloads
// the content at that pin. The rule keeps third party text out of the
// tree. See the prohibition of the change audit-skills-harness.
func TestSkillsManifestExternalSourceIsNotVendored(t *testing.T) {
	for _, e := range skillManifest(t) {
		if strings.HasPrefix(e.source, "tool:") {
			continue
		}
		if strings.HasPrefix(e.path, "local/") {
			t.Errorf("entry %s: the external source %s must hold no path in the tree", e.id, e.source)
		}
	}
}

// A tool skill is the other half of the rule above. A tool of the
// workspace writes its skill in the tree, and the manifest pins the
// version of the tool that wrote it.
func TestSkillsManifestToolSkillLivesInTheTree(t *testing.T) {
	for _, e := range skillManifest(t) {
		if !strings.HasPrefix(e.source, "tool:") {
			continue
		}
		if !strings.HasPrefix(e.path, "local/") {
			t.Errorf("entry %s: the tool skill must hold a path in the tree", e.id)
		}
	}
}
