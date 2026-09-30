package gitsync

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// IsDerivedTextPath reports whether a path is the html/ copy of a text file
// that lives in md/. See internal/storage/note_files.go. Only the md/ file
// belongs in git. A device that pulls it makes its own html/ copy.
func IsDerivedTextPath(name string) bool {
	name = filepath.ToSlash(name)
	return strings.HasPrefix(name, "html/") && storage.IsSyncedNoteFile(name)
}

// untrackReason says why a tracked path must leave the index, or "" when it
// stays. The removal and the upload preview both read it. The preview thus
// cannot promise more than the commit does.
func untrackReason(name string) string {
	switch {
	case storage.IsLocalOnlyPath(name):
		return LocalOnlyPreviewNote
	case IsDerivedTextPath(name):
		return DerivedTextPreviewNote
	}
	return ""
}

// untrackLocalOnlyPaths removes each path that untrackReason names from the
// index, like "git rm --cached". The file stays on disk. The next commit
// records a deletion, and a pull deletes the copy on another device. That is
// correct for both rules. Worktree.Remove of go-git also deletes the file
// from disk, thus this code writes the index itself.
//
// Only these two rules remove a path. An old repository can track
// md/UserManual.md or a compiled page. A removal of each such file would
// delete many files on the other devices.
func (svc Service) untrackLocalOnlyPaths(repo *git.Repository) int {
	idx, err := repo.Storer.Index()
	if err != nil {
		svc.Log(logx.Sync).Errf("cannot read the index to find the files to untrack: %v", err)
		return 0
	}

	kept := make([]*index.Entry, 0, len(idx.Entries))
	removed := 0
	for _, entry := range idx.Entries {
		if why := untrackReason(entry.Name); why != "" {
			svc.Log(logx.Sync).Debugf("%s%s", entry.Name, why)
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	if removed == 0 {
		return 0
	}

	idx.Entries = kept
	if err := repo.Storer.SetIndex(idx); err != nil {
		svc.Log(logx.Sync).Errf("cannot write the index after the removal of %d file(s): %v", removed, err)
		return 0
	}
	return removed
}

// LocalOnlyPreviewNote follows a path in the upload preview. The commit
// deletes the file from the repository, and not from this device.
const LocalOnlyPreviewNote = " (local-only: git stops to track it)"

// DerivedTextPreviewNote is the same note for a .txt under html/.
const DerivedTextPreviewNote = " (a copy of the file in md/: git stops to track it)"

// UntrackTrackedPaths answers each path that untrackLocalOnlyPaths would
// remove, with the reason, in sorted order. It changes nothing. The upload
// preview reads it.
func (svc Service) UntrackTrackedPaths(repo *git.Repository) []string {
	idx, err := repo.Storer.Index()
	if err != nil {
		svc.Log(logx.Sync).Errf("cannot read the index to find the files to untrack: %v", err)
		return nil
	}
	var out []string
	for _, entry := range idx.Entries {
		if why := untrackReason(entry.Name); why != "" {
			out = append(out, entry.Name+why)
		}
	}
	sort.Strings(out)
	return out
}

func (svc Service) CommitLocalChanges(repo *git.Repository, wTree *git.Worktree, message string) (bool, error) {
	matcher, err := svc.LoadGitignoreMatcher(wTree)
	if err != nil {
		svc.Log(logx.Sync).Errf("could not load .gitignore: %v", err)
		matcher = gitignore.NewMatcher(nil) // no ignore
	}

	// A local-only file can be in the index from a time before the rule, or
	// before the file got its name. A .gitignore pattern does not remove a
	// tracked file. Remove it before the status test, because an unchanged
	// tracked file gives no status entry.
	unstaged := svc.untrackLocalOnlyPaths(repo)

	svc.Log(logx.Sync).Debugf("Checking worktree status")
	status, err := wTree.Status()
	if err != nil {
		return false, fmt.Errorf("status check error: %v", err)
	}
	_, mergePending := svc.LoadMergeParent()
	if status.IsClean() && !mergePending && unstaged == 0 {
		svc.Log(logx.Sync).Infof("Nothing to commit")
		return false, nil
	}

	hasRealChanges := unstaged > 0
	for name, fileStat := range status {

		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			svc.Log(logx.Sync).Debugf("Ignoring %s (matches .gitignore)", name)
			continue
		}

		// config.json stays on this device. See CleanUntrackedFiles.
		if name == "config.json" {
			svc.Log(logx.Sync).Debugf("Ignoring root config.json (preserve locally)")
			continue
		}

		if fileStat.Worktree == git.Deleted {
			svc.Log(logx.Sync).Debugf("Staging deletion: %s", name)
			_, err := wTree.Remove(name)
			if err != nil {
				svc.Log(logx.Sync).Errf("failed to remove %s: %v", name, err)
			} else {
				hasRealChanges = true
			}
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			svc.Log(logx.Sync).Debugf("Staging file: %s", name)
			if err := svc.manualStageFile(repo, wTree, name); err != nil {
				svc.Log(logx.Sync).Errf("manual staging failed for %s: %v", name, err)
			} else {
				svc.Log(logx.Sync).Debugf("Staged %s successfully", name)
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges && !mergePending {
		svc.Log(logx.Sync).Infof("No real changes could be staged (FUSE false-dirty or ignored)")
		return false, nil
	}

	svc.Log(logx.Sync).Debugf("Committing staged changes")
	authorName := svc.configAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	commitOpts := &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	}

	// A pending pull_mark merge makes a merge commit with two parents: HEAD
	// and the remote tip. go-git fills Parents with HEAD only when the list
	// is empty, thus the code names HEAD here.
	var pendingMergeParent plumbing.Hash
	hasPendingMerge := false
	if h, ok := svc.LoadMergeParent(); ok {
		headRef, hErr := repo.Head()
		if hErr != nil {
			return false, fmt.Errorf("could not resolve HEAD for pending merge commit: %v", hErr)
		}
		pendingMergeParent = h
		hasPendingMerge = true
		commitOpts.Parents = []plumbing.Hash{headRef.Hash(), h}
		// A resolution can equal one parent, and the result is still a merge
		// commit.
		commitOpts.AllowEmptyCommits = true
	}

	commitHash, err := wTree.Commit(message, commitOpts)
	if err == git.ErrEmptyCommit {
		svc.Log(logx.Sync).Infof("Commit aborted: git.ErrEmptyCommit")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}

	if hasPendingMerge {
		svc.ClearMergeParent()
		svc.Log(logx.Sync).Infof("Committed merge with hash: %s (parents: HEAD, %s)", commitHash.String(), pendingMergeParent.String())
	} else {
		svc.Log(logx.Sync).Infof("Committed with hash: %s", commitHash.String())
	}
	return true, nil
}
