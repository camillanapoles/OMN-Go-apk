package backend

import (
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
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

// serveStatusPage answers /OMNGoStatus.html. The page reads /api/status and
// shows the answer. It holds no facts of its own.
func (a *App) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	compiled := a.compilePageWithBody("Status",
		[]byte("Title: Status\nCategory: System\n\n"), statusPageTmpl)
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
	cfg := a.config.get()
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
