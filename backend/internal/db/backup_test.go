package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/storage"
	"net.basov.omngo/backend/internal/testkit"
)

// These tests cover the whole-database JSONL backups of internal/db/backup*.go.
// They test the round trip, the full replace, trigger safety and the endpoints.
// They also test indexes, sqlite_sequence, BLOBs, int64 values, the prune, the
// bootstrap of a fresh device and the refusal of a damaged file.
//
// Each helper here has the prefix dbb, thus it cannot collide with a helper
// of another test file.

func dbbApp(t *testing.T) *testApp {
	t.Helper()
	return &testApp{App: &testkit.App{StorageDir: t.TempDir()}}
}

func dbbExec(t *testing.T, a *testApp, db, stmt string, args ...interface{}) {
	t.Helper()
	h, err := a.databases().Open(db)
	if err != nil {
		t.Fatalf("open %s: %v", db, err)
	}
	if _, err := h.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

func dbbQueryInt(t *testing.T, a *testApp, db, query string) int64 {
	t.Helper()
	h, err := a.databases().Open(db)
	if err != nil {
		t.Fatalf("open %s: %v", db, err)
	}
	var n int64
	if err := h.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// dbbBackup creates a backup and returns the bare backup filename.
func dbbBackup(t *testing.T, a *testApp, db string) string {
	t.Helper()
	rel, _, err := a.databases().CreateBackup(db)
	if err != nil {
		t.Fatalf("Service.CreateBackup(%s): %v", db, err)
	}
	return filepath.Base(rel)
}

func dbbRestore(t *testing.T, a *testApp, db, file string) {
	t.Helper()
	err := a.databases().Restore(db, file)
	if err != nil {
		t.Fatalf("Service.Restore(%s, %s): %v", db, file, err)
	}
}

func TestDBBackupRoundTripSchemaAndData(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE items(id INTEGER PRIMARY KEY, txt TEXT, num REAL)`)
	dbbExec(t, a, "t1", `CREATE INDEX idx_items_txt ON items(txt)`)
	dbbExec(t, a, "t1", `CREATE VIEW v_items AS SELECT txt FROM items WHERE num > 1`)
	dbbExec(t, a, "t1", `INSERT INTO items VALUES (1, 'hello; "world"', 2.5)`)
	dbbExec(t, a, "t1", `INSERT INTO items VALUES (2, ?, 0.5)`, "unicode: привет ✓\nsecond line")

	file := dbbBackup(t, a, "t1")

	// Wreck the live data, then restore.
	dbbExec(t, a, "t1", `DELETE FROM items`)
	dbbExec(t, a, "t1", `DROP INDEX idx_items_txt`)
	dbbRestore(t, a, "t1", file)

	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM items`); n != 2 {
		t.Fatalf("row count after restore = %d, want 2", n)
	}
	h, _ := a.databases().Open("t1")
	var txt string
	if err := h.QueryRow(`SELECT txt FROM items WHERE id = 2`).Scan(&txt); err != nil {
		t.Fatalf("read restored row: %v", err)
	}
	if txt != "unicode: привет ✓\nsecond line" {
		t.Fatalf("restored text mismatch: %q", txt)
	}
	// Index and view must be recreated (the old engine silently lost indexes).
	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_items_txt'`); n != 1 {
		t.Fatalf("index not restored")
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM v_items`); n != 1 {
		t.Fatalf("view not restored or wrong content: %d", n)
	}
}

func TestDBBackupRestoreIsFullReplace(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE keep(a)`)
	file := dbbBackup(t, a, "t1")

	// Created AFTER the backup - must not survive a restore.
	dbbExec(t, a, "t1", `CREATE TABLE extra(b)`)
	dbbRestore(t, a, "t1", file)

	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='extra'`); n != 0 {
		t.Fatalf("table created after backup survived the restore")
	}
	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='keep'`); n != 1 {
		t.Fatalf("backed-up table missing after restore")
	}
}

func TestDBBackupRestoreDoesNotFireTriggers(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE items(id INTEGER PRIMARY KEY, v TEXT)`)
	dbbExec(t, a, "t1", `CREATE TABLE audit(n INTEGER)`)
	dbbExec(t, a, "t1", `CREATE TRIGGER tr AFTER INSERT ON items BEGIN INSERT INTO audit VALUES (new.id); END`)
	dbbExec(t, a, "t1", `INSERT INTO items(v) VALUES ('a'), ('b')`)

	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 2 {
		t.Fatalf("precondition: audit = %d, want 2", n)
	}

	file := dbbBackup(t, a, "t1")
	dbbRestore(t, a, "t1", file)

	// Bulk data load must not have fired the trigger again: audit rows
	// come only from the backup's own data.
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 2 {
		t.Fatalf("audit after restore = %d, want 2 (trigger fired during data load?)", n)
	}
	// ... but the trigger itself must work again after the restore.
	dbbExec(t, a, "t1", `INSERT INTO items(v) VALUES ('c')`)
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 3 {
		t.Fatalf("trigger not functional after restore: audit = %d, want 3", n)
	}
}

func TestDBBackupPreservesSequenceAndBigIntsAndBlobs(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE s(id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)`)
	dbbExec(t, a, "t1", `INSERT INTO s(v) VALUES ('a'), ('b'), ('c')`)
	dbbExec(t, a, "t1", `DELETE FROM s WHERE id = 3`)
	dbbExec(t, a, "t1", `CREATE TABLE nums(big INTEGER)`)
	dbbExec(t, a, "t1", `INSERT INTO nums VALUES (9007199254740993)`) // 2^53 + 1: dies in float64
	blob := []byte{0x00, 0x01, 0xFF, 0xFE, 0x80}
	dbbExec(t, a, "t1", `CREATE TABLE bin(data BLOB)`)
	dbbExec(t, a, "t1", `INSERT INTO bin VALUES (?)`, blob)

	file := dbbBackup(t, a, "t1")
	dbbRestore(t, a, "t1", file)

	// AUTOINCREMENT sequence preserved: next id must be 4, never a reuse of 3.
	dbbExec(t, a, "t1", `INSERT INTO s(v) VALUES ('d')`)
	if got := dbbQueryInt(t, a, "t1", `SELECT MAX(id) FROM s`); got != 4 {
		t.Fatalf("AUTOINCREMENT id after restore = %d, want 4 (sequence lost)", got)
	}
	if got := dbbQueryInt(t, a, "t1", `SELECT big FROM nums`); got != 9007199254740993 {
		t.Fatalf("big integer mangled by restore: %d", got)
	}
	h, _ := a.databases().Open("t1")
	var back []byte
	if err := h.QueryRow(`SELECT data FROM bin`).Scan(&back); err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if !bytes.Equal(back, blob) {
		t.Fatalf("blob mangled by restore: % x", back)
	}
}

func TestDBBackupPruneKeepsNewest(t *testing.T) {
	a := dbbApp(t)
	a.Config.Update(func(c *config.Config) { c.BackupPruneDepth = 2 })
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)

	var files []string
	for i := 0; i < 3; i++ {
		dbbExec(t, a, "t1", `INSERT INTO x VALUES (?)`, i) // content change per backup
		// No sleep between the backups. Before BackupNewerThan, this
		// test waited more than one second for each backup, because the
		// prune kept the wrong files in one second.
		files = append(files, dbbBackup(t, a, "t1"))
	}

	left, err := a.databases().ListBackupFiles("t1")
	if err != nil {
		t.Fatalf("Service.ListBackupFiles: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("after 3 backups with depth 2, %d files remain: %v", len(left), left)
	}
	if left[0] != files[2] || left[1] != files[1] {
		t.Fatalf("prune kept wrong files: have %v, want [%s %s]", left, files[2], files[1])
	}
	if _, err := os.Stat(filepath.Join(a.databases().BackupDir("t1"), files[0])); !os.IsNotExist(err) {
		t.Fatalf("oldest backup %s not pruned", files[0])
	}
}

func TestDBBackupBootstrapRestoresMissingDatabase(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (41), (42)`)
	dbbBackup(t, a, "t1")

	// Simulate a fresh device: backups exist, the .sqlite cache does not.
	a.databases().Evict("t1")
	if err := os.Remove(a.databases().UserDBPath("t1")); err != nil {
		t.Fatalf("remove sqlite: %v", err)
	}

	// The very first open must transparently restore the newest backup.
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 2 {
		t.Fatalf("bootstrap restore missing: count = %d, want 2", n)
	}
}

func TestDBRestoreRejectsDamagedAndForeignFiles(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (1)`)
	good := dbbBackup(t, a, "t1")

	// A copy with a git-conflict-marker line must be rejected whole, and
	// the live database must stay untouched.
	raw, err := os.ReadFile(filepath.Join(a.databases().BackupDir("t1"), good))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	damagedName := "99991231T235959Z_corrupt.jsonl"
	damaged := append(append([]byte{}, raw...), []byte("<<<<<<< HEAD\n")...)
	if err := os.WriteFile(filepath.Join(a.databases().BackupDir("t1"), damagedName), damaged, 0644); err != nil {
		t.Fatalf("write damaged copy: %v", err)
	}
	err = a.databases().Restore("t1", damagedName)
	if err == nil {
		t.Fatalf("damaged backup accepted")
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 1 {
		t.Fatalf("failed restore must not touch the database: count = %d", n)
	}

	// A backup whose header names another database must be rejected.
	dbbExec(t, a, "other", `CREATE TABLE y(b)`)
	otherFile := dbbBackup(t, a, "other")
	src := filepath.Join(a.databases().BackupDir("other"), otherFile)
	dst := filepath.Join(a.databases().BackupDir("t1"), "99991231T235958Z_foreign.jsonl")
	data, _ := os.ReadFile(src)
	os.WriteFile(dst, data, 0644)
	err = a.databases().Restore("t1", "99991231T235958Z_foreign.jsonl")
	if err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("foreign-database backup not rejected properly: %v", err)
	}

	// Path traversal / invalid names never reach the filesystem.
	err = a.databases().Restore("t1", "../../../etc/passwd")
	if err == nil {
		t.Fatalf("invalid backup filename accepted")
	}
}

func TestDBBackupEndpoints(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (7)`)
	// A second database that never gets a backup: it must still list
	// cleanly (fresh-install case - see the "backups":null regression).
	dbbExec(t, a, "nobak", `CREATE TABLE y(b)`)

	// POST /api/db/backup?db=t1
	rec := httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/db/backup?db=t1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("backup endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Status string `json:"status"`
		File   string `json:"file"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Status != "success" {
		t.Fatalf("backup endpoint response: %s (%v)", rec.Body.String(), err)
	}

	// GET /api/db/backups
	rec = httptest.NewRecorder()
	a.handleDBBackupList(rec, httptest.NewRequest(http.MethodGet, "/api/db/backups", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Status    string `json:"status"`
		Databases []struct {
			Name    string `json:"name"`
			State   string `json:"state"`
			Backups []struct {
				File  string `json:"file"`
				Valid bool   `json:"valid"`
				Rows  int    `json:"rows"`
			} `json:"backups"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list endpoint JSON: %v", err)
	}
	var file string
	for _, d := range listed.Databases {
		if d.Name != "t1" {
			continue
		}
		if len(d.Backups) != 1 || !d.Backups[0].Valid || d.Backups[0].Rows != 1 {
			t.Fatalf("list endpoint content: %+v", d)
		}
		if d.State != "insync" {
			t.Fatalf("state right after backup = %q, want insync", d.State)
		}
		file = d.Backups[0].File
	}
	if file == "" {
		t.Fatalf("database t1 missing from list: %s", rec.Body.String())
	}
	// The backup-less database serializes an empty ARRAY, never null -
	// the page JS reads .backups.length unconditionally.
	if strings.Contains(rec.Body.String(), `"backups":null`) {
		t.Fatalf("zero-backup database serialized backups as null: %s", rec.Body.String())
	}
	foundNobak := false
	for _, d := range listed.Databases {
		if d.Name == "nobak" {
			foundNobak = true
			if d.State != "none" {
				t.Fatalf("backup-less database state = %q, want none", d.State)
			}
		}
	}
	if !foundNobak {
		t.Fatalf("backup-less database missing from list: %s", rec.Body.String())
	}

	// Change data, then POST /api/db/restore?db=t1&file=...
	dbbExec(t, a, "t1", `DELETE FROM x`)
	rec = httptest.NewRecorder()
	a.handleDBRestore(rec, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/db/restore?db=t1&file=%s", file), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore endpoint: %d %s", rec.Code, rec.Body.String())
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 1 {
		t.Fatalf("restore via endpoint did not bring data back: %d", n)
	}

	// The endpoint refuses a bad parameter.
	rec = httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/db/backup?db=../evil", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid db name not rejected: %d", rec.Code)
	}
}

func TestDBBackupHeaderIsFirstLineWithCounts(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (1), (2), (3)`)
	file := dbbBackup(t, a, "t1")

	h, err := ReadBackupHeader(filepath.Join(a.databases().BackupDir("t1"), file))
	if err != nil {
		t.Fatalf("ReadBackupHeader: %v", err)
	}
	if h.Format != BackupFormatName || h.Version != BackupFormatVersion {
		t.Fatalf("header format/version: %+v", h)
	}
	if h.Database != "t1" || h.Rows != 3 || h.Objects != 1 {
		t.Fatalf("header counts: %+v", h)
	}
	if h.Hostname == "" {
		t.Fatalf("header hostname empty")
	}
}

// dbbLive answers the rows of table x in database t1 as one string. A
// failed restore must leave this string as it was.
func dbbLive(t *testing.T, a *testApp) string {
	t.Helper()
	h, err := a.databases().Open("t1")
	if err != nil {
		t.Fatalf("open t1: %v", err)
	}
	rows, err := h.Query(`SELECT id, a FROM x ORDER BY id`)
	if err != nil {
		t.Fatalf("read x: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d=%s", id, v))
	}
	return strings.Join(out, ",")
}

// A restore replaces the whole database. It must change all or nothing.
// A backup file can come from a git pull. Such a file can hold a conflict
// marker or a line from a newer version. Each case below damages one step
// of Service.Restore. Each case must give an error. Each case must
// also keep the live database and the database directory as they were.
// TestDBRestoreRejectsDamagedAndForeignFiles covers the conflict marker,
// the foreign header and the bad file name.
func TestDBRestoreFailsWholeAtEachStep(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(id INTEGER PRIMARY KEY, a TEXT NOT NULL)`)
	dbbExec(t, a, "t1", `CREATE INDEX x_a ON x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (1, 'one'), (2, 'two')`)
	raw, err := os.ReadFile(filepath.Join(a.databases().BackupDir("t1"), dbbBackup(t, a, "t1")))
	if err != nil {
		t.Fatal(err)
	}
	good := strings.TrimRight(string(raw), "\n")
	header, body, _ := strings.Cut(good, "\n")
	var hdr map[string]interface{}
	if err := json.Unmarshal([]byte(header), &hdr); err != nil {
		t.Fatalf("the header of a new backup is not JSON: %v", err)
	}
	hdr["version"] = 99
	newer, _ := json.Marshal(hdr)

	// The live data changes after the backup. A failed restore must keep
	// this change.
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (3, 'three')`)
	want := dbbLive(t, a)

	for i, tc := range []struct {
		why, content, errPart string
	}{
		{"an empty file", "", "empty backup file"},
		{"a header that is not JSON", "garbage\n" + body, "bad header"},
		{"a header of a newer format version", string(newer) + "\n" + body, "unsupported format"},
		{"a line that is not JSON", good + "\nnot json", "line"},
		{"an unknown kind of line", good + `
{"kind":"what"}`, "unknown kind"},
		{"a row for a table that the file does not create", good + `
{"kind":"row","table":"nope","v":[1]}`, "unknown table"},
		{"a row with too few values", good + `
{"kind":"row","table":"x","v":[9]}`, "expected"},
		{"a tagged value of an unknown type", good + `
{"kind":"row","table":"x","v":[9,{"x":1}]}`, "unrecognized tagged value"},
		{"a table statement that SQLite refuses", good + `
{"kind":"table","name":"bad","sql":"CREATE TABLEX bad(a)","columns":["a"]}`, "create table"},
		{"a row that breaks a constraint", good + `
{"kind":"row","table":"x","v":[1,"again"]}`, "insert into"},
		{"an index statement that SQLite refuses", good + `
{"kind":"index","name":"bad","sql":"CREATE INDEX bad ON nope(a)"}`, "create index"},
		{"a trigger statement that SQLite refuses", good + `
{"kind":"trigger","name":"bad","sql":"CREATE TRIGGER bad"}`, "create trigger"},
	} {
		file := fmt.Sprintf("99991231T235959Z_case%02d.jsonl", i)
		if err := os.WriteFile(filepath.Join(a.databases().BackupDir("t1"), file), []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}
		err := a.databases().Restore("t1", file)
		if err == nil {
			t.Errorf("%s: the restore gave no error", tc.why)
		} else if !strings.Contains(err.Error(), tc.errPart) {
			t.Errorf("%s: error %q does not name the fault %q", tc.why, err, tc.errPart)
		}
		if got := dbbLive(t, a); got != want {
			t.Errorf("%s: the live data changed to %s, want %s", tc.why, got, want)
		}
		if storage.FileExists(a.databases().UserDBPath("t1") + ".restoretmp") {
			t.Errorf("%s: the temporary database is still on disk", tc.why)
		}
	}
}

// The restore endpoint checks each name before it takes the lock. Each
// fault gives the JSON error shape that the Database Backups page reads. A
// valid name of a missing backup gives 500.
func TestDBRestoreEndpointFaults(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(id INTEGER PRIMARY KEY, a TEXT)`)
	for _, tc := range []struct {
		why, method, query string
		code               int
	}{
		{"a bad database name", http.MethodPost, "db=../t1&file=20260101T000000Z_t1.jsonl", http.StatusBadRequest},
		{"a bad file name", http.MethodPost, "db=t1&file=../../config.json", http.StatusBadRequest},
		{"a backup that does not exist", http.MethodPost, "db=t1&file=20260101T000000Z_t1.jsonl", http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		a.handleDBRestore(rec, httptest.NewRequest(tc.method, "/api/db/restore?"+tc.query, nil))
		if rec.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.why, rec.Code, tc.code)
		}
		var resp struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Status != "error" || resp.Message == "" {
			t.Errorf("%s: answer %q is not the JSON error shape", tc.why, rec.Body.String())
		}
	}
}

// Service.ListBackupFiles must answer the newest backup first. The bootstrap
// of a fresh device restores the first name, and the prune keeps the first
// names. The table holds each name that the string order put in the wrong
// place. These are a counter, a counter of two digits, and a host name that
// starts with a digit.
func TestListBackupFilesNewestFirst(t *testing.T) {
	a := dbbApp(t)
	dir := a.databases().BackupDir("t1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"20260926T170050Z_vm.jsonl",
		"20260926T170049Z_10_vm.jsonl",
		"20260926T170049Z_3_vm.jsonl",
		"20260926T170049Z_2_vm.jsonl",
		"20260926T170049Z_vm.jsonl",
		"20260926T170048Z_9phone.jsonl",
	}
	for _, name := range want {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.databases().ListBackupFiles("t1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the order is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Four backups in a fast loop often fall in one second. The prune keeps
// three. The backup that Service.CreateBackup made last must be one of them.
// Before BackupNewerThan, the prune removed that backup when its name had
// a counter.
func TestDBBackupPruneKeepsTheLastBackup(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	a.Config.Update(func(c *config.Config) { c.BackupPruneDepth = 3 })
	var last string
	for i := 0; i < 4; i++ {
		last = dbbBackup(t, a, "t1")
	}
	if !storage.FileExists(filepath.Join(a.databases().BackupDir("t1"), last)) {
		t.Fatalf("the prune removed the last backup %s", last)
	}
	files, err := a.databases().ListBackupFiles("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0] != last {
		t.Errorf("the backups are %v, want three with %s first", files, last)
	}
}
