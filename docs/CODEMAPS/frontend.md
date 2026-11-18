# Frontend — embedded plain-JS (no build step)

<!-- codemap: refreshed 2026-10-04 · base d9f6c82 · scan: manual -->

## Sources
- backend/frontend/templates/ → server-side page fragments (never extracted)
- backend/frontend/html/ → js/, css/, json/, fonts (extracted on demand, editable)
- backend/frontend/md/ → bundled system notes (Welcome, UserManual…)

## Note page script order (rule, templates/index.html)
1. omn-go-compat.js (ONLY ES5 file; too-old WebView notice; must stay first)
2. omn-go-console.js (console/error hooks)
3. omn-go-core.js (OMN.action, KaTeX init, progress, header controls)
4. omn-go-highlight.js (search marks, fold table OMN_FOLD_TABLE)
5. omn-go-nav.js (link interception, slow-nav guard)
6. omn-go-share.js (share/copy, clipboard writer, metadata panel)
7. omn-go-api.js (ALL backend calls; `file:` guard + stubs; lazy loader
   omnLazy/omnLazyActions)
then vendored (katex.min.js, highlight.min.js, auto-render.min.js),
omn-go-custom.js (user, last), omn-go-custom.css.

## Page scripts (loaded only by their template)
config_page.html → omn-go-config.js · editor.html → omn-go-editor.js (ES5-ish
`var`) · status → omn-go-status.js · logs → omn-go-logs.js ·
search/bookmark/sync → lazy via omnLazyActions (sync, bookmark, search).

## Patterns
- No framework, no bundler. IIFE + explicit `window.*` exports.
- Controls name their work: `data-action="name"` + `OMN.action('name', fn)`;
  no inline onclick in templates (TestTemplatesHoldNoInlineHandler).
- State: none global; per-page, block-scoped; note scripts idempotent.
- CSS: hand-written, design tokens as :root vars (omn-go-core.css),
  theme via data-theme + prefers-color-scheme.
- Lazy files must not reference bare names of omn-go-api.js (const-in-block);
  lazy.test.js enforces.

## Server injection
Runtime vars injected at serve time replacing
`<meta id="omn-go-runtime-vars-marker">` (version bump ≠ cache invalidation).
