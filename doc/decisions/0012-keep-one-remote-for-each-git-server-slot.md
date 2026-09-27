# 0012. Keep one remote for each git server slot

* Status: accepted
* Version: 1.6.8
* Code: `slotRemoteName`, `ensureOriginRemote` and
  `ensureRemotesAndGetActive` in `backend/git_repo.go`

## Context

The Config page has five git server slots, and one of them is active.
The repository first had one remote, `origin`, and the server wrote the
URL of the active slot into it. Each switch of the slot and each edit of
a URL thus moved `origin` to another server. No server had a history or
remote-tracking refs of its own.

## Decision

* Each slot with a URL has its own remote, `gitserver0` to `gitserver4`.
  The name comes from the index of the slot, and not from the name that a
  person can edit. A rename thus keeps the remote.
* The server keeps these remotes the same as the configuration at each
  sync. It adds a remote when a slot gets a URL, changes it when the URL
  changes, and removes it when the slot is cleared.
* A sync uses the remote of the active slot.
* `origin` is made one time, from the active slot at that moment, and the
  server never changes it. A sync uses it only when the active slot has
  no URL.

## Consequences

* A switch of the slot does not change any remote. The refs of each
  server stay correct.
* The upload preview and the push must ask the remote of the active slot.
  A clean worktree does not prove that this remote has each commit. See
  [0011](0011-push-each-time-and-let-the-remote-answer.md).
