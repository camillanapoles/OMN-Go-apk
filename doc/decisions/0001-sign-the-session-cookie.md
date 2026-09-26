# 0001. Sign the session cookie

* Status: accepted
* Version: 26.09.6
* Code: `backend/session.go`, `hasRole` in `backend/middleware.go`

## Context

A connection from the device itself always has the admin role. A client
on the LAN must log in. This applies only when LAN sharing is on, because
the server binds the loopback address when it is off. See
[0002](0002-bind-the-loopback-address-when-lan-sharing-is-off.md).

Before 26.09.6, `/login` wrote the cookie `session_role=admin`. `hasRole`
read that value and trusted it. Nothing tied the value to a password.

A client on the LAN could thus set the cookie itself and get the admin
role with no password. One line in the browser console was sufficient.
The admin role opens `/api/sql`, `/api/upload`, `/api/import/note` and
`/api/restart`. `GET /api/config` gives the admin password and each git
password.

## Decision

The server signs the role.

* The cookie `session_role` holds the role, the expiry time and an
  HMAC-SHA256 of the two. The server accepts the cookie only when it makes
  the same HMAC with the key of this install. A client does not hold the
  key, thus it cannot make the HMAC.
* The key is 32 random bytes in `<StorageDir>/session_secret`, mode 0600.
  The key is not a field of `Config`. `GET /api/config` and the Config
  page both use the whole `Config` struct, and a secret there reaches both.
* `gitignorePatterns` holds `session_secret`. A sync thus does not copy
  the key of one device to another.
* `session_role` is `HttpOnly`. A note can hold a script, because goldmark
  runs with `html.WithUnsafe()`. A script that can read the cookie can
  send it to another machine, which then has the role for 30 days.
* A second cookie, `session_role_hint`, holds the role as plain text.
  `checkRole` in `omn-go-sse.js` reads it and disables each admin control
  for a guest. The server never reads the hint.

## Consequences

* A client that changes the hint changes only what its own page shows.
* A damaged key file gets a new key. Each person on the LAN must log in
  again.
* When the server cannot write the key file, it keeps the key in memory.
  Each session then stops at the next start of the process.
* A login lasts 30 days. The device itself never needs a login.
