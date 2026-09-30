package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/logx"
)

func (a *App) getConfigPageBody() string {
	cfg := a.config.get() // snapshot under RLock; render against the copy

	for len(cfg.GitServers) < maxGitServers {
		cfg.GitServers = append(cfg.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(cfg.GitServers)+1)})
	}

	// The view carries no secret. See gitServerView in config_page.go.
	view := configPageView{
		ServerPort:         cfg.ServerPort,
		Author:             cfg.Author,
		UseInternalEd:      cfg.UseInternalEd,
		DesktopExtCmd:      cfg.DesktopExtCmd,
		Theme:              cfg.Theme,
		ShareLAN:           cfg.ShareLAN,
		Hostname:           cfg.Hostname,
		PruneDepth:         cfg.BackupPruneDepth,
		MaxUploadSizeMB:    cfg.MaxUploadSizeMB,
		EnableIntentURI:    cfg.EnableIntentURI,
		EnableTermuxIntent: cfg.EnableTermuxIntent,
		AndroidFullscreen:  cfg.AndroidFullscreen,
		SearchEnabled:      cfg.SearchEnabled,
		SearchKinds:        normalizeSearchKinds(cfg.SearchKinds),
		SearchBundled:      cfg.SearchBundled,
		SearchScope:        cfg.SearchScope,
		SearchIndexStatus:  a.searchIndexStatus(),
		LogDebug:           cfg.LogDebug,
		LogInfo:            cfg.LogInfo,
		LogTags:            normalizeLogTags(cfg.LogTags),
	}
	for i, gs := range cfg.GitServers {
		view.GitServers = append(view.GitServers, gitServerView{
			Index:   i,
			Slot:    i + 1,
			Active:  cfg.ActiveGitIndex == i,
			Name:    gs.Name,
			URL:     gs.URL,
			HostKey: a.hostKeyText(gs.URL),
		})
	}

	return renderConfigPage(view)
}

// configFormMaxMemory is the defaultMaxMemory of net/http.
const configFormMaxMemory = 32 << 20

// configFieldSent reports whether a POST to /api/config carries one field. A
// field that the request does not carry keeps its value. See
// doc/decisions/0014-change-only-the-settings-that-a-request-names.md. A
// browser sends nothing for a clear checkbox. The Config page thus names its
// checkboxes in the hidden field config_fields, and each name there counts as
// sent.
func configFieldSent(r *http.Request) func(field string) bool {
	if r.Form == nil {
		// ParseMultipartForm fills r.Form for multipart and for urlencoded
		// bodies. For urlencoded, it reports ErrNotMultipart, and that is not
		// a failure.
		_ = r.ParseMultipartForm(configFormMaxMemory)
	}

	governed := map[string]bool{}
	for _, group := range r.Form["config_fields"] {
		for _, field := range strings.Split(group, ",") {
			if field = strings.TrimSpace(field); field != "" {
				governed[field] = true
			}
		}
	}

	return func(field string) bool {
		if _, ok := r.Form[field]; ok {
			return true
		}
		return governed[field]
	}
}

// handleConfigGet answers GET /api/config with the whole Config and each
// password, for "Show passwords". It is admin only.
func (a *App) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, a.config.get())
}

// handleConfigPost answers POST /api/config. It saves the fields of the form.
func (a *App) handleConfigPost(w http.ResponseWriter, r *http.Request) {
	prev := a.config.get()

	sent := configFieldSent(r)

	var next Config
	a.config.update(func(c *Config) {
		applyConfigForm(c, r, sent)
		applyGitServerForm(c, r, sent)
		next = *c
	})

	if err := a.persistConfig(next); err != nil {
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}
	a.applyConfigChange(prev, next)

	if next.ShareLAN != prev.ShareLAN {
		// saveConfig in omn-go-config.js reads this exact word and then
		// calls /api/restart.
		w.Write([]byte("RestartRequired"))
		return
	}
	w.Write([]byte("Saved"))
}

// persistConfig writes one configuration to config.json. It runs OUTSIDE the
// configuration lock, thus a file write does not stop a reader. The caller
// passes the snapshot that it took inside the lock.
func (a *App) persistConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		a.log(logx.Config).Errf("persistConfig: failed to marshal the configuration: %v", err)
		return err
	}
	configPath := a.layout().config()
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		a.log(logx.Config).Errf("persistConfig: failed to write %s: %v", configPath, err)
		return err
	}
	return nil
}

// applyConfigChange starts the work that a saved change needs. Each change
// applies at once, except share_lan, because the socket binds one time at the
// start.
func (a *App) applyConfigChange(prev, next Config) {
	a.applyLogFilter(next)

	if !next.SearchEnabled {
		a.dropSearchIndex()
		return
	}
	if searchIndexNeedsRebuild(prev, next) {
		go a.rebuildSearchIndex()
	}
}

// searchIndexNeedsRebuild reports whether a saved change makes the global
// index wrong: search was off before, or the kinds that the index covers
// changed. A rebuild reads each note. The caller tests SearchEnabled first.
func searchIndexNeedsRebuild(prev, next Config) bool {
	if !prev.SearchEnabled {
		return true
	}
	if prev.SearchBundled != next.SearchBundled {
		return true
	}
	// Compare after normalization. A nil list and the default list are the
	// same set.
	return strings.Join(normalizeSearchKinds(prev.SearchKinds), ",") !=
		strings.Join(normalizeSearchKinds(next.SearchKinds), ",")
}

// handleRestart restarts the process, thus the start-time values come from
// the saved config. It answers first and exits on a delayed goroutine. On
// Android, it calls os.Exit(0), and START_STICKY starts ServerService again.
// On the desktop, it starts a new copy with OMN_GO_RESTARTED=1, thus no
// second browser tab opens.
func (a *App) handleRestart(w http.ResponseWriter, r *http.Request) {
	a.log(logx.Restart).Infof("restart requested via /api/restart")
	w.Write([]byte("Restarting"))

	go func() {
		time.Sleep(500 * time.Millisecond) // let the response flush
		restartHook(a)
	}()
}

// restartHook stops this process. On the desktop it first starts a new
// process. handleRestart calls the hook after it answers. A test replaces
// the hook, because the real hook stops the test binary too.
var restartHook = (*App).restartProcess

func (a *App) restartProcess() {
	if runtime.GOOS == "android" {
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		a.log(logx.Restart).Errf("cannot locate own executable, not restarting: %v", err)
		return
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "OMN_GO_RESTARTED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		// A working old instance is better than none.
		a.log(logx.Restart).Errf("failed to start replacement process, keeping current one: %v", err)
		return
	}
	a.log(logx.Restart).Infof("replacement process started (pid %d), exiting", cmd.Process.Pid)
	os.Exit(0)
}

// serveConfigPage answers /Config.html. The page has no .md and no cache.
func (a *App) serveConfigPage(w http.ResponseWriter, r *http.Request) {
	a.renderPage(w, http.StatusOK, "Config", pageHeader("Config", "Settings"), a.getConfigPageBody())
}
