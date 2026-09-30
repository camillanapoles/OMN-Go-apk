# 0014. Change only the settings that a request names

* Status: accepted
* Version: 26.08.43
* Code: `configFieldSent` in `backend/internal/app/config_handlers.go`,
  `config.ApplyForm` and `config.ApplyGitServerForm` in
  `backend/internal/config/fields.go`, `config.CheckboxFields` and the Config
  page template

## Context

`POST /api/config` once built the whole `Config` again from the form. A
request that named one setting thus cleared each setting that it did not
name. The Config page always sends the whole form, thus the page never
showed the fault. A note that posted `theme` alone emptied the author
name, both passwords, the command of the external editor and the device
label.

A browser sends nothing for a checkbox that is not ticked. "Not ticked"
and "not part of this request" thus look the same to the server.

## Decision

* A field that the request does not carry keeps its value.
* The server writes a field that the request carries, also when the
  value is empty. A person must be able to clear the author name or a password.
* The Config page names each checkbox that it governs in one hidden
  field, `config_fields`. A name in that list counts as sent, also when
  the form has no value for it. That is what a checkbox that is not
  ticked means.
* A caller with no `config_fields` changes only what it names. A note or
  a script gets that safe default.
* `config.CheckboxFields` writes the list from the table in
  `internal/config/fields.go`. A new checkbox thus needs no edit of the markup.

## Consequences

* The same rule applies to each field of a git server slot. See
  [0005](0005-keep-each-secret-out-of-the-config-page.md).
* A test that posts one field must check that each other field keeps its
  value.
