package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The release of the fork ships one completed spec, and no smaller step.
// The Release workflow tagged a version at each push to main. The fork then
// made one small release per pull request. These tests hold the three parts
// of the rule. The Release workflow makes no tag without the spec gate. The
// version of the app carries a humanized changelog. Each spec task carries
// its verification and validation criteria.
// See doc/decisions/0031-release-after-a-completed-spec.md.

// releaseChangelogFileRef finds a file name in a changelog line. The person
// who uses the app reads no file name.
var releaseChangelogFileRef = regexp.MustCompile(
	`[A-Za-z0-9_./-]+\.(go|yml|yaml|json|gradle|java|js|sh|md)\b`)

// releaseChangelogHexSha finds a token that reads as a git commit hash.
var releaseChangelogHexSha = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

// releaseChangelogMinWords is the least number of words of one bullet of the
// changelog. A short bullet says nothing to the person.
const releaseChangelogMinWords = 8

// TestReleaseWorkflowRequiresSpecGate holds the rule at the workflows. The
// Release workflow starts from the dispatch of the maintainer for one
// completed spec. The gate runs before the tag. The release build starts
// from a tag alone.
func TestReleaseWorkflowRequiresSpecGate(t *testing.T) {
	release, err := readRepoFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(release, "branches:") {
		t.Error(".github/workflows/release.yml releases on a branch push. " +
			"A release starts from the dispatch of the maintainer for one " +
			"completed spec, and never from a push.")
	}
	if !strings.Contains(release, "workflow_dispatch") {
		t.Error(".github/workflows/release.yml lost the workflow_dispatch trigger")
	}
	const gateCall = "spec-release-gate.sh"
	gate := strings.Index(release, gateCall)
	if gate < 0 {
		t.Errorf(".github/workflows/release.yml does not run %s. The tag needs the spec gate.", gateCall)
	}
	tagStep := strings.Index(release, "Create the tag and push it")
	if gate >= 0 && tagStep >= 0 && gate > tagStep {
		t.Error(".github/workflows/release.yml creates the tag before the spec gate runs")
	}
	script, err := readRepoFile("local/release/spec-release-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"spec:done", "Verificação", "Validação", "doc/changelog"} {
		if !strings.Contains(script, marker) {
			t.Errorf("local/release/spec-release-gate.sh lost the check for %q", marker)
		}
	}

	build, err := readRepoFile(".github/workflows/android-gomobile-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(build, "branches:") || strings.Contains(build, "pull_request") {
		t.Error(".github/workflows/android-gomobile-release.yml builds on a branch push or " +
			"a pull request. A tag makes the release build, and the Fork CI builds the branches.")
	}
	if !strings.Contains(build, "tags: ['v*']") {
		t.Error(".github/workflows/android-gomobile-release.yml lost the tag trigger")
	}
}

// TestReleaseChangelogIsHumanized holds the rule of the notes. The commit
// that sets the version of the app writes the changelog of that version.
// Each line of the changelog speaks to the person who uses the app. One
// bullet holds eight words or more. The changelog holds no commit hash, no
// file name and no developer word.
func TestReleaseChangelogIsHumanized(t *testing.T) {
	version := releaseAppVersion(t)
	data, err := readRepoFile("doc/changelog/" + version + ".md")
	if err != nil {
		t.Fatalf("doc/changelog/%s.md is missing: %v. The commit that sets the "+
			"version writes its changelog.", version, err)
	}
	for i, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "-->") {
			continue
		}
		if !strings.HasPrefix(trimmed, "•") {
			t.Errorf("doc/changelog/%s.md:%d: each line of the changelog is one "+
				"• bullet. The person reads the bullet, not the raw change.",
				version, i+1)
			continue
		}
		if words := len(strings.Fields(trimmed)); words < releaseChangelogMinWords {
			t.Errorf("doc/changelog/%s.md:%d: a bullet of %d words says too "+
				"little. Write the change for the person.", version, i+1, words)
		}
		if sha := releaseHexShaToken(trimmed); sha != "" {
			t.Errorf("doc/changelog/%s.md:%d: the changelog holds the commit "+
				"hash %s. The person reads no hash.", version, i+1, sha)
		}
		if ref := releaseChangelogFileRef.FindString(trimmed); ref != "" {
			t.Errorf("doc/changelog/%s.md:%d: the changelog names the file %q. "+
				"The person reads no file name.", version, i+1, ref)
		}
		for _, word := range []string{"bump", "refactor", "chore", "merge branch"} {
			if strings.Contains(strings.ToLower(trimmed), word) {
				t.Errorf("doc/changelog/%s.md:%d: the changelog uses the "+
					"developer word %q.", version, i+1, word)
			}
		}
	}
}

// TestEverySpecTaskHasValidationBlocks holds the rule that makes the gate
// able to prove a completed spec. Each task of each change carries one
// Verificação line and one Validação line. The tasks file of a change with
// no task of its own carries none.
func TestEverySpecTaskHasValidationBlocks(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "openspec", "changes"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "archive" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, "openspec", "changes",
			entry.Name(), "tasks.md"))
		if err != nil {
			t.Errorf("openspec/changes/%s holds no tasks.md. Each change of "+
				"the fork carries its checklist.", entry.Name())
			continue
		}
		found++
		releaseScanSpecTasks(t, entry.Name(), string(data))
	}
	if found == 0 {
		t.Error("openspec/changes holds no change with a tasks.md file")
	}
}

// releaseScanSpecTasks checks that each task of one tasks.md file carries its
// verification and validation criteria.
func releaseScanSpecTasks(t *testing.T, name, data string) {
	t.Helper()
	var task string
	var verify, validate bool
	report := func() {
		if task != "" && (!verify || !validate) {
			t.Errorf("openspec/changes/%s/tasks.md: the task %q misses its "+
				"criteria. Each task holds one Verificação line and one "+
				"Validação line.", name, task)
		}
	}
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- ["):
			report()
			task = trimmed
			verify, validate = false, false
		case strings.HasPrefix(trimmed, "## "):
			report()
			task = ""
		case strings.Contains(trimmed, "**Verificação:**"):
			verify = true
		case strings.Contains(trimmed, "**Validação:**"):
			validate = true
		}
	}
	report()
}

// releaseAppVersion reads the version of the app from backend/version.go.
func releaseAppVersion(t *testing.T) string {
	t.Helper()
	data, err := readBackendFile("version.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`APP_VERSION\s*=\s*"([^"]+)"`).FindStringSubmatch(string(data))
	if len(m) != 2 {
		t.Fatal("backend/version.go holds no APP_VERSION line of the known shape")
	}
	return m[1]
}

// releaseHexShaToken answers the first token of the line that reads as a
// git commit hash. The token holds only hex digits, seven or more of them,
// and at least one letter. A token of digits alone is a version number, and
// it is no hash.
func releaseHexShaToken(line string) string {
	for _, token := range releaseChangelogHexSha.FindAllString(line, -1) {
		if strings.ContainsAny(token, "abcdef") {
			return token
		}
	}
	return ""
}
