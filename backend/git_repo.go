package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/filesystem"
	cryptossh "golang.org/x/crypto/ssh"
)

// ----------------------------------------------------------------------
// The repository: the ignore rules, the remotes, and the commit
// ----------------------------------------------------------------------
//
// Four files hold the git code:
//
//	git_fs.go        The go-billy wrappers for Android.
//	git_repo.go      The ignore rules, the remotes, the SSH key, the commit.
//	git_sync.go      The sync paths and SyncRepo.
//	git_handlers.go  The HTTP handlers of /api/sync.

// gitignorePatterns is the one list for the sync .gitignore. ensureGitignore
// writes it for a new install, and it adds each missing line to an old one.
//
// The order is part of the rule. A "!" line must follow the pattern that it
// includes again. A negation of a directory alone, for example
// "!/html/images/", is NOT here. With the go-git matcher, it includes each
// file below the directory, and "/html/images/*" then has no effect.
//
// Keep gitignoreLocalOnlyPattern last. The go-git matcher reads the patterns
// from the end, thus the last match wins. A local-only name thus stays out of
// git below a "!" line, for example html/images/local-map.svg.
var gitignorePatterns = []string{
	"config.json",
	"assets_version",
	// session_secret is the HMAC key of the session cookie. A device with the
	// key of another device can make a valid cookie for it.
	"session_secret",
	"/asset_backups/",
	"*.html",
	"*.woff2",
	"*.woff",
	"/html/images/*",
	"!/html/images/*.svg",
	"/html/images/icons/*",
	"!/html/images/icons/*.svg",
	"/html/css/OMN-Go/omn-go-core.css",
	"/html/css/OMN-Go/Bookmarker.css",
	"/html/css/OMN-Go/omn-go-logs.css",
	"/html/css/OMN-Go/omn-go-status.css",
	"/html/css/OMN-Go/highlight.default.min.css",
	"/html/css/OMN-Go/katex.min.css",
	"/html/js/OMN-Go/omn-go-compat.js",
	"/html/js/OMN-Go/omn-go-core.js",
	"/html/js/OMN-Go/omn-go-sse.js",
	"/html/js/OMN-Go/omn-go-config.js",
	"/html/js/OMN-Go/omn-go-sync.js",
	"/html/js/OMN-Go/omn-go-bookmark.js",
	"/html/js/OMN-Go/omn-go-search.js",
	"/html/js/OMN-Go/omn-go-logs.js",
	"/html/js/OMN-Go/omn-go-status.js",
	"/html/js/OMN-Go/omn-go-editor.js",
	"/html/js/OMN-Go/auto-render.min.js",
	"/html/js/OMN-Go/katex.min.js",
	"/html/js/OMN-Go/highlight.min.js",
	"/html/js/OMN-Go/Bookmarker.js",
	// A .txt beside a note lives in md/, and the server copies it into html/.
	// Only the md/ file goes to git. See isDerivedTextPath.
	"/html/**/*.txt",
	"/md/AndroidIntents.md",
	"/md/BookmarksHowTo.md",
	"/md/Database.md",
	"/md/Editor.md",
	"/md/OMNGoTags.md",
	"/md/ScriptRules.md",
	"/md/SQLImport.md",
	"/md/UserManual.md",
	"/md/local/",
	"/db/",
	gitignoreLocalOnlyPattern,
}

// The local-only name rule: a file or a directory with a name that starts
// with "local-" stays on this device.
//
//	html/user_json/local-data.json     a file name
//	md/local-drafts/Monday.md          a directory name
//	html/db_backup/local-counters/...  a database backup
//
// The match is on a whole path segment, and it is case-sensitive, thus
// "mylocal-data.json" is a normal file. A commit does not take a local-only
// file. A force pull keeps it, because cleanUntrackedFiles keeps an ignored
// file.
const localOnlyPrefix = "local-"

// gitignoreLocalOnlyPattern is the .gitignore form of the same rule. The
// pattern has no "/", thus go-git compares it with each segment of a
// path, at each depth.
const gitignoreLocalOnlyPattern = localOnlyPrefix + "*"

// isLocalOnlyPath tells if the name of the file, or of a directory above it,
// starts with "local-". It is the rule for the index.
// gitignoreLocalOnlyPattern is the rule for a new file.
// TestGitignoreMatchesEachLocalOnlyPath compares the two.
func isLocalOnlyPath(name string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(name), "/") {
		if strings.HasPrefix(segment, localOnlyPrefix) {
			return true
		}
	}
	return false
}

// obsoleteGitignoreLines are the lines that ensureGitignore deletes from an
// existing file.
var obsoleteGitignoreLines = map[string]bool{
	// These lines negate a directory alone. See gitignorePatterns.
	"!/html/images/":       true,
	"!/html/images/icons/": true,
	// The general local-* rule covers the same files.
	"/html/db_backup/local-*/": true,
	// These lines name the old places of the app files. removeRetiredAssets
	// in assets.go deletes the files. See retiredAssets.
	"/html/css/omn-go-core.css":           true,
	"/html/css/Bookmarker.css":            true,
	"/html/css/highlight.default.min.css": true,
	"/html/css/katex.min.css":             true,
	"/html/css/markdown.css":              true,
	"/html/js/omn-go-compat.js":           true,
	"/html/js/omn-go-core.js":             true,
	"/html/js/omn-go-sse.js":              true,
	"/html/js/omn-go-editor.js":           true,
	"/html/js/auto-render.min.js":         true,
	"/html/js/katex.min.js":               true,
	"/html/js/highlight.min.js":           true,
	"/html/js/Bookmarker.js":              true,
}

func (a *App) ensureGitignore() {
	gitignorePath := filepath.Join(a.StorageDir, ".gitignore")
	gitignoreBase := "# OMN-Go sync ignore\n" + strings.Join(gitignorePatterns, "\n") + "\n"
	content, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		a.logInfof(logSync, "Created .gitignore")
		return
	}
	if err != nil {
		return
	}

	// Delete each obsolete line. The loop below cannot do it, because the
	// lines are present. See obsoleteGitignoreLines.
	rewritten := false
	{
		var kept []string
		for _, line := range strings.Split(string(content), "\n") {
			if obsoleteGitignoreLines[strings.TrimSpace(line)] {
				rewritten = true
				continue
			}
			kept = append(kept, line)
		}
		if rewritten {
			content = []byte(strings.Join(kept, "\n"))
		}
	}

	// Add each missing pattern to an existing file. Without this, an old
	// install never ignores a new entry, for example the SQLite files in
	// /db/. The test is a whole-line match, because "*.woff" is a substring
	// of "*.woff2".
	present := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, patt := range gitignorePatterns {
		if !present[patt] {
			missing = append(missing, patt)
		}
	}
	appended := len(missing) > 0
	if appended {
		text := string(content)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		content = []byte(text + strings.Join(missing, "\n") + "\n")
	}

	if rewritten || appended {
		if err := os.WriteFile(gitignorePath, content, 0644); err != nil {
			a.logErrf(logSync, "cannot update .gitignore: %v", err)
			return
		}
		a.logInfof(logSync, "Updated .gitignore (rewritten=%v, appended=%v)", rewritten, appended)
	}
}

func (a *App) getOrInitRepo() (*git.Repository, error) {
	a.logDebugf(logSync, "Opening repo at %s", a.StorageDir)

	baseFS := osfs.New(a.StorageDir)
	stableFS := &stableMtimeFS{baseFS}
	wtFS := &NoLockFS{stableFS}

	dotFS, err := wtFS.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("chroot .git failed: %v", err)
	}

	storer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
	repo, err := git.Open(storer, wtFS)

	if err != nil {
		a.logInfof(logSync, "Repo not found, initializing...")
		if initErr := a.manualGitInit(a.StorageDir); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
		}
		a.ensureGitignore()
		a.logInfof(logSync, "Repo initialized")
	} else {
		a.logDebugf(logSync, "Repo opened successfully")
		// Add each new pattern at each open. Without this, a commit can take
		// a file that .gitignore must cover, for example a test image.
		a.ensureGitignore()
	}

	// Only a sync needs a remote, thus ensureRemotesAndGetActive sets the
	// remotes. A status read touches no remote.
	return repo, nil
}

// The app keeps one git remote for each server slot. See
// doc/decisions/0012-keep-one-remote-for-each-git-server-slot.md.
//
// "origin" gets the URL of the active slot one time, and it never changes. A
// sync uses it only when the active slot has no URL. Each slot with a URL has
// the remote gitserver<index>. The name does not come from the Name field,
// because a person can change that field. ensureSlotRemotes adds, changes and
// removes these remotes at each sync.

// slotRemoteName answers the remote name of a slot index.
func slotRemoteName(index int) string {
	return fmt.Sprintf("gitserver%d", index)
}

// ensureOriginRemote makes "origin" from fallbackURL when it is missing. It
// never changes an existing origin.
func (a *App) ensureOriginRemote(repo *git.Repository, fallbackURL string) error {
	if _, err := repo.Remote("origin"); err == nil {
		return nil // already exists — this remote is never modified again
	}
	if fallbackURL == "" {
		return nil // nothing to seed it with yet; try again on a later sync
	}
	a.logInfof(logSync, "Remote origin missing, seeding it once from %s", fallbackURL)
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{fallbackURL},
	})
	return err
}

// ensureSlotRemotes makes the remotes match cfg. It answers the remote of the
// active slot, or "origin" when that slot has no URL.
func (a *App) ensureSlotRemotes(repo *git.Repository, cfg Config) (activeRemoteName string, err error) {
	for i, gs := range cfg.GitServers {
		name := slotRemoteName(i)
		url := strings.TrimSpace(gs.URL)

		remote, rErr := repo.Remote(name)
		if url == "" {
			if rErr == nil {
				a.logInfof(logSync, "Removing remote %s (slot %d cleared)", name, i)
				if dErr := repo.DeleteRemote(name); dErr != nil {
					a.logErrf(logSync, "failed to remove remote %s: %v", name, dErr)
				}
			}
			continue
		}

		if rErr != nil {
			a.logInfof(logSync, "Adding remote %s -> %s", name, url)
			if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
				return "", fmt.Errorf("failed to add remote %s: %v", name, cErr)
			}
			continue
		}

		existing := remote.Config().URLs
		if len(existing) == 1 && existing[0] == url {
			continue // already up to date
		}
		a.logInfof(logSync, "Remote %s URL changed (%v -> %s), updating", name, existing, url)
		if dErr := repo.DeleteRemote(name); dErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, dErr)
		}
		if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, cErr)
		}
	}

	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		if strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL) != "" {
			return slotRemoteName(cfg.ActiveGitIndex), nil
		}
	}
	a.logInfof(logSync, "Active server slot has no URL configured, falling back to origin")
	return "origin", nil
}

// ensureRemotesAndGetActive makes each remote match the config. It answers
// the remote for this sync.
func (a *App) ensureRemotesAndGetActive(repo *git.Repository) (string, error) {
	cfg := a.GetConfig()

	bootstrapURL := ""
	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		bootstrapURL = strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL)
	}
	if err := a.ensureOriginRemote(repo, bootstrapURL); err != nil {
		return "", err
	}

	return a.ensureSlotRemotes(repo, cfg)
}

func (a *App) manualGitInit(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/master\n"), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		return err
	}
	a.protectGitDirs()
	a.ensureGitignore()

	config := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}

// loadGitignoreMatcher answers a matcher for the .gitignore of the worktree
// and gitignorePatterns. The built-in list comes last, thus it wins. A force
// pull can write an old or a changed .gitignore from the remote. The list
// then still protects /db/, each local- file and session_secret.
func (a *App) loadGitignoreMatcher(wt *git.Worktree) (gitignore.Matcher, error) {
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, []string{})
	if err != nil {
		return nil, err
	}
	for _, p := range gitignorePatterns {
		patterns = append(patterns, gitignore.ParsePattern(p, nil))
	}
	return gitignore.NewMatcher(patterns), nil
}

// manualStageFile writes the file into a new blob and sets its index entry.
// It does not use Add of go-git.
func (a *App) manualStageFile(repo *git.Repository, wt *git.Worktree, name string) error {
	fullPath := filepath.Join(a.StorageDir, name)
	stat, err := os.Lstat(fullPath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return nil
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Stream the file, thus a large file needs little memory.
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	w.Close()

	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return err
	}

	idx, err := repo.Storer.Index()
	if err != nil {
		return err
	}
	var entry *index.Entry
	for _, e := range idx.Entries {
		if e.Name == name {
			entry = e
			break
		}
	}
	if entry == nil {
		entry = &index.Entry{Name: name}
		idx.Entries = append(idx.Entries, entry)
	}
	entry.Hash = hash
	entry.Size = uint32(stat.Size())
	entry.ModifiedAt = stat.ModTime()
	entry.Mode = filemode.Regular

	return repo.Storer.SetIndex(idx)
}

func (a *App) getSSHAuth() (transport.AuthMethod, error) {
	// Read one copy of the config. Two separate reads could mix the fields of
	// two servers.
	cfg := a.GetConfig()
	gs := cfg.GitServers[cfg.ActiveGitIndex]

	sshUser := "git"
	if idx := strings.Index(gs.URL, "@"); idx != -1 {
		sshUser = gs.URL[:idx]
	}
	a.logDebugf(logSync, "SSH user: %s", sshUser)

	keyData := gs.SSHKeyData
	if keyData == "" {
		a.logErrf(logSync, "No SSH key configured")
		return nil, fmt.Errorf("no SSH key configured")
	}

	var signer cryptossh.Signer
	var err error
	passphrase := gs.Password
	if passphrase == "" {
		signer, err = cryptossh.ParsePrivateKey([]byte(keyData))
	} else {
		signer, err = cryptossh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(passphrase))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %v", err)
	}

	publicKeys := &gitssh.PublicKeys{User: sshUser, Signer: signer}
	publicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
	}
	a.logDebugf(logSync, "SSH auth method created using inline key data")
	return publicKeys, nil
}

// isDerivedTextPath reports whether a path is the html/ copy of a text file
// that lives in md/. See note_files.go. Only the md/ file belongs in git. A
// device that pulls it makes its own html/ copy.
func isDerivedTextPath(name string) bool {
	name = filepath.ToSlash(name)
	return strings.HasPrefix(name, "html/") && isSyncedNoteFile(name)
}

// untrackReason says why a tracked path must leave the index, or "" when it
// stays. The removal and the upload preview both read it. The preview thus
// cannot promise more than the commit does.
func untrackReason(name string) string {
	switch {
	case isLocalOnlyPath(name):
		return localOnlyPreviewNote
	case isDerivedTextPath(name):
		return derivedTextPreviewNote
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
func (a *App) untrackLocalOnlyPaths(repo *git.Repository) int {
	idx, err := repo.Storer.Index()
	if err != nil {
		a.logErrf(logSync, "cannot read the index to find the files to untrack: %v", err)
		return 0
	}

	kept := make([]*index.Entry, 0, len(idx.Entries))
	removed := 0
	for _, entry := range idx.Entries {
		if why := untrackReason(entry.Name); why != "" {
			a.logDebugf(logSync, "%s%s", entry.Name, why)
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
		a.logErrf(logSync, "cannot write the index after the removal of %d file(s): %v", removed, err)
		return 0
	}
	return removed
}

// localOnlyPreviewNote follows a path in the upload preview. The commit
// deletes the file from the repository, and not from this device.
const localOnlyPreviewNote = " (local-only: git stops to track it)"

// derivedTextPreviewNote is the same note for a .txt under html/.
const derivedTextPreviewNote = " (a copy of the file in md/: git stops to track it)"

// untrackTrackedPaths answers each path that untrackLocalOnlyPaths would
// remove, with the reason, in sorted order. It changes nothing. The upload
// preview reads it.
func (a *App) untrackTrackedPaths(repo *git.Repository) []string {
	idx, err := repo.Storer.Index()
	if err != nil {
		a.logErrf(logSync, "cannot read the index to find the files to untrack: %v", err)
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

func (a *App) commitLocalChanges(repo *git.Repository, wTree *git.Worktree, message string) (bool, error) {
	matcher, err := a.loadGitignoreMatcher(wTree)
	if err != nil {
		a.logErrf(logSync, "could not load .gitignore: %v", err)
		matcher = gitignore.NewMatcher(nil) // no ignore
	}

	// A local-only file can be in the index from a time before the rule, or
	// before the file got its name. A .gitignore pattern does not remove a
	// tracked file. Remove it before the status test, because an unchanged
	// tracked file gives no status entry.
	unstaged := a.untrackLocalOnlyPaths(repo)

	a.logDebugf(logSync, "Checking worktree status")
	status, err := wTree.Status()
	if err != nil {
		return false, fmt.Errorf("status check error: %v", err)
	}
	_, mergePending := a.loadMergeParent()
	if status.IsClean() && !mergePending && unstaged == 0 {
		a.logInfof(logSync, "Nothing to commit")
		return false, nil
	}

	hasRealChanges := unstaged > 0
	for name, fileStat := range status {

		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			a.logDebugf(logSync, "Ignoring %s (matches .gitignore)", name)
			continue
		}

		// config.json stays on this device. See cleanUntrackedFiles.
		if name == "config.json" {
			a.logDebugf(logSync, "Ignoring root config.json (preserve locally)")
			continue
		}

		if fileStat.Worktree == git.Deleted {
			a.logDebugf(logSync, "Staging deletion: %s", name)
			_, err := wTree.Remove(name)
			if err != nil {
				a.logErrf(logSync, "failed to remove %s: %v", name, err)
			} else {
				hasRealChanges = true
			}
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			a.logDebugf(logSync, "Staging file: %s", name)
			if err := a.manualStageFile(repo, wTree, name); err != nil {
				a.logErrf(logSync, "manual staging failed for %s: %v", name, err)
			} else {
				a.logDebugf(logSync, "Staged %s successfully", name)
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges && !mergePending {
		a.logInfof(logSync, "No real changes could be staged (FUSE false-dirty or ignored)")
		return false, nil
	}

	a.logDebugf(logSync, "Committing staged changes")
	authorName := a.GetConfigAuthor()
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
	if h, ok := a.loadMergeParent(); ok {
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
		a.logInfof(logSync, "Commit aborted: git.ErrEmptyCommit")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}

	if hasPendingMerge {
		a.clearMergeParent()
		a.logInfof(logSync, "Committed merge with hash: %s (parents: HEAD, %s)", commitHash.String(), pendingMergeParent.String())
	} else {
		a.logInfof(logSync, "Committed with hash: %s", commitHash.String())
	}
	return true, nil
}

func (a *App) GetConfigAuthor() string {
	if author := a.GetConfig().Author; author != "" {
		return author
	}
	return "OMN-Go User"
}

// protectGitDirs keeps the empty .git/objects directory on Android. The media
// scanner can delete an empty directory.
func (a *App) protectGitDirs() {
	if runtime.GOOS != "android" {
		return
	}
	for _, dir := range []string{"objects"} {
		p := filepath.Join(a.StorageDir, ".git", dir)
		if err := os.MkdirAll(p, 0755); err != nil {
			a.logErrf(logSync, "MkdirAll %s failed: %v", p, err)
			continue
		}
		keepFile := filepath.Join(p, ".gitkeep")
		if _, err := os.Stat(keepFile); os.IsNotExist(err) {
			if f, err := os.Create(keepFile); err == nil {
				f.Close()
			}
		}
	}
}
