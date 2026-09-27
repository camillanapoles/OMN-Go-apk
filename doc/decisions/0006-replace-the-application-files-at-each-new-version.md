# 0006. Replace the application files at each new version

* Status: accepted
* Version: 1.8.11, 26.08.21
* Code: `refreshEmbeddedAssets` and `AssetsRefreshed` in
  `backend/assets.go`, `MainActivity.java`

## Context

The binary holds the scripts, the style sheets and the bundled notes. The
server writes a file into the storage directory when a person asks for
it, and it never writes that file again. The person can then edit the
file with `?edit=true`.

That rule is correct for a file of the user, for example `md/Welcome.md`.
It is not correct for a file of the application:

* After an update, the old copy on disk hides the new file of the build.
* A note that a new version adds, for example `md/SQLImport.md`, never
  reaches an existing install.

After an update, the Android WebView also used its own cached copy of the
old scripts. Some pages did not operate correctly until the person
cleared the cache.

## Decision

* `versionDependentAssets` lists each file of the application. A file
  that is not on the list belongs to the user.
* `assets_version` in the storage directory holds the version that last
  wrote the files. When the version of the build is different,
  `refreshEmbeddedAssets` writes each listed file from the build.
* `refreshEmbeddedAssets` first moves a copy on disk that differs from the
  build to `asset_backups/<previous-version>/`. When the backup fails, the
  file stays as it is.
* `refreshEmbeddedAssets` writes the version stamp after the loop. When
  the process stops during the refresh, the next start does it again.
* `AssetsRefreshed` tells the Android layer that this start wrote a file.
  `MainActivity` then clears the cache of the WebView one time.

## Consequences

* A person who edits a file of the application loses the edit at the next
  version. The copy in `asset_backups` keeps it.
* A new bundled note is one line in `versionDependentAssets`.
* A start with no version change writes no file and keeps the cache.
