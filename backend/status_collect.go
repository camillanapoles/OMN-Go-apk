package backend

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"net.basov.omngo/backend/internal/logx"
)

// ----------------------------------------------------------------------
// The sections
// ----------------------------------------------------------------------

func (a *App) statusServerSection(cfg Config) *statusServer {
	_, portStr, addr := a.boundAddress()
	port, _ := strconv.Atoi(portStr)
	if addr == "" {
		// The listener is not up yet. The config gives the port.
		port = cfg.ServerPort
	}

	hostname := sanitizeHostname(cfg.Hostname)
	if hostname == "" {
		hostname = defaultHostname()
	}

	s := &statusServer{
		AppVersion:  APP_VERSION,
		Started:     statusTime(a.startedAt),
		UptimeS:     int64(time.Since(a.startedAt).Seconds()),
		BindPort:    port,
		ShareLAN:    cfg.ShareLAN,
		LANURLs:     []string{},
		ActiveConns: a.ActiveConnCount(),
		Hostname:    hostname,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	if cfg.ShareLAN {
		s.LANURLs = lanURLs(port, a.android.lanAddresses())
	}
	return s
}

func statusConfigSection(cfg Config) *statusConfig {
	kinds := cfg.SearchKinds
	if kinds == nil {
		kinds = []string{}
	}
	return &statusConfig{
		InternalEditor:    cfg.UseInternalEd,
		Theme:             cfg.Theme,
		MaxUploadMB:       cfg.MaxUploadSizeMB,
		SearchEnabled:     cfg.SearchEnabled,
		SearchKinds:       kinds,
		SearchScope:       cfg.SearchScope,
		SearchBundled:     cfg.SearchBundled,
		IntentURI:         cfg.EnableIntentURI,
		TermuxIntent:      cfg.EnableTermuxIntent,
		AndroidFullscreen: cfg.AndroidFullscreen,
		BackupPruneDepth:  cfg.BackupPruneDepth,
		Hostname:          cfg.Hostname,
		Author:            cfg.Author,
		LogDebug:          cfg.LogDebug,
		LogInfo:           cfg.LogInfo,
		LogTags:           normalizeLogTags(cfg.LogTags),
	}
}

// statusGitSection reads HEAD and changes nothing. An install that never
// synced has no .git, and that is an answer, not an error.
func (a *App) statusGitSection(cfg Config) (*statusGit, error) {
	out := &statusGit{}

	if idx := cfg.ActiveGitIndex; idx >= 0 && idx < len(cfg.GitServers) {
		slot := cfg.GitServers[idx]
		if strings.TrimSpace(slot.URL) != "" {
			out.Configured = true
			name := strings.TrimSpace(slot.Name)
			if name == "" {
				name = slotRemoteName(idx)
			}
			out.Remote = &statusGitRemote{Name: name, URL: redactGitURL(slot.URL)}
		}
	}

	repo, err := a.openRepoReadOnly()
	if err != nil {
		return out, nil // no repository on disk
	}
	out.RepoExists = true

	head, err := repo.Head()
	if err != nil {
		return out, nil // a repository with no commit yet
	}
	if head.Name().IsBranch() {
		out.Branch = head.Name().Short()
	}
	out.Head = commitSummary(repo, head.Hash())

	// The remote-tracking ref is what the last pull or push wrote for the
	// branch of HEAD. It is a local file, thus the read is cheap, and it can
	// be old.
	for _, remote := range remoteRefCandidates(cfg, out.Branch) {
		refName := plumbing.NewRemoteReferenceName(remote, out.Branch)
		ref, err := repo.Reference(refName, true)
		if err != nil || ref == nil {
			continue
		}
		out.RemoteRef = remote + "/" + out.Branch
		out.RemoteHead = commitSummary(repo, ref.Hash())
		break
	}
	return out, nil
}

// remoteRefCandidates names the remotes to read, most specific first.
// ensureSlotRemotes gives each server slot its own remote, for example
// "gitserver0". "origin" is the fallback remote. See
// doc/decisions/0012-keep-one-remote-for-each-git-server-slot.md. A detached
// HEAD has no branch, and no remote-tracking ref.
func remoteRefCandidates(cfg Config, branch string) []string {
	if branch == "" {
		return nil
	}
	out := []string{}
	if idx := cfg.ActiveGitIndex; idx >= 0 && idx < len(cfg.GitServers) &&
		strings.TrimSpace(cfg.GitServers[idx].URL) != "" {
		out = append(out, slotRemoteName(idx))
	}
	return append(out, "origin")
}

// commitSummary reads one commit and reports it. When the object is not in
// this repository, the answer still holds the hash, because the hash is what
// the caller asked for.
func commitSummary(repo *git.Repository, h plumbing.Hash) *statusGitHead {
	hash := h.String()
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	out := &statusGitHead{Hash: hash, Short: short}
	commit, err := repo.CommitObject(h)
	if err != nil {
		return out
	}
	out.Subject = strings.TrimSpace(strings.SplitN(commit.Message, "\n", 2)[0])
	out.Author = commit.Author.Name
	out.Date = statusTime(commit.Author.When)
	return out
}

// statusGitDirtySection is the slow half of the git answer: go-git hashes
// each tracked file. It first writes one log line, thus the page can show it
// under its progress bar.
func (a *App) statusGitDirtySection() (*statusGitDirty, error) {
	repo, err := a.openRepoReadOnly()
	if err != nil {
		return &statusGitDirty{}, nil
	}
	wTree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open worktree: %v", err)
	}

	a.log(logx.Status).Debugf("Reading the git worktree state")
	started := time.Now()
	st, err := wTree.Status()
	if err != nil {
		return nil, fmt.Errorf("worktree status: %v", err)
	}

	out := &statusGitDirty{}
	for _, fileStat := range st {
		if fileStat.Worktree == git.Untracked {
			out.Untracked++
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			out.Changed++
		}
	}
	out.Dirty = out.Changed > 0
	a.log(logx.Status).Infof("Worktree read in %s: %d changed, %d untracked",
		time.Since(started).Round(time.Millisecond), out.Changed, out.Untracked)
	return out, nil
}

func (a *App) statusSearchSection(cfg Config) *statusSearch {
	out := &statusSearch{
		Enabled: cfg.SearchEnabled,
		Scope:   cfg.SearchScope,
		Kinds:   cfg.SearchKinds,
	}
	if out.Kinds == nil {
		out.Kinds = []string{}
	}
	if a.search == nil {
		return out
	}

	a.search.mu.RLock()
	defer a.search.mu.RUnlock()

	out.Docs = len(a.search.docs)
	out.Lines = a.search.lines
	out.Bytes = a.search.bytes
	out.Dirty = a.search.dirty
	if !a.search.built.IsZero() {
		out.Built = statusTime(a.search.built)
	}
	if !a.search.checked.IsZero() {
		out.Checked = statusTime(a.search.checked)
	}

	// This is an ESTIMATE, and the field name says so. Go cannot measure a
	// live object graph. The count covers one 8-byte mask for each line, the
	// 64-byte signature and the strings. A flat value covers the struct and
	// its map entry.
	const perDocOverhead = 160
	var est int64
	for path, doc := range a.search.docs {
		est += int64(perDocOverhead + len(path))
		est += int64(len(doc.Path) + len(doc.Kind) + len(doc.Name) + len(doc.Title) + len(doc.URL))
		for _, t := range doc.Tags {
			est += int64(len(t) + 16)
		}
		est += int64(8 * len(doc.LineMasks))
		est += 8 + 64 // FieldMask + Tri
	}
	out.IndexBytesEstimate = est
	return out
}

func (a *App) statusRuntimeSection() *statusRuntime {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	stamp := ""
	if raw, err := os.ReadFile(a.layout().file(assetsVersionFilename)); err == nil {
		stamp = strings.TrimSpace(string(raw))
	}
	return &statusRuntime{
		GoVersion:       runtime.Version(),
		Goroutines:      runtime.NumGoroutine(),
		HeapAlloc:       mem.HeapAlloc,
		Sys:             mem.Sys,
		AssetsVersion:   stamp,
		AssetsRefreshed: AssetsRefreshed(),
	}
}

// statusStorageSection walks the storage directory ONE time and sorts each
// file into its group. The walk of the note tree is the whole cost of this
// section on a phone.
func (a *App) statusStorageSection() (*statusStorage, error) {
	out := &statusStorage{Dir: a.StorageDir}
	add := func(g *statusGroup, size int64) {
		g.Files++
		g.Bytes += size
	}

	err := filepath.WalkDir(a.StorageDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable corner must not fail the whole answer
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir // the object store is git's business
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		add(&out.Total, size)

		rel, err := filepath.Rel(a.StorageDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := strings.ToLower(path.Base(rel))
		switch {
		case strings.HasPrefix(rel, "md/") && strings.HasSuffix(name, ".md"):
			add(&out.Notes, size)
		case strings.HasPrefix(rel, "html/db_backup/"):
			add(&out.Backups, size)
		case strings.HasPrefix(rel, "html/images/"):
			add(&out.Images, size)
		case strings.HasPrefix(rel, "html/user_json/"):
			add(&out.UserJSON, size)
		case strings.HasPrefix(rel, "html/") && strings.HasSuffix(name, ".html"):
			add(&out.Pages, size)
		case strings.HasPrefix(rel, "db/") && strings.HasSuffix(name, ".sqlite"):
			add(&out.Databases, size)
		case strings.HasPrefix(rel, "asset_backups/"):
			add(&out.AssetBackups, size)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("walk storage: %v", err)
	}
	return out, nil
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

// statusTime is the one time format of this endpoint: RFC3339 in UTC, the
// same as the backup files.
func statusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// lanURLs lists the addresses that another device on the network can open. It
// drops loopback and link-local: the first is reachable from nowhere else,
// and the second needs a zone index. The address of the default route comes
// first. A desktop can have more addresses, for example of a docker bridge,
// and they follow in sorted order.
func lanURLs(port int, android []string) []string {
	usable := func(ip net.IP) bool {
		return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
			!ip.IsLinkLocalMulticast() && ip.IsGlobalUnicast()
	}
	format := func(ip net.IP) string {
		host := ip.String()
		if ip.To4() == nil {
			host = "[" + host + "]"
		}
		return fmt.Sprintf("http://%s:%d", host, port)
	}

	out := []string{}
	seen := map[string]bool{}
	add := func(ip net.IP) {
		if !usable(ip) {
			return
		}
		url := format(ip)
		if seen[url] {
			return
		}
		seen[url] = true
		out = append(out, url)
	}

	// 1. Add the address of the default route, read now. Another device on
	// the network uses it, thus it comes first.
	add(defaultRouteIP())

	// 2. Add the addresses that the Android layer found. See SetLANAddresses.
	// They can be older than the answer above.
	for _, text := range android {
		add(net.ParseIP(text))
	}

	// 3. Add each interface address that this process can read. This is the
	// desktop path, and it adds bridge addresses.
	rest := []string{}
	restSeen := map[string]bool{}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || !usable(ipNet.IP) {
				continue
			}
			url := format(ipNet.IP)
			if seen[url] || restSeen[url] {
				continue
			}
			restSeen[url] = true
			rest = append(rest, url)
		}
	}
	sort.Strings(rest)
	for _, url := range rest {
		seen[url] = true
		out = append(out, url)
	}
	return out
}

// defaultRouteIP reports the address of the interface with the default route,
// or nil. net.InterfaceAddrs uses NETLINK, and Android 11 and later deny
// NETLINK_ROUTE to an app, thus the call fails on a phone. A UDP "connection"
// sends no packet. The kernel only selects the route and gives its local
// address, which is the address that another device reaches.
func defaultRouteIP() net.IP {
	for _, target := range []string{"8.8.8.8:53", "192.168.1.1:9"} {
		conn, err := net.Dial("udp4", target)
		if err != nil {
			continue
		}
		addr, ok := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if ok && addr.IP != nil && !addr.IP.IsUnspecified() {
			return addr.IP
		}
	}
	return nil
}

// openRepoReadOnly opens the storage repository and makes nothing. It uses
// the same file system wrappers as getOrInitRepo in git_repo.go, thus both
// see one worktree. With no repository on disk, it answers an error.
func (a *App) openRepoReadOnly() (*git.Repository, error) {
	baseFS := osfs.New(a.StorageDir)
	wtFS := &NoLockFS{&stableMtimeFS{baseFS}}
	dotFS, err := wtFS.Chroot(".git")
	if err != nil {
		return nil, err
	}
	storer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
	return git.Open(storer, wtFS)
}
