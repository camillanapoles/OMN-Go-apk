package backend

// ----------------------------------------------------------------------
// Plain files beside the notes
// ----------------------------------------------------------------------
//
// md/ is the notes tree. html/ is what the server sends: a URL that is not a
// page resolves under html/ alone. See materializeAsset.
//
// A person keeps a text file beside the note that links to it, for example
// md/Log.md and md/log.txt. Git sync carries md/, and a file manager shows
// it. The link "[log](log.txt)" asks for "/log.txt", and the server reads
// html/. This file thus keeps a copy in each tree:
//
//	- syncNoteFilesToHTML copies md/ to html/ at start and after a pull.
//	- syncNoteFileToMD copies html/ to md/ after a save in the editor.
//
// A copy gets the mtime of its source, thus "newer" keeps its meaning. See
// copyFileWithTime.
//
// NOTHING HERE DELETES A FILE. This code cannot tell a removal on purpose
// from a tree that it did not see before. A wrong answer deletes the data of
// the user.

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"net.basov.omngo/backend/internal/logx"
)

// syncedNoteFileExts lists the extensions to copy. A new plain-text kind, for
// example ".csv", is one more entry.
//
// ".md" MUST NOT BE HERE. A markdown file in md/ is a NOTE, and its page is
// "/Name.html". A copy under html/ would be the same note at a second URL.
var syncedNoteFileExts = []string{".txt"}

func isSyncedNoteFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range syncedNoteFileExts {
		if ext == e {
			return true
		}
	}
	return false
}

// syncNoteFilesToHTML copies each listed file in md/ to html/, when html/ has
// no copy or the md/ copy is newer. It runs at start and after a pull.
//
// It compares the mtime, and not the content, because a content test reads
// each file at each start. A git checkout, a file manager, an editor and
// copyFileWithTime all set the mtime. The walk reads no file until it finds
// one to copy. It runs at once, because a link tap in the first second must
// find the copy.
func (a *App) syncNoteFilesToHTML() {
	mdRoot := a.layout().md()
	htmlRoot := a.layout().html()
	copied := 0

	filepath.WalkDir(mdRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isSyncedNoteFile(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(mdRoot, p)
		if relErr != nil {
			return nil
		}
		src, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		dst := filepath.Join(htmlRoot, rel)
		if dstInfo, statErr := os.Stat(dst); statErr == nil &&
			!src.ModTime().After(dstInfo.ModTime()) {
			return nil
		}
		if copyErr := copyFileWithTime(p, dst); copyErr != nil {
			a.log(logx.NoteFiles).Errf("md/%s to html/: %v", filepath.ToSlash(rel), copyErr)
			return nil
		}
		copied++
		return nil
	})

	if copied > 0 {
		a.log(logx.NoteFiles).Infof("copied %d file(s) from md/ to html/", copied)
	}
}

// syncNoteFileToMD copies a file from html/ back to md/ after a save.
// htmlPath is the path that the caller already resolved with resolvePageName.
// The function also writes a new file to md/, thus a .txt that the editor
// made joins the notes tree.
//
// KNOWN GAP: /api/edit-external opens the html/ copy in another app, and the
// server gets no event when the edit ends. Such an edit stays in html/ until
// a person saves the file in the app editor.
func (a *App) syncNoteFileToMD(htmlPath string) {
	if !isSyncedNoteFile(htmlPath) {
		return
	}
	// Stop for a path outside html/. filepath.Join RESOLVES a "../" in a
	// name, and it does not refuse it. This function must not carry such a
	// name into md/.
	rel, ok := relInside(a.layout().html(), htmlPath)
	if !ok || rel == "." {
		return
	}
	dst := a.layout().md(rel)
	if copyErr := copyFileWithTime(htmlPath, dst); copyErr != nil {
		a.log(logx.NoteFiles).Errf("html/%s to md/: %v", filepath.ToSlash(rel), copyErr)
	}
}

// copyFileWithTime copies src over dst, makes the directory of dst, and gives
// dst the mtime of src. Without the mtime, each copy is newer than its
// source, and each start copies the pair again. It streams the file, because
// a log file can be large.
func copyFileWithTime(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
