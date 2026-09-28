package backend

import "sync/atomic"

// ----------------------------------------------------------------------
// Log tags and log levels
// ----------------------------------------------------------------------
//
// Each log line goes to stdout, to the /api/logs stream and to the history
// ring. Each open page copies the stream into the browser console. See
// logger.go and
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// A reader of the console can ask for less, thus a line has two properties:
//
//	tag    The subsystem that wrote the line: one of the constants below.
//	level  How much the reader wants it: debug, info or error.
//
// The text is "[tag] (level) message". The debugf, infof and errf methods of
// a logger write the brackets and the parentheses, thus a format string never
// holds them.
//
// THIS FILE IS THE ONLY AUTHORITY FOR THE TAG SET. Add a new tag to the
// constant block and to allLogTags together. The Config page makes its
// checkboxes from allLogTags. TestAllLogTagsIsComplete holds the rule.
//
//	error  A fault: an operation failed, the server refused it, or it is not
//	       available.
//	       The reader must know, whatever the configuration says.
//	info   The result of an operation that a person asked for.
//	debug  One step inside an operation, for the search for a fault.

// logTag names the subsystem that wrote a line.
type logTag string

// This is the tag set. The value is the text between the brackets.
const (
	log404         logTag = "404"
	logAssets      logTag = "assets"
	logConfig      logTag = "config"
	logDB          logTag = "db"
	logDBBackup    logTag = "db-backup"
	logDBBootstrap logTag = "db-bootstrap"
	logDBRestore   logTag = "db-restore"
	logEdit        logTag = "edit"
	logExchange    logTag = "exchange"
	logNoteFiles   logTag = "note-files"
	logPage        logTag = "page"
	logPrecompile  logTag = "precompile"
	logRestart     logTag = "restart"
	logSearch      logTag = "search"
	logServer      logTag = "server"
	logSession     logTag = "session"
	logStatus      logTag = "status"
	logStorage     logTag = "storage"
	logSync        logTag = "sync"
	logTags        logTag = "tags"
	logTemplates   logTag = "templates"
	logUpload      logTag = "upload"
)

// allLogTags holds each tag, in the order of the Config page.
// normalizeLogTags in config.go keeps only these tags, thus config.json
// cannot keep a tag that the page does not show.
var allLogTags = []logTag{
	log404,
	logAssets,
	logConfig,
	logDB,
	logDBBackup,
	logDBBootstrap,
	logDBRestore,
	logEdit,
	logExchange,
	logNoteFiles,
	logPage,
	logPrecompile,
	logRestart,
	logSearch,
	logServer,
	logSession,
	logStatus,
	logStorage,
	logSync,
	logTags,
	logTemplates,
	logUpload,
}

// logLevel tells how much the reader wants a line.
type logLevel string

const (
	levelDebug logLevel = "debug"
	levelInfo  logLevel = "info"
	levelError logLevel = "error"
)

// logger writes the lines of one tag. It holds the tag, the filter of the
// configuration and the hub, thus a function that gets a logger needs no
// *App.
type logger struct {
	tag    logTag
	filter *atomic.Value // the logFilter cache of the App
	hub    *logHub       // the logs of the App
}

// log gives the logger of one tag.
func (a *App) log(tag logTag) logger {
	return logger{tag: tag, filter: &a.logFilter, hub: &a.logs}
}

// debugf writes one step of an operation. It is off on a fresh install.
func (l logger) debugf(format string, args ...any) { l.emit(levelDebug, format, args...) }

// infof writes the result of an operation. It is off on a fresh install.
func (l logger) infof(format string, args ...any) { l.emit(levelInfo, format, args...) }

// errf writes a fault. This level has no switch: a person who asks for less
// noise never asks for fewer faults. The parentheses already hold "error",
// thus do not write "Error:" or "Warning:" in the message.
func (l logger) errf(format string, args ...any) { l.emit(levelError, format, args...) }
