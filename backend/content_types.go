package backend

import (
	"mime"
	"path/filepath"
	"strings"
)

// builtinMIME is the content-type table of OMN-Go. It names the web fonts,
// for a container whose own table is small.
var builtinMIME = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".json": "application/json",
	// This row is for the database backups. The type is text/plain, because
	// the Android WebView has no download handler and shows only what it can
	// render. application/json would fail: a backup is JSON Lines, and a JSON
	// viewer stops at the second line.
	".jsonl": "text/plain; charset=utf-8",
	".md":    "text/markdown; charset=utf-8",
	// The Go table has no ".txt", and a phone has no /etc/mime.types.
	// editableFileType reads this table, thus without this row a .txt on
	// Android gets no editor. A file beside a note is a .txt.
	".txt":   "text/plain; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
}

// resolveContentType is the single MIME resolver. It reads Config.MimeTypes,
// then builtinMIME, then mime.TypeByExtension. The override is empty on a new
// install. The answer is "" when no source knows the extension, and net/http
// then reads the content.
func (a *App) resolveContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := a.config.get().MimeTypes[ext]; ok && ct != "" {
		return ct
	}
	if ct, ok := builtinMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}

// hasKnownAssetExtension reports whether the last extension of name is one
// that this install serves as a file. It is the one answer to "is this name a
// note, or a file under html/".
//
//	.md                     the source of a note.
//	.html                   a compiled note.
//	a known extension       a file under html/, for example .js or .txt.
//	an unknown extension    a note, for example "Report.2026".
//	no extension            a note.
//
// A note named "Draft.txt" is md/Draft.txt.md and html/Draft.txt.html, thus
// it never collides with the file html/Draft.txt.
//
// IT MUST NOT CALL mime.TypeByExtension. The standard library reads
// /etc/mime.types, which differs between devices. Git sync carries a name to
// each device, and the name must be a note on each one or a file on each one.
func (a *App) hasKnownAssetExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	if ct, ok := a.config.get().MimeTypes[ext]; ok && ct != "" {
		return true
	}
	_, ok := builtinMIME[ext]
	return ok
}
