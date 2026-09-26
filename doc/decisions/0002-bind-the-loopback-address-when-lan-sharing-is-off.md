# 0002. Bind the loopback address when LAN sharing is off

* Status: accepted
* Version: 1.7.10
* Code: `StartServer` in `backend/server.go`, `ShareLAN` in
  `backend/config.go`

## Context

Before 1.7.10, the server bound `0.0.0.0` at each start. Each device on
the network could thus connect. Only the password check in the handlers
stopped a request from another device.

## Decision

The socket controls who can connect.

* When "Share on LAN" (`share_lan`) is off, the server binds `127.0.0.1`.
  Another device cannot complete a TCP handshake. A fault in the
  authorization code thus cannot open the server to the network.
* When the option is on, the server binds `0.0.0.0`. `authMiddleware`
  then asks each client that is not local for the admin password or the
  guest password.

## Consequences

* The listener binds one time. A change of the option on the Config page
  applies at the next start of the server.
* A connection from `127.0.0.1` or `::1` always has the admin role. The
  Android WebView and the desktop browser connect that way.
