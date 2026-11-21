# 0031. Release after a completed spec

* Status: accepted
* Version: 26.11.36
* Code: `.github/workflows/release.yml`, `local/release/spec-release-gate.sh`, `backend/internal/repocheck/release_gate_test.go`

## Context

The Release workflow tagged `v<APP_VERSION>-f.N` at each push to `main`.
Each pull request that carried a version bump then made one small release.
The version 26.11.35 made two releases with no change of the application
between them. The notes of a release held the credits of the pull requests
that GitHub generates, and they spoke to the reviewer of a commit, not to
the person who uses the app.

The spec of a change holds the tasks of the change, and each task holds the
criteria that verify and validate it. A spec is complete when the issue of
the spec is closed, the tasks file holds no open task, and the tests prove
the criteria. Only a completed spec gives the person a change that is worth
one release.

## Decision

The maintainer dispatches the Release workflow for one completed spec, with
the number of its issue. The workflow runs the spec gate before it tags.
The gate proves five facts:

1. The issue of the spec is closed, and it carries the `spec:done` label.
2. The tasks file of the spec holds no open task.
3. Each task of the tasks file holds its `Verificação` and `Validação`
   lines.
4. The three files of the version agree (`backend/version.go`,
   `android/app/build.gradle`, `doc/CODE_MAP.md`).
5. The humanized changelog of the version exists at
   `doc/changelog/<APP_VERSION>.md`.

A red gate publishes nothing, and its message names the fact that failed.

The changelog of a version speaks to the person who uses the app. Each line
is one bullet of eight words or more. The changelog holds no commit hash, no
file name, and no developer word. The commit that sets the version of the
app writes the changelog of that version in the same commit. The release
build puts the changelog in the notes of the GitHub Release, before the
notes that GitHub generates.

`backend/internal/repocheck/release_gate_test.go` holds the rule at each
Quality Gate. It fails when the Release workflow releases on a push, when
the gate leaves the path before the tag, when the release build builds a
branch, when the version of the app holds no humanized changelog, and when
a task of a spec misses its criteria.

## Rejected alternatives

* **One release per pull request.** The version numbers of the fork rose
  faster than the value of each release, and the notes held nothing for the
  person.
* **A manual tag with no gate.** The gate is the record of the proof. A
  habit can forget one proof, and no test holds a habit.
* **A changelog that the commit messages generate.** A commit message speaks
  to the reviewer of the code, in the language of the code, and it names the
  files that the person never sees.

## Consequences

* The person gets one release per completed spec, with notes that the
  person reads.
* A release waits for the full spec. A fix that one task of a spec needs
  waits with the spec.
* The maintainer runs one command for a release:
  `gh workflow run Release -f issue=<number>`.
* Each version carries its changelog from the commit that sets it, and the
  Quality Gate fails a version with no changelog.
* The branch pushes of the fork make no release build. The Fork CI builds
  the branches, and the APK of a pull request stays a test artifact.
