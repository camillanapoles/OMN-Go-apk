# Decision records

A decision record tells why the code follows a rule that is not obvious.
Each record gives the problem, the decision and the result. A comment in
the code keeps one sentence of the reason and names the record.

The git log holds the history of each change. A decision record does not
replace it. Write a record only for a rule that still controls the code.

## When to write a record

Write a record when one of these conditions is true:

* The reason for a rule is a fault that occurred before. A reader who does
  not know the fault can remove the rule and cause the fault again.
* A decision had an alternative that looks better. The record says why the
  alternative is worse.

Do not write a record for a change that sets no lasting rule. A bug fix
that the tests hold is an example.

## What a comment says

A comment says what the code does now and why. It does not tell the
history of the code:

* No version number. `TestNoVersionNumberInComments` in
  `backend/comment_style_test.go` counts them.
* No account of what the code did before, for example "until", "used to"
  or "before this change".
* No account of the fault that caused a change. Put that in a record, and
  write one sentence of the reason in the comment.

A comment names a record with its path:

```go
// The cookie is signed, because the server must not trust a role that a
// client sends. See doc/decisions/0001-sign-the-session-cookie.md.
```

Two tests in `backend/decisions_test.go` check the records:

* `TestEachDecisionRecordIsListed` checks the name, the number and the
  first line of each record. It also checks that the index below links
  each record.
* `TestEachCommentNamesARealDecisionRecord` checks that each path in a
  comment names a record that exists.

`.dockerignore` excludes `doc/`, but it includes `doc/decisions/` again.
The Docker build thus copies the records, and both tests run in the gate
of each Docker build. A tree with no `doc/decisions` skips them.

## The format of a record

* The file name is `NNNN-short-title.md`. NNNN is the next free number.
  Do not use a number two times.
* The first line is `# NNNN. Title`. The title is an instruction.
* A list of three lines follows the title:
  * Status: `accepted`, or `replaced by NNNN`.
  * Version: each version that made the decision.
  * Code: the files that hold the rule.
* Three sections follow: `## Context`, `## Decision` and
  `## Consequences`. A record can add `## Rejected alternatives` before
  `## Consequences`.
* Write the record in Simplified Technical English. See section 10 of
  `CLAUDE.md`.

Do not delete a record. When a new decision replaces an old one, change
the status of the old record to `replaced by NNNN`.

## Index

| Number | Decision | Code |
| --- | --- | --- |
| [0001](0001-sign-the-session-cookie.md) | Sign the session cookie. | `session.go`, `middleware.go` |
| [0002](0002-bind-the-loopback-address-when-lan-sharing-is-off.md) | Bind the loopback address when LAN sharing is off. | `server.go`, `config.go` |
| [0003](0003-use-one-table-for-each-content-type.md) | Use one table for each content type. | `serving.go`, `config.go` |
| [0004](0004-tell-the-browser-to-ask-before-it-uses-a-copy.md) | Tell the browser to ask before it uses a copy. | `middleware.go` |
| [0005](0005-keep-each-secret-out-of-the-config-page.md) | Keep each secret out of the Config page. | `config_fields.go`, `templates_config.go`, `omn-go-config.js` |
| [0006](0006-replace-the-application-files-at-each-new-version.md) | Replace the application files at each new version. | `assets.go`, `MainActivity.java` |
| [0007](0007-keep-the-application-files-in-omn-go-directories.md) | Keep the application files in OMN-Go directories. | `assets.go`, `serving.go`, `git_repo.go` |
| [0008](0008-render-the-pages-without-html-template.md) | Render the pages without html/template. | `templates.go` |
| [0009](0009-show-only-the-search-rows-that-carry-a-word-of-the-query.md) | Show only the search rows that carry a word of the query. | `search_http.go`, `search_index.go` |
| [0010](0010-write-a-pull-without-the-checkout-of-go-git.md) | Write a pull without the checkout of go-git. | `git_pull.go` |
| [0011](0011-push-each-time-and-let-the-remote-answer.md) | Push each time, and let the remote answer. | `git_push.go` |
| [0012](0012-keep-one-remote-for-each-git-server-slot.md) | Keep one remote for each git server slot. | `git_repo.go` |
| [0013](0013-send-each-log-line-to-three-places-and-to-the-admin-only.md) | Send each log line to three places, and to the admin only. | `log_levels.go`, `logger.go` |
| [0014](0014-change-only-the-settings-that-a-request-names.md) | Change only the settings that a request names. | `config_handlers.go`, `config_fields.go` |
| [0015](0015-load-the-click-driven-scripts-on-demand.md) | Load the click-driven scripts on demand. | `omn-go-sse.js`, `omn-go-sync.js`, `omn-go-bookmark.js`, `omn-go-search.js` |
| [0016](0016-give-each-route-one-method.md) | Give each route one method. | `server.go` |
