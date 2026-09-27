# 0011. Push each time, and let the remote answer

* Status: accepted
* Version: 1.8.9, 26.07.52, 26.09.11
* Code: `syncPush`, `isNonFastForward` and `unpushedState` in
  `backend/git_sync.go`

## Context

A person can configure up to five git servers and switch between them.
Each server has a remote of its own. See
[0012](0012-keep-one-remote-for-each-git-server-slot.md).

`syncPush` once asked the active remote first, and it skipped the push
when the remote seemed to hold the local HEAD. That check failed after a
switch of the server. A person pushed to server 1, switched to server 2,
and pressed Upload. The log said "nothing to commit, nothing to push",
and server 2 never got the commit.

The upload preview had the same fault. It showed only the changed files,
and the page read an empty list as "nothing to do". A commit whose push
failed on the network could thus not be pushed again.

A refused push also had a fault. go-git makes the non-fast-forward error
with `fmt.Errorf` and wraps no sentinel. `git.ErrNonFastForwardUpdate`
belongs to Pull alone. A test of that value thus never matched, and each
refused push gave the status `error` and not `push_conflict`. The page
then showed a plain alert, and not the dialog that offers a force push.

## Decision

* `syncPush` always calls `repo.Push`. `git.NoErrAlreadyUpToDate` from the
  remote is the only answer that means "nothing to push".
* The upload preview answers `unpushedState` beside the file list. When
  the local refs cannot prove that the remote is level, it says "maybe"
  and offers a push. A push that is not necessary costs one round trip. A
  push that the page hides costs the commits of the person.
* `isNonFastForward` tests the sentinel first, and then the text
  "non-fast-forward update". A later go-git that wraps the sentinel thus
  matches with no change.

## Consequences

* Do not add a check that skips the push. Such a check can be wrong about
  a remote that the push itself would ask correctly.
* A go-git upgrade that changes the text of the error breaks the
  push-conflict dialog. The tests of `git_sync_test.go` push against a
  real remote and check the status `push_conflict`.
