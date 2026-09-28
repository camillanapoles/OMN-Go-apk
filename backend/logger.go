package backend

// ----------------------------------------------------------------------
// The log transport
// ----------------------------------------------------------------------
//
// logHub.broadcast is the only fan-out. It sends each line to THREE places:
// stdout, the /api/logs stream and the history ring. stdout is for a desktop
// user and for adb logcat. Each open page reads the stream, copies it into
// the browser console, and the sync overlay reads its stage text there. See
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// Each App holds one logHub. Two callers reach its broadcast:
//
//	JSLogger.Write  The standard log package, for the two call sites that
//	                cannot reach an *App. See TestNoDirectLogPrintf.
//	logger.emit     Each other line, through debugf, infof or errf.
//
// THE STREAM ALWAYS CARRIES EACH LINE. The sync overlay needs the "[sync]"
// debug lines also when a reader asks for less. The browser can also change
// its filter with no restart. The switches thus control stdout only.
//
// The stream is a live sample. A client with a full channel loses the line,
// and the writer never waits. The stream must thus never drive a state that
// needs each event.

import (
	"fmt"
	"log"
	"sync"
	"time"
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

// logHub holds the stream clients and the history ring of one App. mu keeps
// two lines apart, and it guards each field.
type logHub struct {
	mu      sync.Mutex
	clients []chan string
	history [logHistoryCap]string
	next    int
	count   int
}

// record writes one line into the ring. The caller holds mu. When the ring
// is full, the new line replaces the oldest one. A log that stops at a limit
// keeps the start and loses the fault.
func (h *logHub) record(msg string) {
	h.history[h.next] = msg
	h.next = (h.next + 1) % logHistoryCap
	if h.count < logHistoryCap {
		h.count++
	}
}

// snapshot answers a COPY of the ring, oldest line first. The caller reads
// it without the lock while a writer adds lines.
func (h *logHub) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]string, 0, h.count)
	start := (h.next - h.count + logHistoryCap) % logHistoryCap
	for i := 0; i < h.count; i++ {
		out = append(out, h.history[(start+i)%logHistoryCap])
	}
	return out
}

// logTimeLayout is the time prefix of the standard log package with
// log.LstdFlags. logger.emit writes the stamp itself. Both sources must look the
// same, or the page must parse two shapes.
const logTimeLayout = "2006/01/02 15:04:05 "

// broadcast sends one line to each stream client and to the ring, and to
// stdout when toStdout is true. It holds mu, thus two lines cannot mix.
func (h *logHub) broadcast(msg string, toStdout bool) {
	h.mu.Lock()
	h.record(msg)
	for _, c := range h.clients {
		select {
		case c <- msg:
		default:
		}
	}
	if toStdout {
		fmt.Print(msg)
	}
	h.mu.Unlock()
}

// subscribe adds one stream client. The channel holds 10 lines.
func (h *logHub) subscribe() chan string {
	ch := make(chan string, 10)
	h.mu.Lock()
	h.clients = append(h.clients, ch)
	h.mu.Unlock()
	return ch
}

// unsubscribe removes one stream client.
func (h *logHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, c := range h.clients {
		if c == ch {
			h.clients = append(h.clients[:i], h.clients[i+1:]...)
			return
		}
	}
}

// clientCount answers the number of stream clients.
func (h *logHub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// JSLogger sends the lines of the standard log package to the hub of one App.
type JSLogger struct{ hub *logHub }

func (l *JSLogger) Write(p []byte) (n int, err error) {
	// A line from the standard log package has no level. It always goes to
	// stdout, because no filter applies to it.
	l.hub.broadcast(string(p), true)
	return len(p), nil
}

// emit makes one line "[tag] (level) message", stamps it, and gives it to
// the hub. It is the only writer of a line with a level.
func (l logger) emit(lvl logLevel, format string, args ...any) {
	line := time.Now().Format(logTimeLayout) +
		"[" + string(l.tag) + "] (" + string(lvl) + ") " +
		fmt.Sprintf(format, args...) + "\n"
	l.hub.broadcast(line, l.enabled(lvl))
}

// logFilter is the cached form of Config.LogDebug, Config.LogInfo and
// Config.LogTags.
type logFilter struct {
	debug bool
	info  bool
	tags  map[logTag]bool
}

// logLineEnabled tells whether one line reaches stdout and the browser
// console. An error always does. A debug or info line needs its level on and
// its tag checked. Before loadConfig runs, the cache is empty and allows
// faults only, the same as a fresh install.
func (a *App) logLineEnabled(lvl logLevel, tag logTag) bool {
	return a.log(tag).enabled(lvl)
}

// enabled is the test of logLineEnabled for the tag of l.
func (l logger) enabled(lvl logLevel) bool {
	if lvl == levelError {
		return true
	}
	f, ok := l.filter.Load().(logFilter)
	if !ok {
		return false
	}
	if lvl == levelDebug && !f.debug {
		return false
	}
	if lvl == levelInfo && !f.info {
		return false
	}
	return f.tags[l.tag]
}

// initLogger sends the standard logger into the /api/logs stream.
// registerRoutes in server.go registers the route. The function is not
// exported. See section 3 of CLAUDE.md for the exported names.
func (a *App) initLogger() {
	log.SetOutput(&JSLogger{hub: &a.logs})
}
