# 0010. Write a pull without the checkout of go-git

* Status: accepted
* Version: 1.7.3, 1.8.5
* Code: `writeTreeToWorktree`, `oldTrackedPaths`, `syncPull` and
  `syncPullForce` in `backend/git_pull.go`

## Context

The storage directory holds files that git tracks. It also holds files
that git must never touch. Examples are `config.json`, the `.sqlite` file
of each user database, `session_secret`, and each file that `.gitignore`
covers.

`Worktree.Checkout`, `Worktree.Reset` and `Worktree.Pull` of go-git make
the whole worktree match a tree. They do not limit that work to the files
that git tracks. Two faults came from this:

* A force pull on a fresh install deleted `config.json`. The file was
  never tracked, and `.gitignore` covered it.
* A plain pull deleted and wrote again the `.sqlite` file of a user
  database. An open connection then saw a different file at its path. The
  next write failed with "attempt to write a readonly database (1032)",
  which is `SQLITE_READONLY_DBMOVED`.

## Decision

A pull does not call the checkout of go-git. It writes the files itself:

* `writeTreeToWorktree` writes each file of the remote tree, and it
  touches no other file. It also writes a new index.
* `oldTrackedPaths` gives each path that the old HEAD tracks.
* After the write, a pull removes each path that the old HEAD tracked and
  the new tree does not hold. The pull thus deletes a note that another
  device deleted. It never deletes a file that git never tracked.
* Only a force pull removes a file that git does not track, and only
  when `.gitignore` does not cover it.

## Consequences

* `config.json`, the databases and the other local files survive each
  kind of pull.
* A change of this code needs care. The tests of `git_sync_test.go` run a
  real sync against a bare repository on disk, and they check each of
  these files.
* `loadGitignoreMatcher` adds the built-in `gitignorePatterns` to the
  `.gitignore` of the worktree. A stale `.gitignore` from the remote thus
  cannot expose a local file to a force pull.
