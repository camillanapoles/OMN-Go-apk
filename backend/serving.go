package backend

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// ----------------------------------------------------------------------
// The single asset-serving layer
// ----------------------------------------------------------------------
//
// Two helpers serve each static asset under html/:
//
//   - serveEmbeddableAsset: /js/, /css/, /json/ and the root catch-all.
//     These files ship in the binary and reach html/ at the first request.
//   - serveStorageSubdir: /images/ and /user_json/, user content only.
//
// Both take the content type from resolveContentType, the ONE MIME resolver.
// See doc/decisions/0003-use-one-table-for-each-content-type.md.

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
	if ct, ok := a.GetConfig().MimeTypes[ext]; ok && ct != "" {
		return ct
	}
	if ct, ok := builtinMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}

// writeHTMLHeader is the ONE place that sets the type of a page, with the
// charset: a page that the server renders has no <meta charset>.
// pageCacheWriter in middleware.go reads the prefix "text/html". See
// TestConnectionMiddlewareUsesNoStoreForAPage.
func writeHTMLHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", htmlContentType)
}

const htmlContentType = "text/html; charset=utf-8"

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
	if ct, ok := a.GetConfig().MimeTypes[ext]; ok && ct != "" {
		return true
	}
	_, ok := builtinMIME[ext]
	return ok
}

// legacyAssetPaths maps the old URL of each app asset to its place under
// OMN-Go/. It comes from versionDependentAssets and retiredFonts, thus it
// follows each move. Each key and value starts with a slash, the same as in
// materializeAsset.
var legacyAssetPaths = func() map[string]string {
	out := map[string]string{}
	add := func(rel string) {
		urlNew := strings.TrimPrefix(rel, "html")
		dir, name := path.Split(urlNew)
		oldDir := strings.TrimSuffix(dir, "OMN-Go/")
		out[oldDir+name] = urlNew
	}
	for _, rel := range versionDependentAssets {
		if strings.HasPrefix(rel, "html/") {
			add(rel)
		}
	}
	for _, rel := range retiredFonts {
		name := path.Base(rel)
		out["/css/fonts/"+name] = "/css/OMN-Go/fonts/" + name
	}
	return out
}()

// legacyAssetURL gives the new URL for the old URL of an app asset.
// md/Bookmarks.md and notes of the user name the old paths, and the server
// never rewrites a note of the user. A user file such as /js/mine.js is not
// in the table, and it still answers 404. See
// doc/decisions/0007-keep-the-application-files-in-omn-go-directories.md.
func legacyAssetURL(urlPath string) (string, bool) {
	moved, ok := legacyAssetPaths[urlPath]
	return moved, ok
}

// materializeAsset answers the disk path of the html/ asset for urlPath, and
// it extracts the embedded file at its first request. ok is false for a 404
// and for a directory. This is the ONE lazy extraction.
func (a *App) materializeAsset(urlPath string) (physPath string, ok bool) {
	clean := filepath.Clean(urlPath)

	// An old URL resolves to the new place FIRST. An old copy can stay on
	// disk until removeRetiredAssets runs, and the reader must get the file
	// of this build.
	if moved, isLegacy := legacyAssetURL(filepath.ToSlash(clean)); isLegacy {
		clean = filepath.FromSlash(moved)
	}
	physPath = filepath.Join(a.StorageDir, "html", clean)

	if stat, err := os.Stat(physPath); err == nil {
		if stat.IsDir() {
			return "", false
		}
		return physPath, true
	}

	embedPath := "frontend/html" + filepath.ToSlash(clean)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		os.MkdirAll(filepath.Dir(physPath), 0755)
		os.WriteFile(physPath, data, 0644)
		return physPath, true
	}
	return "", false
}

// serveEmbeddableAsset serves one static asset under html/, with the
// extraction of materializeAsset. ?edit=true opens the editor. For the
// catch-all, serveFrontend already handles the edit intent.
func (a *App) serveEmbeddableAsset(w http.ResponseWriter, r *http.Request, urlPath string) {
	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, urlPath)
		return
	}
	physPath, ok := a.materializeAsset(urlPath)
	if !ok {
		a.serveNotFound(w, r)
		return
	}
	if ct := a.resolveContentType(urlPath); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, physPath)
}

// serveStorageSubdir serves /images/ or /user_json/ from html/<subDir>/. A
// forcedType pins the content type of the whole tree. Otherwise
// resolveContentType decides for each file. The binary embeds none of these
// files, thus there is no extraction.
func (a *App) serveStorageSubdir(subDir, forcedType string) http.Handler {
	dirPath := filepath.Join(a.StorageDir, "html", subDir)
	os.MkdirAll(dirPath, 0755)
	fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ?edit=true opens the editor here too. The documented link "[Edit
		// shared data](/user_json/inventory.json?edit=true)" needs it.
		if r.URL.Query().Get("edit") == "true" {
			a.serveEditor(w, r, r.URL.Path)
			return
		}
		if forcedType != "" {
			w.Header().Set("Content-Type", forcedType)
		} else if ct := a.resolveContentType(r.URL.Path); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// FileServer answers a missing file itself. notFoundInterceptor
		// replaces that answer with serveNotFound.
		fsHandler.ServeHTTP(&notFoundInterceptor{ResponseWriter: w, app: a, req: r}, r)
	})
}

// ----------------------------------------------------------------------
// 404 handling
// ----------------------------------------------------------------------
//
// Each 404 goes through serveNotFound. A page navigation (Accept text/html)
// gets a themed page. A fetch, an <img> or a <script> gets the same facts as
// plain text, because the editor shows the text of a failed /api/note
// directly.

// wantsHTMLError reports a page navigation. Only an explicit text/html
// counts. Anything else gets plain text, which is safe in any context.
func wantsHTMLError(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// notFoundSuggestion answers "<name>.html" when the path names a note that
// exists: the usual cause of a 404 is [text](name) in place of
// [text](name.html). It asks hasKnownAssetExtension, thus /Report.2026 still
// gets a suggestion.
//
// It answers "" unless the resolved path stays inside StorageDir. A request
// such as "/../../etc/passwd" can thus confirm nothing outside the note tree.
func (a *App) notFoundSuggestion(urlPath string) string {
	name := strings.TrimPrefix(urlPath, "/")
	if name == "" || strings.Contains(name, "..") || a.hasKnownAssetExtension(name) {
		return ""
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	if !isPage || baseName == "" {
		return ""
	}
	root, err := filepath.Abs(a.StorageDir)
	if err != nil {
		return ""
	}
	within := func(p string) bool {
		abs, err := filepath.Abs(p)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(root, abs)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	for _, candidate := range []string{htmlPath, mdPath} {
		if candidate == "" || !within(candidate) {
			continue
		}
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return "/" + baseName + ".html"
		}
	}
	return ""
}

// serveNotFound writes the detailed 404. The caller must not have written a
// body. It overwrites a Content-Type that is already set.
func (a *App) serveNotFound(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Path
	if r.URL.RawQuery != "" {
		requested += "?" + r.URL.RawQuery
	}

	// Show only a Referer of this server, and only its path. It goes into an
	// href, and it must not become a link out of the app.
	referer := ""
	if raw := r.Referer(); raw != "" {
		if ref, err := url.Parse(raw); err == nil && ref.Path != "" &&
			(ref.Host == "" || ref.Host == r.Host) && !strings.Contains(ref.Path, "..") {
			referer = ref.Path
		}
	}

	view := notFoundView{
		URL:       requested,
		Method:    r.Method,
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Referer:   referer,
		Suggested: a.notFoundSuggestion(r.URL.Path),
	}

	// One log line for each miss, thus a broken link shows in the console and
	// in /api/logs.
	a.logInfof(log404, "%s %s (referer %q)", view.Method, view.URL, view.Referer)

	if !wantsHTMLError(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "404 Not Found\n\nRequested: %s\nMethod:    %s\nTime:      %s\n",
			view.URL, view.Method, view.Time)
		if view.Referer != "" {
			fmt.Fprintf(w, "Linked from: %s\n", view.Referer)
		}
		if view.Suggested != "" {
			fmt.Fprintf(w, "Did you mean: %s\n", view.Suggested)
		}
		return
	}

	body := renderNotFoundPage(view)
	compiled := a.compilePageWithBody("Not found",
		[]byte("Title: Not found\nCategory: Error\n\n"), body)
	writeHTMLHeader(w)
	w.WriteHeader(http.StatusNotFound)
	w.Write(a.injectRuntimeVars(compiled))
}

// serveNotEditable answers an editor request for a file that is not text:
// 415, with the same negotiation as serveNotFound. The server still serves
// the file normally.
func (a *App) serveNotEditable(w http.ResponseWriter, r *http.Request, relPath string) {
	urlPath := "/" + strings.TrimPrefix(relPath, "/")
	ct := a.resolveContentType(relPath)

	a.logErrf(logEdit, "refused %s (%s): not a text file", urlPath, ct)

	if !wantsHTMLError(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnsupportedMediaType)
		typ := ct
		if typ == "" {
			typ = "unknown"
		}
		fmt.Fprintf(w, "415 Not a text file\n\nFile: %s\nType: %s\n\n"+
			"OMN-Go does not open a picture, a font, an audio file or a video\n"+
			"file in an editor. Open %s to view it.\n", urlPath, typ, urlPath)
		return
	}

	body := renderNotEditablePage(notEditableView{Path: urlPath, Type: ct})
	compiled := a.compilePageWithBody("Not a text file",
		[]byte("Title: Not a text file\nCategory: Error\n\n"), body)
	writeHTMLHeader(w)
	w.WriteHeader(http.StatusUnsupportedMediaType)
	w.Write(a.injectRuntimeVars(compiled))
}

// notFoundInterceptor keeps the path handling and the range handling of
// http.FileServer, and it replaces only its 404. A test of the file before
// the call would copy the traversal defenses of FileServer.
type notFoundInterceptor struct {
	http.ResponseWriter
	app      *App
	req      *http.Request
	replaced bool
}

func (w *notFoundInterceptor) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.replaced = true
		w.app.serveNotFound(w.ResponseWriter, w.req)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	// Drop the 404 body of FileServer. Report the full length, or the handler
	// reads a short write.
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
