package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// backupState answers the state that GET /api/db/backups gives for one
// database: "insync", "dirty", "backup_newer" and so on.
func backupState(t *testing.T, a *App, name string) string {
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

// A pull with no change for a backup leaves the database "in sync".
//
// THE FAULT THAT THIS TEST HOLDS. A pull wrote each file of the remote
// tree again, and the backup got a new modification time. The page
// compares that time with the time of the database. It showed "backup
// newer" after each pull, on each device, with no change in a database or
// in a backup.
//
// The test uses the real database, the real backup and the real writer of
// a pull. It needs no remote: a pull gives WriteTreeToWorktree the tree of
// the remote, and this test gives it the tree of HEAD.
func TestBackupStaysInSyncAfterAPull(t *testing.T) {
	a := newTestApp(t)
	h, err := a.databases().Open("t1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Exec(`CREATE TABLE x(a); INSERT INTO x VALUES (7)`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/db/backup?db=t1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("the backup failed: %d %s", rec.Code, rec.Body.String())
	}
	if got := backupState(t, a, "t1"); got != "insync" {
		t.Fatalf("the state after a backup is %q, want insync", got)
	}

	// Commit the backup, the same as a push does.
	repo, err := a.gitSync().GetOrInitRepo()
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(a.databases().BackupDir("t1"), "*.jsonl"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("want one backup file, got %v (%v)", backups, err)
	}
	rel, err := filepath.Rel(a.StorageDir, backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(filepath.ToSlash(rel)); err != nil {
		t.Fatalf("git add %s: %v", rel, err)
	}
	commit, err := wt.Commit("a backup", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.CommitObject(commit)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}

	// A file system can keep a time with a low precision. The wait makes a
	// new write of the backup visible as a newer time.
	time.Sleep(20 * time.Millisecond)

	written, err := a.gitSync().WriteTreeToWorktree(repo, wt, tree)
	if err != nil {
		t.Fatalf("WriteTreeToWorktree: %v", err)
	}
	if !written[filepath.ToSlash(rel)] {
		t.Fatalf("the pull did not see the backup: %v", written)
	}

	if got := backupState(t, a, "t1"); got != "insync" {
		t.Errorf("the state after a pull is %q, want insync. No database and no "+
			"backup changed. The pull gave the backup a new time. See "+
			"worktreeHoldsBlob in backend/internal/gitsync/pull.go.", got)
	}
}
