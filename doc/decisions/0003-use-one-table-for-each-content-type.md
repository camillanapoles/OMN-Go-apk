# 0003. Use one table for each content type

* Status: accepted
* Version: 1.10.14, 26.08.76, 26.09.16, 26.09.17
* Code: `builtinMIME`, `resolveContentType`, `hasKnownAssetExtension` and
  `writeHTMLHeader` in `backend/serving.go`, `legacyMimeSeeds` in
  `backend/config.go`

## Context

Before 1.10.14, three sources set the content type of a file:

* the `mime_types` map of `config.json`,
* calls to `mime.AddExtensionType` at the start of the server,
* the table of the Go standard library, through `http.FileServer`.

The three did not always agree. The standard library also reads
`/etc/mime.types`. A desktop Linux with `mime-support` thus knows more
types than a phone.

Four faults came from this:

* The Go table has no `.txt`. On Android, a `.txt` file got no edit link,
  and the editor refused to open it. The same file worked on a desktop.
* Before 26.09.16, a new install wrote a default map into `mime_types`.
  Each row hid the built-in row and had no charset.
* Before 26.09.17, nine handlers wrote `text/html` with no charset. A page
  that the server renders has no `<meta charset>`, thus the browser
  guessed the encoding of the file index and of the tags page.
* Before 26.08.76, a name with a dot was a file and not a note. The note
  "Report.2026" thus gave 404.

## Decision

* `builtinMIME` is the one table of content types. `resolveContentType`
  reads `mime_types` first, then `builtinMIME`, then the standard library.
* A new install writes no `mime_types` map. At the load of an older
  `config.json`, the server removes a map that is exactly equal to one of
  `legacyMimeSeeds`. A map that a person changed stays.
* `writeHTMLHeader` is the only place that writes the type of a page. The
  type is `text/html; charset=utf-8`.
* `hasKnownAssetExtension` decides if a name is a note or a file. The last
  extension decides. An unknown extension makes a note.
* `hasKnownAssetExtension` does not ask the standard library. Git sync
  copies a name to each device. The name must be a note on each device,
  or a file on each device.
* `.jsonl` is `text/plain`. A browser shows `text/plain`, and the Android
  WebView has no download handler. A JSON viewer fails on the second line
  of a JSON Lines file.

## Consequences

* A new type is one row in `builtinMIME`.
* A person can still change a type in `mime_types`. That change also
  applies to `hasKnownAssetExtension`.
