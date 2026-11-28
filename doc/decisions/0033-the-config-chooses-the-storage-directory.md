# 0033. The config chooses the storage directory

* Status: accepted
* Version: 26.11.41
* Code: `backend/internal/app/storage_dir.go`, `backend/internal/app/storage.go`, `backend/internal/config/fields.go`

## Context

The desktop keeps the notes in `./data` beside the binary, and the person
cannot move them. A person who holds the notes in a synced folder, in a
document folder or on another disk needs a choice. The config lives inside
the storage directory, thus the config cannot say where the storage is
before something reads it. Android passes the directory of its flavor at
each start, and the wrapper owns that path.

## Decision

`config.json` holds a `storage_dir` field, and the Config page edits it. At
the start, the desktop follows the field: it reads the config of the default
directory, and it moves to the directory the field names. The walk repeats
up to three hops, while each new directory names another one. A directory
that repeats, a pair that nests and an empty field each end the walk. The
walk happens before any write, and the config of the final directory is the
config of the session.

## Rejected alternatives

* **A command line flag only.** The desktop build of the fork starts from a
  shortcut, and a flag never reaches it.
* **One hop, no walk.** A person who edits the setting from the moved
  directory would lose the change at the next start, because only the config
  of the default directory would speak.
* **The setting moves the notes.** The app moves the storage pointer, and it
  copies no note. A move of files is a job for the person, with the tool of
  the person, and a wrong copy must not lose a note.

## Consequences

* The choice applies at the next start of the app, and the Config page says
  so.
* The walk reads one small file per hop, and it writes nothing before the
  final directory stands.
* The Android app keeps the directory of its flavor, and the field waits
  unused there.
