package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// The two tests of this file run the real JavaScript through node. They
// compare it with the lines and the log filter of a real App, thus they stay
// in this package. The other JavaScript tests are in
// internal/repocheck/js_test.go.

// backendDir is the backend/ directory. node runs there, because the scripts
// of the tests load ./frontend/test/page-stub.js.
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
	for _, line := range a.logs.Snapshot() {
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

// ----------------------------------------------------------------------
// The log filter, on both sides
// ----------------------------------------------------------------------
//
// One decision has two implementations. logLineEnabled in log_app.go says
// whether a line reaches stdout. logLinePrints in omn-go-sse.js says
// whether the same line reaches the browser console.
//
// Rule 7 of CLAUDE.md section 1 asks for one authority. This pair is an
// exception of the same kind as the fold table. The answer is needed in
// a page that the server does not reach again. The rule therefore asks
// for a test that compares the two.
//
// THE TWO DO NOT TAKE THE SAME INPUT. The Go side takes a level and a
// tag. The JavaScript side takes the whole line and reads both out of it
// with LOG_LINE_RE. The line that it reads is the one that logx.Logger.emit
// writes, thus the test builds the line the same way logx.Logger.emit does.
//
// A DIFFERENCE IS NOT COSMETIC. A person turns a level off, sees a quiet
// stdout and a loud console, and cannot tell which one lies.

// jsFilterCase is one row of the table that both sides answer.
type jsFilterCase struct {
	Level string `json:"level"`
	Tag   string `json:"tag"`
	Debug bool   `json:"debug"`
	Info  bool   `json:"info"`
	Tags  string `json:"tags"`
	Line  string `json:"line"`
}

func TestLogFilterPortAgreesWithTheRealJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}

	levels := []logx.Level{logx.LevelDebug, logx.LevelInfo, logx.LevelError}
	tags := []logx.Tag{logx.Sync, logx.Assets}

	// THE FOUR STATES OF Config.LogTags, and each one is reachable.
	//
	// nil is a configuration that never held the key, and
	// config.NormalizeLogTags answers the whole default set for it. An EMPTY
	// slice is a person who unticked every box on the Config page, and
	// it answers an empty set. The two are different, and the first
	// draft of this test used nil where it meant empty. It then read a
	// disagreement that no page can meet.
	tagSets := [][]string{
		nil,
		{},
		{"sync"},
		{"sync", "assets"},
		{"assets"},
	}

	var cases []jsFilterCase
	var want []bool
	a := newTestApp(t)
	for _, lvl := range levels {
		for _, tag := range tags {
			for _, set := range tagSets {
				for _, debug := range []bool{true, false} {
					for _, info := range []bool{true, false} {
						cfg := config.Config{LogDebug: debug, LogInfo: info, LogTags: set}
						a.applyLogFilter(cfg)
						want = append(want, a.logLineEnabled(lvl, tag))
						cases = append(cases, jsFilterCase{
							Level: string(lvl), Tag: string(tag),
							Debug: debug, Info: info,
							// EXACTLY what injectRuntimeVars sends to the
							// page. A test that builds this value another
							// way compares the two sides against a state
							// that no browser ever holds.
							Tags: strings.Join(config.NormalizeLogTags(cfg.LogTags), ","),
							// The shape that logx.Logger.emit writes. See internal/logx/hub.go.
							Line: "2026/09/06 12:00:00 [" + string(tag) + "] (" +
								string(lvl) + ") a message",
						})
					}
				}
			}
		}
	}

	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatalf("cannot encode the cases: %v", err)
	}
	file := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatalf("cannot write the cases: %v", err)
	}

	script := `
const fs = require('fs');
const { newPage, run } = require('./frontend/test/page-stub.js');
const out = [];
for (const c of JSON.parse(fs.readFileSync(process.argv[1], 'utf8'))) {
    const page = newPage();
    page.OMN_LOG_DEBUG = c.debug;
    page.OMN_LOG_INFO = c.info;
    page.OMN_LOG_TAGS = c.tags;
    run(page, 'omn-go-core.js');
    run(page, 'omn-go-sse.js');
    if (typeof page.logLinePrints !== 'function') {
        process.stdout.write(JSON.stringify({ missing: true }));
        process.exit(0);
    }
    out.push(page.logLinePrints(c.line));
}
process.stdout.write(JSON.stringify({ got: out }));
`
	cmd := exec.Command(node, "-e", script, file)
	cmd.Dir = backendDir
	out, runErr := cmd.Output()
	if runErr != nil {
		t.Fatalf("node failed: %v\n%s", runErr, out)
	}
	var answer struct {
		Missing bool   `json:"missing"`
		Got     []bool `json:"got"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("node did not answer JSON: %v\n%s", err, out)
	}
	if answer.Missing {
		t.Fatal("omn-go-sse.js does not export logLinePrints, thus this test " +
			"cannot compare the two implementations.")
	}
	if len(answer.Got) != len(want) {
		t.Fatalf("node answered %d values for %d cases", len(answer.Got), len(want))
	}

	for i, c := range cases {
		if answer.Got[i] == want[i] {
			continue
		}
		t.Errorf("the two sides disagree for level %q, tag %q, debug=%v, info=%v, tags=%q.\n"+
			"  logLineEnabled in log_app.go says %v\n"+
			"  logLinePrints in omn-go-sse.js says %v\n"+
			"  A person then sees a quiet stdout and a loud console, or the reverse.",
			c.Level, c.Tag, c.Debug, c.Info, c.Tags, want[i], answer.Got[i])
	}
	t.Logf("%d cases, both sides agree", len(cases))
}
