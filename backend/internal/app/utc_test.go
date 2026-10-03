package app

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/noteheader"
)

// ----------------------------------------------------------------------
// Each time is UTC
// ----------------------------------------------------------------------
//
// THE FAULT THAT THIS FILE HOLDS. The desktop wrote each time in the zone
// of the computer. Go on Android has no zone file, thus Android wrote UTC.
// One note then held two kinds of time after a sync. The Modified line of
// the desktop was hours away from the same moment on the phone.
//
// A build machine in UTC cannot see that fault. Each test here thus moves
// the local zone of the process eleven hours away from UTC first.

// useFarLocalZone sets the local zone of the process to UTC+11 for one
// test. No test of this package runs in parallel, thus the change is safe.
func useFarLocalZone(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("far", 11*60*60)
	t.Cleanup(func() { time.Local = old })
}

// wantUTCNow fails when text is not the present moment in UTC. The limit is
// one minute. A time in the zone of useFarLocalZone is eleven hours away.
func wantUTCNow(t *testing.T, what, layout, text string) {
	t.Helper()
	got, err := time.ParseInLocation(layout, text, time.UTC)
	if err != nil {
		t.Fatalf("%s: %q is not a time of the form %q: %v", what, text, layout, err)
	}
	if d := time.Since(got); d < -time.Minute || d > time.Minute {
		t.Errorf("%s is %q, which is %v away from now in UTC. The time is in the "+
			"zone of the computer. Use noteheader.Stamp, or .UTC() before Format.",
			what, text, d.Round(time.Minute))
	}
}

var stampRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)

func TestQuickNoteStampIsUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	p := baseWriteMD(t, a, "QuickNotes.md", "Title: Quick Notes\nCategory: Log\n\n")
	if rec := postForm(t, a.handleQuickNote, "/api/quick", url.Values{"note": {"a thought"}}); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	src, _ := os.ReadFile(p)
	m := regexp.MustCompile(`##### (.*)\n`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("the note has no heading with a time:\n%s", src)
	}
	wantUTCNow(t, "the heading of a quick note", noteheader.StampLayout, string(m[1]))
}

func TestBookmarkDateIsUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	const marker = "<!-- Don't edit body below this line -->"
	p := baseWriteMD(t, a, "Bookmarks.md",
		"Title: Bookmarks\n\n<script>bookmarks = [\n"+marker+"\n];\n</script>")
	if rec := postForm(t, a.handleBookmark, "/api/bookmark",
		url.Values{"url": {"https://example.com/"}, "title": {"Example"}}); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	src, _ := os.ReadFile(p)
	text := string(src)
	start := strings.Index(text, "{")
	end := strings.Index(text, "},")
	if start < 0 || end < start {
		t.Fatalf("the note holds no bookmark:\n%s", text)
	}
	var bm struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &bm); err != nil {
		t.Fatal(err)
	}
	wantUTCNow(t, "the date of a bookmark", noteheader.StampLayout, bm.Date)
	// The save of the bookmark also sets the Modified line of the note.
	if m := regexp.MustCompile(`(?m)^Modified: (.*)$`).FindStringSubmatch(text); m != nil {
		wantUTCNow(t, "the Modified line of Bookmarks.md", noteheader.StampLayout, m[1])
	} else {
		t.Errorf("Bookmarks.md has no Modified line:\n%s", text)
	}
}

func TestSavedNoteModifiedIsUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	if rec := postForm(t, a.handleSaveNote, "/api/save", url.Values{
		"name":    {"Saved"},
		"content": {"Title: Saved\nDate: 2026-01-01 00:00:00\n\nBody\n"},
	}); rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	src, err := os.ReadFile(a.layout().MD("Saved.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^Modified: (.*)$`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("the saved note has no Modified line:\n%s", src)
	}
	wantUTCNow(t, "the Modified line of a saved note", noteheader.StampLayout, string(m[1]))
}

func TestNewNoteDateIsUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	// A page that does not exist yet gets a start text with a Date line.
	if rec := getPage(t, a, "/FreshNote.html"); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	src, err := os.ReadFile(a.layout().MD("FreshNote.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^Date: (.*)$`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("the new note has no Date line:\n%s", src)
	}
	wantUTCNow(t, "the Date line of a new note", noteheader.StampLayout, string(m[1]))
}

func TestNotFoundPageTimeIsUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	rec := getPage(t, a, "/no/such/file.png")
	body := rec.Body.String()
	at := strings.Index(body, "<dt>Time</dt>")
	if at < 0 {
		t.Skipf("the answer %d is not the page of a file that does not exist", rec.Code)
	}
	stamp := stampRe.FindString(body[at:])
	wantUTCNow(t, "the time of the page for a file that does not exist", noteheader.StampLayout, stamp)
	if !strings.Contains(body[at:], stamp+" UTC") {
		t.Error("the page shows the time with no zone. A person on the desktop " +
			"reads it as the time of the computer.")
	}
}

// The log page shows the lines of two sources: Logger.emit and the standard
// log package. Both must write UTC, or the lines of one page are not in
// the order of their times.
func TestLogLinesAreUTC(t *testing.T) {
	useFarLocalZone(t)
	a := newTestApp(t)
	a.initLogger()
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })

	a.log(logx.Page).Errf("a line of the logger")
	log.Printf("a line of the standard package")

	seen := 0
	for _, line := range a.logs.Snapshot() {
		for _, text := range []string{"a line of the logger", "a line of the standard package"} {
			if !strings.Contains(line, text) {
				continue
			}
			seen++
			if len(line) < len(logx.TimeLayout) {
				t.Fatalf("the line %q has no time", line)
			}
			wantUTCNow(t, text, logx.TimeLayout, line[:len(logx.TimeLayout)])
		}
	}
	if seen != 2 {
		t.Errorf("the ring holds %d of the two lines", seen)
	}
}
