# 0032. Open note templates

* Status: accepted
* Version: 26.11.41
* Code: `backend/internal/storage/templates.go`, `backend/internal/app/templates_app.go`, `backend/frontend/html/js/OMN-Go/omn-go-editor.js`

## Context

A new note starts with no text, and the person then rebuilds the same shape
each day: the meeting notes, the task list, the daily log. A template feature
solves that. Templates often live in a format of the tool that holds them,
and a person who moves then writes each template again. The fork holds notes
as plain Markdown files in an open directory, and a template is one Markdown
file of the same kind.

## Decision

The templates live in the `templates` directory of the storage, and each
`.md` file there is one template. `GET /api/templates` reads the directory at
each request and answers the list with the text of each template. Four
built-in templates ship with the app, and a file of the same name shadows the
built-in text. The editor holds one box that lists the templates and puts the
text of one template into the note, at the cursor, or at the start of an
empty note.

## Rejected alternatives

* **Templates in the binary only.** The person cannot bring an own file, and
  each edit needs a new build of the app.
* **Templates as notes in `md/`.** Each template would join the note list,
  the tags page, the search index and the sync tree, and a template is not a
  note.
* **A template format of the fork.** Each format of our own is one more
  format the person must leave behind.

## Consequences

* Any `.md` file from any editor or tool becomes a template, with no
  conversion and no lock-in.
* A dropped file appears in the editor at once, because the list reads the
  directory at each request.
* The built-ins stay available while no file shadows them, and an own file
  replaces a built-in text without losing the name.
