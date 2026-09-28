package backend

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ----------------------------------------------------------------------
// The HTTP endpoints and the page
// ----------------------------------------------------------------------

var dbBackupsPageTmpl = loadTemplate("db_backups.html")

// serveDBBackupsPage renders the Database Backups page. The button at the top
// of the Config page opens it. The page gets its data from GET
// /api/db/backups, thus the template needs no fill().
func (a *App) serveDBBackupsPage(w http.ResponseWriter, r *http.Request) {
	a.renderPage(w, http.StatusOK, "DB_Backups", pageHeader("Database Backups", "Settings"), dbBackupsPageTmpl)
}

// handleDBBackupCreate answers POST /api/db/backup?db=NAME.
func (a *App) handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	if !dbNameRe.MatchString(name) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	file, pruned, err := a.createDBBackup(name)
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"file":   file,
		"pruned": pruned,
	})
}

// handleDBRestore answers POST /api/db/restore?db=NAME&file=FILENAME.
func (a *App) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	fileName := r.URL.Query().Get("file")
	if !dbNameRe.MatchString(name) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	if !backupFileRe.MatchString(fileName) {
		a.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid backup filename %q", fileName))
		return
	}
	a.dbRestoreMu.Lock()
	err := a.restoreDBFromBackup(name, fileName)
	a.dbRestoreMu.Unlock()
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

type backupFileView struct {
	File     string `json:"file"`
	Size     int64  `json:"size"`
	MTime    string `json:"mtime"`
	Created  string `json:"created"`
	Hostname string `json:"hostname"`
	Objects  int    `json:"objects"`
	Rows     int    `json:"rows"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

type backupDBView struct {
	Name         string           `json:"name"`
	SQLiteExists bool             `json:"sqlite_exists"`
	Size         int64            `json:"size"`
	MTime        string           `json:"mtime"`
	State        string           `json:"state"`
	Backups      []backupFileView `json:"backups"`
}

// handleDBBackupList answers GET /api/db/backups with each value of the
// /db_backups page. It never opens a database, because an open can start the
// bootstrap restore, and a listing must change nothing.
func (a *App) handleDBBackupList(w http.ResponseWriter, r *http.Request) {

	// Take each database that has a .sqlite file or only backups, as on a
	// fresh device before the first open.
	names := map[string]bool{}
	if entries, err := os.ReadDir(a.layout().db()); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sqlite") {
				n := strings.TrimSuffix(e.Name(), ".sqlite")
				if dbNameRe.MatchString(n) {
					names[n] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(dbBackupRoot(a)); err == nil {
		for _, e := range entries {
			if e.IsDir() && dbNameRe.MatchString(e.Name()) {
				names[e.Name()] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	depth := a.config.get().BackupPruneDepth
	if depth <= 0 {
		depth = 3
	}

	dbs := make([]backupDBView, 0, len(sorted))
	for _, name := range sorted {
		v := backupDBView{Name: name}
		// Keep the raw mtime for the state test below. The RFC3339 text in
		// v.MTime has a precision of one second. A test against it would show
		// "backup newer" directly after a backup, where createDBBackup made
		// the two mtimes equal.
		var dbMTime time.Time
		if info, err := os.Stat(a.userDBPath(name)); err == nil && info.Size() > 0 {
			v.SQLiteExists = true
			v.Size = info.Size()
			dbMTime = info.ModTime()
			v.MTime = dbMTime.UTC().Format(time.RFC3339)
		}

		files, _ := a.listBackupFiles(name)
		var newestMTime time.Time
		newestValid := false
		for i, fn := range files {
			full := filepath.Join(a.dbBackupDir(name), fn)
			bv := backupFileView{File: fn}
			if info, err := os.Stat(full); err == nil {
				bv.Size = info.Size()
				bv.MTime = info.ModTime().UTC().Format(time.RFC3339)
				if i == 0 {
					newestMTime = info.ModTime()
				}
			}
			if h, err := readBackupHeader(full); err == nil && h.Database == name {
				bv.Valid = true
				bv.Created = h.Created
				bv.Hostname = h.Hostname
				bv.Objects = h.Objects
				bv.Rows = h.Rows
				if i == 0 {
					newestValid = true
				}
			} else if err != nil {
				bv.Error = err.Error()
			} else {
				bv.Error = fmt.Sprintf("header names database %q", h.Database)
			}
			v.Backups = append(v.Backups, bv)
		}
		if v.Backups == nil {
			// A database with no backup must give "backups":[]. A nil slice
			// gives JSON null, and the page script fails on .length.
			v.Backups = []backupFileView{}
		}

		switch {
		case len(files) == 0:
			v.State = "none"
		case !newestValid:
			v.State = "invalid"
		case !v.SQLiteExists:
			v.State = "missing"
		default:
			switch {
			case newestMTime.After(dbMTime):
				v.State = "backup_newer"
			case dbMTime.After(newestMTime):
				v.State = "dirty"
			default:
				v.State = "insync"
			}
		}
		dbs = append(dbs, v)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "success",
		"hostname":    a.config.get().Hostname,
		"prune_depth": depth,
		"databases":   dbs,
	})
}
