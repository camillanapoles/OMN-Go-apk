# 0030. Refuse the default admin password on the network

* Status: accepted
* Version: 26.11.37
* Code: `backend/internal/config/config.go` (`DefaultAdminPassword`),
  `backend/internal/app/session.go` (`handleLogin`),
  `backend/internal/repocheck/default_password_test.go`

## Context

The fresh install writes one admin password, and the value is the same in
each copy of the project: the user manual names it, the API reference
names it, and the source held it in one line. The login accepted it from
each host. With LAN sharing on, each device on the network then logged in
as admin with a credential that a public fork publishes. The reader of
the manual alone knows it.

The tension is the first run against the credential. An install with no
password and no login gives a new person no way in. The local connection
already needs no login at all (section 1 rule 10 of CLAUDE.md), and the
Config page hides the password value (ADR 0005).

## Decision

* `config.DefaultAdminPassword` is the one spelling of the value in the
  code. `config.Load` writes it on a fresh install, and each other file
  reads the constant.
* `handleLogin` refuses a login from a non-local address while the
  configured password equals the constant. The answer is `403` with a
  text that names the next step: change the password. An error line names
  the cause.
* A local caller still logs in with the default password. The person
  opens the Config page and saves a new password. The network login then
  works again, and the first-run warning on the Config page disappears.
* The Config page and `/api/status` carry a derived boolean
  (`admin_password_default`, `ADMIN_PASSWORD_DEFAULT_CHECKED`), and the
  same signal for the author name (`author_default`). The warning is the
  onboarding of both defaults. See `page_values.go`.
* `TestNoSourceFileSpellsTheDefaultPassword` keeps the literal in
  `config.go` alone. A document may name the value, because a document
  warns the reader with it.

## Rejected alternatives

* **Generate a random password on the first boot and show it one time.**
  A person who loses the shown value locks the device out of the network
  login with no path back. The maintainer keeps this option open for a
  later change.
* **Remove the default and demand a password on the first start.** A
  first run with no Config page access would trap the person on a device
  with no keyboard focus in the WebView flow.

## Consequences

* A fresh install is safe to put on a LAN before the person changes the
  password: the public value grants nothing from the network.
* Each document that names the password stays a warning and stops being a
  key.
* A test that needs the value reads `config.DefaultAdminPassword`, and a
  second spelling of the literal fails the gate.
* The Config page shows one more row per default, and the rows disappear
  when the person sets the values.
