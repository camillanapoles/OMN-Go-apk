# Dependencies & Integrations

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Go (go.mod, module net.basov.omngo, go 1.25; real build = Go 1.26)
- github.com/yuin/goldmark v1.8.2 (markdown; WithHardWraps, WithUnsafe)
- github.com/go-git/go-git/v5 v5.19.2 (sync; pull w/o checkout)
- modernc.org/sqlite v1.34.4 (pure-Go SQLite; CGO off)
- tool: golang.org/x/mobile (gomobile bind → android/app/libs/omngo.aar)
New deps need maintainer reason; go.sum built inside Docker (never committed).

## Android
minSdk 23 · target/compile API 34 · build-tools 34.0.0 · NDK 25.2.9519653 ·
Gradle 8.5 · AGP 8.2.2 · ZERO AndroidX/AppCompat (only fileTree libs/).
Flavors: standard (release) + fdroid (F-Droid server only). versionCode =
YY*100000+MM*1000+S (fdroid ×10 + abi offset).

## Frontend vendored (offline assets in html/)
KaTeX + auto-render · highlight.js · web fonts. Updated only via
local/initial/offline_asset_downloader.sh.

## CI/CD (GitHub Actions, public repo = free)
| Workflow | Trigger | Role |
|---|---|---|
| test.yml (upstream) | push/PR → master | gate: Dockerfile.ci target test |
| android-gomobile-release.yml (upstream) | push master/DS, tags v* | signed APK+desktop, Release on tag |
| fork-ci.yml (fork) | push main/dev/feat…+PR | Quality Gate ∥ APK Build; cache import; dev=only exporter |
| fork-sync.yml (fork) | weekly + dispatch | upstream ff → PR master→main (PAT FORK_SYNC_TOKEN) |
| sync-gitlab.yml | disabled on fork | upstream GitLab mirror |
Build: Dockerfile.base+Dockerfile (local), Dockerfile.ci (CI, one graph).
Cache: type=gha scopes omngo/omngo-test, mode=max, export-only-on-dev
(10 GB pool self-eviction). Signing: repo secrets (fork keystore
~/keystores/omngo-fork outside repo); F-Droid re-signs with own key.

## External services
- GitHub: PR auto-merge, branch protection (main+dev), Actions cache, Releases.
- Git servers (user-configured) for app-level sync over SSH (gitsync).
- GitLab: none on fork (mirror disabled).

## Docs & metadata
doc/ (API.md, TERMINOLOGY, TESTING, decisions/) ·
metadata/net.basov.omngo.fdroid.yml · fastlane changelogs per versionCode.
