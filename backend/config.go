package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxGitServers is the fixed number of git server slots. The loader,
// config_fields.go and getConfigPageBody read it, thus a new count is a
// change of one line.
const maxGitServers = 5

// These are the theme values of Config.Theme. ThemeAuto follows the dark mode
// of the system, through the prefers-color-scheme media query of the CSS.
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// normalizeTheme maps each input to a valid theme. An unknown or empty value,
// also a missing key, becomes ThemeAuto. The loader, the POST handler and the
// renderer all use it, thus each reader after them gets one of the three
// constants.
func normalizeTheme(s string) string {
	switch s {
	case ThemeLight, ThemeDark:
		return s
	default:
		return ThemeAuto
	}
}

// These are the Android system bar modes of Config.AndroidFullscreen. The
// default is FullscreenOn, and NOT the zero value. AndroidManifest.xml sets
// Theme.NoTitleBar.Fullscreen, thus the app hides the status bar when
// config.json has no value. With a bool, a missing key would mean false, and
// an old install would change its look.
const (
	FullscreenOff       = "off"        // status and navigation bars visible
	FullscreenOn        = "fullscreen" // status bar hidden (the default)
	FullscreenImmersive = "immersive"  // status AND navigation bars hidden
)

// normalizeFullscreen maps each input to a valid mode. An unknown or empty
// value, also a missing key, becomes FullscreenOn.
// MainActivity.readFullscreenMode reads config.json itself, and it must apply
// the same default.
func normalizeFullscreen(s string) string {
	switch s {
	case FullscreenOff, FullscreenImmersive:
		return s
	default:
		return FullscreenOn
	}
}

// These are the search kinds of Config.SearchKinds. They tell what the GLOBAL
// index covers. Page search ignores this setting, and it reads the open file.
// The default is notes and bookmarks. Scripts and JSON are optional, because
// each kind costs memory for the life of the process.
const (
	SearchKindMD        = "md"
	SearchKindBookmarks = "bookmarks"
	SearchKindJS        = "js"
	SearchKindJSON      = "json"
	SearchKindUserJSON  = "user_json"
)

var searchKindsAll = []string{
	SearchKindMD, SearchKindBookmarks, SearchKindJS, SearchKindJSON, SearchKindUserJSON,
}

var searchKindsDefault = []string{SearchKindMD, SearchKindBookmarks}

// These are the scope values of Config.SearchScope. They tell where a search
// STARTS. The dialog can change the scope of one query.
const (
	SearchScopeAll  = "all"
	SearchScopePage = "page"
)

// normalizeSearchKinds keeps only known kinds, with no duplicate, in their
// order. nil and empty differ on purpose. A config.json with NO search_kinds
// key gives nil, and nil gets the default. A person who clears each box gets
// an empty list, which means "index nothing".
func normalizeSearchKinds(kinds []string) []string {
	if kinds == nil {
		return append([]string(nil), searchKindsDefault...)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, k := range kinds {
		k = strings.ToLower(strings.TrimSpace(k))
		if seen[k] {
			continue
		}
		for _, known := range searchKindsAll {
			if k == known {
				seen[k] = true
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// logTagsDefault holds each tag of allLogTags in log_levels.go. A fresh
// install checks each tag, because the two level switches are the control
// that a reader finds first. The tag list narrows a level that is on.
var logTagsDefault = func() []string {
	out := make([]string, 0, len(allLogTags))
	for _, t := range allLogTags {
		out = append(out, string(t))
	}
	return out
}()

// normalizeLogTags keeps only known tags, with no duplicate, in the order of
// allLogTags. As in normalizeSearchKinds, nil gets each tag, and an empty
// list means "no debug or info line".
func normalizeLogTags(tags []string) []string {
	if tags == nil {
		return append([]string(nil), logTagsDefault...)
	}
	want := map[string]bool{}
	for _, t := range tags {
		want[strings.ToLower(strings.TrimSpace(t))] = true
	}
	out := []string{}
	for _, known := range logTagsDefault {
		if want[known] {
			out = append(out, known)
		}
	}
	return out
}

// normalizeSearchScope maps each unknown or empty value to SearchScopeAll.
func normalizeSearchScope(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == SearchScopePage {
		return SearchScopePage
	}
	return SearchScopeAll
}

// ----------------------------------------------------------------------
// The mime_types map of config.json
// ----------------------------------------------------------------------
//
// Config.MimeTypes OVERRIDES builtinMIME in serving.go, and
// resolveContentType reads it first. builtinMIME is the one authority for a
// content type. A FRESH INSTALL WRITES NO MAP, because each row of a map
// hides the table and has no charset. See
// doc/decisions/0003-use-one-table-for-each-content-type.md.
//
// legacyMimeSeeds holds the two maps that an older version wrote.
// dropLegacyMimeSeed removes a map only when it is EXACTLY one of them. A map
// with one changed row is a choice of the user, and it stays.
var legacyMimeSeeds = []map[string]string{
	{
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".html":  "text/html",
		".md":    "text/markdown",
		".svg":   "image/svg+xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".woff2": "font/woff2",
	},
	{
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".woff2": "font/woff2",
	},
}

// dropLegacyMimeSeed removes a mime_types map that an older version wrote,
// and it reports whether it changed something. The caller then writes
// config.json. A nil map is the state of a fresh install, thus the function
// answers false.
func (a *App) dropLegacyMimeSeed() bool {
	if a.Config.MimeTypes == nil {
		return false
	}
	for _, seed := range legacyMimeSeeds {
		if !sameStringMap(a.Config.MimeTypes, seed) {
			continue
		}
		a.Config.MimeTypes = nil
		a.logInfof(logConfig, "removed the mime_types map that an older version wrote, "+
			"thus the content types of this build answer again")
		return true
	}
	return false
}

// sameStringMap answers whether two maps hold the same keys and values.
func sameStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type GitServerConfig struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SSHKeyData string `json:"ssh_key_data"`
	Password   string `json:"password"`
}

// defaultMaxUploadSizeMB is the default limit, in MB, for an uploaded image
// or JSON file. saveUploadedFile in upload_handlers.go applies it. The Android
// "share to OMN-Go" path writes the file without the Go server, thus
// MainActivity.java reads the same value from config.json. See
// Config.MaxUploadSizeMB.
const defaultMaxUploadSizeMB = 3

type Config struct {
	ForcePullOneTime bool   `json:"force_pull_one_time"`
	ServerPort       int    `json:"server_port"`
	AdminPassword    string `json:"admin_password"`
	GuestPassword    string `json:"guest_password"`
	Author           string `json:"author"`
	UseInternalEd    bool   `json:"use_internal_editor"`
	DesktopExtCmd    string `json:"desktop_ext_cmd"`
	Theme            string `json:"theme"` // "auto" | "light" | "dark", see normalizeTheme
	// ShareLAN sets the listen address. False, the default, binds 127.0.0.1,
	// and only this device can connect. True binds 0.0.0.0, and the admin and
	// guest passwords protect the connection. The socket binds one time, thus
	// a change applies at the next start. See
	// doc/decisions/0002-bind-the-loopback-address-when-lan-sharing-is-off.md.
	ShareLAN         bool              `json:"share_lan"`
	Hostname         string            `json:"hostname"`
	BackupPruneDepth int               `json:"backup_prune_depth"`
	MimeTypes        map[string]string `json:"mime_types"`
	ActiveGitIndex   int               `json:"active_git_index"`
	GitServers       []GitServerConfig `json:"git_servers"`
	// SearchEnabled turns on GLOBAL search, which builds and keeps an index.
	// The default is FALSE. The index uses about half the size of the indexed
	// text, for the life of the process. Page search does not need it.
	SearchEnabled bool `json:"search_enabled"`
	// SearchKinds is what the global index covers. See normalizeSearchKinds
	// for why absent and empty differ.
	SearchKinds []string `json:"search_kinds"`
	// SearchBundled also indexes the scripts that OMN-Go ships, from the
	// versionDependentAssets list. It is off by default, because they are
	// larger than a typical note collection and rarely the target of a
	// search.
	SearchBundled bool `json:"search_bundled"`
	// SearchScope is where a search starts: "all" or "page".
	SearchScope string `json:"search_scope"`
	// MaxUploadSizeMB limits an uploaded image or JSON file, in MB. See
	// defaultMaxUploadSizeMB.
	MaxUploadSizeMB int `json:"max_upload_size_mb"`
	// EnableIntentURI is the main switch for an Android "intent:" link in a
	// note, for example
	// [Wi-Fi](intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;).
	// The default is false, and MainActivity.shouldOverrideUrlLoading then
	// refuses each intent URI. MainActivity reads the value from config.json
	// at each tap, thus a change needs no restart. The desktop ignores it.
	EnableIntentURI bool `json:"enable_intent_uri"`
	// EnableTermuxIntent also allows the Termux RUN_COMMAND path: a note that
	// runs a shell command through com.termux/.app.RunCommandService. The
	// default is false, and it needs EnableIntentURI too. Termux must be
	// installed, with its RUN_COMMAND permission, and each tap needs a
	// confirmation. The Android side enforces each rule, and it reads the
	// value from config.json at each tap.
	EnableTermuxIntent bool `json:"enable_termux_intent"`
	// AndroidFullscreen selects the system bars that the Android app hides.
	// See normalizeFullscreen. MainActivity reads it from config.json on
	// resume and after each page load, thus a change needs no restart. The
	// desktop ignores it.
	AndroidFullscreen string `json:"android_fullscreen"`
	// LogDebug and LogInfo turn on the two quiet log levels. Both are false
	// on a fresh install, because each open page copies the log into the
	// browser console. The error level has no switch: a person who asks for
	// less noise never asks for fewer faults. See log_levels.go.
	LogDebug bool `json:"log_debug"`
	LogInfo  bool `json:"log_info"`
	// LogTags is the second axis. A debug or info line prints when its level
	// is on AND its tag is in this list. An error line ignores the list. See
	// normalizeLogTags.
	LogTags []string `json:"log_tags"`
}

func (a *App) loadConfig(storageDir string) {
	a.ConfigMutex.Lock()
	defer a.ConfigMutex.Unlock()

	configPath := filepath.Join(a.StorageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		a.Config = Config{
			// Do not use a literal 8080. The fdroid flavor of Android passes
			// 8081, because the two flavors can run side by side. The loader
			// writes the port to config.json, thus each later reader sees it.
			ServerPort:      a.fallbackPort(),
			AdminPassword:   "admin_secret_changeme",
			GuestPassword:   "guest_secret_changeme",
			Author:          "Anonymous",
			UseInternalEd:   true,
			DesktopExtCmd:   "subl",
			Theme:           ThemeAuto,
			MaxUploadSizeMB: defaultMaxUploadSizeMB,

			// Global search is off on a fresh install. When a person turns it
			// on, it covers notes and bookmarks.
			SearchEnabled: false,
			SearchKinds:   append([]string(nil), searchKindsDefault...),
			SearchScope:   SearchScopeAll,

			// Both quiet levels are off, and each tag is checked. A fresh
			// install thus writes faults and nothing else.
			LogDebug: false,
			LogInfo:  false,
			LogTags:  append([]string(nil), logTagsDefault...),
			// This is the same as Theme.NoTitleBar.Fullscreen in the
			// manifest.
			AndroidFullscreen: FullscreenOn,

			// Hostname labels this device in the file names of database
			// backups. See db_backup.go. BackupPruneDepth is the number of
			// backups that each database keeps.
			Hostname:         defaultHostname(),
			BackupPruneDepth: 3,

			// Write NO MimeTypes MAP. A fresh install overrides nothing. See
			// legacyMimeSeeds.
		}
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			a.logErrf(logConfig, "loadConfig: failed to marshal default config: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			a.logErrf(logConfig, "loadConfig: failed to write default config.json: %v", err)
		}
	} else {
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			// The loader cannot read an existing config.json. Leave a.Config
			// at its zero value, and write an error line. Do not run with an
			// empty config in silence.
			a.logErrf(logConfig, "loadConfig: failed to read %s: %v", configPath, readErr)
		} else if err := json.Unmarshal(data, &a.Config); err != nil {
			// A config.json that does not parse leaves a.Config partly zero.
			// The error line explains why the passwords and settings seem to
			// reset.
			a.logErrf(logConfig, "loadConfig: failed to parse %s (using defaults for any unparsed fields): %v", configPath, err)
		}
	}
	// A config.json with no server_port, or a value below 1, gets the
	// fallback port.
	if a.Config.ServerPort <= 0 {
		a.Config.ServerPort = a.fallbackPort()
	}
	// normalizeConfig applies each other repair from the table in
	// config_fields.go. An old config.json can have an empty theme and no
	// log_tags key. The next save of config.json writes the repair.
	normalizeConfig(&a.Config)
	// The slot array always holds maxGitServers rows. A config.json with
	// fewer rows, or with "git_servers": null, gets the missing rows here.
	// getConfigPageBody holds a second guard for its snapshot.
	for len(a.Config.GitServers) < maxGitServers {
		a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
	}

	// Remove a map that an older version wrote, thus the table of the build
	// answers again. See dropLegacyMimeSeed.
	if a.dropLegacyMimeSeed() {
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			a.logErrf(logConfig, "loadConfig: failed to marshal config after the mime-type repair: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			a.logErrf(logConfig, "loadConfig: failed to write config.json after the mime-type repair: %v", err)
		}
	}

	// Call this last. Each line above is a fault, and a fault always prints.
	// See applyLogFilter.
	a.applyLogFilter(a.Config)
}
