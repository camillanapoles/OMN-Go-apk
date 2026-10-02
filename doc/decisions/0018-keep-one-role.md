# 0018. Keep one role

* Status: accepted
* Version: 26.09.107
* Code: `roleAdmin`, `readSessionRole` and `handleLogin` in
  `backend/internal/app/session.go`, `checkSession` in `omn-go-api.js`

## Context

The login knew two passwords. The admin password gave the `admin` role,
and the guest password gave the `guest` role. No route accepted the guest
role. Without a login, a remote caller could already read the notes, the
search, the images and `user_json`, because these routes ask for no role.
The guest password thus protected nothing.

The README said that the two passwords protect the access to the notes.
That sentence was false for each read.

The maintainer chose between two repairs: make each read ask for a role,
or remove the guest password. The maintainer chose the removal.

## Decision

* `admin` is the one role. The login compares the password with
  `admin_password` alone.
* `readSessionRole` gives no role for a guest cookie, also when this
  install signed it.
* `Config` has no `guest_password` field. A `guest_password` key in an old
  `config.json` has no effect, and the next save of the configuration
  removes it.
* The page shows the notes when the hint cookie says `admin`. For each
  other remote caller it shows the login box. An old `guest` hint shows the
  login box too.

## Consequences

* A read needs no login on the server. A remote caller with no password can
  get a note with `/api/note`, and the README says so.
* Each write, each setting, each database, the sync and each system page
  need the admin password on another machine.
* The texts use "remote caller", the term of `doc/TERMINOLOGY.md`, for a
  caller on another machine. The word "guest" names no role now.
