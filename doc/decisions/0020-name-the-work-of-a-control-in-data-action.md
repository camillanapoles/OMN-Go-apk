# 0020. Name the work of a control in data-action

* Status: accepted
* Version: 26.10.7
* Code: `OMN.action` and the click listener in
  `backend/frontend/html/js/OMN-Go/omn-go-core.js`, the action blocks of
  `omn-go-core.js`, `omn-go-api.js` and `omn-go-config.js`, each template
  of `backend/frontend/templates`

## Context

Each control of a template held an inline `onclick` with JavaScript text.
The text called a global function, thus each such function was a property
of `window`. The scripts held more than 50 names on `window`, and a note
script can replace each of them by accident. A reader of a template
could not find the function of a control without a search of each script.

Some controls held a whole statement in the attribute. No test ran that
text, and no editor checked it.

## Decision

* A control names its work in `data-action`. It can give one value in
  `data-arg`.
* `OMN.action(name, fn)` in `omn-go-core.js` gives a name its function.
  The function gets the control and the event.
* One click listener on the document finds the nearest element with
  `data-action` and calls its function. It stops the default work of a
  link.
* A script registers its actions when it loads. The listener reads the
  table at the click, thus the order of the scripts does not matter.
* The actions that need the server are at the end of `omn-go-api.js`,
  outside the `file:` guard. A page from disk thus has the same actions.
* An action of one page is in the script of that page. The Config page
  has its actions in `omn-go-config.js`, and the backup page has them in
  the script block of `db_backups.html`.
* A value from the server goes into `data-arg` as text, with the HTML
  escape alone. No template puts a value into JavaScript text of an
  attribute.
* `window.OMN` is the one namespace of the application scripts.

## Rejected alternatives

* **A listener for each control, by id.** Each control then needs an id,
  and a script must run later than the markup. The modals arrive at serve
  time, and a system page adds controls later.
* **The name of the global function in the attribute.** The listener
  could call `window[name]`. The functions then stay on `window`, which is
  the fault.

## Consequences

* An action with no function is a dead control, and the only report is a
  console warning. `TestEachDataActionHasAFunction` in
  `backend/internal/repocheck/frontend_test.go` finds it in the source.
* `TestTemplatesHoldNoInlineHandler` keeps an inline handler out of each
  template.
* `backend/frontend/test/actions.test.js` sends a click to the real
  listener.
* A handler that stops the propagation of a click also stops the action.
  No script of the application does that.
* The function of an action has no name on `window`. The actions of the
  lazy files are the exception: `omnLazy` needs a global name for each
  function that it loads. A later change gives the lazy files actions of
  their own.
* The names of the User Manual stay on `window`. `window.refreshPage` also
  stays, because the bundled note `AppApiTest` calls it.
* A note of the user can still use an inline `onclick`. This rule is for
  the templates of the application.
