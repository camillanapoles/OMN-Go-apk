package backend

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

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
