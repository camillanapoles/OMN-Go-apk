package db

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ----------------------------------------------------------------------
// The state of a database against its newest backup
// ----------------------------------------------------------------------
//
// THE FAULT THAT THESE TESTS HOLD. The state came from the time of the
// backup FILE. A pull gave that file a new time, and the page showed
// "backup newer" with no change in a database or in a backup. The same
// new time could also hide a real change of the database.
//
// Each helper here has the prefix dbs.

// dbsState answers the state that GET /api/db/backups gives for name.
func dbsState(t *testing.T, a *testApp, name string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	a.handleDBBackupList(rec, httptest.NewRequest(http.MethodGet, "/api/db/backups", nil))
	var listed struct {
		Databases []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("the list is not JSON: %v\n%s", err, rec.Body.String())
	}
	for _, d := range listed.Databases {
		if d.Name == name {
			return d.State
		}
	}
	t.Fatalf("the list has no database %q: %s", name, rec.Body.String())
	return ""
}

// dbsBackup makes a database t1 with one row and one backup. It answers
// the path of the backup file and its header.
func dbsBackup(t *testing.T, a *testApp) (string, backupHeader) {
	t.Helper()
	dbbExec(t, a, "t1", `CREATE TABLE IF NOT EXISTS x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (7)`)
	if _, _, err := a.databases().CreateBackup("t1"); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	files, err := a.databases().ListBackupFiles("t1")
	if err != nil || len(files) == 0 {
		t.Fatalf("no backup file: %v", err)
	}
	full := filepath.Join(a.databases().BackupDir("t1"), files[0])
	h, err := ReadBackupHeader(full)
	if err != nil {
		t.Fatalf("ReadBackupHeader: %v", err)
	}
	return full, h
}

// dbsRewriteHeader changes the header line of a backup file and keeps each
// other line. It is how a test makes the backup of an older version, or the
// backup of another device.
func dbsRewriteHeader(t *testing.T, path string, change func(h map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	nl := bytes.IndexByte(raw, '\n')
	if nl < 0 {
		t.Fatalf("%s has no header line", path)
	}
	var h map[string]any
	if err := json.Unmarshal(raw[:nl], &h); err != nil {
		t.Fatal(err)
	}
	change(h)
	line, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(line, raw[nl:]...), 0644); err != nil {
		t.Fatal(err)
	}
}

// dbsSetTime gives one file a modification time.
func dbsSetTime(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func dbsModTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

// A backup gives the database exactly the Created time of its header, and
// the header says so.
func TestBackupGivesTheDatabaseTheCreatedTime(t *testing.T) {
	a := dbbApp(t)
	_, h := dbsBackup(t, a)

	if !h.ExactTime {
		t.Error("the header of a new backup has no exact_time mark")
	}
	created, ok := createdTime(h)
	if !ok {
		t.Fatalf("the header has no Created time: %q", h.Created)
	}
	if got := dbsModTime(t, a.databases().UserDBPath("t1")); !got.Equal(created) {
		t.Errorf("the database has the time %s, want the Created time %s", got, created)
	}
	if got := dbsState(t, a, "t1"); got != stateInSync {
		t.Errorf("the state after a backup is %q, want %q", got, stateInSync)
	}
}

// The time of the backup FILE does not change the state. A pull, a copy of
// the storage directory and a file tool each give a file a new time.
func TestBackupFileTimeDoesNotChangeTheState(t *testing.T) {
	a := dbbApp(t)
	backup, _ := dbsBackup(t, a)

	for _, when := range []time.Time{
		time.Now().Add(48 * time.Hour),
		time.Now().Add(-48 * time.Hour),
	} {
		dbsSetTime(t, backup, when)
		if got := dbsState(t, a, "t1"); got != stateInSync {
			t.Errorf("with the backup file at %s the state is %q, want %q. "+
				"No database and no backup changed.", when.Format(time.RFC3339), got, stateInSync)
		}
	}
}

// A change of the database shows as "dirty", also when the backup file got
// a newer time after the change. The old rule showed "backup newer" there,
// and a restore would then lose the change.
func TestAChangeOfTheDatabaseIsDirtyAfterAPull(t *testing.T) {
	a := dbbApp(t)
	backup, h := dbsBackup(t, a)
	created, _ := createdTime(h)

	// A write gives the database the time of the clock. The test sets a
	// time one minute after the backup, thus it does not wait.
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (8)`)
	dbsSetTime(t, a.databases().UserDBPath("t1"), created.Add(time.Minute))
	if got := dbsState(t, a, "t1"); got != stateDirty {
		t.Fatalf("the state after a change is %q, want %q", got, stateDirty)
	}

	// A pull then writes the backup file again.
	dbsSetTime(t, backup, created.Add(time.Hour))
	if got := dbsState(t, a, "t1"); got != stateDirty {
		t.Errorf("the state is %q, want %q. The database has a change that no "+
			"backup holds, and a restore would lose it.", got, stateDirty)
	}
}

// A backup that another device made later is "newer". A restore of it makes
// the database agree with it. A restore of the older one shows that a newer
// backup exists.
func TestABackupOfAnotherDeviceIsNewer(t *testing.T) {
	a := dbbApp(t)
	first, h := dbsBackup(t, a)
	created, _ := createdTime(h)

	// The backup of another device: the same content, one hour later.
	later := created.Add(time.Hour)
	name := later.Format("20060102T150405") + "Z_other.jsonl"
	second := filepath.Join(filepath.Dir(first), name)
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, raw, 0644); err != nil {
		t.Fatal(err)
	}
	dbsRewriteHeader(t, second, func(h map[string]any) {
		h["created"] = later.Format(time.RFC3339)
		h["hostname"] = "other"
	})
	// The pull wrote the file long before now. The state must not read that.
	dbsSetTime(t, second, created.Add(-24*time.Hour))

	if got := dbsState(t, a, "t1"); got != stateBackupNewer {
		t.Fatalf("the state with a later backup is %q, want %q", got, stateBackupNewer)
	}
	if err := a.databases().Restore("t1", name); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := dbsModTime(t, a.databases().UserDBPath("t1")); !got.Equal(later) {
		t.Errorf("after the restore the database has the time %s, want %s", got, later)
	}
	if got := dbsState(t, a, "t1"); got != stateInSync {
		t.Errorf("the state after the restore is %q, want %q", got, stateInSync)
	}

	if err := a.databases().Restore("t1", filepath.Base(first)); err != nil {
		t.Fatalf("Restore of the older backup: %v", err)
	}
	if got := dbsState(t, a, "t1"); got != stateBackupNewer {
		t.Errorf("the state after a restore of the older backup is %q, want %q",
			got, stateBackupNewer)
	}
}

// An install of an older version has backups with no exact_time mark. Its
// database has the time of the backup FILE at the moment of the backup.
// That time is Created plus a second or two.
func TestAnOldBackupKeepsItsState(t *testing.T) {
	a := dbbApp(t)
	backup, h := dbsBackup(t, a)
	created, _ := createdTime(h)
	dbsRewriteHeader(t, backup, func(h map[string]any) { delete(h, "exact_time") })
	if old, err := ReadBackupHeader(backup); err != nil || old.ExactTime {
		t.Fatalf("the test did not make an old header: %+v, %v", old, err)
	}
	dbPath := a.databases().UserDBPath("t1")

	cases := []struct {
		what   string
		dbTime time.Time
		fileAt time.Time
		want   string
	}{
		{"the database at the old file time, and a pull wrote the file again",
			created.Add(1300 * time.Millisecond), created.Add(72 * time.Hour), stateInSync},
		{"a slow write of the backup, and no pull since",
			created.Add(40 * time.Second), created.Add(40 * time.Second), stateInSync},
		{"a change of the database one minute later",
			created.Add(time.Minute), created.Add(72 * time.Hour), stateDirty},
		{"a database that is older than the backup",
			created.Add(-time.Minute), created.Add(72 * time.Hour), stateBackupNewer},
	}
	for _, c := range cases {
		dbsSetTime(t, dbPath, c.dbTime)
		dbsSetTime(t, backup, c.fileAt)
		if got := dbsState(t, a, "t1"); got != c.want {
			t.Errorf("%s: the state is %q, want %q", c.what, got, c.want)
		}
	}
}

// The tolerance of an old backup does not apply to a new one. A change one
// second after a new backup is a change.
func TestANewBackupHasNoTolerance(t *testing.T) {
	a := dbbApp(t)
	_, h := dbsBackup(t, a)
	created, _ := createdTime(h)
	dbsSetTime(t, a.databases().UserDBPath("t1"), created.Add(time.Second))
	if got := dbsState(t, a, "t1"); got != stateDirty {
		t.Errorf("the state one second after a new backup is %q, want %q. "+
			"The backup does not hold that change.", got, stateDirty)
	}
}

// A header with no time that the code can read gives the rule of the file
// times, thus the page still answers.
func TestAHeaderWithNoCreatedTimeUsesTheFileTime(t *testing.T) {
	now := time.Now()
	h := backupHeader{Created: "not a time"}
	for _, c := range []struct {
		db   time.Time
		want string
	}{
		{now, stateInSync},
		{now.Add(-time.Minute), stateBackupNewer},
		{now.Add(time.Minute), stateDirty},
	} {
		if got := backupState(h, now, c.db); got != c.want {
			t.Errorf("backupState with the database at %s = %q, want %q",
				c.db.Sub(now), got, c.want)
		}
	}
}
