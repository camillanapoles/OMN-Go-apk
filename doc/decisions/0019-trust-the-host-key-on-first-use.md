# 0019. Trust the host key on first use

* Status: accepted
* Version: 26.09.109
* Code: `gitsync.Service.HostKeyCallback`, `writeHostKey` and
  `gitsync.Service.HandleTrustHostKey` in
  `backend/internal/gitsync/host_keys.go`, `gitsync.Service.GetSSHAuth` in
  `backend/internal/gitsync/repo.go`, `trustHostKey` in `omn-go-sync.js`

## Context

The sync over SSH accepted each host key with `InsecureIgnoreHostKey`. A
machine between the device and the git server could thus answer for the
server. It could read each note of a push, and it could give a pull its own
notes.

The Android application has no `~/.ssh/known_hosts`, and a person on a phone
cannot run `ssh-keyscan`. A rule that needs a key before the first sync thus
stops the sync on the phone.

## Decision

* The first connection to a server stores its key in
  `<StorageDir>/known_hosts`, in the OpenSSH format, with mode 0600. The log
  names the fingerprint.
* Each later connection must show a stored key. `gitsync.Service.GetSSHAuth`
  asks the server for the key types that the file holds.
* A changed key stops the sync. `/api/sync` answers the status word
  `host_key_changed` with the host and both fingerprints.
* The page shows the two fingerprints. OK sends
  `POST /api/sync/trust-host-key` with the host and the new fingerprint. The
  server stores the key only when both name the key that waits. The page
  then runs the same action again.
* The Config page shows the stored fingerprint below each git server slot.
* `gitsync.GitignorePatterns` stops the sync of `known_hosts`. Each device
  decides for itself, and a pull cannot change the keys that it trusts.

## Rejected alternatives

* **Refuse each unknown key.** The phone has no tool to add the first key.
* **Keep the file in git.** A pull would then carry a key from the server
  that the key must protect.

## Consequences

* The first connection has no protection. A key that the first connection
  stores from a false server stays until the person trusts another key.
  The fingerprint on the Config page lets the person compare it.
* A server that changes its key, for example after a new install, stops
  the sync one time, and the person must decide.
* `TestHostKeyIsTrustedOnFirstUse` and `TestSyncAnswersAChangedHostKey`
  hold the rule. The second test runs a real SSH server.
