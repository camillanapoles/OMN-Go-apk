package backend

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// ----------------------------------------------------------------------
// GET /api/status: what this build does now
// ----------------------------------------------------------------------
//
// One endpoint answers the questions of a fault report. Which address does
// the server listen on? Which commit are the notes at? How large is the
// search index? Which Android package runs? The Status page reads this
// endpoint, and it has no second source.
//
// ADMIN ONLY. The answer holds LAN addresses, absolute paths and a commit
// subject. hasRole treats a local connection as the owner, thus the Android
// WebView and a desktop browser need no login.
//
// Two sections cost real work, and the default answer NEVER holds them.
// "storage" walks the storage directory, and "git_dirty" walks the worktree.
// A caller names them. The page thus shows the cheap facts at once, with a
// progress bar for the slow request.
//
// Nothing here opens a network connection or writes a file. The git section
// opens the repository read-only, because getOrInitRepo would CREATE one.

// statusCheapSections is the default answer. A caller must name each of
// statusSlowSections in "sections", or ask for "all".
var (
	statusCheapSections = []string{"server", "config", "git", "search", "runtime", "android"}
	statusSlowSections  = []string{"storage", "git_dirty"}
)

// androidPackage holds the applicationId, net.basov.omngo or
// net.basov.omngo.fdroid. The Go runtime cannot ask Android for it, thus the
// Android layer sets it through SetAndroidPackage.
var (
	androidPackageMu sync.RWMutex
	androidPackage   string
)

// SetAndroidPackage records the applicationId of the Android app.
// ServerService.java calls it before Backend.startServer. gomobile exports
// it. Without the call, statusAndroidPackage takes the last element of the
// storage directory, which IS the package name on Android.
func SetAndroidPackage(name string) {
	androidPackageMu.Lock()
	androidPackage = strings.TrimSpace(name)
	androidPackageMu.Unlock()
}

// lanAddressList holds the addresses that the Android layer found.
// java.net.NetworkInterface uses getifaddrs(), which an app can call. Go asks
// the kernel over a NETLINK_ROUTE socket, and Android 11 and later deny that
// to an app.
var (
	lanAddressesMu sync.RWMutex
	lanAddressList []string
)

// SetLANAddresses records the addresses of this device as one list with
// commas. ServerService.java calls it with each site-local IPv4 address that
// is not loopback, each time it builds the notification. The notification and
// the Status page thus show the same addresses. The gomobile binding carries
// no slice of strings, thus the value is one string. An empty list clears it.
func SetLANAddresses(list string) {
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	lanAddressesMu.Lock()
	lanAddressList = out
	lanAddressesMu.Unlock()
}

func androidLANAddresses() []string {
	lanAddressesMu.RLock()
	defer lanAddressesMu.RUnlock()
	return append([]string(nil), lanAddressList...)
}

func (a *App) statusAndroidPackage() string {
	androidPackageMu.RLock()
	name := androidPackage
	androidPackageMu.RUnlock()
	if name != "" {
		return name
	}
	base := filepath.Base(filepath.Clean(a.StorageDir))
	if strings.Contains(base, ".") {
		return base // derived, see SetAndroidPackage
	}
	return ""
}

// ----------------------------------------------------------------------
// The document
// ----------------------------------------------------------------------

type statusResponse struct {
	Generated string            `json:"generated"`
	Server    *statusServer     `json:"server,omitempty"`
	Config    *statusConfig     `json:"config,omitempty"`
	Git       *statusGit        `json:"git,omitempty"`
	Search    *statusSearch     `json:"search,omitempty"`
	Runtime   *statusRuntime    `json:"runtime,omitempty"`
	Android   *statusAndroid    `json:"android,omitempty"`
	Storage   *statusStorage    `json:"storage,omitempty"`
	GitDirty  *statusGitDirty   `json:"git_dirty,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// statusServer holds no listen ADDRESS. The listener binds "::" or "0.0.0.0",
// and "[::]:8080" answers no question. The port is the useful half, and
// share_lan with lan_urls tells the rest.
type statusServer struct {
	AppVersion  string   `json:"app_version"`
	Started     string   `json:"started"`
	UptimeS     int64    `json:"uptime_s"`
	BindPort    int      `json:"bind_port"`
	ShareLAN    bool     `json:"share_lan"`
	LANURLs     []string `json:"lan_urls"`
	ActiveConns int64    `json:"active_conns"`
	Hostname    string   `json:"hostname"`
	GOOS        string   `json:"goos"`
	GOARCH      string   `json:"goarch"`
}

type statusConfig struct {
	InternalEditor    bool     `json:"internal_editor"`
	Theme             string   `json:"theme"`
	MaxUploadMB       int      `json:"max_upload_mb"`
	SearchEnabled     bool     `json:"search_enabled"`
	SearchKinds       []string `json:"search_kinds"`
	SearchScope       string   `json:"search_scope"`
	SearchBundled     bool     `json:"search_bundled"`
	IntentURI         bool     `json:"intent_uri"`
	TermuxIntent      bool     `json:"termux_intent"`
	AndroidFullscreen string   `json:"android_fullscreen"`
	BackupPruneDepth  int      `json:"backup_prune_depth"`
	Hostname          string   `json:"hostname"`
	Author            string   `json:"author"`
	LogDebug          bool     `json:"log_debug"`
	LogInfo           bool     `json:"log_info"`
	LogTags           []string `json:"log_tags"`
}

type statusGitHead struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

type statusGitRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type statusGit struct {
	RepoExists bool             `json:"repo_exists"`
	Configured bool             `json:"configured"`
	Branch     string           `json:"branch,omitempty"`
	Head       *statusGitHead   `json:"head,omitempty"`
	Remote     *statusGitRemote `json:"remote,omitempty"`

	// RemoteRef and RemoteHead are what the LAST sync left in this
	// repository, for example "gitserver0/master". Nothing asks the remote
	// server. Compare head.hash with remote_head.hash to see whether the two
	// ends agree.
	RemoteRef  string         `json:"remote_ref,omitempty"`
	RemoteHead *statusGitHead `json:"remote_head,omitempty"`
}

type statusGitDirty struct {
	Dirty     bool `json:"dirty"`
	Changed   int  `json:"changed"`
	Untracked int  `json:"untracked"`
}

type statusSearch struct {
	Enabled            bool     `json:"enabled"`
	Docs               int      `json:"docs"`
	Lines              int      `json:"lines"`
	Bytes              int64    `json:"bytes"`
	IndexBytesEstimate int64    `json:"index_bytes_estimate"`
	Built              string   `json:"built,omitempty"`
	Checked            string   `json:"checked,omitempty"`
	Dirty              bool     `json:"dirty"`
	Kinds              []string `json:"kinds"`
	Scope              string   `json:"scope"`
}

type statusRuntime struct {
	GoVersion     string `json:"go_version"`
	Goroutines    int    `json:"goroutines"`
	HeapAlloc     uint64 `json:"heap_alloc"`
	Sys           uint64 `json:"sys"`
	AssetsVersion string `json:"assets_version"`
	// AssetsRefreshed tells whether this start wrote a version-dependent
	// file. It is true one time after an update.
	AssetsRefreshed bool `json:"assets_refreshed"`
}

type statusAndroid struct {
	Package     string `json:"package"`
	DefaultPort int    `json:"default_port"`
	Fullscreen  string `json:"fullscreen"`
}

// statusGroup is one counted group of files in the storage directory.
type statusGroup struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type statusStorage struct {
	Dir          string      `json:"dir"`
	Notes        statusGroup `json:"notes"`
	Pages        statusGroup `json:"pages"`
	Images       statusGroup `json:"images"`
	UserJSON     statusGroup `json:"user_json"`
	Databases    statusGroup `json:"databases"`
	Backups      statusGroup `json:"backups"`
	AssetBackups statusGroup `json:"asset_backups"`
	Total        statusGroup `json:"total"`
}

// ----------------------------------------------------------------------
// The handler
// ----------------------------------------------------------------------

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	want, unknown := parseStatusSections(r.URL.Query().Get("sections"))
	if len(unknown) > 0 {
		http.Error(w, "unknown section: "+strings.Join(unknown, ", "), http.StatusBadRequest)
		return
	}

	res := a.buildStatus(want)

	if strings.EqualFold(r.URL.Query().Get("format"), "md") {
		// Answer text/plain and not text/markdown, because the Android
		// WebView shows only text/plain. For the same reason, builtinMIME
		// serves .jsonl as text/plain.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(renderStatusMarkdown(res)))
		return
	}

	a.writeJSON(w, http.StatusOK, res)
}

// statusDeniedBody is the page that a guest sees, the same as on the file
// index. It is not the line of plain text of authMiddleware.
const statusDeniedBody = `<div class="config-panel">` +
	`<h2 class="config-title">Status</h2>` +
	`<p class="config-hint">This page is for the admin of this device. ` +
	`Log in as admin on a note page, then open the page again.</p>` +
	`</div>`

// serveStatusPage answers /OMNGoStatus.html. The page reads /api/status and
// shows the answer. It holds no facts of its own.
func (a *App) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	body := statusPageTmpl
	if !a.hasRole(r) {
		body = statusDeniedBody
	}
	compiled := a.compilePageWithBody("Status",
		[]byte("Title: Status\nCategory: System\n\n"), body)
	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars(compiled))
}

// parseStatusSections changes the "sections" parameter into a set. Empty
// means the cheap sections, and "all" means each section. An unknown name is
// an error, thus a caller who asks for "sarch" knows it.
func parseStatusSections(raw string) (want map[string]bool, unknown []string) {
	want = map[string]bool{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		for _, s := range statusCheapSections {
			want[s] = true
		}
		return want, nil
	}
	known := map[string]bool{}
	for _, s := range append(append([]string{}, statusCheapSections...), statusSlowSections...) {
		known[s] = true
	}
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		switch {
		case name == "":
		case name == "all":
			for s := range known {
				want[s] = true
			}
		case known[name]:
			want[name] = true
		default:
			unknown = append(unknown, name)
		}
	}
	return want, unknown
}

func (a *App) buildStatus(want map[string]bool) *statusResponse {
	cfg := a.GetConfig()
	res := &statusResponse{Generated: statusTime(time.Now())}
	fail := func(section string, err error) {
		if res.Errors == nil {
			res.Errors = map[string]string{}
		}
		res.Errors[section] = err.Error()
	}

	if want["server"] {
		res.Server = a.statusServerSection(cfg)
	}
	if want["config"] {
		res.Config = statusConfigSection(cfg)
	}
	if want["git"] {
		// The name is not "git", because this file imports that package.
		section, err := a.statusGitSection(cfg)
		res.Git = section
		if err != nil {
			fail("git", err)
		}
	}
	if want["search"] {
		res.Search = a.statusSearchSection(cfg)
	}
	if want["runtime"] {
		res.Runtime = a.statusRuntimeSection()
	}
	if want["android"] && runtime.GOOS == "android" {
		res.Android = &statusAndroid{
			Package:     a.statusAndroidPackage(),
			DefaultPort: a.fallbackPort(),
			Fullscreen:  cfg.AndroidFullscreen,
		}
	}
	if want["storage"] {
		storage, err := a.statusStorageSection()
		res.Storage = storage
		if err != nil {
			fail("storage", err)
		}
	}
	if want["git_dirty"] {
		dirty, err := a.statusGitDirtySection()
		res.GitDirty = dirty
		if err != nil {
			fail("git_dirty", err)
		}
	}
	return res
}

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
		s.LANURLs = lanURLs(port)
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

	a.logDebugf(logStatus, "Reading the git worktree state")
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
	a.logInfof(logStatus, "Worktree read in %s: %d changed, %d untracked",
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
	if raw, err := os.ReadFile(filepath.Join(a.StorageDir, assetsVersionFilename)); err == nil {
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
func lanURLs(port int) []string {
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
	for _, text := range androidLANAddresses() {
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

// redactGitURL removes the password from a remote URL. The user name stays,
// because it is part of the address and not a secret. An address that the
// function cannot parse shows as "(hidden)".
func redactGitURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return raw // scp form, "git@host:path" - it carries no password
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "(hidden)"
	}
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			u.User = url.User(name)
		} else {
			u.User = nil
		}
	}
	return u.String()
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

// ----------------------------------------------------------------------
// The Markdown form (?format=md)
// ----------------------------------------------------------------------

func renderStatusMarkdown(res *statusResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# OMN-Go status\n\nGenerated: %s\n", res.Generated)

	table := func(title string, rows [][2]string) {
		fmt.Fprintf(&b, "\n## %s\n\n| Field | Value |\n| --- | --- |\n", title)
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
		}
	}
	yes := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}

	if s := res.Server; s != nil {
		table("Server", [][2]string{
			{"app_version", s.AppVersion},
			{"started", s.Started},
			{"uptime_s", strconv.FormatInt(s.UptimeS, 10)},
			{"bind_port", strconv.Itoa(s.BindPort)},
			{"share_lan", yes(s.ShareLAN)},
			{"lan_urls", strings.Join(s.LANURLs, ", ")},
			{"active_conns", strconv.FormatInt(s.ActiveConns, 10)},
			{"hostname", s.Hostname},
			{"goos", s.GOOS},
			{"goarch", s.GOARCH},
		})
	}
	if c := res.Config; c != nil {
		table("Config", [][2]string{
			{"internal_editor", yes(c.InternalEditor)},
			{"theme", c.Theme},
			{"max_upload_mb", strconv.Itoa(c.MaxUploadMB)},
			{"search_enabled", yes(c.SearchEnabled)},
			{"search_kinds", strings.Join(c.SearchKinds, ", ")},
			{"search_scope", c.SearchScope},
			{"search_bundled", yes(c.SearchBundled)},
			{"intent_uri", yes(c.IntentURI)},
			{"termux_intent", yes(c.TermuxIntent)},
			{"android_fullscreen", c.AndroidFullscreen},
			{"backup_prune_depth", strconv.Itoa(c.BackupPruneDepth)},
			{"hostname", c.Hostname},
			{"author", c.Author},
			{"log_debug", yes(c.LogDebug)},
			{"log_info", yes(c.LogInfo)},
			{"log_tags", strings.Join(c.LogTags, ", ")},
		})
	}
	if g := res.Git; g != nil {
		rows := [][2]string{
			{"repo_exists", yes(g.RepoExists)},
			{"configured", yes(g.Configured)},
			{"branch", g.Branch},
		}
		if g.Head != nil {
			rows = append(rows,
				[2]string{"head_hash", g.Head.Hash},
				[2]string{"head_short", g.Head.Short},
				[2]string{"head_subject", g.Head.Subject},
				[2]string{"head_author", g.Head.Author},
				[2]string{"head_date", g.Head.Date})
		}
		if g.Remote != nil {
			rows = append(rows,
				[2]string{"remote_name", g.Remote.Name},
				[2]string{"remote_url", g.Remote.URL})
		}
		if g.RemoteRef != "" {
			rows = append(rows, [2]string{"remote_ref", g.RemoteRef})
		}
		if g.RemoteHead != nil {
			rows = append(rows,
				[2]string{"remote_head_hash", g.RemoteHead.Hash},
				[2]string{"remote_head_short", g.RemoteHead.Short},
				[2]string{"remote_head_subject", g.RemoteHead.Subject},
				[2]string{"remote_head_author", g.RemoteHead.Author},
				[2]string{"remote_head_date", g.RemoteHead.Date})
		}
		table("Git", rows)
	}
	if d := res.GitDirty; d != nil {
		table("Git worktree", [][2]string{
			{"dirty", yes(d.Dirty)},
			{"changed", strconv.Itoa(d.Changed)},
			{"untracked", strconv.Itoa(d.Untracked)},
		})
	}
	if s := res.Search; s != nil {
		table("Search", [][2]string{
			{"enabled", yes(s.Enabled)},
			{"docs", strconv.Itoa(s.Docs)},
			{"lines", strconv.Itoa(s.Lines)},
			{"bytes", strconv.FormatInt(s.Bytes, 10)},
			{"index_bytes_estimate", strconv.FormatInt(s.IndexBytesEstimate, 10)},
			{"built", s.Built},
			{"checked", s.Checked},
			{"dirty", yes(s.Dirty)},
			{"kinds", strings.Join(s.Kinds, ", ")},
			{"scope", s.Scope},
		})
	}
	if rt := res.Runtime; rt != nil {
		table("Runtime", [][2]string{
			{"go_version", rt.GoVersion},
			{"goroutines", strconv.Itoa(rt.Goroutines)},
			{"heap_alloc", strconv.FormatUint(rt.HeapAlloc, 10)},
			{"sys", strconv.FormatUint(rt.Sys, 10)},
			{"assets_version", rt.AssetsVersion},
			{"assets_refreshed", yes(rt.AssetsRefreshed)},
		})
	}
	if an := res.Android; an != nil {
		table("Android", [][2]string{
			{"package", an.Package},
			{"default_port", strconv.Itoa(an.DefaultPort)},
			{"fullscreen", an.Fullscreen},
		})
	}
	if st := res.Storage; st != nil {
		fmt.Fprintf(&b, "\n## Storage\n\nDirectory: `%s`\n\n| Group | Files | Bytes |\n| --- | --- | --- |\n", st.Dir)
		for _, g := range [][2]any{
			{"notes", st.Notes}, {"pages", st.Pages}, {"images", st.Images},
			{"user_json", st.UserJSON}, {"databases", st.Databases},
			{"backups", st.Backups}, {"asset_backups", st.AssetBackups},
			{"total", st.Total},
		} {
			grp := g[1].(statusGroup)
			fmt.Fprintf(&b, "| %s | %d | %d |\n", g[0], grp.Files, grp.Bytes)
		}
	}
	if len(res.Errors) > 0 {
		rows := [][2]string{}
		names := make([]string, 0, len(res.Errors))
		for k := range res.Errors {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			rows = append(rows, [2]string{k, res.Errors[k]})
		}
		table("Errors", rows)
	}
	return b.String()
}
