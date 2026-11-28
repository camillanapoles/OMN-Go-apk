package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// --- The storage directory that config.json chooses ---

// followStorageChain moves the storage to the directory that the config
// names, and it walks one pointer at a time while each new directory names
// another. Android keeps the directory of its wrapper, and the desktop
// walks. See
// doc/decisions/0033-the-config-chooses-the-storage-directory.md.
func (a *App) followStorageChain() {
	next := storageChainDir(a.StorageDir, peekStorageDir)
	if next == a.StorageDir {
		return
	}
	if err := os.MkdirAll(next, 0755); err != nil {
		a.log(logx.Storage).Errf("storage_dir %s: %v", next, err)
		return
	}
	a.StorageDir = next
}

// storageChainDir walks the pointers of config.json files from start, up to
// three hops. A repeat of a directory, an empty answer and a nested pair
// each end the walk at the place it stands. The walk reads no file and
// writes nothing: the peek function of the caller does the read.
func storageChainDir(start string, peek func(string) string) string {
	cur := start
	seen := map[string]bool{start: true}
	for hop := 0; hop < 3; hop++ {
		next := strings.TrimSpace(peek(filepath.Join(cur, storage.ConfigFilename)))
		if next == "" || seen[next] {
			return cur
		}
		if inside(next, cur) || inside(cur, next) ||
			inside(next, start) || inside(start, next) {
			return cur
		}
		seen[next] = true
		cur = next
	}
	return cur
}

// inside answers whether child sits in parent, with both read as
// directories. A storage that holds its own next hop would carry its sync
// tree inside itself, thus the walk refuses the pair.
func inside(parent, child string) bool {
	_, ok := storage.RelInside(parent, child)
	return ok
}

// peekStorageDir reads the storage_dir field of the config at path. An
// absent file, a broken file and a field that is absent each answer "". The
// read writes nothing, thus a broken config keeps the app on the directory
// it already has.
func peekStorageDir(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var c struct {
		StorageDir string `json:"storage_dir"`
	}
	if json.Unmarshal(data, &c) != nil {
		return ""
	}
	return strings.TrimSpace(c.StorageDir)
}
