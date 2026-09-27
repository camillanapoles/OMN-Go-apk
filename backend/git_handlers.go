package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// ----------------------------------------------------------------------
// The sync HTTP handlers
// ----------------------------------------------------------------------
//
// /api/sync runs one action and answers with a status word. /api/sync/preview
// tells what an upload would send, and it changes nothing. doc/API.md holds
// both shapes. The banner of git_repo.go says what each git file holds.

// writeSyncJSON writes a {"status", "message"} JSON body. The encoder escapes
// a quote in an error message, thus the body stays valid.
func writeSyncJSON(w http.ResponseWriter, status, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status, "message": message})
}

// writeSyncConflictJSON also sends "files", the paths in conflict, for the
// conflict dialog. The list is never null, thus the page needs no guard.
func writeSyncConflictJSON(w http.ResponseWriter, message string, files []string) {
	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "conflict",
		"message": message,
		"files":   files,
	})
}

func (a *App) handleSync(w http.ResponseWriter, r *http.Request) {
	// r.FormValue reads the form body first, and then the query string. The
	// page posts a form. TestHandleSyncReadsTheQueryString tests the query
	// string.
	if err := r.ParseForm(); err != nil {
		writeSyncJSON(w, "error", fmt.Sprintf("bad request: %v", err))
		return
	}

	action := r.FormValue("action")
	if action == "" {
		action = "pull"
	}
	message := r.FormValue("message")
	force := r.FormValue("force") == "true"

	// The Force checkbox is a separate field. The handler changes the action
	// to its *_force name, thus SyncRepo reads one set of action names.
	if force {
		switch action {
		case "pull", "pull_ff", "download":
			action = "pull_force"
		case "push", "upload":
			action = "push_force"
		}
	}

	if err := a.SyncRepo(action, message); err != nil {
		if status, msg, ok := syncErrorStatus(err); ok {
			// A conflict carries the paths in conflict, and the dialog lists
			// them. Each other status gets the plain body.
			var ce *syncConflictError
			if status == "conflict" && errors.As(err, &ce) {
				writeSyncConflictJSON(w, msg, ce.Files)
			} else {
				writeSyncJSON(w, status, msg)
			}
		} else {
			writeSyncJSON(w, "error", err.Error())
		}
		return
	}

	writeSyncJSON(w, "success", "")
}

func (a *App) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	action := r.URL.Query().Get("action")
	if action != "upload" {
		http.Error(w, "Only upload preview supported", 400)
		return
	}

	// GitMutex stops this read while a sync changes the worktree.
	a.GitMutex.Lock()
	defer a.GitMutex.Unlock()

	repo, err := a.getOrInitRepo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
		return
	}
	wTree, err := repo.Worktree()
	if err != nil {
		http.Error(w, fmt.Sprintf("Worktree error: %v", err), 500)
		return
	}

	matcher, err := a.loadGitignoreMatcher(wTree)
	if err != nil {
		matcher = gitignore.NewMatcher(nil)
	}

	status, err := wTree.Status()
	if err != nil {
		http.Error(w, fmt.Sprintf("Status error: %v", err), 500)
		return
	}

	var files []string
	for name, fileStat := range status {
		// Skip an ignored path and the config.json at the root.
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			continue
		}
		if name == "config.json" {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			files = append(files, name)
		}
	}

	// A tracked path that git must not track leaves the repository at the
	// next commit, and the other devices delete it. See
	// untrackLocalOnlyPaths. The status does not show such a path when its
	// content did not change, thus the preview reads the index too.
	files = append(files, a.untrackTrackedPaths(repo)...)

	// A clean worktree can still hold a commit that the remote does not have:
	// a push failed, or the active profile changed. The page must then offer
	// the push. The check runs only when no file waits. With a file to
	// commit, the upload pushes in all cases. See
	// doc/decisions/0011-push-each-time-and-let-the-remote-answer.md.
	ahead := unpushedState{}
	if len(files) == 0 {
		if remoteName, rErr := a.ensureRemotesAndGetActive(repo); rErr != nil {
			ahead.Error = rErr.Error()
		} else if auth, aErr := a.getSSHAuth(); aErr != nil {
			// With no usable key, the local half of the check still finds a
			// failed push.
			ahead = a.aheadOfRemote(repo, remoteName, nil)
			if !ahead.Verified && ahead.Error == "" {
				ahead.Error = aErr.Error()
			}
		} else {
			ahead = a.aheadOfRemote(repo, remoteName, auth)
		}
	}

	// The list is never null, thus the page reads .length with no guard.
	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(syncPreviewResponse{
		Files:       files,
		Unpushed:    ahead.Unpushed,
		Remote:      ahead.Remote,
		Verified:    ahead.Verified,
		RemoteError: ahead.Error,
	})
}
