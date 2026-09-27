package backend

import (
	"fmt"
	"strings"
)

// --- The Config page ---

// gitServerView is one git server slot on the Config page. It holds NO SSH
// key and NO key password, and configPageView holds no user password, because
// /Config.html needs no login. See
// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
type gitServerView struct {
	Index  int
	Slot   int
	Active bool
	Name   string
	URL    string
}

type configPageView struct {
	ServerPort         int
	Author             string
	UseInternalEd      bool
	DesktopExtCmd      string
	Theme              string // "auto" | "light" | "dark" (normalized)
	ShareLAN           bool
	Hostname           string
	PruneDepth         int
	MaxUploadSizeMB    int
	EnableIntentURI    bool
	EnableTermuxIntent bool
	AndroidFullscreen  string // "off" | "fullscreen" | "immersive" (normalized)
	SearchEnabled      bool
	SearchKinds        []string // normalized
	SearchBundled      bool
	SearchScope        string // "all" | "page" (normalized)
	SearchIndexStatus  string // human-readable line for the Search screen
	LogDebug           bool
	LogInfo            bool
	LogTags            []string // normalized
	GitServers         []gitServerView
}

// logTagLabels gives the text beside each log tag checkbox. A tag with no
// entry shows its own name. allLogTags is the authority for the tag set.
var logTagLabels = map[logTag]string{
	log404:         "Requests for a page that does not exist",
	logAssets:      "Bundled asset refresh at startup",
	logConfig:      "Reading and writing config.json",
	logDB:          "SQLite handles behind /api/sql",
	logDBBackup:    "Database backup and pruning",
	logDBBootstrap: "First-run restore on a new device",
	logDBRestore:   "Database restore from a backup",
	logEdit:        "The external editor",
	logExchange:    "Note import and export",
	logNoteFiles:   "Files carried between md/ and html/",
	logPage:        "Reading and writing a note",
	logPrecompile:  "Compiling notes to HTML",
	logRestart:     "Restarting the server process",
	logSearch:      "The global search index",
	logServer:      "Startup, the listener and crashes",
	logSession:     "The login and the session key",
	logStatus:      "The Status page",
	logStorage:     "The storage directory",
	logSync:        "Git sync, the loudest subsystem",
	logTags:        "The tags index",
	logTemplates:   "The embedded page templates",
	logUpload:      "File uploads",
}

// renderLogTagBoxes makes one checkbox for each tag in allLogTags. A new tag
// thus needs one line in log_levels.go and nothing else.
func renderLogTagBoxes(checked map[string]string) string {
	var b strings.Builder
	for _, tag := range allLogTags {
		label, ok := logTagLabels[tag]
		if !ok {
			label = string(tag)
		}
		b.WriteString(`                <div class="config-checkbox-row">` + "\n")
		b.WriteString(`                    <input type="checkbox" name="log_tags" value="` +
			escapeHTML(string(tag)) + `" ` + checked[string(tag)] + ` />` + "\n")
		b.WriteString(`                    <label class="config-label"><code>` +
			escapeHTML(string(tag)) + `</code> - ` + escapeHTML(label) + `</label>` + "\n")
		b.WriteString("                </div>\n")
	}
	return b.String()
}

func renderConfigPage(v configPageView) string {
	var cards strings.Builder
	for _, gs := range v.GitServers {
		checked := ""
		if gs.Active {
			checked = "checked"
		}
		// Put no SSH_KEY and no PASSWORD here. See gitServerView.
		cards.WriteString(fill(gitServerCardTmpl, map[string]string{
			"INDEX":          fmt.Sprintf("%d", gs.Index),
			"SLOT":           fmt.Sprintf("%d", gs.Slot),
			"ACTIVE_CHECKED": checked,
			"NAME":           escapeHTML(gs.Name),
			"URL":            escapeHTML(gs.URL),
		}))
	}

	internalEdChecked := ""
	if v.UseInternalEd {
		internalEdChecked = "checked"
	}
	shareLanChecked := ""
	if v.ShareLAN {
		shareLanChecked = "checked"
	}
	intentUriChecked := ""
	if v.EnableIntentURI {
		intentUriChecked = "checked"
	}
	termuxIntentChecked := ""
	if v.EnableTermuxIntent {
		termuxIntentChecked = "checked"
	}
	searchEnabledChecked := ""
	if v.SearchEnabled {
		searchEnabledChecked = "checked"
	}
	searchBundledChecked := ""
	if v.SearchBundled {
		searchBundledChecked = "checked"
	}
	// Make one checkbox for each kind. Check it when the normalized list
	// holds the kind.
	kindChecked := map[string]string{}
	for _, k := range v.SearchKinds {
		kindChecked[k] = "checked"
	}
	logDebugChecked := ""
	if v.LogDebug {
		logDebugChecked = "checked"
	}
	logInfoChecked := ""
	if v.LogInfo {
		logInfoChecked = "checked"
	}
	// Make one checkbox for each tag. Check it when the normalized list holds
	// the tag.
	logTagChecked := map[string]string{}
	for _, t := range v.LogTags {
		logTagChecked[t] = "checked"
	}

	searchScopeAllSel, searchScopePageSel := "checked", ""
	if normalizeSearchScope(v.SearchScope) == SearchScopePage {
		searchScopeAllSel, searchScopePageSel = "", "checked"
	}

	// Mark exactly one option as selected. normalizeTheme answers one of the
	// three values, and auto for an unknown one.
	themeSel := map[string]string{
		"THEME_AUTO_SEL":  "",
		"THEME_LIGHT_SEL": "",
		"THEME_DARK_SEL":  "",
	}
	switch normalizeTheme(v.Theme) {
	case ThemeLight:
		themeSel["THEME_LIGHT_SEL"] = "selected"
	case ThemeDark:
		themeSel["THEME_DARK_SEL"] = "selected"
	default:
		themeSel["THEME_AUTO_SEL"] = "selected"
	}

	// Mark exactly one option as selected. normalizeFullscreen answers one of
	// the three values, and FullscreenOn for an unknown one. config.go tells
	// why on is the default.
	fsSel := map[string]string{
		"FS_OFF_SEL":       "",
		"FS_ON_SEL":        "",
		"FS_IMMERSIVE_SEL": "",
	}
	switch normalizeFullscreen(v.AndroidFullscreen) {
	case FullscreenOff:
		fsSel["FS_OFF_SEL"] = "selected"
	case FullscreenImmersive:
		fsSel["FS_IMMERSIVE_SEL"] = "selected"
	default:
		fsSel["FS_ON_SEL"] = "selected"
	}

	// Put no ADMIN_PWD and no GUEST_PWD here. See gitServerView.
	return fill(configPageTmpl, map[string]string{
		// Give the names of the checkboxes of this page, from the table in
		// config_fields.go. See configCheckboxFields.
		"CONFIG_FIELDS":          configCheckboxFields(),
		"SERVER_PORT":            fmt.Sprintf("%d", v.ServerPort),
		"AUTHOR":                 escapeHTML(v.Author),
		"INTERNAL_ED_CHECKED":    internalEdChecked,
		"SHARE_LAN_CHECKED":      shareLanChecked,
		"INTENT_URI_CHECKED":     intentUriChecked,
		"TERMUX_INTENT_CHECKED":  termuxIntentChecked,
		"DESKTOP_EXT_CMD":        escapeHTML(v.DesktopExtCmd),
		"HOSTNAME":               escapeHTML(normalizeHostname(v.Hostname)),
		"BACKUP_PRUNE_DEPTH":     fmt.Sprintf("%d", normalizePruneDepth(v.PruneDepth)),
		"THEME_AUTO_SEL":         themeSel["THEME_AUTO_SEL"],
		"THEME_LIGHT_SEL":        themeSel["THEME_LIGHT_SEL"],
		"THEME_DARK_SEL":         themeSel["THEME_DARK_SEL"],
		"MAX_UPLOAD_MB":          fmt.Sprintf("%d", v.MaxUploadSizeMB),
		"FS_OFF_SEL":             fsSel["FS_OFF_SEL"],
		"FS_ON_SEL":              fsSel["FS_ON_SEL"],
		"FS_IMMERSIVE_SEL":       fsSel["FS_IMMERSIVE_SEL"],
		"SEARCH_ENABLED_CHECKED": searchEnabledChecked,
		"SEARCH_BUNDLED_CHECKED": searchBundledChecked,
		"SEARCH_KIND_MD":         kindChecked[SearchKindMD],
		"SEARCH_KIND_BOOKMARKS":  kindChecked[SearchKindBookmarks],
		"SEARCH_KIND_JS":         kindChecked[SearchKindJS],
		"SEARCH_KIND_JSON":       kindChecked[SearchKindJSON],
		"SEARCH_KIND_USER_JSON":  kindChecked[SearchKindUserJSON],
		"SEARCH_SCOPE_ALL_SEL":   searchScopeAllSel,
		"SEARCH_SCOPE_PAGE_SEL":  searchScopePageSel,
		"SEARCH_INDEX_STATUS":    escapeHTML(v.SearchIndexStatus),
		"LOG_DEBUG_CHECKED":      logDebugChecked,
		"LOG_INFO_CHECKED":       logInfoChecked,
		"LOG_TAG_BOXES":          renderLogTagBoxes(logTagChecked),
		"GIT_SERVERS":            cards.String(),
	})
}
