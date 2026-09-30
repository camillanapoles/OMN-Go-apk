package logx

import "sync/atomic"

// ----------------------------------------------------------------------
// Log tags and log levels
// ----------------------------------------------------------------------
//
// Each log line goes to stdout, to the /api/logs stream and to the history
// ring. Each open page copies the stream into the browser console. See
// hub.go and
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// A reader of the console can ask for less, thus a line has two properties:
//
//	tag    The subsystem that wrote the line: one of the constants below.
//	level  How much the reader wants it: debug, info or error.
//
// The text is "[tag] (level) message". The Debugf, Infof and Errf methods of
// a logger write the brackets and the parentheses, thus a format string never
// holds them.
//
// THIS FILE IS THE ONLY AUTHORITY FOR THE TAG SET. Add a new tag to the
// constant block and to AllTags together. The Config page makes its
// checkboxes from AllTags. TestAllTagsIsComplete holds the rule.
//
//	error  A fault: an operation failed, the server refused it, or it is not
//	       available.
//	       The reader must know, whatever the configuration says.
//	info   The result of an operation that a person asked for.
//	debug  One step inside an operation, for the search for a fault.

// Tag names the subsystem that wrote a line.
type Tag string

// This is the tag set. The value is the text between the brackets.
const (
	NotFound    Tag = "404"
	Assets      Tag = "assets"
	Config      Tag = "config"
	DB          Tag = "db"
	DBBackup    Tag = "db-backup"
	DBBootstrap Tag = "db-bootstrap"
	DBRestore   Tag = "db-restore"
	Edit        Tag = "edit"
	Exchange    Tag = "exchange"
	NoteFiles   Tag = "note-files"
	Page        Tag = "page"
	Precompile  Tag = "precompile"
	Restart     Tag = "restart"
	Search      Tag = "search"
	Server      Tag = "server"
	Session     Tag = "session"
	Status      Tag = "status"
	Storage     Tag = "storage"
	Sync        Tag = "sync"
	Tags        Tag = "tags"
	Templates   Tag = "templates"
	Upload      Tag = "upload"
)

// AllTags holds each tag, in the order of the Config page.
// config.NormalizeLogTags in internal/config/config.go keeps only these tags,
// thus config.json cannot keep a tag that the page does not show.
var AllTags = []Tag{
	NotFound,
	Assets,
	Config,
	DB,
	DBBackup,
	DBBootstrap,
	DBRestore,
	Edit,
	Exchange,
	NoteFiles,
	Page,
	Precompile,
	Restart,
	Search,
	Server,
	Session,
	Status,
	Storage,
	Sync,
	Tags,
	Templates,
	Upload,
}

// Level tells how much the reader wants a line.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelError Level = "error"
)

// Logger writes the lines of one tag. It holds the tag, the filter of the
// configuration and the hub, thus a function that gets a Logger needs no
// *App.
type Logger struct {
	tag    Tag
	filter *atomic.Value // the logFilter cache of the App
	hub    *Hub          // the logs of the App
}

// New answers the logger of tag. filter holds the Filter cache of the
// App, and hub holds its logs.
func New(tag Tag, filter *atomic.Value, hub *Hub) Logger {
	return Logger{tag: tag, filter: filter, hub: hub}
}

// Debugf writes one step of an operation. It is off on a fresh install.
func (l Logger) Debugf(format string, args ...any) { l.emit(LevelDebug, format, args...) }

// Infof writes the result of an operation. It is off on a fresh install.
func (l Logger) Infof(format string, args ...any) { l.emit(LevelInfo, format, args...) }

// Errf writes a fault. This level has no switch: a person who asks for less
// noise never asks for fewer faults. The parentheses already hold "error",
// thus do not write "Error:" or "Warning:" in the message.
func (l Logger) Errf(format string, args ...any) { l.emit(LevelError, format, args...) }
