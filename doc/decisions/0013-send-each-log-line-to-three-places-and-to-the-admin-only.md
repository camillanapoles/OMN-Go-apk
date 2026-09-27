# 0013. Send each log line to three places, and to the admin only

* Status: accepted
* Version: 26.08.70, 26.09.38, 26.09.59
* Code: `backend/log_levels.go`, `broadcastLogLine`, `recordLogLine` and
  `handleLogHistory` in `backend/logger.go`, `registerRoutes` in
  `backend/server.go`

## Context

Each call site once typed its own subsystem name in brackets. 119 of 153
call sites had one, 34 had none, and one had the text
`[a.protectGitDirs]`. The browser console of each page showed the full
detail of each subsystem, and a person could not ask for less.

The SSE stream `/api/logs` is a live sample. A page that opens after an
event never sees the lines of that event. A person who reads a fault
report must then make the fault occur again, with a page open.

`/api/logs/history` was admin only, and the stream was open to a guest of
a LAN share. A guest who kept a page open thus read each line as the
server wrote it. The guard on the history protected nothing.

## Decision

* Each line has a tag (the subsystem) and a level (`debug`, `info` or
  `error`). `a.logDebugf`, `a.logInfof` and `a.logErrf` write the text
  `[tag] (level) message`.
* `broadcastLogLine` sends each line to three places: stdout, the SSE
  stream and a ring of the last 500 lines.
* The configuration filters stdout and the browser console only. The
  stream and the ring always get each line. The sync progress overlay
  reads `[sync] (debug)` lines from the stream, and a person who meets a
  fault needs the debug lines of that moment.
* The ring has its own endpoint, `/api/logs/history`. A replay on the
  stream would feed the overlay the lines of an old sync. Each page load
  would then show a sync that is not running.
* `/api/logs` and `/api/logs/history` are both admin only. A local
  connection is always admin, thus the device itself keeps both.
* `omn-go-sse.js` does not open the stream when the role hint says guest.

## Consequences

* A guest of a LAN share reads no log line, live or held.
* The ring costs about 60 KB for the life of the process.
* A client with a full channel loses a line and does not block the
  writer. The stream must thus never drive state that needs each line.
