package backend

import (
	"fmt"
	"strings"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
)

// --- The Config page ---

var (
	configPageTmpl    = render.LoadTemplate("config_page.html")
	gitServerCardTmpl = render.LoadTemplate("git_server_card.html")
)

// gitServerView is one git server slot on the Config page. It holds NO SSH
// key and NO key password, and configPageView holds no user password. See
// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
type gitServerView struct {
	Index   int
	Slot    int
	Active  bool
	Name    string
	URL     string
	HostKey string // the line of hostKeyText, or ""
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
// entry shows its own name. logx.AllTags is the authority for the tag set.
var logTagLabels = map[logx.Tag]string{
	logx.NotFound:    "Requests for a page that does not exist",
	logx.Assets:      "Bundled asset refresh at startup",
	logx.Config:      "Reading and writing config.json",
	logx.DB:          "SQLite handles behind /api/sql",
	logx.DBBackup:    "Database backup and pruning",
	logx.DBBootstrap: "First-run restore on a new device",
	logx.DBRestore:   "Database restore from a backup",
	logx.Edit:        "The external editor",
	logx.Exchange:    "Note import and export",
	logx.NoteFiles:   "Files carried between md/ and html/",
	logx.Page:        "Reading and writing a note",
	logx.Precompile:  "Compiling notes to HTML",
	logx.Restart:     "Restarting the server process",
	logx.Search:      "The global search index",
	logx.Server:      "Startup, the listener and crashes",
	logx.Session:     "The login and the session key",
	logx.Status:      "The Status page",
	logx.Storage:     "The storage directory",
	logx.Sync:        "Git sync, the loudest subsystem",
	logx.Tags:        "The tags index",
	logx.Templates:   "The embedded page templates",
	logx.Upload:      "File uploads",
}

// renderLogTagBoxes makes one checkbox for each tag in logx.AllTags. A new tag
// thus needs one line in internal/logx/levels.go and nothing else.
func renderLogTagBoxes(checked map[string]string) string {
	var b strings.Builder
	for _, tag := range logx.AllTags {
		label, ok := logTagLabels[tag]
		if !ok {
			label = string(tag)
		}
		b.WriteString(`                <div class="config-checkbox-row">` + "\n")
		b.WriteString(`                    <input type="checkbox" name="log_tags" value="` +
			render.EscapeHTML(string(tag)) + `" ` + checked[string(tag)] + ` />` + "\n")
		b.WriteString(`                    <label class="config-label"><code>` +
			render.EscapeHTML(string(tag)) + `</code> - ` + render.EscapeHTML(label) + `</label>` + "\n")
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
		cards.WriteString(render.Fill(gitServerCardTmpl, map[string]string{
			"INDEX":          fmt.Sprintf("%d", gs.Index),
			"SLOT":           fmt.Sprintf("%d", gs.Slot),
			"ACTIVE_CHECKED": checked,
			"NAME":           render.EscapeHTML(gs.Name),
			"URL":            render.EscapeHTML(gs.URL),
			"HOST_KEY":       render.EscapeHTML(gs.HostKey),
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
	if config.NormalizeSearchScope(v.SearchScope) == config.SearchScopePage {
		searchScopeAllSel, searchScopePageSel = "", "checked"
	}

	// Mark exactly one option as selected. config.NormalizeTheme answers one of
	// the three values, and auto for an unknown one.
	themeSel := map[string]string{
		"THEME_AUTO_SEL":  "",
		"THEME_LIGHT_SEL": "",
		"THEME_DARK_SEL":  "",
	}
	switch config.NormalizeTheme(v.Theme) {
	case config.ThemeLight:
		themeSel["THEME_LIGHT_SEL"] = "selected"
	case config.ThemeDark:
		themeSel["THEME_DARK_SEL"] = "selected"
	default:
		themeSel["THEME_AUTO_SEL"] = "selected"
	}

	// Mark exactly one option as selected. config.NormalizeFullscreen answers one
	// of the three values, and FullscreenOn for an unknown one.
	// internal/config/config.go tells why on is the default.
	fsSel := map[string]string{
		"FS_OFF_SEL":       "",
		"FS_ON_SEL":        "",
		"FS_IMMERSIVE_SEL": "",
	}
	switch config.NormalizeFullscreen(v.AndroidFullscreen) {
	case config.FullscreenOff:
		fsSel["FS_OFF_SEL"] = "selected"
	case config.FullscreenImmersive:
		fsSel["FS_IMMERSIVE_SEL"] = "selected"
	default:
		fsSel["FS_ON_SEL"] = "selected"
	}

	// Put no ADMIN_PWD here. See gitServerView.
	return render.Fill(configPageTmpl, map[string]string{
		// Give the names of the checkboxes of this page, from the table in
		// internal/config/fields.go. See config.CheckboxFields.
		"CONFIG_FIELDS":          config.CheckboxFields(),
		"SERVER_PORT":            fmt.Sprintf("%d", v.ServerPort),
		"AUTHOR":                 render.EscapeHTML(v.Author),
		"INTERNAL_ED_CHECKED":    internalEdChecked,
		"SHARE_LAN_CHECKED":      shareLanChecked,
		"INTENT_URI_CHECKED":     intentUriChecked,
		"TERMUX_INTENT_CHECKED":  termuxIntentChecked,
		"DESKTOP_EXT_CMD":        render.EscapeHTML(v.DesktopExtCmd),
		"HOSTNAME":               render.EscapeHTML(config.NormalizeHostname(v.Hostname)),
		"BACKUP_PRUNE_DEPTH":     fmt.Sprintf("%d", config.NormalizePruneDepth(v.PruneDepth)),
		"THEME_AUTO_SEL":         themeSel["THEME_AUTO_SEL"],
		"THEME_LIGHT_SEL":        themeSel["THEME_LIGHT_SEL"],
		"THEME_DARK_SEL":         themeSel["THEME_DARK_SEL"],
		"MAX_UPLOAD_MB":          fmt.Sprintf("%d", v.MaxUploadSizeMB),
		"FS_OFF_SEL":             fsSel["FS_OFF_SEL"],
		"FS_ON_SEL":              fsSel["FS_ON_SEL"],
		"FS_IMMERSIVE_SEL":       fsSel["FS_IMMERSIVE_SEL"],
		"SEARCH_ENABLED_CHECKED": searchEnabledChecked,
		"SEARCH_BUNDLED_CHECKED": searchBundledChecked,
		"SEARCH_KIND_MD":         kindChecked[config.SearchKindMD],
		"SEARCH_KIND_BOOKMARKS":  kindChecked[config.SearchKindBookmarks],
		"SEARCH_KIND_JS":         kindChecked[config.SearchKindJS],
		"SEARCH_KIND_JSON":       kindChecked[config.SearchKindJSON],
		"SEARCH_KIND_USER_JSON":  kindChecked[config.SearchKindUserJSON],
		"SEARCH_SCOPE_ALL_SEL":   searchScopeAllSel,
		"SEARCH_SCOPE_PAGE_SEL":  searchScopePageSel,
		"SEARCH_INDEX_STATUS":    render.EscapeHTML(v.SearchIndexStatus),
		"LOG_DEBUG_CHECKED":      logDebugChecked,
		"LOG_INFO_CHECKED":       logInfoChecked,
		"LOG_TAG_BOXES":          renderLogTagBoxes(logTagChecked),
		"GIT_SERVERS":            cards.String(),
	})
}
