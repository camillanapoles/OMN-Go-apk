package backend

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
)

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
