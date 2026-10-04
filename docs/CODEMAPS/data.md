# Data — SQLite, config, storage layout

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Storage layout (storage.Layout; Android: Android/media/<appId>/)
```
md/          note sources (*.md; last extension decides note vs file)
html/        compiled pages (render cache) + assets + db_backup/
db/          <name>.sqlite (per-database SQLite files)
omn-go-custom.css|js   user files (created once, never overwritten)
asset_backups/<prev>/  old version-dependent assets before replacement
```

## SQLite (modernc.org/sqlite, CGO_ENABLED=0)
- Name guard: `^[A-Za-z0-9_-]{1,64}$` (path-traversal guard), file at
  `<StorageDir>/db/<name>.sqlite`.
- `POST /api/sql`: admin only, 1 MiB body cap, ≤500 statements, ONE
  transaction per request. Frontend handle: `omnGoOpenDatabase(name)`
  (exec/batch/transaction/readTransaction).
- Backups: full DB as JSONL, format v2, immutable names
  `html/db_backup/<db>/<UTCts>_<hostname>.jsonl`, user-started only.
  Auto-restore only on bootstrap (backups exist, no .sqlite).
  DB state compared by `.sqlite` mtime vs newest backup `created` field.

## Config (config.Config → config.Store)
- Single holder; `a.config.Get()` returns copy under RLock;
  `a.config.Update(func(*Config))` under Lock.
- Every field: row in configFields (fields.go) + control in config_page.html
  (KEY / KEY_CHECKED / KEY_OPTION placeholders); NormalizeXxx repairs enums.
- Log switches log_debug/log_info + per-tag checkboxes; errors ignore both.

## Notes metadata
Header block (NOT front matter) parsed only by noteheader.Parse
(first line holds `:`, ends at blank line); SetKey/SplitRegion splice bytes;
JS port in omn-go-editor.js must stay identical.
