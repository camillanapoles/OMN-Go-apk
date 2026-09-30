# 0004. Tell the browser to ask before it uses a copy

* Status: accepted
* Version: 26.08.21, 26.08.61
* Code: `connectionMiddleware` and `pageCacheWriter` in
  `backend/middleware.go`

## Context

`http.ServeFile` sends `Last-Modified` and no expiry time. A browser then
keeps a file for a part of its age, and the Android WebView does the
same. After an update, the new pages used the old `omn-go-core.js` for
days. A person had to clear the cache.

`no-cache` repaired that in 26.08.21. A Back or Forward load still showed
an old page, because Chromium reads its copy for that load and does not
ask the server. The + button showed the fault. It writes a link into the
page that the person started from, and Back showed that page with no
link.

## Decision

* `connectionMiddleware` sets `Cache-Control: no-cache` on each response.
  The browser keeps the file and asks the server each time. The server
  answers 304 while the file does not change.
* `pageCacheWriter` changes `no-cache` to `no-store` for a response of
  type `text/html`. Chromium then keeps no copy of a page, and Back must
  ask the server.
* An asset keeps `no-cache`. KaTeX, highlight.js and the fonts are
  hundreds of kilobytes, and `no-store` would load them again for each
  page.
* A handler that writes its own `Cache-Control` keeps it. The log stream
  needs that.

## Consequences

* A page must set a content type that starts with `text/html`, or it
  keeps `no-cache`. `render.WriteHTMLHeader` sets that type.
* `pageCacheWriter` must implement `Flush`, or the log stream holds each
  line until the response ends.
