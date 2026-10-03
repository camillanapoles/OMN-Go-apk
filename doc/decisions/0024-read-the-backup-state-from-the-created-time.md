# 0024. Read the backup state from the Created time

* Status: accepted
* Version: 26.10.23
* Code: `backupState` and `markInSync` in
  `backend/internal/db/backup_state.go`, `CreateBackup` in
  `backend/internal/db/backup_create.go`, the restore in
  `backend/internal/db/backup_restore.go`, `HandleBackupList` in
  `backend/internal/db/backup_http.go`

## Context

The DB backup page shows a state for each database: in sync, not backed
up, or backup newer. The `.sqlite` file is not in git, and the backups
are. The state compared the modification time of the two files. A backup
and a restore gave the `.sqlite` file the time of the backup file.

The time of a file is not a property of its content. A pull wrote each
backup again and gave it a new time. Each database then showed "backup
newer" with no change. The same new time could hide a real change of the
database: the page showed "backup newer", and a restore would lose the
change. A copy of the storage directory, or a file tool, changes a time in
the same way.

## Decision

* The header of a backup holds `created`, the UTC time of the backup to
  the second. It is in the content, thus each device reads the same value.
* A backup and a restore give the `.sqlite` file exactly the `created`
  time of that backup. `markInSync` is the one function for that.
* `backupState` compares the time of the `.sqlite` file with `created` of
  the newest backup:
  * the same time: `insync`.
  * the database before `created`: `backup_newer`.
  * the database after `created`: `dirty`.
* The state does not read the time of the backup file.
* A new backup has `"exact_time": true` in its header. The format version
  stays 2. An older version of the application ignores a field that it
  does not know, thus it still reads the backup.
* A backup with no `exact_time` comes from an older version. That version
  gave the database the time of the backup file, which is `created` plus
  the time of the write. Such a backup has two more cases of `insync`. The
  database is at most 5 seconds after `created`, or the database has the
  time of the backup file.
* The maintainer decided to use `created`.

## Rejected alternatives

* **Keep the file times and prevent each new time.** The pull now keeps a
  file with no change. A copy of the directory and a file tool still give
  a new time, and no code of the application can prevent that.
* **One tolerance for each backup.** A change of the database in the
  seconds after a new backup would then show as in sync. The mark
  `exact_time` limits the tolerance to the old backups.
* **A new format version.** An older version of the application refuses a
  version that it does not know. A device with no update would then show
  each new backup as invalid.
* **Compare the content.** A dump of each database for each view of the
  page costs too much.

## Consequences

* A pull, a copy of the storage directory and a file tool do not change
  the state.
* A real change of the database shows as `dirty`, also after a pull.
* A device that made its newest backup with an older version shows
  `insync` again with no action, when its database has no change.
* A device that RESTORED its newest backup with an older version has a
  database time that no rule can read. It is the time of a pull. Such a
  database shows `dirty`. One backup on one device, and one restore on
  each other device, give each device the new mark. No data is lost.
* The clocks of two devices must agree to some seconds. A backup from a
  device with a slow clock can show as `dirty` and not as `backup_newer`.
* The file system must keep a time to the second. FAT keeps two seconds,
  and a database on FAT can show a wrong state.
* The tests of `backend/internal/db/backup_state_test.go` hold each rule.
