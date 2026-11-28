# 0035. A slow door for the network logins

* Status: accepted
* Version: 26.11.43
* Code: `backend/internal/app/session.go`, `backend/internal/app/middleware.go`

## Context

The login of the network accepted password attempts without a cost, so a
host of the LAN could try passwords as fast as the wire allowed. The answers
carried no `X-Content-Type-Options` and no `Content-Security-Policy`, and
the local check compared the address with the word `localhost`, which
`SplitHostPort` never answers. The cost of the fix must stay small, because
every response of the server walks through the same middleware.

## Decision

The middleware sets `X-Content-Type-Options: nosniff` on every answer. It
sets a strict `Content-Security-Policy` on the system pages alone, read from
the same registry that the route table uses, and the shell of the note pages
stays untouched. Five failed logins of one address close the login for
thirty seconds, and each further failure doubles the wait up to fifteen
minutes. The count lives in memory, keyed by the app and by the address,
one success clears it, and the device itself never enters it. The login
answers `429` with `Retry-After` while the wait stands.

## Rejected alternatives

* **A count on disk.** A restart or a reboot then keeps the door shut
  against the own person, and a file invites a tool to edit it.
* **A CSP on the note pages.** The note is the document of the person, and
  a policy of the app must not break what the person writes.
* **A package of rate limiting.** The rule is one loop over one map, and a
  dependency for that is a second authority on the door.

## Consequences

* A brute force of the login slows down to minutes between attempts, and
  each answer tells the caller how long.
* A restart clears the counts, and the memory of one install holds a few
  addresses.
* The strict policy of the system pages forbids inline scripts there, and
  the templates of the house hold none.
