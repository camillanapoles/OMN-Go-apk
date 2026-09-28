package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ----------------------------------------------------------------------
// The sync paths
// ----------------------------------------------------------------------
//
// SyncRepo at the end of this file is the one entry point. Two files under
// .git/ make a merge reversible, also over a restart. OMNGO_PREMERGE_HEAD
// holds the HEAD from before pull_mark, and pull_abort resets to it.

func (a *App) premergeHeadPath() string {
	return a.layout().git("OMNGO_PREMERGE_HEAD")
}

func (a *App) savePremergeHead(h plumbing.Hash) {
	if err := os.WriteFile(a.premergeHeadPath(), []byte(h.String()), 0644); err != nil {
		a.log(logSync).errf("failed to save pre-merge HEAD: %v", err)
	}
}

func (a *App) loadPremergeHead() (plumbing.Hash, bool) {
	data, err := os.ReadFile(a.premergeHeadPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (a *App) clearPremergeHead() {
	os.Remove(a.premergeHeadPath())
}

// OMNGO_MERGE_PARENT holds the remote tip that pull_mark merges. HEAD does
// not move. At the next commit, commitLocalChanges reads the file and makes a
// real merge commit with two parents.

func (a *App) mergeParentPath() string {
	return a.layout().git("OMNGO_MERGE_PARENT")
}

func (a *App) saveMergeParent(h plumbing.Hash) {
	if err := os.WriteFile(a.mergeParentPath(), []byte(h.String()), 0644); err != nil {
		a.log(logSync).errf("failed to save pending merge parent: %v", err)
	}
}

func (a *App) loadMergeParent() (plumbing.Hash, bool) {
	data, err := os.ReadFile(a.mergeParentPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (a *App) clearMergeParent() {
	os.Remove(a.mergeParentPath())
}

// cleanUntrackedFiles deletes each untracked file that .gitignore does not
// match. Only a force pull calls it. A plain pull or push never touches a
// file that git does not track.
func (a *App) cleanUntrackedFiles(wTree *git.Worktree, matcher gitignore.Matcher) {
	status, err := wTree.Status()
	if err != nil {
		a.log(logSync).errf("force pull: could not compute status for cleanup: %v", err)
		return
	}
	for name, fileStat := range status {
		if fileStat.Worktree != git.Untracked {
			continue
		}
		// Test the name as a safety net, the same as the other sync paths.
		// config.json holds the passwords of this device, whatever the
		// .gitignore of the remote says.
		if name == "config.json" {
			a.log(logSync).debugf("force pull: keeping root config.json (preserve locally)")
			continue
		}
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			a.log(logSync).debugf("force pull: keeping ignored file %s", name)
			continue
		}
		full := a.layout().file(name)
		if err := os.Remove(full); err != nil {
			a.log(logSync).errf("force pull: failed to delete %s: %v", name, err)
		} else {
			a.log(logSync).debugf("force pull: deleted untracked file %s", name)
		}
	}
}

// trackedWorktreeIsDirty reports whether a TRACKED file has a change that is
// not committed. A pull overwrites each tracked file, thus it must refuse
// first. The native Pull of go-git made this check. See
// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
func trackedWorktreeIsDirty(wTree *git.Worktree) (bool, error) {
	status, err := wTree.Status()
	if err != nil {
		return false, err
	}
	for _, fileStat := range status {
		if fileStat.Worktree == git.Untracked {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// syncProgressWriter sends the sideband progress of git, for example
// "Counting objects: 45%", to the log and thus to the sync overlay. A
// progress line ends with '\r', because the remote writes it again in place.
// The writer sends one line for each interval, because JSLogger drops a line
// when a client channel is full. go-git calls Write from one goroutine.
type syncProgressWriter struct {
	// go-git makes the call, thus the writer carries the App that logs.
	app  *App
	buf  []byte
	last time.Time
}

const syncProgressInterval = 300 * time.Millisecond

func (w *syncProgressWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	// Keep the text after the last terminator. A newer progress line replaces
	// an older one.
	if i := bytes.LastIndexAny(w.buf, "\r\n"); i != -1 {
		line := string(w.buf[:i])
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
		if j := strings.LastIndexAny(line, "\r\n"); j != -1 {
			line = line[j+1:]
		}
		line = strings.TrimSpace(line)
		// The client adds the label "remote:". Remove a label that the server
		// sent, or the line reads "remote: remote:".
		line = strings.TrimSpace(strings.TrimPrefix(line, "remote:"))
		if line != "" && time.Since(w.last) >= syncProgressInterval {
			w.last = time.Now()
			w.app.log(logSync).debugf("remote: %s", line)
		}
	}
	// Never return an error. A fault of the progress log must not stop a
	// fetch or a push.
	return len(p), nil
}

// These are the sentinel errors of the sync. Each one is a state that the
// user must resolve, and not a failure. syncErrorStatus maps each one to its
// wire status with errors.Is.
var (
	// ErrSyncConflict: a plain pull cannot fast-forward. The user chooses
	// abort or a 3-way merge.
	ErrSyncConflict = errors.New("sync: fast-forward not possible")
	// ErrPushConflict: the remote refused a push without force. The user must
	// pull first.
	ErrPushConflict = errors.New("sync: push rejected, remote has new commits")
	// ErrCommitMessageRequired: a commit or a force push needs a message.
	ErrCommitMessageRequired = errors.New("sync: commit message required")
)

// syncConflictError carries the paths in conflict to the dialog. It unwraps
// to ErrSyncConflict, thus errors.Is still matches it.
type syncConflictError struct {
	Files []string
}

func (e *syncConflictError) Error() string { return ErrSyncConflict.Error() }
func (e *syncConflictError) Unwrap() error { return ErrSyncConflict }

// conflictingPaths answers, sorted, each tracked path with an uncommitted
// change that differs from the remote copy. The conflict dialog and the
// marker loop of syncPullMerge both read it, thus the two always agree.
func conflictingPaths(wTree *git.Worktree, remoteTree *object.Tree) ([]string, error) {
	status, err := wTree.Status()
	if err != nil {
		return nil, err
	}
	var paths []string
	for path, fileStatus := range status {
		if fileStatus.Worktree != git.Modified && fileStatus.Staging != git.Modified {
			continue
		}
		file, err := wTree.Filesystem.Open(path)
		if err != nil {
			continue
		}
		localContent, _ := io.ReadAll(file)
		file.Close()

		remoteFile, err := remoteTree.File(path)
		if err != nil {
			continue // remote does not have this file - nothing to reconcile
		}
		remoteContentStr, _ := remoteFile.Contents()
		if string(localContent) == remoteContentStr {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

// newSyncConflict answers a syncConflictError with the paths in conflict.
// When the list fails, it answers the bare ErrSyncConflict. A conflict thus
// never becomes a hard error.
func (a *App) newSyncConflict(repo *git.Repository, wTree *git.Worktree, remoteRef *plumbing.Reference) error {
	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return ErrSyncConflict
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return ErrSyncConflict
	}
	files, err := conflictingPaths(wTree, remoteTree)
	if err != nil {
		return ErrSyncConflict
	}
	return &syncConflictError{Files: files}
}

// syncErrorStatus maps a sync error to the {status, message} that runSync in
// omn-go-sync.js reads. ok is false for each other error, and the caller then
// answers "error".
func syncErrorStatus(err error) (status, message string, ok bool) {
	switch {
	case errors.Is(err, ErrSyncConflict):
		return "conflict", "Fast-forward not possible. Choose abort or 3-way merge.", true
	case errors.Is(err, ErrPushConflict):
		return "push_conflict", "Remote has new commits. Pull before pushing.", true
	case errors.Is(err, ErrCommitMessageRequired):
		return "needs_commit_message", "Please provide a commit message.", true
	default:
		return "", "", false
	}
}

// SyncRepo runs one sync action under GitMutex. The switch at the end lists
// each action and its aliases. Only push and push_force read message.
func (a *App) SyncRepo(action string, message string) error {
	a.GitMutex.Lock()
	defer a.GitMutex.Unlock()

	repo, err := a.getOrInitRepo()
	if err != nil {
		return err
	}
	wTree, err := repo.Worktree()
	if err != nil {
		return err
	}
	remoteName, err := a.ensureRemotesAndGetActive(repo)
	if err != nil {
		return err
	}
	auth, err := a.getSSHAuth()
	if err != nil {
		return err
	}

	// After a pull, make the html/ copy of each text file beside a note
	// again. Git carries only the md/ file. See isDerivedTextPath. The walk
	// is cheap when nothing changed.
	pullDone := func(err error) error {
		if err == nil {
			a.syncNoteFilesToHTML()
		}
		return err
	}

	switch action {
	case "push", "upload":
		return a.syncPush(repo, wTree, auth, remoteName, message, false)
	case "push_force", "upload_force":
		return a.syncPush(repo, wTree, auth, remoteName, message, true)
	case "pull", "pull_ff", "download":
		return pullDone(a.syncPull(repo, wTree, auth, remoteName))
	case "pull_mark":
		return pullDone(a.syncPullMerge(repo, wTree, auth, remoteName))
	case "pull_abort", "abort":
		return pullDone(a.syncPullAbort(wTree))
	case "pull_force", "download_force":
		return pullDone(a.syncPullForce(repo, wTree, auth, remoteName))
	}
	return fmt.Errorf("unknown sync action: %s", action)
}
