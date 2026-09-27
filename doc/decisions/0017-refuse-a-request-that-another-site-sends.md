# 0017. Refuse a request that another site sends

* Status: accepted
* Version: 26.09.106
* Code: `foreignRequest` and `isKnownHost` in `backend/request_guard.go`,
  `connectionMiddleware` in `backend/middleware.go`

## Context

A connection from the device itself is always admin. The Android WebView
and the desktop browser thus need no login. But each other page that the
browser of the device opens sends its requests from the same device.

A probe got `200 Saved` for a write with `Origin: https://evil.example`
and `Host: attacker.example:8080`. Two attacks use this gap:

* **A cross-site request.** A page of another site sends a form or a
  `fetch` to `http://localhost:8080`. The browser sends it, and the server
  sees a local connection.
* **DNS rebinding.** A name of the attacker first points to the attacker,
  and then to `127.0.0.1`. The page of that name then reads the answers of
  the server as its own origin, for example the passwords of
  `GET /api/config`.

## Decision

`connectionMiddleware` asks `foreignRequest` before each route. It answers
`403` with a plain text reason for each of these requests:

* **The `Host` header names another machine.** A name must be `localhost`,
  the system name of the device, or the device label of the Config page.
  Each of these can end in `.local`. An IP address always passes, because
  the rebinding attack needs a name. A request with no `Host` passes.
* **A write comes from another origin.** A write is each method except
  `GET`, `HEAD` and `OPTIONS`. Its `Origin` header must name the same host
  and port as `Host`. `Origin: null` fails.
* **A write carries `Sec-Fetch-Site: cross-site` and no `Origin`.**

A request with no `Origin` passes. The Java layer of the Android
application and a command-line client send none.

The server writes each refusal to the log with the tag `server`.

## Rejected alternatives

* **A list of the addresses of the device.** Android denies the interface
  list to an application, and an address can change while the server runs.
  An address does not help an attacker, thus the list adds nothing.
* **A token in each page for each write.** Each note script that calls
  `/api/sql` or `/api/save` would need it. The `Origin` test gives the same
  protection with no change of the pages.

## Consequences

* A LAN client must use an IP address, the system name or the device
  label. A name of a local DNS server, for example `pc.lan`, gets `403`.
* `GET /api/config` keeps the passwords for a local caller, because a
  rebound name cannot reach it.
* `TestAnotherSiteCannotWriteANote` keeps the probe.
  `TestAReboundNameCannotReadTheConfig` keeps the read.
