package backend

// ----------------------------------------------------------------------
// The file index: /OMNGoFiles.html
// ----------------------------------------------------------------------
//
// Three trees, one directory at a time:
//
//	Bundled  What this build carries: staticFS. The templates are in a
//	         separate embed. TestFilesPage_NeverListsTemplates holds that.
//	Served   What a URL finds: StorageDir/html, without db_backup/.
//	Source   What the person wrote: StorageDir/md.
//
// Each NAME has one row in its tree. SILENCE IS THE NORMAL CASE: a row speaks
// ONLY when the app is involved. The word says what the file IS, and the
// color says what HAPPENS to it. The table in filesLegend lists each word and
// each color. The color never works alone.
//
// NOTHING HERE MAY WRITE. Never call materializeAsset, because a listing
// would then extract each embedded file. TestFilesPage_WritesNothing holds
// the rule.

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// filesDirLimit limits the FILES that one directory shows. It never hides a
// directory row, because a hidden directory hides a whole branch.
const filesDirLimit = 200

// The listing leaves filesExcludedDir out at each level. It holds the
// database backups. A URL can still fetch them, thus this is a listing
// decision and not a security control.
const filesExcludedDir = "db_backup"

// filesCompareMax limits the byte comparison of two files with the SAME size,
// because a fixed typo often keeps the length. Above this limit, the row says
// "same size".
const filesCompareMax = 2 << 20

// These are the three trees. The key is the value of ?tree= and the text of
// the crumb.
const (
	filesTreeBundled = "bundled"
	filesTreeServed  = "served"
	filesTreeSource  = "source"
)

// indexedFile is one file in one tree, keyed by its LOGICAL path: relative to
// the root of the tree, with slashes. In the Bundled tree,
// frontend/html/js/x.js is "js/x.js", and frontend/md/Note.md is
// "md/Note.md".
type indexedFile struct {
	path string
	size int64
	mod  time.Time // zero for embedded files; embed.FS has no mtime
}

// filesEntry is one NAME in one tree, with its two sides. In the Bundled
// tree, device is always nil. In the Served and Source trees, ships is the
// embedded file, and device is the file in StorageDir.
type filesEntry struct {
	path   string
	ships  *indexedFile
	device *indexedFile
}

func embeddedFiles() []indexedFile {
	var out []indexedFile
	for _, root := range []struct{ dir, prefix string }{
		{"frontend/html", ""},
		{"frontend/md", "md/"},
	} {
		fs.WalkDir(staticFS, root.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, iErr := d.Info()
			if iErr != nil {
				return nil
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(p, root.dir), "/")
			out = append(out, indexedFile{path: root.prefix + rel, size: info.Size()})
			return nil
		})
	}
	return out
}

// walkStorage lists one tree of the storage directory. sub is "html" or "md".
func (a *App) walkStorage(sub string) []indexedFile {
	base := filepath.Join(a.StorageDir, sub)
	var out []indexedFile
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rErr := filepath.Rel(base, p)
		if rErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if sub == "html" && rel == filesExcludedDir {
				return fs.SkipDir
			}
			return nil
		}
		info, iErr := d.Info()
		if iErr != nil {
			return nil
		}
		out = append(out, indexedFile{path: rel, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	return out
}

// treeEntries makes the entry list of one tree, with the two sides paired by
// name. Each row below reads the result.
func (a *App) treeEntries(tree string) []filesEntry {
	byPath := map[string]*filesEntry{}
	add := func(p string, f indexedFile, ships bool) {
		e := byPath[p]
		if e == nil {
			e = &filesEntry{path: p}
			byPath[p] = e
		}
		copyOf := f
		if ships {
			e.ships = &copyOf
		} else {
			e.device = &copyOf
		}
	}

	embedded := embeddedFiles()
	switch tree {
	case filesTreeBundled:
		for _, f := range embedded {
			add(f.path, f, true)
		}
	case filesTreeSource:
		for _, f := range embedded {
			// The md/ half of the embed is the shipped side of this tree.
			if rest, ok := strings.CutPrefix(f.path, "md/"); ok {
				add(rest, indexedFile{path: rest, size: f.size}, true)
			}
		}
		for _, f := range a.walkStorage("md") {
			add(f.path, f, false)
		}
	default: // served
		for _, f := range embedded {
			if strings.HasPrefix(f.path, "md/") {
				continue // a note is not served from html/
			}
			add(f.path, f, true)
		}
		for _, f := range a.walkStorage("html") {
			add(f.path, f, false)
		}
	}

	out := make([]filesEntry, 0, len(byPath))
	for _, e := range byPath {
		out = append(out, *e)
	}
	return out
}

// normalizeFilesDir changes ?dir= into a logical directory prefix, "" or a
// string that ends with "/". The code uses it ONLY as a string prefix, and
// never joins it into a file system path.
func normalizeFilesDir(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(filepath.ToSlash(raw), "/")
	clean := path.Clean(raw)
	if clean == "." || clean == "/" {
		return ""
	}
	// path.Clean keeps a leading "..", thus check after Clean.
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	if clean == filesExcludedDir || strings.HasPrefix(clean, filesExcludedDir+"/") {
		return ""
	}
	return clean + "/"
}

// normalizeFilesTree accepts only the three known values. A link with a dir
// and no tree is from an older page that showed only html/, thus it goes to
// the Served tree.
func normalizeFilesTree(raw, dir string) string {
	switch raw {
	case filesTreeBundled, filesTreeServed, filesTreeSource:
		return raw
	}
	if dir != "" {
		return filesTreeServed
	}
	return ""
}

// foldToDir splits one tree at dir into the directories below it and the
// entries in it. The directory totals are RECURSIVE, and a name counts one
// time.
func foldToDir(entries []filesEntry, dir string) (dirs []filesDirRow, here []filesEntry, bytes int64, count int) {
	byDir := map[string]*filesDirRow{}
	for _, e := range entries {
		if dir != "" && !strings.HasPrefix(e.path, dir) {
			continue
		}
		rest := e.path[len(dir):]
		if rest == "" {
			continue
		}
		size := e.bytes()
		bytes += size
		count++

		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			row := byDir[name]
			if row == nil {
				row = &filesDirRow{Name: name, Dir: dir + name + "/",
					everyShips: true, everyDevice: true}
				byDir[name] = row
			}
			row.Files++
			row.Bytes += size
			row.note(e)
			continue
		}
		here = append(here, e)
	}

	for _, row := range byDir {
		dirs = append(dirs, *row)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(here, func(i, j int) bool { return here[i].path < here[j].path })
	return dirs, here, bytes, count
}

// bytes answers the size to show for one name: the copy on the device when it
// exists, because that copy uses the storage.
func (e filesEntry) bytes() int64 {
	if e.device != nil {
		return e.device.size
	}
	if e.ships != nil {
		return e.ships.size
	}
	return 0
}

// note records the one fact that a directory row can show: the subtree is all
// shipped, or all made on the device.
func (d *filesDirRow) note(e filesEntry) {
	if e.device == nil {
		d.everyDevice = false
	} else {
		d.anyDevice = true
	}
	if e.ships == nil {
		d.everyShips = false
	} else {
		d.anyShips = true
		d.shipCount++
	}
}

// These are the five color classes. omn-go-core.css holds one token for each
// class, with a value for each theme.
const (
	filesColorApp     = "files-c-app"     // the next version replaces this file
	filesColorAlert   = "files-c-alert"   // ... and the device copy differs
	filesColorKeep    = "files-c-keep"    // yours, kept
	filesColorDerived = "files-c-derived" // made again when needed
	filesColorPlain   = "files-c-plain"   // nothing at stake
)

// filesKindIcon answers the Material Icons ligature for the kind of a file.
// The bundled font css/fonts/material-icons.woff2 has each name here.
// "javascript", "css" and "html" draw as monograms.
func filesKindIcon(name string, isDir bool) string {
	if isDir {
		return "folder"
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "html"
	case ".md":
		return "article"
	case ".js":
		return "javascript"
	case ".css":
		return "css"
	case ".json", ".jsonl":
		return "data_object"
	case ".txt":
		return "subject"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp":
		return "image"
	case ".woff", ".woff2", ".ttf", ".otf":
		return "text_fields"
	}
	return "insert_drive_file"
}

// filesEditable decides whether a row offers an "edit" link. A compiled .html
// page has its own Edit button. A file that is not text gets no link. See
// editableFileType. SVG is an image, thus it gets no link.
func (a *App) filesEditable(logical string) bool {
	if strings.HasSuffix(strings.ToLower(logical), ".html") {
		return false
	}
	return a.editableFileType(logical)
}

// editableFileType reports whether the content type of logical is text that
// an editor can open. The editor routes use it too. A picture, a font, an
// audio file or a video file must not open an editor.
func (a *App) editableFileType(logical string) bool {
	ct := a.resolveContentType(logical)
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

// isVersionDependent reports whether the app owns a path, from
// versionDependentAssets in assets.go. want is relative to the storage
// directory.
func isVersionDependent(want string) bool {
	for _, v := range versionDependentAssets {
		if v == want {
			return true
		}
	}
	return false
}

// filesSameBytes answers whether two copies are the same file. It reads the
// bytes only up to filesCompareMax. Above that, checked is false.
func filesSameBytes(embeddedLogical, diskPath string, size int64) (same bool, checked bool) {
	if size > filesCompareMax {
		return false, false
	}
	emb, err := staticFS.ReadFile(embeddedLogical)
	if err != nil {
		return false, false
	}
	disk, err := os.ReadFile(diskPath)
	if err != nil {
		return false, false
	}
	return bytes.Equal(emb, disk), true
}

// filesMirrorState describes the .txt pair of note_files.go. An equal size
// and mtime is the answer with no read, because copyFileWithTime copies the
// mtime. An older copy "waits for restart". A newer copy was "edited
// outside", and one save in the app editor copies it back. A pair that agrees
// says NOTHING.
func filesMirrorState(source, copyOf *indexedFile) (word, color string, extra string) {
	if source == nil || copyOf == nil {
		return "", "", ""
	}
	if source.size == copyOf.size && source.mod.Equal(copyOf.mod) {
		return "", "", ""
	}
	if copyOf.mod.After(source.mod) {
		return "edited outside", filesColorAlert, "save it once in the editor"
	}
	return "waits for restart", filesColorDerived, ""
}

// filesRowFor makes one row of the tree in view.
func (a *App) filesRowFor(tree string, e filesEntry) filesFileRow {
	name := path.Base(e.path)
	row := filesFileRow{
		Name: name,
		Path: e.path,
		Kind: filesKindIcon(name, false),
		Size: filesSize(e.bytes()),
	}

	switch tree {
	case filesTreeBundled:
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		row.OwnerColor = filesColorApp
		// A starter note opens as its PAGE. This tree has no edit link,
		// because an edit changes the copy on the device, which has its own
		// row.
		if md, ok := strings.CutPrefix(e.path, "md/"); ok {
			row.URL = "/" + strings.TrimSuffix(md, ".md") + ".html"
		} else {
			row.URL = "/" + e.path
		}
		return row

	case filesTreeSource:
		row.URL = "/" + strings.TrimSuffix(e.path, ".md") + ".html"
		if !strings.HasSuffix(strings.ToLower(e.path), ".md") {
			row.URL = "/" + e.path
		}
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if a.filesEditable(e.path) {
			row.EditURL = row.URL + "?edit=true"
			if strings.HasSuffix(strings.ToLower(e.path), ".md") {
				// The page address opens the editor on the source, the same
				// as the Edit button of the page.
				row.EditURL = "/" + strings.TrimSuffix(e.path, ".md") + ".html?edit=true"
			}
		}
		if isLocalOnlyPath("md/"+e.path) || strings.HasPrefix(e.path, "local/") {
			row.Extra = append(row.Extra, "local only")
		}

	default: // served
		row.URL = "/" + e.path
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if a.filesEditable(e.path) {
			row.EditURL = row.URL + "?edit=true"
		}
	}

	a.filesState(tree, e, &row)
	return row
}

// filesStoragePath maps a logical path of one tree to the form of
// versionDependentAssets.
func filesStoragePath(tree, logical string) string {
	switch tree {
	case filesTreeSource:
		return "md/" + logical
	case filesTreeBundled:
		if strings.HasPrefix(logical, "md/") {
			return logical
		}
		return "html/" + logical
	}
	return "html/" + logical
}

// filesEmbeddedPath maps a logical path to its path in staticFS.
func filesEmbeddedPath(tree, logical string) string {
	if tree == filesTreeSource {
		return "frontend/md/" + logical
	}
	if strings.HasPrefix(logical, "md/") {
		return "frontend/" + logical
	}
	return "frontend/html/" + logical
}

// filesState sets the word of the first line, its color and the other facts.
// Here most rows get no word at all.
func (a *App) filesState(tree string, e filesEntry, row *filesFileRow) {
	if e.device != nil {
		row.Mod = e.device.mod.Format("2006-01-02")
		row.ModFull = e.device.mod.Format("2006-01-02 15:04")
	}
	row.OwnerColor = filesColorApp

	switch {
	case e.ships != nil && e.device == nil:
		// The file ships, and no request asked for it yet.
		row.State, row.StateColor = "not extracted", filesColorPlain
		if row.AppOwned {
			row.StateColor = filesColorApp
		}
		row.Size = filesSize(e.ships.size)
		row.Mod, row.ModFull = "", ""

	case e.ships != nil && e.device != nil:
		same, checked := true, true
		if e.ships.size != e.device.size {
			same = false
		} else {
			same, checked = filesSameBytes(filesEmbeddedPath(tree, e.path), a.filesDiskPath(tree, e.path), e.device.size)
		}
		switch {
		case !checked && e.ships.size == e.device.size:
			row.State, row.StateColor = "same size", filesColorPlain
		case same:
			// The row stays SILENT, because the copy here equals the build.
			// "app-owned" on the second line still tells what the next
			// version does to the file.
		default:
			row.State, row.StateColor = "changed here", filesColorKeep
			if row.AppOwned {
				row.StateColor = filesColorAlert
				row.OwnerColor = filesColorAlert
			}
			// Show both sizes only when they READ differently. Two files that
			// differ by one line can both show "68 KB".
			if from, to := filesSize(e.ships.size), filesSize(e.device.size); from != to {
				row.Size = from + " → " + to
			}
		}

	default:
		// A file only on the device says NOTHING. The .txt pair of
		// note_files.go is the exception, when the two copies differ.
		if !isSyncedNoteFile(e.path) {
			return
		}
		var source, copyOf *indexedFile
		switch tree {
		case filesTreeServed:
			source, copyOf = a.filesStat("md", e.path), e.device
		case filesTreeSource:
			source, copyOf = e.device, a.filesStat("html", e.path)
		}
		word, color, extra := filesMirrorState(source, copyOf)
		if word == "" {
			return
		}
		row.State, row.StateColor = word, color
		if extra != "" {
			row.Extra = append(row.Extra, extra)
		}
	}
}

// filesDiskPath answers the disk path of a logical path in the tree in view.
func (a *App) filesDiskPath(tree, logical string) string {
	sub := "html"
	if tree == filesTreeSource {
		sub = "md"
	}
	return filepath.Join(a.StorageDir, sub, filepath.FromSlash(logical))
}

// filesStat reads one file of the storage tree for the .txt comparison.
func (a *App) filesStat(sub, logical string) *indexedFile {
	st, err := os.Stat(filepath.Join(a.StorageDir, sub, filepath.FromSlash(logical)))
	if err != nil || st.IsDir() {
		return nil
	}
	return &indexedFile{path: logical, size: st.Size(), mod: st.ModTime()}
}

// filesFromTheApp is the end of the word that a directory row can show.
// filesLegend must find the same text, thus it is a constant.
const filesFromTheApp = "from the app"

// serveFilesPage answers GET /OMNGoFiles.html. It needs a login, thus it has
// its own route outside the catch-all. It asks hasRole and answers a refusal
// with a page, not with the plain text of authMiddleware.
// TestFilesPage_Authorization holds the rule.
func (a *App) serveFilesPage(w http.ResponseWriter, r *http.Request) {
	dir := normalizeFilesDir(r.URL.Query().Get("dir"))
	view := filesPageView{
		Dir:  dir,
		Tree: normalizeFilesTree(r.URL.Query().Get("tree"), dir),
	}

	if !a.hasRole(r) {
		view.Denied = true
		a.writeFilesPage(w, view)
		return
	}

	if view.Tree == "" {
		view.Cards = a.filesCards()
		a.writeFilesPage(w, view)
		return
	}

	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	view.ShowingAll = all
	view.Crumbs = filesCrumbs(view.Tree, view.Dir)

	dirs, here, subtreeBytes, subtreeCount := foldToDir(a.treeEntries(view.Tree), view.Dir)
	view.Dirs = dirs
	view.Total = len(here)
	if !all && len(here) > filesDirLimit {
		view.Hidden = len(here) - filesDirLimit
		here = here[:filesDirLimit]
	}
	for _, e := range here {
		view.Files = append(view.Files, a.filesRowFor(view.Tree, e))
	}
	view.Empty = len(view.Dirs) == 0 && len(view.Files) == 0
	view.Summary = filesSummary(subtreeCount, subtreeBytes, view.Dir != "" || len(dirs) > 0, view.Files)
	view.Legend = filesLegend(view.Tree, view.Files, view.Dirs)

	a.writeFilesPage(w, view)
}

// filesCards makes the first screen: one button for each tree, with its size.
func (a *App) filesCards() []filesTreeCard {
	count := func(entries []filesEntry) (int, int64) {
		var b int64
		for _, e := range entries {
			b += e.bytes()
		}
		return len(entries), b
	}
	nb, bb := count(a.treeEntries(filesTreeBundled))
	ns, bs := count(a.treeEntries(filesTreeServed))
	nm, bm := count(a.treeEntries(filesTreeSource))
	return []filesTreeCard{
		{Key: filesTreeBundled, Icon: "inventory_2", Title: "Bundled",
			Where: "inside the application",
			Count: filesCountLabel(nb) + " · " + filesSize(bb)},
		{Key: filesTreeServed, Icon: "public", Title: "Served", Class: "files-card-served",
			Where: "html/ — what a URL finds",
			Count: filesCountLabel(ns) + " · " + filesSize(bs)},
		{Key: filesTreeSource, Icon: "article", Title: "Source", Class: "files-card-source",
			Where: "md/ — your notes",
			Count: filesCountLabel(nm) + " · " + filesSize(bm)},
	}
}

// filesSummary is the one line under the crumb. The count and the size are
// RECURSIVE: "how large is this whole folder". The state counts are for the
// rows of THIS directory, because a reader can act on those rows.
func filesSummary(count int, bytes int64, below bool, rows []filesFileRow) string {
	out := filesCountLabel(count) + " · " + filesSize(bytes)
	if below {
		out += " below"
	}
	var changed, absent int
	for _, r := range rows {
		switch r.State {
		case "changed here":
			changed++
		case "not extracted":
			absent++
		}
	}
	if changed > 0 {
		out += fmt.Sprintf(" · %d changed here", changed)
	}
	if absent > 0 {
		out += fmt.Sprintf(" · %d not extracted", absent)
	}
	return out
}

// filesLegend explains the words that THIS page uses. The key is the pair of
// word and color, because "changed here" is green on a kept file and red on a
// replaced one.
func filesLegend(tree string, rows []filesFileRow, dirs []filesDirRow) []filesLegendItem {
	type key struct{ word, color string }
	seen := map[key]bool{}
	for _, r := range rows {
		if r.State != "" {
			seen[key{r.State, r.StateColor}] = true
		}
		if r.AppOwned {
			seen[key{"app-owned", r.OwnerColor}] = true
		}
		for _, x := range r.Extra {
			seen[key{x, filesColorPlain}] = true
		}
	}
	for _, d := range dirs {
		if w, c := filesDirNote(tree, d); w != "" {
			seen[key{w, c}] = true
		}
	}

	// Give one line for each pair, in a fixed order. A pair without a line
	// here gets none.
	all := []filesLegendItem{
		{Color: filesColorAlert, Word: "changed here",
			Text: "you changed it, and the next version of OMN-Go replaces it. OMN-Go backs up your copy first"},
		{Color: filesColorKeep, Word: "changed here",
			Text: "you changed a file that came with OMN-Go. OMN-Go keeps your copy"},
		{Color: filesColorApp, Word: "app-owned",
			Text: "the next version of OMN-Go replaces this file"},
		// This is the same word in the alert color. filesState makes BOTH
		// words of such a row red, because both describe one outcome.
		{Color: filesColorAlert, Word: "app-owned",
			Text: "the next version of OMN-Go replaces this file, and your change goes to a backup"},
		{Color: filesColorApp, Word: "not extracted",
			Text: "OMN-Go carries this file, and this device has no copy of it yet"},
		{Color: filesColorPlain, Word: "not extracted",
			Text: "OMN-Go carries this file, and this device has no copy of it yet"},
		{Color: filesColorPlain, Word: "same size",
			Text: "the file is too large to compare, and the two copies have the same size"},
		{Color: filesColorAlert, Word: "edited outside",
			Text: "an editor outside OMN-Go wrote the copy in html/. Save the file one time in the editor to copy it back to md/"},
		{Color: filesColorDerived, Word: "waits for restart",
			Text: "the file in md/ is newer. The next start of OMN-Go copies it into html/"},
		{Color: filesColorPlain, Word: "local only",
			Text: "git synchronization does not carry this file. It stays on this device"},
	}
	var out []filesLegendItem
	for _, item := range all {
		k := key{item.Word, item.Color}
		if seen[k] {
			out = append(out, item)
			delete(seen, k)
		}
	}
	// A directory line holds a count, thus it cannot be in the table above.
	// One line covers each of them.
	for k := range seen {
		if strings.Contains(k.word, filesFromTheApp) {
			out = append(out, filesLegendItem{Color: k.color, Word: "… " + filesFromTheApp,
				Text: "OMN-Go delivered that many of the files below this directory"})
			break
		}
	}
	return out
}

func (a *App) writeFilesPage(w http.ResponseWriter, view filesPageView) {
	title := "Files"
	switch view.Tree {
	case filesTreeBundled:
		title = "Bundled files"
	case filesTreeServed:
		title = "Served files"
	case filesTreeSource:
		title = "Note source"
	}
	if view.Dir != "" {
		title += ": " + strings.TrimSuffix(view.Dir, "/")
	}
	body := renderFilesPage(view)
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars(compiled))
}

// filesCrumbs makes the breadcrumb, from the root of the tree to the current
// directory. Each crumb carries its own trailing slash, and NOTHING separates
// two crumbs. A separator would show "html/ / js/".
func filesCrumbs(tree, dir string) []filesCrumb {
	root := "bundled/"
	switch tree {
	case filesTreeServed:
		root = "html/"
	case filesTreeSource:
		root = "md/"
	}
	out := []filesCrumb{{Label: root, Dir: ""}}
	if dir == "" {
		return out
	}
	acc := ""
	for _, part := range strings.Split(strings.TrimSuffix(dir, "/"), "/") {
		acc += part + "/"
		out = append(out, filesCrumb{Label: part + "/", Dir: acc})
	}
	return out
}

// filesSize shows a byte count in a short form.
func filesSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func filesCountLabel(n int) string {
	if n == 1 {
		return "1 file"
	}
	return itoa(n) + " files"
}
