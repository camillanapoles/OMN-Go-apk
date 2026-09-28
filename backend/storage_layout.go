package backend

import "path/filepath"

// storageLayout is the storage directory. Its methods name each place in
// it, thus no other code joins a directory name to StorageDir.
//
//	md/             The notes as Markdown.
//	html/           Each file that the server sends.
//	db/             The SQLite files.
//	asset_backups/  The old copies of the application files.
//	.git/           The repository of the sync.
//
// The files at the top, for example config.json, go through file.
type storageLayout string

// configFilename and gitignoreFilename are files at the top of the storage
// directory.
const (
	configFilename    = "config.json"
	gitignoreFilename = ".gitignore"
)

func (a *App) layout() storageLayout { return storageLayout(a.StorageDir) }

func (l storageLayout) under(dir string, elem []string) string {
	return filepath.Join(append([]string{string(l), dir}, elem...)...)
}

func (l storageLayout) md(elem ...string) string           { return l.under("md", elem) }
func (l storageLayout) html(elem ...string) string         { return l.under("html", elem) }
func (l storageLayout) db(elem ...string) string           { return l.under("db", elem) }
func (l storageLayout) git(elem ...string) string          { return l.under(".git", elem) }
func (l storageLayout) assetBackups(elem ...string) string { return l.under("asset_backups", elem) }

// file answers a path below the top of the storage directory.
func (l storageLayout) file(elem ...string) string { return l.under("", elem) }
