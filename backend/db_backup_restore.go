package backend

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
				a.log(logDBRestore).errf("%s: sequence for %s not restorable: %v", name, s.Table, err)
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

	a.log(logDBRestore).infof("%s: restored from %s (%d objects, %d rows)",
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
	a.log(logDBBootstrap).infof("%s: no database file yet, restoring newest backup %s", name, files[0])
	if err := a.restoreDBFromBackup(name, files[0]); err != nil {
		return nil, err
	}
	return a.openUserDBLocked(name)
}
