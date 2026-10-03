# 0023. Write each time in UTC

* Status: accepted
* Version: 26.10.21
* Code: `backend/internal/noteheader/noteheader.go`,
  `backend/internal/logx/hub.go`, `backend/internal/app/log_app.go`,
  `backend/frontend/html/js/OMN-Go/omn-go-editor.js`

## Context

The server wrote each time with `time.Now().Format`. On the desktop that
is the time of the zone of the computer. Go on Android finds no zone file,
thus the same code wrote UTC there.

A person who used the two devices with a git sync got one note with two
kinds of time. The `Modified:` line of the desktop was hours away from the
same moment on the phone. The quick notes and the bookmarks of one page
were not in the order of their times.

The divider of the editor is the same heading as the heading of a quick
note. The script made it from the clock of the browser, which is the zone
of the device on each platform.

## Decision

* Each time that the application writes is UTC, on each platform. The
  maintainer decided that.
* `noteheader.Stamp` is the one function for a time in a note:
  `Date:`, `Modified:`, `Imported:`, a quick note and a bookmark.
  `noteheader.StampLayout` is the one layout.
* Each other place calls `.UTC()` before `Format`. These places are the
  name and the index line of an import, the log lines, the Files page and
  the search report.
* The standard log package gets `log.LUTC`, thus its lines agree with the
  lines of `Logger.emit`.
* A git commit gets its time in UTC.
* `mdStamp` of `omn-go-editor.js` uses the UTC methods of `Date`.
* A time in a note has no zone mark, because the form of the header block
  does not change. A time that only a page shows has the word `UTC`. The
  Files page, the search report and the page of a file that does not exist
  show such a time.

## Rejected alternatives

* **Set `time.Local = time.UTC` at the start.** One line changes each
  place, also in a library. No reader of a call site then sees the rule,
  and no test of a function can hold it.
* **Local time on each platform.** Android then needs a zone from Java,
  and two devices in two zones still write two kinds of time.
* **A zone offset in each time.** The header block of each present note
  has the form `YYYY-MM-DD HH:MM:SS`, and the Pelican form has no offset.

## Consequences

* A time that the desktop wrote before this version stays as it is. The
  application cannot know the zone of an old time. A note of the desktop
  can thus hold an old local time above a new UTC time.
* A person on the desktop reads a time that is not the time of the clock
  on the wall.
* `TestEachFormattedTimeIsUTC` fails for a `Format` with no `.UTC()` on
  the same line, and for `time.Local`, `.Local()` and `.In(`.
* The tests of `backend/internal/app/utc_test.go` move the local zone
  eleven hours away, because a build machine in UTC cannot see the fault.
