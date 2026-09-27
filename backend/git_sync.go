package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// ----------------------------------------------------------------------
// The sync paths
// ----------------------------------------------------------------------
//
// SyncRepo at the end of this file is the one entry point. Two files under
// .git/ make a merge reversible, also over a restart. OMNGO_PREMERGE_HEAD
// holds the HEAD from before pull_mark, and pull_abort resets to it.

func (a *App) premergeHeadPath() string {
	return filepath.Join(a.StorageDir, ".git", "OMNGO_PREMERGE_HEAD")
}

func (a *App) savePremergeHead(h plumbing.Hash) {
	if err := os.WriteFile(a.premergeHeadPath(), []byte(h.String()), 0644); err != nil {
		a.logErrf(logSync, "failed to save pre-merge HEAD: %v", err)
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
	return filepath.Join(a.StorageDir, ".git", "OMNGO_MERGE_PARENT")
}

func (a *App) saveMergeParent(h plumbing.Hash) {
	if err := os.WriteFile(a.mergeParentPath(), []byte(h.String()), 0644); err != nil {
		a.logErrf(logSync, "failed to save pending merge parent: %v", err)
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
		a.logErrf(logSync, "force pull: could not compute status for cleanup: %v", err)
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
			a.logDebugf(logSync, "force pull: keeping root config.json (preserve locally)")
			continue
		}
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			a.logDebugf(logSync, "force pull: keeping ignored file %s", name)
			continue
		}
		full := filepath.Join(a.StorageDir, name)
		if err := os.Remove(full); err != nil {
			a.logErrf(logSync, "force pull: failed to delete %s: %v", name, err)
		} else {
			a.logDebugf(logSync, "force pull: deleted untracked file %s", name)
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
			w.app.logDebugf(logSync, "remote: %s", line)
		}
	}
	// Never return an error. A fault of the progress log must not stop a
	// fetch or a push.
	return len(p), nil
}

// syncPull fast-forwards the local branch to the remote tip. It returns a
// conflict for a diverged history or for a tracked change that is not
// committed. The page then offers pull_abort and pull_mark.
func (a *App) syncPull(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	a.logInfof(logSync, "Pull: fetching %s", remoteName)
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	localHead, headErr := repo.Head()
	if headErr == nil && localHead.Hash() == remoteRef.Hash() {
		a.logInfof(logSync, "Pull: already up to date")
		return nil
	}

	// Refuse when a tracked file has a change. See trackedWorktreeIsDirty.
	dirty, dErr := trackedWorktreeIsDirty(wTree)
	if dErr != nil {
		return fmt.Errorf("status check failed: %v", dErr)
	}
	if dirty {
		a.logInfof(logSync, "Pull: local tracked changes present, cannot fast-forward")
		return a.newSyncConflict(repo, wTree, remoteRef)
	}

	// Allow only a fast-forward. A HEAD that is not an ancestor of the remote
	// tip holds commits that the remote does not have. A jump to the remote
	// tree would lose them. An unborn branch has nothing to lose.
	if headErr == nil {
		localCommit, cErr := repo.CommitObject(localHead.Hash())
		if cErr != nil {
			return fmt.Errorf("local HEAD commit lookup failed: %v", cErr)
		}
		remoteCommit, rcErr := repo.CommitObject(remoteRef.Hash())
		if rcErr != nil {
			return fmt.Errorf("remote commit lookup failed: %v", rcErr)
		}
		isAncestor, aErr := localCommit.IsAncestor(remoteCommit)
		if aErr != nil {
			return fmt.Errorf("ancestry check failed: %v", aErr)
		}
		if !isAncestor {
			a.logInfof(logSync, "Pull: fast-forward not possible (local has unpushed commits)")
			return a.newSyncConflict(repo, wTree, remoteRef)
		}
	}

	// Read the paths that HEAD tracks. A pull removes a path that the remote
	// deleted. It never removes a path that git never tracked, for example
	// config.json or a database file.
	oldPaths, err := oldTrackedPaths(repo)
	if err != nil {
		return fmt.Errorf("failed to read current tracked tree: %v", err)
	}

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	// writeTreeToWorktree writes only the paths of the remote tree. See
	// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
	newPaths, err := a.writeTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove each path that HEAD tracked and that the remote tree does not
	// hold, for example a note that another device deleted.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := filepath.Join(a.StorageDir, p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.logErrf(logSync, "pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			a.logDebugf(logSync, "pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	a.logInfof(logSync, "Pull: fast-forward complete")
	return nil
}

// syncPullMerge is the action pull_mark. It writes diff3 conflict markers
// into each file of conflictingPaths, with a BASE part from the merge base.
// HEAD does not move, and the next commit becomes a merge commit. See
// OMNGO_MERGE_PARENT.
func (a *App) syncPullMerge(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("remote master not found: %v", err)
	}

	localHead, err := repo.Head()
	if err != nil {
		return fmt.Errorf("local HEAD not found: %v", err)
	}
	a.savePremergeHead(localHead.Hash())

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	var baseTree *object.Tree
	if localCommit, cErr := repo.CommitObject(localHead.Hash()); cErr == nil {
		if bases, mErr := localCommit.MergeBase(remoteCommit); mErr == nil && len(bases) > 0 {
			baseTree, _ = bases[0].Tree()
		}
	}

	// conflictingPaths gives the same list that the conflict dialog showed.
	paths, err := conflictingPaths(wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}

	for _, path := range paths {
		file, err := wTree.Filesystem.Open(path)
		if err != nil {
			continue
		}
		localContent, _ := io.ReadAll(file)
		file.Close()

		remoteFile, err := remoteTree.File(path)
		if err != nil {
			continue
		}
		remoteContentStr, _ := remoteFile.Contents()

		baseSection := ""
		if baseTree != nil {
			if baseFile, bErr := baseTree.File(path); bErr == nil {
				if baseContent, cErr := baseFile.Contents(); cErr == nil && baseContent != string(localContent) {
					baseSection = fmt.Sprintf("||||||| BASE\n%s", baseContent)
				}
			}
		}

		conflictText := fmt.Sprintf("<<<<<<< LOCAL (Your changes)\n%s%s=======\n%s>>>>>>> REMOTE (Incoming from origin)\n",
			string(localContent), baseSection, remoteContentStr)

		if outFile, oErr := wTree.Filesystem.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0644); oErr == nil {
			outFile.Write([]byte(conflictText))
			outFile.Close()
		}
	}

	// Record the remote tip as the second parent of the next commit. Do not
	// move HEAD onto it. That makes no merge commit, and the local commit
	// becomes unreachable.
	a.saveMergeParent(remoteRef.Hash())
	a.logInfof(logSync, "Pull: 3-way conflict markers written, awaiting manual resolution")
	return nil
}

// syncPullAbort resets the branch and the working tree to the HEAD from
// before pull_mark. With no saved HEAD, it does nothing.
func (a *App) syncPullAbort(wTree *git.Worktree) error {
	hash, ok := a.loadPremergeHead()
	if !ok {
		a.logInfof(logSync, "pull_abort: nothing to abort")
		return nil
	}
	if err := wTree.Reset(&git.ResetOptions{Commit: hash, Mode: git.HardReset}); err != nil {
		return fmt.Errorf("abort reset failed: %v", err)
	}
	a.clearPremergeHead()
	a.clearMergeParent()
	a.logInfof(logSync, "pull_abort: restored local state to %s", hash.String())
	return nil
}

// writeTreeToWorktree writes each blob of tree into the worktree, and it
// answers the paths that it wrote. It touches nothing else. Checkout and
// Reset of go-git can delete each file outside the tree, also config.json.
// See doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
func (a *App) writeTreeToWorktree(repo *git.Repository, wTree *git.Worktree, tree *object.Tree) (map[string]bool, error) {
	newIndex := &index.Index{Version: 2}
	written := map[string]bool{}

	fileIter := tree.Files()
	defer fileIter.Close()

	err := fileIter.ForEach(func(f *object.File) error {
		reader, err := f.Reader()
		if err != nil {
			return fmt.Errorf("open blob for %s: %v", f.Name, err)
		}
		defer reader.Close()

		if dir := filepath.Dir(f.Name); dir != "." {
			if err := wTree.Filesystem.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir for %s: %v", f.Name, err)
			}
		}
		out, err := wTree.Filesystem.Create(f.Name)
		if err != nil {
			return fmt.Errorf("create %s: %v", f.Name, err)
		}
		_, copyErr := io.Copy(out, reader)
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("write %s: %v", f.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %v", f.Name, closeErr)
		}

		size := uint32(0)
		var modTime time.Time
		if info, statErr := wTree.Filesystem.Stat(f.Name); statErr == nil {
			size = uint32(info.Size())
			modTime = info.ModTime()
		}

		newIndex.Entries = append(newIndex.Entries, &index.Entry{
			Name:       f.Name,
			Hash:       f.Hash,
			Mode:       f.Mode,
			Size:       size,
			ModifiedAt: modTime,
		})
		written[f.Name] = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := repo.Storer.SetIndex(newIndex); err != nil {
		return nil, fmt.Errorf("failed to update index: %v", err)
	}
	return written, nil
}

// oldTrackedPaths answers each path that HEAD tracks. An unborn branch gives
// an empty set.
func oldTrackedPaths(repo *git.Repository) (map[string]bool, error) {
	paths := map[string]bool{}
	head, err := repo.Head()
	if err != nil {
		return paths, nil
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("HEAD commit lookup failed: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("HEAD tree lookup failed: %v", err)
	}
	fileIter := tree.Files()
	defer fileIter.Close()
	err = fileIter.ForEach(func(f *object.File) error {
		paths[f.Name] = true
		return nil
	})
	return paths, err
}

// syncPullForce makes the local branch match the remote tip. It then deletes
// each file that git does not track and that .gitignore does not cover. Only
// a force pull may delete such a file.
func (a *App) syncPullForce(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	a.logInfof(logSync, "Force pull: fetching %s", remoteName)

	if runtime.GOOS == "android" {
		tmpDir := filepath.Join(a.StorageDir, ".git", "tmp")
		os.MkdirAll(tmpDir, 0755)
		os.Setenv("TMPDIR", tmpDir)
		a.ensureGitignore()
	}

	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	// Read the paths that HEAD tracks. See syncPull.
	oldPaths, err := oldTrackedPaths(repo)
	if err != nil {
		return fmt.Errorf("failed to read current tracked tree: %v", err)
	}

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	newPaths, err := a.writeTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove each path that the remote dropped. See syncPull.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := filepath.Join(a.StorageDir, p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.logErrf(logSync, "force pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			a.logDebugf(logSync, "force pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	matcher, mErr := a.loadGitignoreMatcher(wTree)
	if mErr != nil {
		a.logErrf(logSync, "force pull: could not load .gitignore, skipping untracked cleanup: %v", mErr)
	} else {
		a.cleanUntrackedFiles(wTree, matcher)
	}

	a.clearPremergeHead() // any pending 3-way merge is now moot
	a.clearMergeParent()
	a.logInfof(logSync, "Force pull complete")
	return nil
}

// syncPush is the action push, or push_force when force is true. It always
// tries the push, also with nothing to commit. Do not add a check that skips
// the push. See
// doc/decisions/0011-push-each-time-and-let-the-remote-answer.md. A change to
// commit needs a message, and so does a force push. A refused push without
// force returns ErrPushConflict and does nothing more.
func (a *App) syncPush(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName, message string, force bool) error {
	matcher, mErr := a.loadGitignoreMatcher(wTree)
	if mErr != nil {
		matcher = gitignore.NewMatcher(nil)
	}
	status, err := wTree.Status()
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}

	hasRelevantChanges := false
	for name, fileStat := range status {
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			continue
		}
		if name == "config.json" {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			hasRelevantChanges = true
			break
		}
	}

	// A pending pull_mark merge must end in a commit, also when the
	// resolution equals HEAD and the status is clean.
	if _, mergePending := a.loadMergeParent(); mergePending {
		hasRelevantChanges = true
	}

	needsMessage := hasRelevantChanges || force
	if needsMessage && strings.TrimSpace(message) == "" {
		return ErrCommitMessageRequired
	}

	if hasRelevantChanges {
		if _, cErr := a.commitLocalChanges(repo, wTree, message); cErr != nil {
			return fmt.Errorf("commit failed: %v", cErr)
		}
	}

	a.logInfof(logSync, "Pushing to %s master (force=%v)", remoteName, force)
	err = repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		Auth:       auth,
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
		Force:      force,
		Progress:   &syncProgressWriter{app: a},
	})
	if err == git.NoErrAlreadyUpToDate {
		a.logInfof(logSync, "push: remote %s already up to date", remoteName)
		return nil
	}
	if err != nil {
		if !force && isNonFastForward(err) {
			a.logErrf(logSync, "push: rejected as non-fast-forward, leaving local state untouched")
			return ErrPushConflict
		}
		return fmt.Errorf("push failed: %v", err)
	}
	return nil
}

// nonFastForwardText is the message that go-git writes when a remote
// refuses a push. See isNonFastForward.
const nonFastForwardText = "non-fast-forward update"

// isNonFastForward tells whether the remote refused a push because it holds a
// commit that this device does not have. go-git makes this error with
// fmt.Errorf and no sentinel, thus the test reads the text.
// TestIsNonFastForward holds the known text. The value test stays first, for
// a later go-git.
func isNonFastForward(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, git.ErrNonFastForwardUpdate) {
		return true
	}
	return strings.HasPrefix(err.Error(), nonFastForwardText)
}

// unpushedState answers whether the active remote lacks a local commit. A
// clean worktree does not answer it: a push can fail after its commit. The
// check can offer a push that is not necessary, and it must never hide one
// that is.
type unpushedState struct {
	// Unpushed leans toward true. A push that is not necessary costs a round
	// trip. A hidden push costs the commits.
	Unpushed bool
	Remote   string
	// Verified is true when the remote itself answered, and not only the
	// local tracking ref.
	Verified bool
	Error    string
}

// syncPreviewResponse is the body of GET /api/sync/preview?action=upload. An
// empty Files list does not mean "nothing to do". The three fields after it
// tell whether a commit waits for its push.
type syncPreviewResponse struct {
	Files       []string `json:"files"`
	Unpushed    bool     `json:"unpushed"`
	Remote      string   `json:"remote,omitempty"`
	Verified    bool     `json:"verified,omitempty"`
	RemoteError string   `json:"remote_error,omitempty"`
}

// aheadOfRemote compares the local HEAD with the active remote. It reads the
// local tracking ref first, and a slot with no ref reads as "maybe ahead".
// Only the answer "level" goes to the network, as a list of refs.
func (a *App) aheadOfRemote(repo *git.Repository, remoteName string, auth transport.AuthMethod) unpushedState {
	out := unpushedState{Remote: remoteName}

	head, err := repo.Head()
	if err != nil {
		// With no commit, nothing waits for a push.
		a.logDebugf(logSync, "preview: no local HEAD (%v)", err)
		return out
	}

	trackRef, tErr := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if tErr != nil {
		a.logDebugf(logSync, "preview: %s has no known master yet - treating local HEAD as unpushed", remoteName)
		out.Unpushed = true
		return out
	}
	if trackRef.Hash() != head.Hash() {
		a.logDebugf(logSync, "preview: local HEAD %s differs from %s/master %s",
			head.Hash().String()[:7], remoteName, trackRef.Hash().String()[:7])
		out.Unpushed = true
		return out
	}

	// The local view says level. That answer can stop a necessary push, thus
	// the remote must confirm it.
	remote, rErr := repo.Remote(remoteName)
	if rErr != nil {
		out.Error = rErr.Error()
		return out
	}
	refs, lErr := remote.List(&git.ListOptions{Auth: auth})
	if lErr != nil {
		// The remote cannot answer. Report that, and claim no check. A push
		// would fail with the same error.
		a.logErrf(logSync, "preview: could not list %s: %v", remoteName, lErr)
		out.Error = lErr.Error()
		return out
	}

	out.Verified = true
	for _, ref := range refs {
		if ref.Name() == plumbing.Master {
			out.Unpushed = ref.Hash() != head.Hash()
			if out.Unpushed {
				a.logDebugf(logSync, "preview: %s/master is at %s, local HEAD is %s",
					remoteName, ref.Hash().String()[:7], head.Hash().String()[:7])
			}
			return out
		}
	}
	// The remote has no master branch. The first push makes it.
	a.logDebugf(logSync, "preview: %s has no master branch yet", remoteName)
	out.Unpushed = true
	return out
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
