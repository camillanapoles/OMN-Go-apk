package backend

// ----------------------------------------------------------------------
// The log transport
// ----------------------------------------------------------------------
//
// broadcastLogLine is the only fan-out. It sends each line to THREE places:
// stdout, the /api/logs stream and the history ring. stdout is for a desktop
// user and for adb logcat. Each open page reads the stream, copies it into
// the browser console, and the sync overlay reads its stage text there. See
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// Two callers reach broadcastLogLine:
//
//	JSLogger.Write  The standard log package, for the two call sites that
//	                cannot reach an *App. See TestNoDirectLogPrintf.
//	App.emitLog     Each other line, through logDebugf, logInfof or logErrf.
//
// THE STREAM ALWAYS CARRIES EACH LINE. The sync overlay needs the "[sync]"
// debug lines also when a reader asks for less. The browser can also change
// its filter with no restart. The switches thus control stdout only.
//
// The stream is a live sample. A client with a full channel loses the line,
// and the writer never waits. The stream must thus never drive a state that
// needs each event.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	logMutex   sync.Mutex
	logClients []chan string
)

// ----------------------------------------------------------------------
// The history ring
// ----------------------------------------------------------------------
//
// A page that opens after an event never sees its lines on the stream. The
// ring holds the last logHistoryCap lines, and /api/logs/history answers with
// them.
//
// THE RING DOES NOT REPLAY ON THE STREAM. applySyncLogLine in omn-go-sse.js
// reads the "[sync] (debug)" lines of the stream. A replay would show an old
// sync on each page load.
//
// THE RING HOLDS EACH LINE, the same as the stream. A person who turned debug
// off and then met a fault needs those debug lines most.
//
// 500 lines of about 120 bytes use about 60 KB for the life of the process.
// That size suits a phone. It is a constant and not a setting.

// logHistoryCap is the number of lines that the ring holds.
const logHistoryCap = 500

var (
	logHistory      [logHistoryCap]string
	logHistoryNext  int
	logHistoryCount int
)

// recordLogLine writes one line into the ring. The caller holds logMutex.
// When the ring is full, the new line replaces the oldest one. A log that
// stops at a limit keeps the start and loses the fault.
func recordLogLine(msg string) {
	logHistory[logHistoryNext] = msg
	logHistoryNext = (logHistoryNext + 1) % logHistoryCap
	if logHistoryCount < logHistoryCap {
		logHistoryCount++
	}
}

// logHistorySnapshot answers a COPY of the ring, oldest line first. The
// caller reads it without the lock while a writer adds lines.
func logHistorySnapshot() []string {
	logMutex.Lock()
	defer logMutex.Unlock()

	out := make([]string, 0, logHistoryCount)
	start := (logHistoryNext - logHistoryCount + logHistoryCap) % logHistoryCap
	for i := 0; i < logHistoryCount; i++ {
		out = append(out, logHistory[(start+i)%logHistoryCap])
	}
	return out
}

// logTimeLayout is the time prefix of the standard log package with
// log.LstdFlags. emitLog writes the stamp itself. Both sources must look the
// same, or the page must parse two shapes.
const logTimeLayout = "2006/01/02 15:04:05 "

// broadcastLogLine sends one line to each stream subscriber and to the ring,
// and to stdout when toStdout is true. It holds logMutex, thus two lines
// cannot mix.
func broadcastLogLine(msg string, toStdout bool) {
	logMutex.Lock()
	recordLogLine(msg)
	for _, c := range logClients {
		select {
		case c <- msg:
		default:
		}
	}
	if toStdout {
		fmt.Print(msg)
	}
	logMutex.Unlock()
}

type JSLogger struct{}

func (l *JSLogger) Write(p []byte) (n int, err error) {
	// A line from the standard log package has no level. It always goes to
	// stdout, because no filter applies to it.
	broadcastLogLine(string(p), true)
	return len(p), nil
}

// emitLog makes one line "[tag] (level) message", stamps it, and gives it to
// broadcastLogLine. It is the only writer of a line with a level.
func (a *App) emitLog(lvl logLevel, tag logTag, format string, args ...any) {
	line := time.Now().Format(logTimeLayout) +
		"[" + string(tag) + "] (" + string(lvl) + ") " +
		fmt.Sprintf(format, args...) + "\n"
	broadcastLogLine(line, a.logLineEnabled(lvl, tag))
}

// logFilter is the cached form of Config.LogDebug, Config.LogInfo and
// Config.LogTags.
type logFilter struct {
	debug bool
	info  bool
	tags  map[logTag]bool
}

// applyLogFilter caches the log switches of one configuration.
//
// A LOG LINE MUST NEVER TAKE THE CONFIG LOCK. loadConfig holds the write lock
// and can write a log line, and a Go RWMutex is not reentrant. A read of the
// config from emitLog would thus deadlock the start. An atomic value costs
// one load for each line. loadConfig and the POST branch of handleConfig
// refresh the cache.
func (a *App) applyLogFilter(c Config) {
	f := logFilter{
		debug: c.LogDebug,
		info:  c.LogInfo,
		tags:  make(map[logTag]bool, len(allLogTags)),
	}
	for _, t := range normalizeLogTags(c.LogTags) {
		f.tags[logTag(t)] = true
	}
	a.logFilter.Store(f)
}

// logLineEnabled tells whether one line reaches stdout and the browser
// console. An error always does. A debug or info line needs its level on and
// its tag checked. Before loadConfig runs, the cache is empty and allows
// faults only, the same as a fresh install.
func (a *App) logLineEnabled(lvl logLevel, tag logTag) bool {
	if lvl == levelError {
		return true
	}
	f, ok := a.logFilter.Load().(logFilter)
	if !ok {
		return false
	}
	if lvl == levelDebug && !f.debug {
		return false
	}
	if lvl == levelInfo && !f.info {
		return false
	}
	return f.tags[tag]
}

// initLogger sends the standard logger into the /api/logs stream.
// registerRoutes in server.go registers the route. The function is not
// exported. See section 3 of CLAUDE.md for the exported names.
func (a *App) initLogger() {
	log.SetOutput(&JSLogger{})
}

func (a *App) HandleLogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 10)
	logMutex.Lock()
	logClients = append(logClients, ch)
	logMutex.Unlock()

	defer func() {
		logMutex.Lock()
		for i, c := range logClients {
			if c == ch {
				logClients = append(logClients[:i], logClients[i+1:]...)
				break
			}
		}
		logMutex.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// logsDeniedBody is the page that a guest sees, not the line of plain text of
// authMiddleware.
const logsDeniedBody = `<div class="config-panel">` +
	`<h2 class="config-title">Log</h2>` +
	`<p class="config-hint">This page is for the admin of this device. ` +
	`Log in as admin on a note page, then open the page again.</p>` +
	`</div>`

// serveLogsPage answers /OMNGoLogs.html. The page reads /api/logs/history one
// time, and then it adds each new line of /api/logs. omn-go-logs.js does that
// work.
//
// Android has no terminal. Without this page, a person on a phone needs adb
// logcat, or a second browser at the history endpoint. The route follows
// serveStatusPage, and it asks hasRole itself. See statusDeniedBody.
func (a *App) serveLogsPage(w http.ResponseWriter, r *http.Request) {
	body := logsPageTmpl
	if !a.hasRole(r) {
		body = logsDeniedBody
	}
	compiled := a.compilePageWithBody("Log",
		[]byte("Title: Log\nCategory: System\n\n"), body)
	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars(compiled))
}

// handleLogHistory answers the ring of the last logHistoryCap lines, oldest
// first. It is a separate endpoint, because a replay on /api/logs breaks the
// sync overlay. See the banner of the ring.
//
// IT IS ADMIN ONLY, and so is /api/logs. A LAN guest reads no log line. See
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// The answer follows section 1.4 of doc/API.md: JSON with a status word.
func (a *App) handleLogHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	lines := logHistorySnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"cap":    logHistoryCap,
		"lines":  lines,
	})
}
