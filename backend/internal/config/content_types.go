package config

import (
	"mime"
	"path/filepath"
	"strings"
)

// BuiltinMIME is the content-type table of OMN-Go. It names the web fonts,
// for a container whose own table is small.
var BuiltinMIME = map[string]string{
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
	// A contact and a calendar are text/plain for the same reason. No
	// Chromium renders text/vcard or text/calendar. See
	// doc/decisions/0025-serve-a-contact-and-a-calendar-as-plain-text.md.
	".vcf": "text/plain; charset=utf-8",
	".ics": "text/plain; charset=utf-8",
	".vcs": "text/plain; charset=utf-8",
	".md":  "text/markdown; charset=utf-8",
	// The Go table has no ".txt", and a phone has no /etc/mime.types.
	// EditableFileType reads this table, thus without this row a .txt on
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

// ResolveContentType is the single MIME resolver. It reads overrides, the
// Config.MimeTypes map,
// then BuiltinMIME, then mime.TypeByExtension. The override is empty on a new
// install. The answer is "" when no source knows the extension, and net/http
// then reads the content.
func ResolveContentType(overrides map[string]string, path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := overrides[ext]; ok && ct != "" {
		return ct
	}
	if ct, ok := BuiltinMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}

// HasKnownAssetExtension reports whether the last extension of name is one
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
func HasKnownAssetExtension(overrides map[string]string, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	if ct, ok := overrides[ext]; ok && ct != "" {
		return true
	}
	_, ok := BuiltinMIME[ext]
	return ok
}

// EditableFileType reports whether the content type of name is text that an
// editor can open. The editor routes and the Files page use it. A picture, a
// font, an audio file or a video file must not open an editor.
func EditableFileType(overrides map[string]string, name string) bool {
	ct := ResolveContentType(overrides, name)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i] // drop "; charset=utf-8"
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case ct == "":
		return false // an unknown extension is not assumed to be text
	// Check the media types BEFORE the +xml and +json suffixes, or
	// image/svg+xml would count as text.
	case strings.HasPrefix(ct, "image/"), strings.HasPrefix(ct, "font/"),
		strings.HasPrefix(ct, "audio/"), strings.HasPrefix(ct, "video/"):
		return false
	case strings.HasPrefix(ct, "text/"):
		return true
	// The builtin table serves .jsonl as text/plain, thus the Android WebView
	// can show it. A mime_types entry in config.json can map it to
	// application/jsonl, and that is still text.
	case ct == "application/javascript", ct == "application/x-javascript",
		ct == "application/json", ct == "application/jsonl",
		ct == "application/xml":
		return true
	case strings.HasSuffix(ct, "+json"), strings.HasSuffix(ct, "+xml"):
		return true
	}
	return false
}
