# 0034. The network never uses the default password

* Status: accepted
* Version: 26.11.42
* Code: `backend/internal/config/config.go`, `backend/internal/app/session.go`, `backend/internal/config/page_values.go`

## Context

A fresh install writes the default admin password into `config.json`, and
that password is a public constant of this fork. With LAN sharing on, any
device of the network could log in as admin with a credential that the whole
world can read. The page gave no sign that the credential was still the
installed one. The fresh install must still answer the person who holds the
device, because that person needs the default password to set the first real
one.

## Decision

The default password and the default author become exported constants of the
config package, and no other file holds their literal. The login refuses the
default password from a connection that is not local, with `403`, and it
keeps accepting it from the device. The Config page carries a first-run
warning while the default password or the default author stand. The config
section of `/api/status` marks the default password. A repocheck meta-test
forbids the literal outside `config.go`.

## Rejected alternatives

* **A random password on the first boot.** The Android build shows no
  console, and a password that the app prints once is a lock-out waiting to
  happen.
* **No default at all.** An empty password matches nothing, and the person
  could never log in from the network at all.
* **The warning alone.** A sign without a refusal leaves the door open, and
  a person who never opens the Config page never reads the sign.

## Consequences

* A fresh install on the network answers `403` to each remote login until
  the person sets a real password on the device.
* The login flow, the Config page and the Status page agree through the two
  constants, and the meta-test keeps it that way.
* The change of the password stays exactly where it was, and the local
  bypass of the app is untouched.
