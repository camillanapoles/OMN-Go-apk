# Backend — Go server (package app)

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Routes (server.go · registerRoutes · a.route(path, role, kind, handler))
```
POST /login                open   → handleLogin (signed session cookie)
GET  /api/note             open   → handleGetNote        (raw file)
GET  /api/search           open   → handleSearch
GET  /        (catch-all)  open   → serveFrontend (pages/assets/md)
POST /api/quick            admin  → handleQuickNote
POST /api/bookmark         admin  → handleBookmark
POST /api/upload           admin  → handleUpload (+ UserFileTrees variants)
POST /api/save             admin  → handleSaveNote
POST /api/newpage          admin  → handleNewPage
GET/POST /api/config       admin  → handleConfigGet/Post (configFields table)
POST /api/restart          admin  → handleRestart
POST /api/sql              admin  → handleSQL (db/sqlite.go, one txn/request)
POST /api/db/backup        admin  → handleDBBackupCreate   GET backups · POST restore
POST /api/sync             admin  → handleSync  GET preview · POST trust-host-key
GET  /api/edit-external    admin  → handleEditExternal
GET  /api/export/note      admin  → handleExportNote   POST import/note
GET  /api/status           admin  → handleStatus (JSON/Markdown)
GET  /api/logs             admin  → handleLogsSSE (SSE)  GET logs/history
GET  /<systemPages>        open|adminPage → pageHandler (page_access.go table)
```

## Handler → package mapping (backend/internal/)
| Area | Package | App glue |
|---|---|---|
| settings | config (Store, NormalizeXxx, fields.go) | config_app.go |
| files/layout | storage (Layout, assets, ResolvePageName) | storage_app.go |
| pages/render | render (Renderer, templates, EscapeHTML/JS) | render_app.go |
| sqlite | db (sqlite.go, backups JSONL v2) | db_app.go |
| git sync | gitsync (pull w/o checkout, host keys) | gitsync_app.go |
| search | search + textmatch (fold table) | search_app.go |
| files page | files | files_app.go |
| exchange | exchange | exchange_app.go |
| status | status | status_app.go |
| logging | logx (hub → stdout+SSE; levels/tags) | log_app.go |

## Rules enforced by repocheck tests
importLayers (pkg layers), fileGroups/layers in app, one writeJSON path,
no log.Printf, no bare 64-bit atomics, no html/template, route baseline.

## Key files
backend/internal/app/server.go (~340 ln) · handlers_test.go (newTestApp) ·
baseline_test.go (golden route set) · page_access.go (systemPages)
