# 0025. Serve a contact and a calendar as plain text

* Status: accepted
* Version: 26.10.24
* Code: `BuiltinMIME` in `backend/internal/config/content_types.go`,
  `UserFileTrees` in `backend/internal/config/user_files.go`

## Context

OMN-Go keeps a vCard file (`.vcf`), an iCalendar file (`.ics`) and a
vCalendar file (`.vcs`) in a tree of their own, the same as an uploaded JSON
file. A link in a note opens the file.

The registered types are `text/vcard`, `text/calendar` and
`text/x-vcalendar`. Chromium does not render these three types. It starts a
download for each one. The Android WebView has no download handler, thus a
press on the link does nothing there.

## Decision

* `BuiltinMIME` gives `.vcf`, `.ics` and `.vcs` the type
  `text/plain; charset=utf-8`. The row of `.jsonl` has the same type for the
  same reason.
* `config.EditableFileType` reads that type, thus `?edit=true` opens the
  editor for each of these files.
* A user who wants the registered type writes it in `mime_types` of
  `config.json`. That map has precedence over the table. See record 0003.

## Rejected alternatives

* **Send the registered type.** A desktop browser then gives the file to the
  contacts application or to the calendar application. The Android
  application shows nothing, and Android is the main device.
* **Send the registered type, and give the file to another application on
  Android.** This needs a download handler in the Android layer, and JSON has
  none. It can come later without a change to this record: the handler reads
  the extension of the file, and not the type.

## Consequences

* A press on the link shows the text of the contact or of the calendar, on
  Android and on the desktop.
* No browser offers to add the contact or the event to another application.
  The user saves the file from the browser to do that.
* `TestEachUserFileExtensionIsAKnownTextFile` in
  `backend/internal/config/user_files_test.go` holds the type of each
  extension.
