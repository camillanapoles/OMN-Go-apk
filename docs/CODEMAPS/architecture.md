# OMN-Go-apk — System Architecture

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Type
Single app: Go HTTP server + plain-JS embedded frontend, wrapped by an Android
WebView app (gomobile AAR). Also builds desktop binaries (linux/windows).

## Layers (top → bottom)
```
 Android (android/app, java, no AndroidX)          Desktop (main_desktop.go)
   MainActivity ─ ServerService ── starts ─┐       main_desktop → backend
   WebViewSetup (WebView UI)               │
   ShareIn/ShareOut · IntentBridge         ▼
   Shortcuts · Fullscreen · ExportProvider   backend.StartServer
                                             backend/ (facade, 6 funcs)
                                                      │
                                     backend/internal/app (package app)
                                       server.go · routes · *_app.go
   browser ──HTTP 127.0.0.1:<port>──────────────┤
                                                ├── render   (md→html, templates)
                                                ├── storage  (file layout)
                                                ├── config   (settings store)
                                                ├── db       (SQLite+backups)
                                                ├── gitsync  (git pull/push)
                                                ├── search   (index)
                                                ├── files    (Files page)
                                                ├── exchange (note import/export)
                                                ├── status   (Status page)
                                                └── logx     (log hub → SSE)
```

## Data flow
note `.md` → noteheader.Parse → goldmark render → html cache on disk →
WebView/browser. Writes go through /api/save → storage. Sync = gitsync over
SSH, LAN sharing off by default (binds 127.0.0.1).

## GitOps CI/CD (fork layer, added 2026-10-04)
```
upstream/master ──ff──► origin/master (mirror; test.yml gates it)
origin/dev  (staging)  : Fork CI = Quality Gate ∥ APK Build; ONLY cache exporter
origin/main (release)  : PR dev→main auto-merge; tag v*-f.N →
                         android-gomobile-release.yml → signed Release (7 assets)
fork-sync.yml (weekly) : upstream ff → PR master→main (PAT FORK_SYNC_TOKEN)
Protection main+dev    : required "Quality Gate"+"APK Build", auto-merge,
                         strict, enforce_admins=false
Measured loop          : push→merge 23–40s; post-merge build 22s
```

## Key entry points
- main_desktop.go (`//go:build !android`)
- backend/backend.go (StartServer, WaitUntilReady, ServerPort…)
- backend/internal/app/server.go (registerRoutes)
- android/app/src/.../ServerService.java (Android lifecycle)
