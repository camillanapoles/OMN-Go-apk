package backend

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

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
			a.log(logDBBackup).errf("touch %s.sqlite: %v", name, err)
		}
	}

	pruned, err = a.pruneDBBackups(name)
	if err != nil {
		// The backup worked. A prune fault does not fail the request.
		a.log(logDBBackup).errf("prune %s: %v", name, err)
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
			a.log(logDBBackup).errf("prune %s: %v", files[i], err)
			continue
		}
		a.log(logDBBackup).infof("%s: pruned %s", name, files[i])
		removed = append(removed, a.relStoragePath(full))
	}
	return removed, nil
}
