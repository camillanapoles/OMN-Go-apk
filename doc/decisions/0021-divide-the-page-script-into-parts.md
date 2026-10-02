# 0021. Divide the page script into parts

* Status: accepted
* Version: 26.10.11
* Code: `backend/frontend/templates/index.html`, and in
  `backend/frontend/html/js/OMN-Go/` the files `omn-go-console.js`,
  `omn-go-core.js`, `omn-go-highlight.js`, `omn-go-nav.js` and
  `omn-go-share.js`

## Context

`omn-go-core.js` held 1607 lines. It held the console of the page, the
marks of a search and the links of a note. It also held the clipboard, the
metadata panel and the controls of the page header. A reader who looked
for one part read past each other part.

## Decision

* `omn-go-core.js` is five files, one for each logical part:
  * `omn-go-console.js`: the console of the page.
  * `omn-go-core.js`: the KaTeX call, `OMN.action`, the progress overlay,
    the controls of the page header and the load listener.
  * `omn-go-highlight.js`: the marks of a search and the fold table.
  * `omn-go-nav.js`: the links of a note and the slow-navigation guard.
  * `omn-go-share.js`: send and copy of a note, the clipboard, the page
    link and the metadata panel.
* `index.html` names each file in the head, in that order.
  `omn-go-console.js` is first after `omn-go-compat.js`, thus its hooks
  exist before each other script runs. `omn-go-api.js` stays last.
* No part loads on demand. A plain script of a note runs while the page
  parses, and the User Manual promises it functions of `omn-go-core.js`,
  `omn-go-highlight.js` and `omn-go-share.js`. `omn-go-console.js` must
  see the first line of each script. `omn-go-nav.js` must listen before
  the first click.
* The text of each part is the same as before. A top-level name of one
  file is a global name, and a later file can read it.
* `omn-go-editor.js` stays one file. The maintainer decided that.

## Rejected alternatives

* **One file with clear sections.** The file had sections, and it still
  grew to 1607 lines.
* **A build step that joins the parts.** The frontend has no build step.
  See `CLAUDE.md` section 4.

## Consequences

* A note page asks for four more files. Each answer has `no-cache`, thus
  the browser asks the server each time and gets `304` for a file that
  did not change.
* The page shell is about 280 bytes larger for each compiled note.
  `TestCompiledPageShellStaysSmall` still holds its limit.
* The load listener of `omn-go-core.js` calls functions of
  `omn-go-highlight.js`. That is correct, because each script loaded
  before the listener runs. A call at the top level of `omn-go-core.js` would
  fail.
* `actions.test.js` loads the scripts that `index.html` names, in its
  order. A script that throws while it loads fails each test.
* The one clipboard writer is now in `omn-go-share.js`.
  `TestClipboardHasOneAuthority` names that file.
