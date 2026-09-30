package gitsync

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The test of this file runs the real JavaScript through node, against the
// lines of a real sync. The other JavaScript tests are in
// internal/repocheck/js_test.go.

// backendDir is the backend/ directory. node runs there, because the script
// of the test loads ./frontend/test/page-stub.js.
const backendDir = "../.."

// ----------------------------------------------------------------------
// The sync progress overlay, against a real sync
// ----------------------------------------------------------------------
//
// Section 3 of CLAUDE.md holds this rule:
//
//	"applySyncLogLine in omn-go-sse.js removes the level word before it
//	 matches a sync stage. Keep the two in agreement, or the progress
//	 overlay loses a stage."
//
// Nothing held it. internal/repocheck/ports_test.go reads the level
// pattern out of the SOURCE of omn-go-sse.js and compiles it. That proves
// that the pattern exists. It says nothing about the lines that the Go side
// writes.
//
// THE OVERLAY FAILS QUIETLY. A message that no prefix of SYNC_STAGES
// matches leaves the bar where it was. A person watching a sync sees a
// stage that ended, and no fault reaches any log.
//
// This test runs a WHOLE SYNC and reads the lines that it really wrote.
// The history ring keeps them, thus the test needs no capture of its
// own. Each line then goes through the REAL JavaScript, in a page whose
// OMNProgress records the stage.
//
// A line of the Go side with no stage in the JavaScript side is the
// failure. That is the drift the rule names.
//
// WHAT IT DOES NOT PROVE. SYNC_STAGES holds prefixes that this scenario
// never reaches, for example the sideband text of a remote over a
// network. A test that asked for every prefix to fire would be wrong,
// and it would grow a list of exceptions. This one asks the other
// question, which is the one that hurts a person.

// jsSyncLines runs a whole life of a sync and answers each [sync] line
// that it wrote.
//
// The App of the test holds its own ring. The filter for "[sync]" keeps the
// sync lines only.
func jsSyncLines(t *testing.T) []string {
	t.Helper()

	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)

	// A first pull, a push, a push that the remote refuses, a conflict,
	// a marked merge, an abort, a force pull and a force push.
	_ = a.gitSync().SyncRepo("pull", "")
	gsWrite(t, a, "md/Two.md", "two\n")
	_ = a.gitSync().SyncRepo("push", "add a note")
	gsSeedRemote(t, remote, "other", map[string]string{"md/Other.md": "other\n"})
	gsWrite(t, a, "md/Three.md", "three\n")
	_ = a.gitSync().SyncRepo("push", "refused")
	_ = a.gitSync().SyncRepo("pull", "")
	gsWrite(t, a, "md/One.md", "changed here\n")
	gsSeedRemote(t, remote, "third", map[string]string{"md/One.md": "changed there\n"})
	_ = a.gitSync().SyncRepo("pull", "")
	_ = a.gitSync().SyncRepo("pull_mark", "")
	_ = a.gitSync().SyncRepo("pull_abort", "")
	_ = a.gitSync().SyncRepo("pull_force", "")
	_ = a.gitSync().SyncRepo("push_force", "take mine")

	var out []string
	for _, line := range a.Logs.Snapshot() {
		if strings.Contains(line, "[sync]") {
			out = append(out, strings.TrimRight(line, "\n"))
		}
	}
	if len(out) < 20 {
		t.Fatalf("the sync wrote %d lines. The scenario stopped early, thus "+
			"this test proves little.", len(out))
	}
	return out
}

// Each line of a real sync must move the progress overlay.
func TestEverySyncLineReachesTheOverlay(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}
	lines := jsSyncLines(t)

	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatalf("cannot encode the lines: %v", err)
	}
	file := filepath.Join(t.TempDir(), "lines.json")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatalf("cannot write the lines: %v", err)
	}

	// One node process for the whole run. It loads the shipped script in
	// a page whose global is the window stub, the same as a <script src>
	// element does. See frontend/test/page-stub.js.
	script := `
const fs = require('fs');
const { newPage, run } = require('./frontend/test/page-stub.js');
const page = newPage();
run(page, 'omn-go-core.js');
run(page, 'omn-go-sse.js');
if (typeof page.applySyncLogLine !== 'function') {
    process.stdout.write(JSON.stringify({ missing: true }));
    process.exit(0);
}
let stages = 0;
const quiet = [];
page.OMNProgress = { show(){}, hide(){}, detail(){}, stage(){ stages++; } };
for (const line of JSON.parse(fs.readFileSync(process.argv[1], 'utf8'))) {
    const before = stages;
    page.applySyncLogLine(line);
    if (stages === before) quiet.push(line);
}
process.stdout.write(JSON.stringify({ quiet: quiet, stages: stages }));
`
	cmd := exec.Command(node, "-e", script, file)
	cmd.Dir = backendDir
	out, runErr := cmd.Output()
	if runErr != nil {
		t.Fatalf("node failed: %v\n%s", runErr, out)
	}
	var answer struct {
		Missing bool     `json:"missing"`
		Quiet   []string `json:"quiet"`
		Stages  int      `json:"stages"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("node did not answer JSON: %v\n%s", err, out)
	}
	if answer.Missing {
		t.Fatal("omn-go-sse.js does not export applySyncLogLine. " +
			"omn-go-sync.js then reads a bare name, and 26.09.41 says why " +
			"that is a trap.")
	}
	if len(answer.Quiet) > 0 {
		t.Errorf("%d of %d lines of a real sync moved no stage of the overlay.\n"+
			"  A person then watches a bar that stands still, and no fault is logged.\n"+
			"  Add a prefix to SYNC_STAGES in omn-go-sse.js, or repair the message.\n"+
			"  The first three are:\n    %s",
			len(answer.Quiet), len(lines),
			strings.Join(answer.Quiet[:min(3, len(answer.Quiet))], "\n    "))
	}
	t.Logf("%d lines of a real sync, %d stages", len(lines), answer.Stages)
}
