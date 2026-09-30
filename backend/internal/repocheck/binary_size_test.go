package repocheck

// ----------------------------------------------------------------------
// The binary size report
// ----------------------------------------------------------------------
//
// TestBinarySize builds the release binaries of the current tree. It
// compares the size of each one with a baseline build, and it reports the
// change in bytes and in percent. A change is not a fault.
// The test fails only when a growth limit is set and a target goes over
// it.
//
// IT RUNS ONLY ON REQUEST. Five builds take minutes on a cold cache,
// thus the normal gate skips the test. The command is:
//
//	OMN_BINARY_SIZE=1 go test -v -run 'TestBinarySize$' -timeout 30m ./backend/internal/repocheck/
//
// THE BASELINE. testdata/binary_size_baseline.json holds the git reference
// of the baseline build, its sizes, and the Go version that made them. A different Go version
// also changes the size. The report then says so, and the growth limit
// does not apply.
//
// For an exact comparison, set OMN_BINARY_SIZE_BASE to a git reference.
// The test then exports that reference and builds it with the same Go
// version as the current tree. This needs the .git directory. The Docker
// context excludes it, thus this mode works on a clone only.
//
// THE SETTINGS.
//
//   - OMN_BINARY_SIZE=1 runs the test.
//   - OMN_BINARY_SIZE_BASE=<git reference> builds that reference as the
//     baseline. Use the ref of the JSON file for the same baseline.
//   - OMN_BINARY_SIZE_MAX_GROWTH=1.5 fails the test when a target grows
//     by more than 1.5 percent.
//   - OMN_BINARY_SIZE_WRITE=1 writes the sizes of the baseline reference
//     to the JSON file. Use it after a change of the Go version. It builds
//     the reference that the file names, or OMN_BINARY_SIZE_BASE.
//
// THE TARGETS. Each build uses the release flags of the Dockerfile. Each
// build also sets CGO_ENABLED=0, thus the result does not depend on a C
// compiler. A Linux build stands for the Android ABI of the same CPU. The
// Android library holds the same Go code plus the gomobile glue. A change
// of the Go code thus shows here with nearly the same size.

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The file that holds the sizes of the baseline build.
const sizeBaselineFile = "testdata/binary_size_baseline.json"

// The flags of each build, as the report and the JSON file show them.
const sizeFlagsText = "CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags='-s -w'"

type sizeTarget struct {
	name   string
	goos   string
	goarch string
	goarm  string
	stands string
}

// The targets of the report. The order is the order of the report.
var sizeTargets = []sizeTarget{
	{"linux-amd64", "linux", "amd64", "", "desktop Linux, Android x86_64"},
	{"windows-amd64", "windows", "amd64", "", "desktop Windows"},
	{"linux-arm64", "linux", "arm64", "", "Android arm64-v8a"},
	{"linux-arm7", "linux", "arm", "7", "Android armeabi-v7a"},
	{"linux-386", "linux", "386", "", "Android x86"},
}

type sizeBaseline struct {
	Ref       string           `json:"ref"`
	Commit    string           `json:"commit"`
	GoVersion string           `json:"go_version"`
	Flags     string           `json:"flags"`
	Targets   map[string]int64 `json:"targets"`
}

// sizeRow is one line of the report. Base is zero when the baseline has
// no size for the target.
type sizeRow struct {
	target sizeTarget
	base   int64
	cur    int64
}

func (r sizeRow) delta() int64 { return r.cur - r.base }

func (r sizeRow) percent() float64 {
	if r.base == 0 {
		return 0
	}
	return float64(r.cur-r.base) * 100 / float64(r.base)
}

// sizeReport makes the lines of the report and the list of targets that
// grow by more than limit percent. A limit of zero or less sets no limit.
func sizeReport(rows []sizeRow, limit float64) (lines []string, over []string) {
	lines = append(lines, fmt.Sprintf("%-14s %-30s %12s %12s %10s %8s",
		"target", "stands for", "baseline", "current", "delta", "percent"))
	var baseSum, curSum int64
	for _, r := range rows {
		if r.base == 0 {
			lines = append(lines, fmt.Sprintf("%-14s %-30s %12s %12d %10s %8s",
				r.target.name, r.target.stands, "-", r.cur, "-", "-"))
			continue
		}
		baseSum += r.base
		curSum += r.cur
		lines = append(lines, fmt.Sprintf("%-14s %-30s %12d %12d %+10d %+7.2f%%",
			r.target.name, r.target.stands, r.base, r.cur, r.delta(), r.percent()))
		if limit > 0 && r.percent() > limit {
			over = append(over, fmt.Sprintf("%s grows by %.2f%%, over the limit of %.2f%%",
				r.target.name, r.percent(), limit))
		}
	}
	if baseSum > 0 {
		total := sizeRow{target: sizeTarget{name: "total"}, base: baseSum, cur: curSum}
		lines = append(lines, fmt.Sprintf("%-14s %-30s %12d %12d %+10d %+7.2f%%",
			"total", "", baseSum, curSum, total.delta(), total.percent()))
	}
	return lines, over
}

func readSizeBaseline(path string) (sizeBaseline, error) {
	var b sizeBaseline
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// sizeCommand runs a command in dir and answers its trimmed output.
func sizeCommand(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s in %s failed: %v\n%s", name, strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// buildSizes builds main_desktop.go of the module in dir for each target
// and answers the size of each binary.
func buildSizes(t *testing.T, goTool, dir string) map[string]int64 {
	t.Helper()
	out := t.TempDir()
	sizes := map[string]int64{}
	for _, tg := range sizeTargets {
		bin := filepath.Join(out, tg.name)
		env := []string{"CGO_ENABLED=0", "GOFLAGS=", "GOOS=" + tg.goos,
			"GOARCH=" + tg.goarch, "GOARM=" + tg.goarm}
		sizeCommand(t, dir, env, goTool, "build", "-trimpath", "-buildvcs=false",
			"-ldflags=-s -w", "-o", bin, "main_desktop.go")
		info, err := os.Stat(bin)
		if err != nil {
			t.Fatalf("no binary for %s: %v", tg.name, err)
		}
		sizes[tg.name] = info.Size()
	}
	return sizes
}

// exportGitRef writes the tree of ref into a new directory and answers
// the directory and the short commit hash. The repository holds no
// go.sum. The function thus copies the go.sum of the current tree. Then
// it runs go mod tidy, the same as the Docker build.
func exportGitRef(t *testing.T, goTool, root, ref string) (string, string) {
	t.Helper()
	commit := sizeCommand(t, root, nil, "git", "rev-parse", "--short", ref+"^{commit}")
	cmd := exec.Command("git", "archive", "--format=tar", commit)
	cmd.Dir = root
	var archive, stderr bytes.Buffer
	cmd.Stdout = &archive
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git archive %s failed: %v\n%s", ref, err, stderr.String())
	}
	dir := t.TempDir()
	tr := tar.NewReader(&archive)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read the archive of %s: %v", ref, err)
		}
		dest := filepath.Join(dir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(dest, dir+string(os.PathSeparator)) {
			t.Fatalf("the archive of %s holds a path outside its root: %s", ref, hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, data, os.FileMode(hdr.Mode)&0o777); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sum, err := os.ReadFile(filepath.Join(root, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sizeCommand(t, dir, []string{"GOFLAGS="}, goTool, "mod", "tidy")
	return dir, commit
}

// TestBinarySize shows the effect of a change on the size of each release
// binary. The split of the backend into packages must not make the
// binaries larger by accident. This report makes each change visible. See
// the banner at the top of this file for the settings.
func TestBinarySize(t *testing.T) {
	if os.Getenv("OMN_BINARY_SIZE") != "1" {
		t.Skip("set OMN_BINARY_SIZE=1 to build the release binaries and compare their size")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command on this machine")
	}
	const root = repoRoot
	goVersion := sizeCommand(t, root, nil, goTool, "env", "GOVERSION")

	stored, storedErr := readSizeBaseline(sizeBaselineFile)
	ref := os.Getenv("OMN_BINARY_SIZE_BASE")
	write := os.Getenv("OMN_BINARY_SIZE_WRITE") == "1"
	if write && ref == "" {
		ref = stored.Ref
	}
	if write && ref == "" {
		t.Fatalf("OMN_BINARY_SIZE_WRITE=1 needs OMN_BINARY_SIZE_BASE, or a ref in %s", sizeBaselineFile)
	}
	if ref == "" && storedErr != nil {
		t.Fatalf("no baseline: %v. Set OMN_BINARY_SIZE_BASE=v26.09.62 to build one.", storedErr)
	}

	limit := 0.0
	if s := os.Getenv("OMN_BINARY_SIZE_MAX_GROWTH"); s != "" {
		if limit, err = strconv.ParseFloat(s, 64); err != nil {
			t.Fatalf("OMN_BINARY_SIZE_MAX_GROWTH=%q is not a number: %v", s, err)
		}
	}

	var base map[string]int64
	var source string
	exact := true
	if ref != "" {
		dir, commit := exportGitRef(t, goTool, root, ref)
		base = buildSizes(t, goTool, dir)
		source = fmt.Sprintf("%s (%s), built now with %s", ref, commit, goVersion)
		if write {
			out := sizeBaseline{Ref: ref, Commit: commit, GoVersion: goVersion,
				Flags: sizeFlagsText, Targets: base}
			data, mErr := json.MarshalIndent(out, "", "  ")
			if mErr != nil {
				t.Fatal(mErr)
			}
			if wErr := os.WriteFile(sizeBaselineFile, append(data, '\n'), 0o644); wErr != nil {
				t.Fatal(wErr)
			}
			t.Logf("wrote %s for %s with %s", sizeBaselineFile, ref, goVersion)
		}
	} else {
		base = stored.Targets
		exact = stored.GoVersion == goVersion
		source = fmt.Sprintf("%s (%s) from %s, built with %s",
			stored.Ref, stored.Commit, sizeBaselineFile, stored.GoVersion)
	}

	current := buildSizes(t, goTool, root)
	var rows []sizeRow
	for _, tg := range sizeTargets {
		rows = append(rows, sizeRow{target: tg, base: base[tg.name], cur: current[tg.name]})
	}

	gate := limit
	if !exact {
		gate = 0
	}
	lines, over := sizeReport(rows, gate)
	t.Logf("baseline: %s", source)
	t.Logf("current:  this tree, built with %s", goVersion)
	t.Logf("flags:    %s", sizeFlagsText)
	for _, l := range lines {
		t.Log(l)
	}
	if !exact {
		t.Logf("NOTE: the Go version differs from the baseline, thus the delta includes the " +
			"effect of the Go version. The growth limit does not apply. " +
			"Set OMN_BINARY_SIZE_BASE=v26.09.62 for an exact comparison.")
	}
	sort.Strings(over)
	for _, o := range over {
		t.Error(o)
	}
}

// TestBinarySizeReport checks the arithmetic and the lines of the report.
// TestBinarySize runs only on request, thus a fault in the report could
// stay hidden for a long time. This test needs no build, and the normal
// gate runs it.
func TestBinarySizeReport(t *testing.T) {
	rows := []sizeRow{
		{target: sizeTargets[0], base: 1000, cur: 1030},
		{target: sizeTargets[1], base: 2000, cur: 1900},
		{target: sizeTargets[2], base: 0, cur: 500},
	}
	lines, over := sizeReport(rows, 2)
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want a header, three rows and a total:\n%s",
			len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[1], "+30") || !strings.Contains(lines[1], "+3.00%") {
		t.Errorf("the growth row is wrong: %s", lines[1])
	}
	if !strings.Contains(lines[2], "-100") || !strings.Contains(lines[2], "-5.00%") {
		t.Errorf("the decrease row is wrong: %s", lines[2])
	}
	if !strings.Contains(lines[3], " - ") {
		t.Errorf("a target with no baseline must show no delta: %s", lines[3])
	}
	if !strings.Contains(lines[4], "-70") {
		t.Errorf("the total must add the rows with a baseline: %s", lines[4])
	}
	if len(over) != 1 || !strings.HasPrefix(over[0], sizeTargets[0].name) {
		t.Errorf("only %s is over the limit of 2%%, got %v", sizeTargets[0].name, over)
	}
	if _, none := sizeReport(rows, 0); len(none) != 0 {
		t.Errorf("a limit of zero sets no limit, got %v", none)
	}
}

// TestBinarySizeBaselineFile keeps the baseline file valid. A hand edit
// that breaks the JSON, or a new target with no baseline size, fails here
// in the normal gate. Without this test, the fault shows only when a
// person requests the report.
func TestBinarySizeBaselineFile(t *testing.T) {
	b, err := readSizeBaseline(sizeBaselineFile)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref == "" || b.GoVersion == "" || b.Flags != sizeFlagsText {
		t.Errorf("%s needs a ref, a Go version and the flags %q, got %+v",
			sizeBaselineFile, sizeFlagsText, b)
	}
	for _, tg := range sizeTargets {
		if b.Targets[tg.name] <= 0 {
			t.Errorf("%s has no size for %s", sizeBaselineFile, tg.name)
		}
	}
}
