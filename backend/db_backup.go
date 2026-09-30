package backend

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ----------------------------------------------------------------------
// Whole-database JSONL backups of the user databases
// ----------------------------------------------------------------------
//
// An automatic copy of each table, at each push and open, made the sync of
// devices fragile: an old cache could overwrite newly pulled data. This
// design is the opposite:
//
//	- MANUAL. The /db_backups page and its /api/db/* endpoints make and
//	  restore a backup. The one exception is the bootstrap: a database with
//	  backups and no .sqlite file restores the newest backup on open.
//	- WHOLE DATABASE. One .jsonl file holds the header, each schema object,
//	  the sqlite_sequence values and each row.
//	- IMMUTABLE. The code writes a backup one time and never changes it. The
//	  name holds a time stamp and the Hostname of the config. git thus sees
//	  only adds and deletes, and two devices never write one path.
//	- FULL REPLACE. A restore builds a temporary .sqlite file and renames it
//	  over the real one after it evicts the cached handle.
//
// The layout is
// <StorageDir>/html/db_backup/<db>/<UTCtimestamp>_<hostname>.jsonl. Under
// html/, the server sends it, thus a download link works, and git tracks it.
// A database named local-* stays out of git through the local-only name rule.
// See storage.LocalOnlyPrefix in internal/storage/paths.go.
//
// The file format, version 2, has one JSON object on each line:
//
//	{"format":"omn-db-backup","version":2,"database":"mydata",
//	 "created":"2026-07-14T10:30:00Z","hostname":"pixel7",
//	 "objects":4,"rows":123}
//	{"kind":"table","name":"t","sql":"CREATE TABLE ...","columns":["a","b"]}
//	{"kind":"index","name":"i1","sql":"CREATE INDEX ..."}
//	{"kind":"view","name":"v1","sql":"CREATE VIEW ..."}
//	{"kind":"seq","table":"t","value":41}
//	{"kind":"row","table":"t","v":[1,"x"]}
//	{"kind":"trigger","name":"tr1","table":"t","sql":"CREATE TRIGGER ..."}
//
// A number keeps its exact value, and json.Number keeps an int64 on read. A
// BLOB, and TEXT that is not valid UTF-8, get the form {"b64":"..."}. One
// line for each row keeps a git diff small. A git conflict marker fails the
// parse of its line, and the restore applies nothing.

const (
	backupFormatName    = "omn-db-backup"
	backupFormatVersion = 2
	backupMaxLineBytes  = 10 << 20 // same 10MB row cap the old loader used
)

// backupFileRe checks a backup file name that reaches the restore endpoint.
// The endpoint builds a path from it, thus this is the traversal guard. It
// also filters a directory listing. See backupNewerThan for the order.
var backupFileRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z(_[0-9]+)?_[A-Za-z0-9_-]{1,64}\.jsonl$`)

// backupOrderRe reads the time stamp and the counter of a backup name.
var backupOrderRe = regexp.MustCompile(`^([0-9]{8}T[0-9]{6}Z)(?:_([0-9]+))?_`)

// backupNewerThan tells whether backup name a is newer than b. The time stamp
// decides first, and then the counter. createDBBackup gives no counter to the
// first backup of a second, 2 to the second, and so on, thus no counter
// counts as 1. The full name decides last, thus the order is stable.
//
// The string order is wrong: "..Z_2_host" sorts before "..Z_host", and
// "..Z_10_host" before "..Z_9_host". The prune would then remove the newest
// backup. TestListBackupFilesNewestFirst holds the rule.
func backupNewerThan(a, b string) bool {
	sa, ca := backupOrder(a)
	sb, cb := backupOrder(b)
	if sa != sb {
		return sa > sb
	}
	if ca != cb {
		return ca > cb
	}
	return a > b
}

// backupOrder answers the time stamp and the counter of one backup name. No
// counter gives 1.
func backupOrder(name string) (stamp string, counter int) {
	m := backupOrderRe.FindStringSubmatch(name)
	if m == nil {
		return "", 0
	}
	counter = 1
	if m[2] != "" {
		if n, err := strconv.Atoi(m[2]); err == nil {
			counter = n
		}
	}
	return m[1], counter
}

func dbBackupRoot(a *App) string {
	return a.layout().HTML("db_backup")
}
func (a *App) dbBackupDir(name string) string {
	return filepath.Join(dbBackupRoot(a), name)
}
func (a *App) userDBPath(name string) string {
	return a.layout().DB(name + ".sqlite")
}

// relStoragePath changes an absolute path under StorageDir into the relative
// form with slashes that git status uses.
func (a *App) relStoragePath(full string) string {
	rel, err := filepath.Rel(a.StorageDir, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(rel)
}

// quoteIdent puts an SQL identifier into a statement that cannot use a
// placeholder for it, for example DDL or PRAGMA. Each identifier here comes
// from sqlite_master or PRAGMA table_info, thus this is a second guard.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// ----------------------------------------------------------------------
// The file format
// ----------------------------------------------------------------------

type backupHeader struct {
	Format   string `json:"format"`
	Version  int    `json:"version"`
	Database string `json:"database"`
	Created  string `json:"created"`
	Hostname string `json:"hostname"`
	Objects  int    `json:"objects"`
	Rows     int    `json:"rows"`
}

type backupLine struct {
	Kind    string        `json:"kind"`
	Name    string        `json:"name,omitempty"`
	Table   string        `json:"table,omitempty"`
	SQL     string        `json:"sql,omitempty"`
	Columns []string      `json:"columns,omitempty"`
	Value   int64         `json:"value,omitempty"`
	V       []interface{} `json:"v,omitempty"`
}
