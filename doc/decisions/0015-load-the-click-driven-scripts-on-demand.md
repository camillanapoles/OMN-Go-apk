# 0015. Load the click-driven scripts on demand

* Status: accepted
* Version: 26.09.24, 26.09.41
* Code: `omnLoadModule` and `omnLazy` in
  `backend/frontend/html/js/OMN-Go/omn-go-sse.js`, `omn-go-sync.js`,
  `omn-go-bookmark.js`, `omn-go-search.js`,
  `backend/frontend/test/lazy.test.js`

## Context

Each note page loads `omn-go-sse.js`. The file held 2045 lines, and each
page parsed all of them. Most of the code runs only after a person
presses a control: a sync button, the bookmark button or the magnifier.

## Decision

* Three parts are files of their own:
  * `omn-go-sync.js`: the sync buttons and the commit dialog.
  * `omn-go-bookmark.js`: the bookmark panel and the tag autocomplete.
  * `omn-go-search.js`: the search overlay.
* `omnLazy` in `omn-go-sse.js` writes a stub on `window` for each global
  name of such a file. The first call of a stub loads the file with
  `omnLoadModule`, and then calls the real function with the same
  arguments.
* Five parts stay in `omn-go-sse.js`, because a stub cannot do their
  work:
  1. `omnGoOpenDatabase`. A plain `<script>` in a note calls it while the
     page parses.
  2. `omnGoInsertCapture`. `MainActivity` compares its answer with
     `true`, and a stub answers with a Promise.
  3. The drag and drop listener. It must listen before a person drops a
     link.
  4. The search keyboard shortcut. Ctrl-K and the slash key must work
     before the overlay exists.
  5. The log stream and the session check. Each page starts both.
* A lazy file defines each name that it reads. The body of
  `omn-go-sse.js` is inside an `if` block. A `const` of that block does
  not reach another file.

## Consequences

* After the split, each press of "Commit & Push" threw "SYNC_TITLES is not
  defined". The map stayed in `omn-go-sse.js`, and `omn-go-sync.js` read
  it. 26.09.41 moved the map into `omn-go-sync.js`.
* Put a function that another file calls on `window`, by name. Annex B
  of JavaScript lifts a function of an `if` block to the global scope,
  but a `const` stays in the block. Do not depend on that difference.
* `lazy.test.js` runs each lazy file alone in `page-stub.js`, where
  `window` is the global object, the same as in a browser. It calls each
  promised name, and a ReferenceError fails the test. `dom-stub.js`
  loads a file as a Node module. That gives the file its own scope, which
  hides this fault.
* `TestJavaScriptUnitTests` in `backend/internal/repocheck/js_test.go` runs that
  test in the gate.
