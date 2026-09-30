# 0008. Render the pages without html/template

* Status: accepted
* Version: 1.7.4
* Code: `render.Fill`, `render.EscapeHTML` and `render.EscapeJS` in
  `backend/internal/render/templates.go`, and each `render...` function in the
  file of its page

## Context

The server renders the Config page, the Tags page, the Files page, the
Search page and the other system pages. Before 1.7.4 it used
`html/template` for them.

`html/template` uses `text/template`, and `text/template` calls
`reflect.Value.MethodByName`. That call stops the dead-code elimination
of the Go linker for methods in the whole program. The linker then keeps
each method of each type.

go-git has the largest set of methods in the binary: each transport,
each storage and each plumbing type. The linker normally removes most of
them. With `html/template`, it kept all of them, and the binary became
much larger.

## Decision

* The pages do not use `html/template` or `text/template`.
* `render.Fill` puts values into `%%NAME%%` places in a template file.
* Each render function escapes each value. The place of the value sets
  the escape function:
  * HTML text or a quoted HTML attribute: `render.EscapeHTML`.
  * A JavaScript string in an inline `<script>`: `render.EscapeJS`.
  * A JavaScript string in an HTML attribute, for example `onclick`:
    `render.EscapeHTML(render.EscapeJS(v))`.
* A render function puts HTML that the server made, for example the body
  of a note, into the page as it is. No function escapes it a second
  time.

## Consequences

* A new page must escape each value by hand. `html/template` did that
  automatically. The tests of `internal/render/templates_test.go` check the
  escape of the values that come from a person.
* No test stops a new import of `html/template`. `TestBinarySize` shows
  the growth of the binary when one appears. See section 6 of
  `doc/TESTING.md`.
