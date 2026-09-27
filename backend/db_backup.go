package backend

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
// See localOnlyPrefix in git_repo.go.
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
	return filepath.Join(a.StorageDir, "html", "db_backup")
}
func (a *App) dbBackupDir(name string) string {
	return filepath.Join(dbBackupRoot(a), name)
}
func (a *App) userDBPath(name string) string {
	return filepath.Join(a.StorageDir, "db", name+".sqlite")
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

// ----------------------------------------------------------------------
// Create
// ----------------------------------------------------------------------

// backupTableInfo answers the column names in their order, and for each
// column whether its declared type holds "BLOB". The affinity rules of SQLite
// use the same test.
func backupTableInfo(tx *sql.Tx, table string) (cols []string, blob []bool, err error) {
	rows, err := tx.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
	if err != nil {
		return nil, nil, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, nil, err
		}
		cols = append(cols, name)
		blob = append(blob, strings.Contains(strings.ToUpper(ctype), "BLOB"))
	}
	return cols, blob, rows.Err()
}

// encodeBackupValue maps a scanned value to its JSON form. A []byte becomes
// {"b64":...} in a BLOB column, or when the bytes are not valid UTF-8,
// because JSON cannot carry raw bytes. Each other []byte becomes a string.
func encodeBackupValue(v interface{}, blobCol bool) interface{} {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	if blobCol || !utf8.Valid(b) {
		return map[string]string{"b64": base64.StdEncoding.EncodeToString(b)}
	}
	return string(b)
}

// createDBBackup writes a new backup file of database name and prunes the
// backups above the configured depth. It answers the relative path of the new
// file and of each pruned file. The whole read runs in one transaction, thus
// the copy is consistent while note scripts write.
func (a *App) createDBBackup(name string) (created string, pruned []string, err error) {
	db, err := a.openUserDB(name)
	if err != nil {
		return "", nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return "", nil, fmt.Errorf("begin snapshot: %w", err)
	}
	defer tx.Rollback() // read-only tx; rollback is the cheap way out

	objRows, err := tx.Query(`
		SELECT type, name, tbl_name, sql FROM sqlite_master
		WHERE type IN ('table','view','trigger','index')
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	if err != nil {
		return "", nil, fmt.Errorf("list objects: %w", err)
	}
	type dbObj struct{ kind, name, table, sqlText string }
	var objs []dbObj
	for objRows.Next() {
		var o dbObj
		var sqlText sql.NullString
		if err := objRows.Scan(&o.kind, &o.name, &o.table, &sqlText); err != nil {
			objRows.Close()
			return "", nil, err
		}
		o.sqlText = sqlText.String
		if o.sqlText == "" {
			continue // implicit index (PK/UNIQUE) - recreated by CREATE TABLE itself
		}
		objs = append(objs, o)
	}
	objRows.Close()
	if err := objRows.Err(); err != nil {
		return "", nil, err
	}

	var body bytes.Buffer
	writeLine := func(v interface{}) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		body.Write(data)
		body.WriteByte('\n')
		return nil
	}

	objects, totalRows := 0, 0
	tables := map[string][]string{} // name -> columns, for the row pass
	blobs := map[string][]bool{}

	// Write the schema first: tables, indexes, views, triggers. The restore
	// groups by kind anyway, and this order keeps the file readable.
	for _, phase := range []string{"table", "index", "view", "trigger"} {
		for _, o := range objs {
			if o.kind != phase {
				continue
			}
			line := backupLine{Kind: o.kind, Name: o.name, SQL: o.sqlText}
			if o.kind == "trigger" {
				line.Table = o.table
			}
			if o.kind == "table" {
				cols, blob, err := backupTableInfo(tx, o.name)
				if err != nil {
					return "", nil, err
				}
				line.Columns = cols
				tables[o.name] = cols
				blobs[o.name] = blob
			}
			if err := writeLine(line); err != nil {
				return "", nil, err
			}
			objects++
		}
	}

	// sqlite_sequence exists only when a table uses AUTOINCREMENT. The backup
	// keeps it, thus a restored database never gives out an id again.
	if seqRows, err := tx.Query(`SELECT name, seq FROM sqlite_sequence`); err == nil {
		for seqRows.Next() {
			var tbl string
			var seq int64
			if err := seqRows.Scan(&tbl, &seq); err != nil {
				seqRows.Close()
				return "", nil, err
			}
			if err := writeLine(backupLine{Kind: "seq", Table: tbl, Value: seq}); err != nil {
				seqRows.Close()
				return "", nil, err
			}
		}
		seqRows.Close()
	}

	// Write one line for each row, in the rowid order of each table.
	for _, o := range objs {
		if o.kind != "table" {
			continue
		}
		cols := tables[o.name]
		blob := blobs[o.name]
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = quoteIdent(c)
		}
		dataRows, err := tx.Query(`SELECT ` + strings.Join(quoted, ",") + ` FROM ` + quoteIdent(o.name))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", o.name, err)
		}
		for dataRows.Next() {
			raw := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := dataRows.Scan(ptrs...); err != nil {
				dataRows.Close()
				return "", nil, err
			}
			for i, v := range raw {
				raw[i] = encodeBackupValue(v, blob[i])
			}
			if err := writeLine(backupLine{Kind: "row", Table: o.name, V: raw}); err != nil {
				dataRows.Close()
				return "", nil, err
			}
			totalRows++
		}
		if err := dataRows.Err(); err != nil {
			dataRows.Close()
			return "", nil, err
		}
		dataRows.Close()
	}

	// The header is line 1, and the code builds it last, because it holds the
	// counts. The list endpoint then reads one line for the metadata.
	cfg := a.GetConfig()
	host := sanitizeHostname(cfg.Hostname)
	if host == "" {
		host = defaultHostname()
	}
	header := backupHeader{
		Format:   backupFormatName,
		Version:  backupFormatVersion,
		Database: name,
		Created:  time.Now().UTC().Format(time.RFC3339),
		Hostname: host,
		Objects:  objects,
		Rows:     totalRows,
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", nil, err
	}

	dir := a.dbBackupDir(name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("create backup directory: %w", err)
	}

	// The name holds a time stamp. On a collision in one second, add a
	// counter, and never overwrite a backup.
	stamp := time.Now().UTC().Format("20060102T150405") + "Z"
	fileName := stamp + "_" + host + ".jsonl"
	target := filepath.Join(dir, fileName)
	for n := 2; ; n++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		fileName = fmt.Sprintf("%s_%d_%s.jsonl", stamp, n, host)
		target = filepath.Join(dir, fileName)
	}

	tmp := target + ".tmp"
	out := append(append(headerBytes, '\n'), body.Bytes()...)
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return "", nil, fmt.Errorf("write backup: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", nil, fmt.Errorf("finalize backup: %w", err)
	}

	// The database now equals this backup. Give the .sqlite file the mtime of
	// the backup, thus the state dot of the page shows "in sync".
	if info, err := os.Stat(target); err == nil {
		if err := os.Chtimes(a.userDBPath(name), info.ModTime(), info.ModTime()); err != nil && !os.IsNotExist(err) {
			a.logErrf(logDBBackup, "touch %s.sqlite: %v", name, err)
		}
	}

	pruned, err = a.pruneDBBackups(name)
	if err != nil {
		// The backup worked. A prune fault does not fail the request.
		a.logErrf(logDBBackup, "prune %s: %v", name, err)
		err = nil
	}
	return a.relStoragePath(target), pruned, nil
}

// listBackupFiles answers the backup file names of name, newest first.
func (a *App) listBackupFiles(name string) ([]string, error) {
	entries, err := os.ReadDir(a.dbBackupDir(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !backupFileRe.MatchString(e.Name()) {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Slice(files, func(i, j int) bool { return backupNewerThan(files[i], files[j]) })
	return files, nil
}

// pruneDBBackups removes the backups above the configured depth and keeps the
// newest. It answers the relative path of each removed file. git carries the
// deletions of a tracked database. For a local-* database, they are final.
func (a *App) pruneDBBackups(name string) ([]string, error) {
	depth := a.GetConfig().BackupPruneDepth
	if depth <= 0 {
		depth = 3
	}
	files, err := a.listBackupFiles(name)
	if err != nil {
		return nil, err
	}
	var removed []string
	for i := depth; i < len(files); i++ {
		full := filepath.Join(a.dbBackupDir(name), files[i])
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.logErrf(logDBBackup, "prune %s: %v", files[i], err)
			continue
		}
		a.logInfof(logDBBackup, "%s: pruned %s", name, files[i])
		removed = append(removed, a.relStoragePath(full))
	}
	return removed, nil
}

// ----------------------------------------------------------------------
// Restore
// ----------------------------------------------------------------------

// readBackupHeader reads and checks only the first line of a backup file. It
// is cheap enough for each file on each page load.
func readBackupHeader(path string) (backupHeader, error) {
	var h backupHeader
	f, err := os.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return h, fmt.Errorf("empty backup file")
	}
	if err := json.Unmarshal(bytes.TrimSpace(line), &h); err != nil {
		return h, fmt.Errorf("bad header: %w", err)
	}
	if h.Format != backupFormatName || h.Version != backupFormatVersion {
		return h, fmt.Errorf("unsupported format %q version %d", h.Format, h.Version)
	}
	return h, nil
}

// decodeBackupValue reverses encodeBackupValue for one value that UseNumber
// decoded. A json.Number becomes an exact int64 or a float64, and {"b64":...}
// becomes []byte.
func decodeBackupValue(v interface{}) (interface{}, error) {
	switch t := v.(type) {
	case json.Number:
		if iv, err := t.Int64(); err == nil {
			return iv, nil
		}
		if fv, err := t.Float64(); err == nil {
			return fv, nil
		}
		return t.String(), nil
	case map[string]interface{}:
		enc, ok := t["b64"].(string)
		if !ok || len(t) != 1 {
			return nil, fmt.Errorf("unrecognized tagged value %v", t)
		}
		return base64.StdEncoding.DecodeString(enc)
	default:
		return v, nil
	}
}

// restoreDBFromBackup replaces database name with the content of the backup
// file fileName in the backup directory of that database. It loads the backup
// into a temporary .sqlite file first. It renames that file over the real one
// only after each statement worked. A damaged file thus changes nothing.
//
// The caller must hold a.dbRestoreMu, as bootstrapIfMissing and
// handleDBRestore do. This function must not take it.
func (a *App) restoreDBFromBackup(name, fileName string) error {
	if !dbNameRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if !backupFileRe.MatchString(fileName) {
		return fmt.Errorf("invalid backup filename %q", fileName)
	}
	backupPath := filepath.Join(a.dbBackupDir(name), fileName)

	f, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), backupMaxLineBytes)

	if !scanner.Scan() {
		return fmt.Errorf("empty backup file")
	}
	var header backupHeader
	if err := json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &header); err != nil {
		return fmt.Errorf("bad header: %w", err)
	}
	if header.Format != backupFormatName || header.Version != backupFormatVersion {
		return fmt.Errorf("unsupported format %q version %d", header.Format, header.Version)
	}
	if header.Database != name {
		return fmt.Errorf("backup is for database %q, not %q", header.Database, name)
	}

	// Parse the whole file first, grouped by kind. A parse error on line N
	// thus stops the restore before it touches a database file.
	var tables, indexes, views, triggers, seqs []backupLine
	type tableRows struct {
		cols []string
		rows [][]interface{}
	}
	data := map[string]*tableRows{}
	order := []string{}
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var line backupLine
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&line); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
		switch line.Kind {
		case "table":
			tables = append(tables, line)
			data[line.Name] = &tableRows{cols: line.Columns}
			order = append(order, line.Name)
		case "index":
			indexes = append(indexes, line)
		case "view":
			views = append(views, line)
		case "trigger":
			triggers = append(triggers, line)
		case "seq":
			seqs = append(seqs, line)
		case "row":
			tr, ok := data[line.Table]
			if !ok {
				return fmt.Errorf("line %d: row for unknown table %q", lineNum, line.Table)
			}
			if len(line.V) != len(tr.cols) {
				return fmt.Errorf("line %d: %d values, expected %d", lineNum, len(line.V), len(tr.cols))
			}
			vals := make([]interface{}, len(line.V))
			for i, v := range line.V {
				dv, err := decodeBackupValue(v)
				if err != nil {
					return fmt.Errorf("line %d: %w", lineNum, err)
				}
				vals[i] = dv
			}
			tr.rows = append(tr.rows, vals)
		default:
			return fmt.Errorf("line %d: unknown kind %q", lineNum, line.Kind)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Build the new database in a temporary file beside the real one.
	finalPath := a.userDBPath(name)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	tmpPath := finalPath + ".restoretmp"
	os.Remove(tmpPath)
	tmpDB, err := sql.Open("sqlite",
		"file:"+tmpPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("open temp database: %w", err)
	}
	tmpDB.SetMaxOpenConns(1)
	cleanup := func() {
		tmpDB.Close()
		os.Remove(tmpPath)
	}

	tx, err := tmpDB.Begin()
	if err != nil {
		cleanup()
		return fmt.Errorf("begin restore: %w", err)
	}

	apply := func(kind string, lines []backupLine) error {
		for _, l := range lines {
			if _, err := tx.Exec(l.SQL); err != nil {
				return fmt.Errorf("create %s %s: %w", kind, l.Name, err)
			}
		}
		return nil
	}
	fail := func(err error) error {
		tx.Rollback()
		cleanup()
		return err
	}

	if err := apply("table", tables); err != nil {
		return fail(err)
	}
	if err := apply("view", views); err != nil {
		return fail(err)
	}
	for _, tbl := range order { // data before indexes (bulk-load faster) and triggers (must not fire)
		tr := data[tbl]
		if len(tr.rows) == 0 {
			continue
		}
		quoted := make([]string, len(tr.cols))
		for i, c := range tr.cols {
			quoted[i] = quoteIdent(c)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tr.cols)), ",")
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteIdent(tbl), strings.Join(quoted, ","), placeholders)
		for _, vals := range tr.rows {
			if _, err := tx.Exec(insertSQL, vals...); err != nil {
				return fail(fmt.Errorf("insert into %s: %w", tbl, err))
			}
		}
	}
	if err := apply("index", indexes); err != nil {
		return fail(err)
	}
	for _, s := range seqs {
		// The AUTOINCREMENT tables already have sqlite_sequence rows. Write
		// the saved counter over them, thus a deleted id is never given out
		// again.
		res, err := tx.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = ?`, s.Value, s.Table)
		if err != nil {
			return fail(fmt.Errorf("restore sequence for %s: %w", s.Table, err))
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err := tx.Exec(`INSERT INTO sqlite_sequence(name, seq) VALUES (?, ?)`, s.Table, s.Value); err != nil {
				// With no AUTOINCREMENT table, the saved counter has no row.
				// Skip it, and do not fail.
				a.logErrf(logDBRestore, "%s: sequence for %s not restorable: %v", name, s.Table, err)
			}
		}
	}
	if err := apply("trigger", triggers); err != nil {
		return fail(err)
	}

	if err := tx.Commit(); err != nil {
		cleanup()
		return fmt.Errorf("commit restore: %w", err)
	}
	if err := tmpDB.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp database: %w", err)
	}

	// Evict the cached handle first, thus no connection keeps the old file.
	// Then rename. A /api/sql batch at the same time recovers through its
	// stale-handle retry.
	a.evictUserDB(name)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("swap database: %w", err)
	}

	// Give the .sqlite file the mtime of the restored backup, because the two
	// now hold the same content. A restore of an older backup then shows
	// "newer backup exists".
	if info, err := os.Stat(backupPath); err == nil {
		os.Chtimes(finalPath, info.ModTime(), info.ModTime())
	}

	a.logInfof(logDBRestore, "%s: restored from %s (%d objects, %d rows)",
		name, fileName, header.Objects, header.Rows)
	return nil
}

// bootstrapIfMissing is the one automatic restore. When a database has a
// backup and no .sqlite file, the newest backup IS the database, for example
// on a fresh device after a pull. Nothing local can be lost. It answers a new
// handle after a restore, because the swap evicted the handle of the caller.
// Else it answers nil.
func (a *App) bootstrapIfMissing(name string) (*sql.DB, error) {
	a.dbRestoreMu.Lock()
	defer a.dbRestoreMu.Unlock()

	if info, err := os.Stat(a.userDBPath(name)); err == nil && info.Size() > 0 {
		return nil, nil
	}
	files, err := a.listBackupFiles(name)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	a.logInfof(logDBBootstrap, "%s: no database file yet, restoring newest backup %s", name, files[0])
	if err := a.restoreDBFromBackup(name, files[0]); err != nil {
		return nil, err
	}
	return a.openUserDBLocked(name)
}

// ----------------------------------------------------------------------
// The HTTP endpoints and the page
// ----------------------------------------------------------------------

var dbBackupsPageTmpl = loadTemplate("db_backups.html")

// serveDBBackupsPage renders the Database Backups page. The button at the top
// of the Config page opens it. The page gets its data from GET
// /api/db/backups, thus the template needs no fill().
func (a *App) serveDBBackupsPage(w http.ResponseWriter, r *http.Request) {
	writeHTMLHeader(w)
	compiled := a.compilePageWithBody("DB_Backups", []byte("Title: Database Backups\nCategory: Settings\n\n"), dbBackupsPageTmpl)
	w.Write(a.injectRuntimeVars(compiled))
}

// handleDBBackupCreate answers POST /api/db/backup?db=NAME.
func (a *App) handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	if !dbNameRe.MatchString(name) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	file, pruned, err := a.createDBBackup(name)
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"file":   file,
		"pruned": pruned,
	})
}

// handleDBRestore answers POST /api/db/restore?db=NAME&file=FILENAME.
func (a *App) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	fileName := r.URL.Query().Get("file")
	if !dbNameRe.MatchString(name) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	if !backupFileRe.MatchString(fileName) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid backup filename %q", fileName))
		return
	}
	a.dbRestoreMu.Lock()
	err := a.restoreDBFromBackup(name, fileName)
	a.dbRestoreMu.Unlock()
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

type backupFileView struct {
	File     string `json:"file"`
	Size     int64  `json:"size"`
	MTime    string `json:"mtime"`
	Created  string `json:"created"`
	Hostname string `json:"hostname"`
	Objects  int    `json:"objects"`
	Rows     int    `json:"rows"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

type backupDBView struct {
	Name         string           `json:"name"`
	SQLiteExists bool             `json:"sqlite_exists"`
	Size         int64            `json:"size"`
	MTime        string           `json:"mtime"`
	State        string           `json:"state"`
	Backups      []backupFileView `json:"backups"`
}

// handleDBBackupList answers GET /api/db/backups with each value of the
// /db_backups page. It never opens a database, because an open can start the
// bootstrap restore, and a listing must change nothing.
func (a *App) handleDBBackupList(w http.ResponseWriter, r *http.Request) {

	// Take each database that has a .sqlite file or only backups, as on a
	// fresh device before the first open.
	names := map[string]bool{}
	if entries, err := os.ReadDir(filepath.Join(a.StorageDir, "db")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sqlite") {
				n := strings.TrimSuffix(e.Name(), ".sqlite")
				if dbNameRe.MatchString(n) {
					names[n] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(dbBackupRoot(a)); err == nil {
		for _, e := range entries {
			if e.IsDir() && dbNameRe.MatchString(e.Name()) {
				names[e.Name()] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	depth := a.GetConfig().BackupPruneDepth
	if depth <= 0 {
		depth = 3
	}

	dbs := make([]backupDBView, 0, len(sorted))
	for _, name := range sorted {
		v := backupDBView{Name: name}
		// Keep the raw mtime for the state test below. The RFC3339 text in
		// v.MTime has a precision of one second. A test against it would show
		// "backup newer" directly after a backup, where createDBBackup made
		// the two mtimes equal.
		var dbMTime time.Time
		if info, err := os.Stat(a.userDBPath(name)); err == nil && info.Size() > 0 {
			v.SQLiteExists = true
			v.Size = info.Size()
			dbMTime = info.ModTime()
			v.MTime = dbMTime.UTC().Format(time.RFC3339)
		}

		files, _ := a.listBackupFiles(name)
		var newestMTime time.Time
		newestValid := false
		for i, fn := range files {
			full := filepath.Join(a.dbBackupDir(name), fn)
			bv := backupFileView{File: fn}
			if info, err := os.Stat(full); err == nil {
				bv.Size = info.Size()
				bv.MTime = info.ModTime().UTC().Format(time.RFC3339)
				if i == 0 {
					newestMTime = info.ModTime()
				}
			}
			if h, err := readBackupHeader(full); err == nil && h.Database == name {
				bv.Valid = true
				bv.Created = h.Created
				bv.Hostname = h.Hostname
				bv.Objects = h.Objects
				bv.Rows = h.Rows
				if i == 0 {
					newestValid = true
				}
			} else if err != nil {
				bv.Error = err.Error()
			} else {
				bv.Error = fmt.Sprintf("header names database %q", h.Database)
			}
			v.Backups = append(v.Backups, bv)
		}
		if v.Backups == nil {
			// A database with no backup must give "backups":[]. A nil slice
			// gives JSON null, and the page script fails on .length.
			v.Backups = []backupFileView{}
		}

		switch {
		case len(files) == 0:
			v.State = "none"
		case !newestValid:
			v.State = "invalid"
		case !v.SQLiteExists:
			v.State = "missing"
		default:
			switch {
			case newestMTime.After(dbMTime):
				v.State = "backup_newer"
			case dbMTime.After(newestMTime):
				v.State = "dirty"
			default:
				v.State = "insync"
			}
		}
		dbs = append(dbs, v)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "success",
		"hostname":    a.GetConfig().Hostname,
		"prune_depth": depth,
		"databases":   dbs,
	})
}
