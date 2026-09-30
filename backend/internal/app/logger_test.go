package app

// ---------------------------------------------------------------------
// The log transport and the level rule
//
// Two things are pinned here. The shape of a line, because the browser
// parses it. And the ban on log.Printf. A line that skips the Debugf, Infof
// and Errf methods of a logger carries no level. It therefore escapes every
// filter a person sets on the Config page.
// ---------------------------------------------------------------------

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// logPrintfAllowed names the only two files that may call log.Printf. No *App
// can reach either call site. render.LoadTemplate in
// internal/render/templates.go runs at package init. logAnchorsOff and
// addBookmarks in internal/search/sections.go run from a package-level function
// inside a sync.Once, and from a method on searchDocument, which has no
// application.
//
// Each of those lines is a fault, and a fault always prints, so the missing
// level costs the reader nothing. They write "(error)" in the text by hand,
// which the second half of this test checks.
var logPrintfAllowed = map[string]bool{
	"internal/render/templates.go": true,
	"internal/search/sections.go":  true,
}

// handWrittenLevelRe matches the shape those two files must produce:
// a bracketed tag, then "(error)", then the message.
var handWrittenLevelRe = regexp.MustCompile(`^log\.Printf\("\[[a-z0-9-]+\] \(error\) `)

// TestNoDirectLogPrintf exists because a log.Printf line reaches stdout and
// the browser with no tag and no level. The Config page can then never
// switch it off, and the person who asked for less noise still gets it.
//
// The scan reads each production file below backend/, thus a package of the
// split cannot hide a call.
func TestNoDirectLogPrintf(t *testing.T) {
	// A test file does not ship to a device, and productionGoFiles skips
	// it. This file names the banned call as a string.
	for _, name := range productionGoFiles(t) {
		src, err := readBackendFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "log.Printf(") {
				continue
			}
			if !logPrintfAllowed[name] {
				t.Errorf("%s:%d calls log.Printf. Use a.log(tag).Debugf, Infof "+
					"or Errf with a tag from internal/logx/levels.go. A line with no "+
					"level cannot be filtered, and the reader has no way to "+
					"switch it off.", name, i+1)
				continue
			}
			if !handWrittenLevelRe.MatchString(trimmed) {
				t.Errorf("%s:%d is an allowed log.Printf, but its text does not "+
					"start with \"[tag] (error) \". The browser reads that shape "+
					"to decide what to print.", name, i+1)
			}
		}
	}
}

// TestEmitLogLineShape pins the text the browser parses. omn-go-sse.js reads
// the tag and the level out of each line, and applySyncLogLine skips the
// level word before it matches a sync stage. A change here breaks both.
func TestEmitLogLineShape(t *testing.T) {
	a := newTestApp(t)

	ch := a.logs.Subscribe()
	defer a.logs.Unsubscribe(ch)

	a.log(logx.Sync).Debugf("Staging file: %s", "Note.md")
	a.log(logx.Assets).Infof("%d asset(s) refreshed", 3)
	a.log(logx.Edit).Errf("cannot run %q", "subl")

	want := []string{
		"[sync] (debug) Staging file: Note.md\n",
		"[assets] (info) 3 asset(s) refreshed\n",
		"[edit] (error) cannot run \"subl\"\n",
	}
	for _, w := range want {
		got := <-ch
		if !strings.HasSuffix(got, w) {
			t.Errorf("log line %q does not end with %q", got, w)
		}
		// The stamp is the standard log package layout, so stdout and the
		// stream look the same whatever wrote the line.
		if len(got) != len(w)+len(logx.TimeLayout) {
			t.Errorf("log line %q does not carry a %d-character time stamp",
				got, len(logx.TimeLayout))
		}
	}
}

// TestNormalizeLogTags pins the nil rule. An install that upgrades to this
// version has no log_tags key in config.json, and it must get every tag. A
// person who unticks every box gets an empty list, which is a different
// thing and must survive a save.
func TestNormalizeLogTags(t *testing.T) {
	if got := config.NormalizeLogTags(nil); len(got) != len(logx.AllTags) {
		t.Errorf("nil gave %d tags, want every one of the %d", len(got), len(logx.AllTags))
	}
	if got := config.NormalizeLogTags([]string{}); len(got) != 0 {
		t.Errorf("an empty list gave %v, want an empty list - unticking every box "+
			"is not the same as an upgrade with no key", got)
	}
	got := config.NormalizeLogTags([]string{"SYNC", " sync ", "not-a-tag", "assets"})
	want := []string{"assets", "sync"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("config.NormalizeLogTags gave %v, want %v - it lowercases, trims, "+
			"drops an unknown tag, and keeps the order of logx.AllTags", got, want)
	}
}

// TestLogLineEnabled pins the two axes. A fault always prints. A debug or an
// info line needs its level on and its tag ticked. A reader who asks for
// less noise never asks for fewer faults.
func TestLogLineEnabled(t *testing.T) {
	a := newTestApp(t)

	a.applyLogFilter(config.Config{LogDebug: false, LogInfo: false, LogTags: config.LogTagsDefault})
	if !a.logLineEnabled(logx.LevelError, logx.Sync) {
		t.Error("an error was filtered out with both levels off")
	}
	if a.logLineEnabled(logx.LevelDebug, logx.Sync) || a.logLineEnabled(logx.LevelInfo, logx.Sync) {
		t.Error("a quiet level printed with both levels off")
	}

	a.applyLogFilter(config.Config{LogDebug: true, LogInfo: true, LogTags: []string{"assets"}})
	if !a.logLineEnabled(logx.LevelDebug, logx.Assets) {
		t.Error("a ticked tag was filtered out with debug on")
	}
	if a.logLineEnabled(logx.LevelDebug, logx.Sync) {
		t.Error("an unticked tag printed with debug on")
	}
	if !a.logLineEnabled(logx.LevelError, logx.Sync) {
		t.Error("an error was filtered out by an unticked tag")
	}

	// Before loadConfig runs the cache is empty. Every line the application
	// writes that early is a fault, so faults only is the safe answer.
	fresh := &App{}
	if !fresh.logLineEnabled(logx.LevelError, logx.Server) {
		t.Error("an error was filtered out before the config loaded")
	}
	if fresh.logLineEnabled(logx.LevelInfo, logx.Server) {
		t.Error("an info line printed before the config loaded")
	}
}

// ----------------------------------------------------------------------
// The history ring
// ----------------------------------------------------------------------
//
// Each App holds its own ring in its logx.Hub. A test of the ring thus starts
// with an empty ring, and no other test writes into it.

// The ring keeps the lines in the order that they arrived.
func TestLogHistoryKeepsTheOrder(t *testing.T) {
	h := &logx.Hub{}
	for _, line := range []string{"first\n", "second\n", "third\n"} {
		h.Broadcast(line, false)
	}

	got := h.Snapshot()
	want := []string{"first\n", "second\n", "third\n"}
	if len(got) != len(want) {
		t.Fatalf("the ring holds %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// The ring writes over the oldest line, and it keeps the newest.
//
// A log that stops at its cap keeps the start of the session and loses
// the fault. The fault is the half that a person needs.
func TestLogHistoryKeepsTheNewestLines(t *testing.T) {
	h := &logx.Hub{}
	for i := 0; i < logx.HistoryCap+25; i++ {
		h.Broadcast(fmt.Sprintf("line %d\n", i), false)
	}

	got := h.Snapshot()
	if len(got) != logx.HistoryCap {
		t.Fatalf("the ring holds %d lines, want the cap of %d", len(got), logx.HistoryCap)
	}
	if want := fmt.Sprintf("line %d\n", 25); got[0] != want {
		t.Errorf("the oldest line is %q, want %q", got[0], want)
	}
	if want := fmt.Sprintf("line %d\n", logx.HistoryCap+24); got[len(got)-1] != want {
		t.Errorf("the newest line is %q, want %q", got[len(got)-1], want)
	}
}

// The ring holds a line that the stdout switches suppressed.
//
// A person who turned debug off and then met a fault needs the debug
// lines of that moment more than anybody. The switches say what a reader
// wants to SEE, and never what the application keeps.
func TestLogHistoryHoldsASuppressedLine(t *testing.T) {
	a := newTestApp(t)
	a.applyLogFilter(config.Config{LogDebug: false, LogInfo: false, LogTags: []string{}})

	if a.logLineEnabled(logx.LevelDebug, logx.Sync) {
		t.Fatal("the filter lets a debug line through, thus this test proves nothing")
	}
	a.log(logx.Sync).Debugf("a step that stdout never shows")

	for _, line := range a.logs.Snapshot() {
		if strings.Contains(line, "a step that stdout never shows") {
			return
		}
	}
	t.Error("the ring lost a line that the stdout filter suppressed")
}

// The snapshot is a copy. A caller that changes it changes no line of
// the ring.
func TestLogHistorySnapshotIsACopy(t *testing.T) {
	h := &logx.Hub{}
	h.Broadcast("the real line\n", false)

	first := h.Snapshot()
	if len(first) != 1 {
		t.Fatalf("the ring holds %d lines, want 1", len(first))
	}
	first[0] = "a line that a caller wrote"

	second := h.Snapshot()
	if second[0] != "the real line\n" {
		t.Errorf("the ring now holds %q, thus the snapshot shares its memory", second[0])
	}
}

// Two goroutines writing at once must not race, and no line may be lost.
//
// Run this one with -race. broadcast takes mu, and record runs under
// it. A ring outside that lock is a data race
// that a test without -race never reports.
func TestLogHistoryUnderConcurrentWriters(t *testing.T) {
	h := &logx.Hub{}
	const writers, each = 8, 20

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				h.Broadcast(fmt.Sprintf("writer %d line %d\n", w, i), false)
			}
		}(w)
	}
	// A reader at the same time, so the snapshot path is under the race
	// detector as well.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = h.Snapshot()
		}
	}()
	wg.Wait()

	got := h.Snapshot()
	if len(got) != writers*each {
		t.Fatalf("the ring holds %d lines, want %d. A line was lost.",
			len(got), writers*each)
	}
	seen := map[string]bool{}
	for _, line := range got {
		if seen[line] {
			t.Errorf("the line %q is in the ring two times", line)
		}
		seen[line] = true
	}
}

// ----------------------------------------------------------------------
// GET /api/logs/history
// ----------------------------------------------------------------------

// The endpoint answers the ring, oldest line first, as JSON.
func TestLogHistoryEndpointAnswersTheRing(t *testing.T) {
	a := newTestApp(t)
	for _, line := range []string{"first\n", "second\n", "third\n"} {
		a.logs.Broadcast(line, false)
	}

	rec := httptest.NewRecorder()
	a.handleLogHistory(rec, httptest.NewRequest(http.MethodGet, "/api/logs/history", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("the endpoint answered %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	var body struct {
		Status string   `json:"status"`
		Cap    int      `json:"cap"`
		Lines  []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Status != "success" {
		t.Errorf("the status is %q, want success. See section 1.4 of doc/API.md.", body.Status)
	}
	if body.Cap != logx.HistoryCap {
		t.Errorf("the answer names a cap of %d, want %d", body.Cap, logx.HistoryCap)
	}
	want := []string{"first\n", "second\n", "third\n"}
	if len(body.Lines) != len(want) {
		t.Fatalf("the answer holds %d lines, want %d: %q", len(body.Lines), len(want), body.Lines)
	}
	for i := range want {
		if body.Lines[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, body.Lines[i], want[i])
		}
	}
}

// An empty ring answers an array, and never null.
//
// A reader of the answer maps over lines without a guard, the same as
// the sync answers do. See newSyncConflict.
func TestLogHistoryEndpointAnswersAnArrayWhenEmpty(t *testing.T) {
	a := newTestApp(t)

	rec := httptest.NewRecorder()
	a.handleLogHistory(rec, httptest.NewRequest(http.MethodGet, "/api/logs/history", nil))
	if strings.Contains(rec.Body.String(), `"lines":null`) {
		t.Errorf("an empty ring answered %s, want an empty array", rec.Body.String())
	}
}

// The endpoint is ADMIN ONLY, and so is the stream beside it.
//
// A LAN share gives no log line, live or held. An open stream would make
// the guard on the ring useless. A remote caller who holds the stream open
// reads the same lines as the server writes them.
//
// This test drives the REAL registration through a real mux. A guard
// that registerRoutes forgets to wrap is then a failure here, and a test
// of the handler alone can never see that.
func TestLogHistoryEndpointIsAdminOnly(t *testing.T) {
	a := newTestApp(t)
	mux := http.NewServeMux()
	a.registerRoutes(mux)

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // the local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // an admin of the LAN
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/logs/history", nil)
		req.RemoteAddr = c.remote
		if c.cookie != "" {
			req.AddCookie(sessionCookie(t, a, c.cookie))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s with the cookie %q answered %d, want %d",
				c.remote, c.cookie, rec.Code, c.want)
		}
	}
}

// The stream is ADMIN ONLY as well.
//
// A log line names a note, a remote, a path and a fault. A remote caller
// reads none of them.
//
// This test drives the REAL registration through a real mux, the same as
// the history test above. The request of each allowed case carries a
// context that is already canceled, because handleLogsSSE blocks until
// the context ends.
func TestLogStreamIsAdminOnly(t *testing.T) {
	a := newTestApp(t)
	mux := http.NewServeMux()
	a.registerRoutes(mux)

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // the local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // an admin of the LAN
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	before := countLogClients(a)
	for _, c := range cases {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil).WithContext(ctx)
		req.RemoteAddr = c.remote
		if c.cookie != "" {
			req.AddCookie(sessionCookie(t, a, c.cookie))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s with the cookie %q answered %d, want %d",
				c.remote, c.cookie, rec.Code, c.want)
		}
	}

	// A refused request must never reach the handler, thus it must
	// register no client of the stream.
	if got := countLogClients(a); got != before {
		t.Errorf("the stream holds %d clients and it held %d before", got, before)
	}
}

// The stream keeps its own address, and the history pattern shadows it
// not at all.
//
// Both are exact patterns of a ServeMux. A trailing slash on either one
// would make it a subtree and take the other address with it.
func TestLogStreamKeepsItsOwnAddress(t *testing.T) {
	a := newTestApp(t)
	rec := &routeRecorder{}
	a.registerRoutes(rec)

	var found int
	for _, p := range rec.patterns {
		switch p {
		case "/api/logs", "/api/logs/history":
			found++
		case "/api/logs/":
			t.Error("the stream is registered as a subtree, thus it takes " +
				"/api/logs/history with it")
		}
	}
	if found != 2 {
		t.Errorf("the two log patterns are not both registered: %d of 2", found)
	}
}

// ----------------------------------------------------------------------
// The Log page
// ----------------------------------------------------------------------
//
// /OMNGoLogs.html holds no log line of its own. It reads
// /api/logs/history one time, and then it adds each new line of
// /api/logs. omn-go-logs.js does that work.
//
// These two tests are the pair that the Status page carries, for the same
// two reasons. See internal/status/status_test.go.

// The page must reach both addresses and carry no line of its own.
func TestLogsPageIsAReaderOfTheTwoAddresses(t *testing.T) {
	a := newTestApp(t)

	// httptest.NewRequest gives each request the address 192.0.2.1, which
	// is another machine as far as hasRole is concerned. The owner of the
	// device connects from the loopback address, and that is the request
	// this test makes. See isLocalConnection in middleware.go.
	req := httptest.NewRequest(http.MethodGet, "/OMNGoLogs.html", nil)
	req.RemoteAddr = "127.0.0.1:41000"

	rec := httptest.NewRecorder()
	a.serveLogsPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"/api/logs/history", "lgReload", "lgCopy", "lgFilterToggle",
		"lgLevels", "lgTags", "lgBody",
		"/js/OMN-Go/omn-go-logs.js", "/css/OMN-Go/omn-go-logs.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
	// The body of the page starts empty. Each line comes from an address,
	// thus the template can carry no log line of its own.
	if !strings.Contains(body, "Loading…") {
		t.Error("the page does not start empty. It must read the ring.")
	}
	// The two sync buttons of the shell are what lets a reader start a
	// sync here and watch it. On Android there is one screen, thus a
	// second page is not an answer. See the banner of omn-go-logs.js.
	if !strings.Contains(body, "syncAction('upload')") {
		t.Error("the page lost the Upload button of the shell")
	}
	if !strings.Contains(body, "syncAction('download')") {
		t.Error("the page lost the Download button of the shell")
	}
	// The script and the stylesheet are files. An inline script or an
	// inline style would cost its bytes in this template alone, and the
	// project keeps the two apart. See section 4 of CLAUDE.md.
	if strings.Contains(logsPageTmpl, "<script>") {
		t.Error("the template holds an inline script")
	}
	if strings.Contains(logsPageTmpl, "<style>") {
		t.Error("the template holds an inline style")
	}
}

// A remote caller gets a page and not a line of plain text.
//
// The route carries no authMiddleware for that reason. See page_access.go.
func TestLogsPageAnswersARemoteCallerWithAPage(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.ShareLAN = true })

	req := httptest.NewRequest(http.MethodGet, "/OMNGoLogs.html", nil)
	req.RemoteAddr = "192.168.1.44:51000" // another machine on the network
	// A signed cookie, and not the bare word "guest". The server refuses
	// an unsigned value. See session.go.
	req.AddCookie(sessionCookie(t, a, "guest"))

	rec := routeServe(a, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want a page", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "for the admin of this device") {
		t.Error("a remote caller did not get the refusal page")
	}
	if strings.Contains(body, "lgReload") {
		t.Error("a remote caller got the reader script")
	}
}

// Each App writes into its own hub. A line of one App never reaches the
// ring or the stream of another App.
func TestEachAppHasItsOwnLog(t *testing.T) {
	a, b := newTestApp(t), newTestApp(t)
	ch := b.logs.Subscribe()
	defer b.logs.Unsubscribe(ch)

	a.log(logx.Sync).Errf("a line of the first App")
	for _, line := range b.logs.Snapshot() {
		if strings.Contains(line, "a line of the first App") {
			t.Fatal("the ring of the second App holds a line of the first App")
		}
	}
	select {
	case line := <-ch:
		t.Errorf("the stream of the second App got %q", line)
	default:
	}
}
