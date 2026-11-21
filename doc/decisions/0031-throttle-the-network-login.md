# 0031. Throttle the network login

* Status: accepted
* Version: 26.11.39
* Code: `backend/internal/app/session.go` (`loginThrottle`, `handleLogin`),
  `backend/internal/app/middleware.go` (`connectionMiddleware`,
  `isLocalConnection`), `backend/internal/render/pages.go` (`PageHeader`,
  `SystemPageCSP`), `backend/internal/repocheck/http_headers_test.go`

## Context

The login is the one door for a credential. The password comparison is
constant-time, but the count of attempts had no limit. With LAN sharing
on, a device on the network tried passwords without cost. The server
also sent no `X-Content-Type-Options`, and no Content-Security-Policy for
the pages that the server makes.

One more detail carried a confusion. `isLocalConnection` compared the
host of `RemoteAddr` with the string `localhost`. A TCP `RemoteAddr`
always holds an IP and a port. The comparison never fired, and it
suggested that a client-owned name could decide locality.

## Decision

* `loginThrottle` counts the wrong passwords of each IP. Five failures
  block the next attempts of that IP. The answer is `429` with a
  `Retry-After` header. The value starts at thirty seconds and doubles
  with each further attempt, up to the window.
* A counter older than `loginFailWindow` (fifteen minutes) resets itself.
  A success resets the counter at once.
* The device itself is never blocked. A local connection is the owner
  (section 1 rule 10 of CLAUDE.md), and the throttle serves the network
  alone.
* Each response carries `X-Content-Type-Options: nosniff` from
  `connectionMiddleware`, beside the cache header of ADR 0004. A handler
  that writes the header later wins, the same as with `Cache-Control`.
* A page that the server makes carries a Content-Security-Policy meta
  tag, from the `Content-Security-Policy` line that `PageHeader` writes.
  The tag renders as `http-equiv`, because a `name` tag holds text and no
  policy. A compiled note carries no policy: a note runs its own scripts,
  and a policy of the server would stop them.
* `isLocalConnection` keeps `127.0.0.1` and `::1`. The `localhost`
  comparison is gone. One line names the reason.
* `TestHTTPHardeningStaysInPlace` in the repocheck pins the source text
  of the header and of the throttle.

## Rejected alternatives

* **Throttle by cookie or by session.** A caller with no cookie sends
  none, and the login is exactly where a caller has no cookie yet. The
  IP is the one fact that the socket gives.
* **A nonce or a hash for the inline script of the pages.** The runtime
  values of `InjectRuntimeVars` change per request, and the template
  lives behind the freeze of the frontend. The policy allows the one
  inline script of the page and blocks each external origin.
* **Put the CSP on the note pages too.** The product compiles a note
  with `html.WithUnsafe` on purpose. A policy would break stored notes
  that carry a script.

## Consequences

* A brute-force run over the network meets a growing delay, and the log
  names each refusal.
* A browser cannot guess a type of an answer, and a page that the server
  makes cannot load an external origin.
* A RemoteAddr of `localhost` now falls to the cookie check. No real
  socket sends such a value.
* The behavioral tests of `internal/app` prove the rules, and the
  repocheck fails the gate when a refactor drops one of them.
