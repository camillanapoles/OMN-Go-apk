package repocheck

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const agentSkillsManifestFixture = "backend/internal/repocheck/testdata/agent-skills-manifest.yml"

type agentSkillEntry struct {
	id        string
	source    string
	pin       string
	path      string
	status    string
	reason    string
	appliesTo string
}

var (
	agentSkillIDPattern     = regexp.MustCompile(`^([a-z0-9]+-)*[a-z0-9]+$`)
	agentSkillPinPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	agentSkillPathPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*\.md$`)
	agentSkillSourcePattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\.git$`)
)

// TestAgentSkillsManifestPinsAndTriages checks the reviewed skill catalog.
func TestAgentSkillsManifestPinsAndTriages(t *testing.T) {
	entries := readAgentSkillsManifest(t)
	want := map[string]agentSkillEntry{
		"karpathy-guidelines": {
			id: "karpathy-guidelines", source: "https://github.com/multica-ai/andrej-karpathy-skills.git",
			pin: "2c606141936f1eeef17fa3043a72095b4765b9c2", path: "skills/karpathy-guidelines/SKILL.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-exploring": {
			id: "gitnexus-exploring", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-exploring.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-impact-analysis": {
			id: "gitnexus-impact-analysis", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-impact-analysis.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-debugging": {
			id: "gitnexus-debugging", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-debugging.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-refactoring": {
			id: "gitnexus-refactoring", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-refactoring.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-guide": {
			id: "gitnexus-guide", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-guide.md",
			status: "global", appliesTo: "all",
		},
		"gitnexus-cli": {
			id: "gitnexus-cli", source: "https://github.com/abhigyanpatwari/GitNexus.git",
			pin: "c4ecf398de637cf779ab2238e69958cfd5d601ce", path: "gitnexus/skills/gitnexus-cli.md",
			status: "global", appliesTo: "all",
		},
		"code-review": {
			id: "code-review", source: "https://github.com/mattpocock/skills.git",
			pin: "abeeaf95660ba9e8b7c753eb38f441ca707a8823", path: "skills/engineering/code-review/SKILL.md",
			status: "inactive", appliesTo: "none",
		},
		"tdd": {
			id: "tdd", source: "https://github.com/mattpocock/skills.git",
			pin: "abeeaf95660ba9e8b7c753eb38f441ca707a8823", path: "skills/engineering/tdd/SKILL.md",
			status: "inactive", appliesTo: "none",
		},
		"kotlin-api-design": {
			id: "kotlin-api-design", source: "https://github.com/chrisbanes/skills.git",
			pin: "edd18ff74aef5c49485dc376c043043f1e767f5a", path: "skills/kotlin-api-design/SKILL.md",
			status: "inactive", appliesTo: "none",
		},
		"security-review": {
			id: "security-review", source: "https://github.com/affaan-m/ECC.git",
			pin: "4adeee6732a12d1540f49a4d50a38ce8c97f9a53", path: "skills/security-review/SKILL.md",
			status: "inactive", appliesTo: "audit-backend-core|audit-config|audit-storage-exchange",
		},
		"docs-governance": {
			id: "docs-governance", source: "https://github.com/affaan-m/ECC.git",
			pin: "4adeee6732a12d1540f49a4d50a38ce8c97f9a53", path: "skills/docs-governance/SKILL.md",
			status: "inactive", appliesTo: "none",
		},
		"deep-research": {
			id: "deep-research", source: "https://github.com/affaan-m/ECC.git",
			pin: "4adeee6732a12d1540f49a4d50a38ce8c97f9a53", path: "skills/deep-research/SKILL.md",
			status: "inactive", appliesTo: "none",
		},
		"ecc-conventions": {
			id: "ecc-conventions", source: "https://github.com/affaan-m/ECC.git",
			pin: "4adeee6732a12d1540f49a4d50a38ce8c97f9a53", path: ".agents/skills/ecc-conventions/SKILL.md",
			status: "rejected", appliesTo: "none",
		},
		"jev-agent": {
			id: "jev-agent", source: "https://github.com/jev-ai/jev-agent-skill.git",
			pin: "8702f435249b3b0bed22ec23ef78eb1e0d7c05f6", path: "SKILL.md",
			status: "inactive", appliesTo: "none",
		},
	}
	if len(entries) != len(want) {
		t.Fatalf("agent skill manifest has %d entries; want %d reviewed entries", len(entries), len(want))
	}

	seenSources := make(map[string]bool)
	globalCount := 0
	for _, entry := range entries {
		if !agentSkillIDPattern.MatchString(entry.id) {
			t.Errorf("manifest skill id %q is invalid", entry.id)
		}
		if !agentSkillSourcePattern.MatchString(entry.source) {
			t.Errorf("manifest skill %s has an unsupported source %q", entry.id, entry.source)
		}
		if !agentSkillPinPattern.MatchString(entry.pin) {
			t.Errorf("manifest skill %s needs a full commit pin", entry.id)
		}
		if !agentSkillPathPattern.MatchString(entry.path) || strings.Contains(entry.path, "..") ||
			path.Clean(entry.path) != entry.path || path.IsAbs(entry.path) {
			t.Errorf("manifest skill %s has an unsafe source path %q", entry.id, entry.path)
		}
		if entry.status != "global" && entry.status != "inactive" && entry.status != "rejected" {
			t.Errorf("manifest skill %s has no reviewed status", entry.id)
		}
		if strings.TrimSpace(entry.reason) == "" {
			t.Errorf("manifest skill %s has no audit reason", entry.id)
		}
		seenSources[entry.source] = true
		if entry.status == "global" {
			globalCount++
		}
		wantEntry, ok := want[entry.id]
		if !ok {
			t.Errorf("manifest skill %s has no reviewed expectation", entry.id)
			continue
		}
		if entry.source != wantEntry.source || entry.pin != wantEntry.pin ||
			entry.path != wantEntry.path || entry.status != wantEntry.status ||
			entry.appliesTo != wantEntry.appliesTo {
			t.Errorf("manifest skill %s changed its reviewed source, pin, path, status, or selection", entry.id)
		}
	}
	if globalCount != 7 {
		t.Errorf("manifest has %d global skills; want Karpathy plus six GitNexus skills", globalCount)
	}
	if len(seenSources) != 6 {
		t.Errorf("manifest covers %d sources; want the five reviewed sources and GitNexus", len(seenSources))
	}
	for id := range want {
		found := false
		for _, entry := range entries {
			if entry.id == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest is missing skill %s", id)
		}
	}

	byID := make(map[string]agentSkillEntry, len(entries))
	for _, entry := range entries {
		byID[entry.id] = entry
	}
	if reason := byID["kotlin-api-design"].reason; !strings.Contains(reason, "§1") || !strings.Contains(reason, "conflit") {
		t.Errorf("kotlin-api-design is not triaged against CLAUDE.md §1: %q", reason)
	}
	if reason := byID["jev-agent"].reason; !strings.Contains(reason, "API key") || !strings.Contains(reason, "opt-in por estágio") {
		t.Errorf("jev-agent must state its external-service and API-key gate: %q", reason)
	}
}

// TestAgentSkillsSpecSelectionsMatchManifest checks the spec selection contract.
func TestAgentSkillsSpecSelectionsMatchManifest(t *testing.T) {
	entries := readAgentSkillsManifest(t)
	changesDir := filepath.Join(repoRoot, "openspec", "changes")
	changes, err := os.ReadDir(changesDir)
	if err != nil {
		t.Fatal(err)
	}

	seenSpecs := make(map[string]bool)
	for _, change := range changes {
		if !change.IsDir() {
			continue
		}
		spec := change.Name()
		agentsPath := filepath.Join(changesDir, spec, "AGENTS.md")
		if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}

		actual := readSpecSkillSelection(t, filepath.ToSlash(filepath.Join("openspec", "changes", spec, "AGENTS.md")))
		expected := make(map[string]bool)
		for _, skill := range entries {
			if skill.status != "inactive" {
				continue
			}
			for _, applicableSpec := range strings.Split(skill.appliesTo, "|") {
				if applicableSpec == spec {
					expected[skill.id] = true
				}
			}
		}
		if got, want := sortedSkillIDs(actual), sortedSkillIDs(expected); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s skill selection is %v; manifest applies_to requires %v", spec, got, want)
		}
		seenSpecs[spec] = true
	}

	for _, skill := range entries {
		if skill.status != "inactive" || skill.appliesTo == "none" {
			continue
		}
		for _, spec := range strings.Split(skill.appliesTo, "|") {
			if !seenSpecs[spec] {
				t.Errorf("manifest skill %s applies to %s, which has no AGENTS.md selection", skill.id, spec)
			}
		}
	}
}

func readSpecSkillSelection(t *testing.T, rel string) map[string]bool {
	t.Helper()
	data, err := readRepoFile(rel)
	if err != nil {
		t.Fatal(err)
	}

	inSection := false
	sectionSeen := false
	selectionSeen := false
	selection := ""
	for _, line := range strings.Split(data, "\n") {
		switch {
		case line == "## Skills ativas":
			if sectionSeen {
				t.Fatalf("%s repeats ## Skills ativas", rel)
			}
			sectionSeen = true
			inSection = true
		case inSection && strings.HasPrefix(line, "## "):
			inSection = false
		case inSection && strings.HasPrefix(line, "Seleção por spec:"):
			if selectionSeen {
				t.Fatalf("%s repeats Seleção por spec", rel)
			}
			selectionSeen = true
			selection = strings.TrimSpace(strings.TrimPrefix(line, "Seleção por spec:"))
		}
	}
	if !sectionSeen || !selectionSeen {
		t.Fatalf("%s needs ## Skills ativas and one Seleção por spec declaration", rel)
	}
	selected := make(map[string]bool)
	if selection == "nenhuma adicional." {
		return selected
	}
	selection = strings.TrimSuffix(selection, ".")
	for _, item := range strings.Split(selection, ", ") {
		if len(item) < 3 || item[0] != '`' || item[len(item)-1] != '`' {
			t.Fatalf("%s has invalid skill selection %q; use backticked IDs only", rel, item)
		}
		id := item[1 : len(item)-1]
		if !agentSkillIDPattern.MatchString(id) {
			t.Fatalf("%s selects invalid skill ID %q", rel, id)
		}
		if selected[id] {
			t.Fatalf("%s selects %s more than once", rel, id)
		}
		selected[id] = true
	}
	return selected
}

func sortedSkillIDs(skills map[string]bool) []string {
	ids := make([]string, 0, len(skills))
	for id := range skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// TestAgentSkillsWorkspaceContractListsGlobals checks the root skill contract.
func TestAgentSkillsWorkspaceContractListsGlobals(t *testing.T) {
	data, err := readRepoFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"karpathy-guidelines", "gitnexus-exploring", "gitnexus-impact-analysis",
		"gitnexus-debugging", "gitnexus-refactoring", "gitnexus-guide", "gitnexus-cli",
	} {
		if !strings.Contains(data, "`"+id+"`") {
			t.Errorf("AGENTS.md does not declare the global skill %s", id)
		}
	}
	for _, marker := range []string{
		"2c606141936f1eeef17fa3043a72095b4765b9c2",
		"c4ecf398de637cf779ab2238e69958cfd5d601ce",
		"bash local/agent-setup/install.sh",
		"--skip-agents-md",
	} {
		if !strings.Contains(data, marker) {
			t.Errorf("AGENTS.md does not hold %q", marker)
		}
	}
}

// TestGitNexusContractFilesStayClean checks files that GitNexus can generate.
func TestGitNexusContractFilesStayClean(t *testing.T) {
	const marker = "<!-- gitnexus:start -->"
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		data, err := readRepoFile(name)
		if os.IsNotExist(err) {
			continue // Docker excludes CLAUDE.md from its build context.
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(data, marker) {
			t.Errorf("%s contains a GitNexus-generated block; use analyze --skip-agents-md", name)
		}
	}
}

// TestAgentSkillsManifestProjectionUsesTheGateFixture checks the maintainer path.
func TestAgentSkillsManifestProjectionUsesTheGateFixture(t *testing.T) {
	linkPath := filepath.Join(repoRoot, "local", "skills", "manifest.yml")
	info, err := os.Lstat(linkPath)
	if os.IsNotExist(err) {
		return // Docker excludes local/ from its build context.
	}
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("local/skills/manifest.yml must link to the manifest fixture used by the gate")
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	const wantTarget = "../../backend/internal/repocheck/testdata/agent-skills-manifest.yml"
	if target != wantTarget {
		t.Fatalf("manifest link target is %q; want %q", target, wantTarget)
	}
	projected, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := readRepoFile(agentSkillsManifestFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(projected, []byte(fixture)) {
		t.Error("local/skills/manifest.yml differs from the manifest checked by the gate")
	}
}

func readAgentSkillsManifest(t *testing.T) []agentSkillEntry {
	t.Helper()
	data, err := readRepoFile(agentSkillsManifestFixture)
	if err != nil {
		t.Fatal(err)
	}

	var entries []agentSkillEntry
	fields := make(map[string]string)
	id := ""
	versionSeen := false
	skillsSeen := false
	flush := func() {
		if id == "" {
			return
		}
		for _, field := range []string{"source", "pin", "path", "status", "reason", "applies_to"} {
			if strings.TrimSpace(fields[field]) == "" {
				t.Fatalf("manifest skill %s has no %s", id, field)
			}
		}
		if len(fields) != 6 {
			t.Fatalf("manifest skill %s has unknown fields", id)
		}
		entries = append(entries, agentSkillEntry{
			id: id, source: fields["source"], pin: fields["pin"], path: fields["path"],
			status: fields["status"], reason: fields["reason"], appliesTo: fields["applies_to"],
		})
		id = ""
		fields = make(map[string]string)
	}

	scanner := bufio.NewScanner(strings.NewReader(data))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		switch {
		case line == "version: 1":
			if versionSeen {
				t.Fatalf("manifest repeats its version at line %d", lineNumber)
			}
			versionSeen = true
		case line == "skills:":
			if !versionSeen || skillsSeen {
				t.Fatalf("manifest has an unexpected skills section at line %d", lineNumber)
			}
			skillsSeen = true
		case strings.HasPrefix(line, "  - id: "):
			if !skillsSeen {
				t.Fatalf("manifest entry precedes skills at line %d", lineNumber)
			}
			flush()
			id = strings.TrimPrefix(line, "  - id: ")
		case strings.HasPrefix(line, "    "):
			if id == "" {
				t.Fatalf("manifest field has no entry at line %d", lineNumber)
			}
			key, value, ok := strings.Cut(strings.TrimPrefix(line, "    "), ": ")
			if !ok {
				t.Fatalf("manifest field is malformed at line %d", lineNumber)
			}
			switch key {
			case "source", "pin", "path", "status", "reason", "applies_to":
			default:
				t.Fatalf("manifest has unknown field %q at line %d", key, lineNumber)
			}
			if _, exists := fields[key]; exists {
				t.Fatalf("manifest repeats field %s for %s", key, id)
			}
			fields[key] = value
		case strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#"):
		default:
			t.Fatalf("manifest has unsupported content at line %d", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	flush()
	if !versionSeen || !skillsSeen || len(entries) == 0 {
		t.Fatal("manifest is missing its version, skills section, or entries")
	}
	return entries
}
