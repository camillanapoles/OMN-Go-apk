package db

import (
	"os"
	"time"
)

// The state of a database against its newest backup.
//
// A backup and a restore give the .sqlite file the Created time of the
// backup header. A write to the database then moves that time forward.
// THE TIME OF THE BACKUP FILE IS NOT THE MARK: a pull or a file tool gives
// a file a new time. See
// doc/decisions/0024-read-the-backup-state-from-the-created-time.md.

// The states of a database with a valid newest backup and a .sqlite file.
const (
	stateInSync      = "insync"
	stateDirty       = "dirty"
	stateBackupNewer = "backup_newer"
)

// oldBackupTolerance is for a backup with no exact_time mark. An older
// version gave the database the time of the backup FILE.
const oldBackupTolerance = 5 * time.Second

// createdTime answers the Created time of a backup header.
func createdTime(h backupHeader) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, h.Created)
	return t, err == nil
}

// markInSync gives the .sqlite file at path the Created time of h. A
// header with no Created time gives the time of the backup file.
func markInSync(path string, h backupHeader, backupPath string) error {
	t, ok := createdTime(h)
	if !ok {
		info, err := os.Stat(backupPath)
		if err != nil {
			return err
		}
		t = info.ModTime()
	}
	return os.Chtimes(path, t, t)
}

// backupState compares the time of the database with the newest backup.
func backupState(h backupHeader, backupMTime, dbMTime time.Time) string {
	created, ok := createdTime(h)
	if !ok {
		created = backupMTime
	}
	switch {
	case dbMTime.Equal(created):
		return stateInSync
	case dbMTime.Before(created):
		return stateBackupNewer
	case !h.ExactTime && (dbMTime.Equal(backupMTime) || dbMTime.Sub(created) <= oldBackupTolerance):
		// An old backup that this device made or restored.
		return stateInSync
	}
	return stateDirty
}
