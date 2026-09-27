package backend

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"strings"
)

// ----------------------------------------------------------------------
// Why this file does NOT use html/template
// ----------------------------------------------------------------------
//
// html/template calls reflect.Value.MethodByName. That call stops the
// dead-code elimination of the linker for methods in the whole program, and
// the method set of go-git is large. See
// doc/decisions/0008-render-the-pages-without-html-template.md.
//
// This file keeps the one guarantee of html/template: the correct escape for
// each context. Each render function escapes each value for the place where
// the value goes:
//
//	escapeHTML(v)            HTML text, or a quoted HTML attribute.
//	escapeJS(v)              A '...' or "..." JS string in an inline <script>.
//	escapeHTML(escapeJS(v))  A JS string inside an HTML attribute, for
//	                         example onclick="...".
//	trusted HTML             The markdown body, or a fragment from a render
//	                         function here. It goes in as it is, and never
//	                         gets a second escape.

// escapeHTML escapes a value for HTML text or a double-quoted HTML attribute.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// escapeJS escapes a value for a quoted JavaScript string in an inline
// <script>. It writes '<' and '>' as hex escapes, thus no value can close the
// </script> block.
func escapeJS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '<':
			b.WriteString(`\x3c`)
		case '>':
			b.WriteString(`\x3e`)
		case '&':
			b.WriteString(`\x26`)
		case '\u2028':
			b.WriteString(`\u2028`)
		case '\u2029':
			b.WriteString(`\u2029`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// loadTemplate reads one page fragment from templatesFS, which server.go
// declares. That embed stays separate from staticFS, because the app extracts
// staticFS to disk as files that a person can edit. A missing file is a
// packaging fault. The first render shows it, and the start does not fail.
func loadTemplate(filename string) string {
	data, err := templatesFS.ReadFile("frontend/templates/" + filename)
	if err != nil {
		log.Printf("[templates] (error) failed to read embedded %s: %v", filename, err)
		return "<p>Missing embedded template: " + escapeHTML(filename) + "</p>"
	}
	return string(data)
}

// incomingIndexTmpl is the incoming index as the app first writes it: a
// header block, the receive box and the list marker. See incomingIndexStarter
// in note_exchange.go.
var incomingIndexTmpl = loadTemplate("incoming_index.md")

var (
	// index.html loads css/omn-go-custom.css as the last stylesheet and
	// js/omn-go-custom.js as the last script. A user rule thus wins against
	// an app rule of the same specificity. The user script sees each name of
	// the app scripts. Both files belong to the user, thus no upgrade
	// replaces them. editor.html loads neither, thus a bad custom file can
	// never lock the user out of the editor.
	//
	// Do not put these notes in the template. The template goes to the
	// browser with each page.
	indexPageTmpl     = loadTemplate("index.html")
	configPageTmpl    = loadTemplate("config_page.html")
	gitServerCardTmpl = loadTemplate("git_server_card.html")
	externalEditTmpl  = loadTemplate("external_edit.html")
	editorPageTmpl    = loadTemplate("editor.html")
	notFoundTmpl      = loadTemplate("not_found.html")
	notEditableTmpl   = loadTemplate("not_editable.html")
	statusPageTmpl    = loadTemplate("status_page.html")
	logsPageTmpl      = loadTemplate("logs_page.html")
	searchPageTmpl    = loadTemplate("search_page.html")
	filesPageTmpl     = loadTemplate("files_page.html")
	// modalsHTML holds the modals that need the server: login, quick note,
	// bookmark, commit and conflict. The cached page holds only modalsMarker,
	// and injectRuntimeVars puts the modals in when the server sends the
	// page. An exported page has no server, thus it stays small.
	modalsHTML = loadTemplate("modals.html")
)

// fill replaces the %%NAME%% placeholders in tmpl. Each value MUST already
// have the escape for its place. See the banner of this file. fill itself
// escapes nothing, thus trusted HTML can also pass through it.
func fill(tmpl string, pairs map[string]string) string {
	oldnew := make([]string, 0, len(pairs)*2)
	for k, v := range pairs {
		oldnew = append(oldnew, "%%"+k+"%%", v)
	}
	return strings.NewReplacer(oldnew...).Replace(tmpl)
}

// --- The page shell (index.html) ---

// metaTagView is one <meta name="..." content="..."> from the header block of
// a page, or the "generator" tag.
type metaTagView struct {
	Name  string
	Value string
}

// indexPageView holds each value of renderIndexPage. PreviewHTML is trusted
// HTML. renderIndexPage escapes each other field.
type indexPageView struct {
	Title       string
	PackageName string
	PageName    string
	PageExt     string
	IsMarkdown  bool
	IsAndroid   bool
	AssetPrefix string // "", "../", "../../", … or "/" — see compilePageWithBody
	MetaTags    []metaTagView
	Tags        []string
	PreviewHTML string
}

func renderIndexPage(v indexPageView) string {
	var metaTags strings.Builder
	for _, m := range v.MetaTags {
		fmt.Fprintf(&metaTags, "    <meta name=\"%s\" content=\"%s\" />\n",
			escapeHTML(m.Name), escapeHTML(m.Value))
	}

	condScripts := ""
	if v.IsMarkdown {
		condScripts += "    <script>var IS_MARKDOWN = true;</script>\n"
	}
	if v.IsAndroid {
		condScripts += "    <script>var IS_ANDROID = true;</script>\n"
	}

	var tags strings.Builder
	for _, t := range v.Tags {
		// Each pill links to the Tags page, OMNGoTags, through AssetPrefix
		// ("", "../" and so on). The link thus works at each directory depth,
		// online and through file://. The fragment is tagSlug(t), the same
		// function that makes the section ids of tags.go. AssetPrefix holds
		// only '.' and '/', thus it needs no escape.
		fmt.Fprintf(&tags, `<a href="%sOMNGoTags.html#%s" class="taglink"><span class="tagmark">%s</span></a>`,
			v.AssetPrefix, escapeHTML(tagSlug(t)), escapeHTML(t))
	}

	return fill(indexPageTmpl, map[string]string{
		"TITLE_HTML":   escapeHTML(v.Title),
		"TITLE_JS":     escapeJS(v.Title),
		"PACKAGE_JS":   escapeJS(v.PackageName),
		"PAGE_NAME_JS": escapeJS(v.PageName),
		"PAGE_EXT_JS":  escapeJS(v.PageExt),
		// The path prefix ("", "../" or "/") holds only '.' and '/', thus it
		// needs no escape.
		"ASSET_PREFIX": v.AssetPrefix,
		"META_TAGS":    metaTags.String(),
		"COND_SCRIPTS": condScripts,
		"TAGS_HTML":    tags.String(),
		"PREVIEW_BODY": v.PreviewHTML,
	})
}

// --- The editor page (editor.html) ---

// editorPageView holds each value of renderEditorPage, and the function
// escapes each one for its place. The text of the note is absent on purpose.
// The editor fetches it from /api/note, thus the page never holds a second
// copy.
type editorPageView struct {
	Title   string // display name (page/asset)
	Name    string // value for /api/note and /api/save
	PageExt string // e.g. ".md", ".js" (informational)
	ViewURL string // where to return after save/cancel
}

func renderEditorPage(v editorPageView) string {
	return fill(editorPageTmpl, map[string]string{
		"TITLE_HTML":  escapeHTML(v.Title),
		"NAME_JS":     escapeJS(v.Name),
		"PAGE_EXT_JS": escapeJS(v.PageExt),
		// Only the JavaScript reads this, as OMN_EDIT_VIEW. The × button of
		// omn-go-editor.js goes to it.
		"VIEW_URL_JS": escapeJS(v.ViewURL),
	})
}

// --- The Config page ---

// gitServerView is one git server slot on the Config page. It holds NO SSH
// key and NO key password, and configPageView holds no admin password and no
// guest password. /Config.html needs no login, and a value in a view reaches
// the HTML. The page thus shows empty boxes, and "Show passwords" reads GET
// /api/config. See
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

// logTagLabels gives the text beside the checkbox of each log tag. A tag with
// no entry shows its own name, thus a new tag needs no template change.
// allLogTags in log_levels.go is the authority for the tag set, and not this
// map.
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

// --- The 404 page ---

// notFoundView holds each value of the detailed 404 page. Each field is RAW,
// and renderNotFoundPage escapes it. An attacker controls the URL and the
// Referer, thus neither may reach the output without an escape.
type notFoundView struct {
	URL       string // path + query, exactly as requested
	Method    string
	Time      string
	Referer   string // "" when absent or not from this server
	Suggested string // "" when there is no plausible alternative
}

// safeLocalPath reports whether s can be an href: a path on this server. It
// refuses a scheme, for example "javascript:...", and a protocol-relative
// "//host/...". A request header thus cannot become a live link out of the
// app. serveNotFound already filters the Referer, thus this is a second
// guard.
func safeLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

// notEditableView holds the values of the "not a text file" page.
// renderNotEditablePage escapes Path and Type.
type notEditableView struct {
	Path string // "/css/OMN-Go/fonts/x.woff2"
	Type string // the resolved content type, "unknown" when there is none
}

// renderNotEditablePage makes the page that an editor route sends for a file
// that is not text. See serveEditor. The view link is the same path without
// the edit query.
func renderNotEditablePage(v notEditableView) string {
	typ := v.Type
	if typ == "" {
		typ = "unknown"
	}
	return fill(notEditableTmpl, map[string]string{
		"PATH":     escapeHTML(v.Path),
		"TYPE":     escapeHTML(typ),
		"VIEW_URL": escapeHTML(v.Path),
	})
}

func renderNotFoundPage(v notFoundView) string {
	// This block is trusted HTML that this function makes. Escape each value
	// where it goes in.
	refererRows := ""
	if v.Referer != "" {
		esc := escapeHTML(v.Referer)
		if safeLocalPath(v.Referer) {
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd><a href="%s">%s</a> &middot; <a href="%s?edit=true">edit that page</a></dd>
`, esc, esc, esc)
		} else {
			// Show the Referer, but never as a link. escapeHTML makes it
			// plain text. In an href, a "javascript:" value would stay live.
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd>%s</dd>
`, esc)
		}
	}

	suggestion := ""
	if v.Suggested != "" && safeLocalPath(v.Suggested) {
		esc := escapeHTML(v.Suggested)
		suggestion = fmt.Sprintf(`    <div class="config-field notfound-suggest">
        <span class="notfound-suggest-label">Did you mean</span>
        <a href="%s" class="notfound-suggest-link">%s</a>
        <span class="config-hint">A note of that name exists. A link written as [text](name) asks the server for a file called "name"; note links need the .html suffix - [text](name.html).</span>
    </div>
`, esc, esc)
	}

	return fill(notFoundTmpl, map[string]string{
		"URL":          escapeHTML(v.URL),
		"METHOD":       escapeHTML(v.Method),
		"TIME":         escapeHTML(v.Time),
		"REFERER_ROWS": refererRows,
		"SUGGESTION":   suggestion,
	})
}

// --- The file index page (files_page.html, see files_index.go) ---

// filesCrumb is one step of the breadcrumb. Dir is the new value of ?dir=.
type filesCrumb struct {
	Label string
	Dir   string
}

// filesTreeCard is one button of the first screen.
type filesTreeCard struct {
	Key   string
	Icon  string
	Title string
	Where string
	Count string
	Class string
}

// filesLegendItem is one line of the legend under the crumb.
type filesLegendItem struct {
	Color string
	Word  string
	Text  string
}

// filesDirRow is one directory below the directory in view. Files and Bytes
// are RECURSIVE totals, and a name counts one time also when both sides hold
// it. The four flags tell whether the whole subtree is of one kind. See
// (*filesDirRow).note in files_index.go.
type filesDirRow struct {
	Name        string
	Dir         string
	Files       int
	Bytes       int64
	anyShips    bool
	shipCount   int
	anyDevice   bool
	everyShips  bool
	everyDevice bool
}

// filesFileRow is one NAME of the tree in view. Each field is raw, and
// renderFilesPage escapes it. A name can come from an upload or a note title.
// State is the word of the first line. StateColor and OwnerColor are the
// color classes. See the banner of files_index.go.
type filesFileRow struct {
	Name       string
	Path       string
	URL        string
	EditURL    string // "" when the row offers no edit link
	Kind       string // a Material Icons ligature
	Size       string
	Mod        string // "" for a file that is not on the device
	ModFull    string
	State      string
	StateColor string
	AppOwned   bool
	OwnerColor string
	Extra      []string
}

type filesPageView struct {
	Tree       string // "" on the first screen
	Dir        string
	Crumbs     []filesCrumb
	Cards      []filesTreeCard
	Legend     []filesLegendItem
	Summary    string
	Dirs       []filesDirRow
	Files      []filesFileRow
	Total      int // files directly in this directory, before the cap
	Hidden     int // ... how many of them are not shown
	Empty      bool
	ShowingAll bool
	Denied     bool
}

// filesDeniedNotice is the page that a user who is not admin sees. A person
// can link to this address, thus it is a page, and not the bare 401 of
// authMiddleware. The page names the reason and the remedy. The markup is
// static, and no value from the request goes into it.
const filesDeniedNotice = `<div class="files-notice">` +
	`<h2>Administrator only</h2>` +
	`<p>This page lists the files stored on the device, so it is shown only ` +
	`to an administrator.</p>` +
	`<p class="files-note">Log in from any note page - the account button in ` +
	`the page header - and come back. A connection from the device itself is ` +
	`always treated as the owner; this only applies to other machines on the ` +
	`network.</p>` +
	`</div>`

// filesOwnerHint is the tooltip of the app-owned mark.
const filesOwnerHint = "The next version of OMN-Go backs up your copy and replaces it"

func renderFilesPage(v filesPageView) string {
	if v.Denied {
		return fill(filesPageTmpl, map[string]string{
			"DENIED": " is-denied",
			"NOTICE": filesDeniedNotice,
			"BODY":   "",
		})
	}
	if v.Tree == "" {
		return fill(filesPageTmpl, map[string]string{
			"DENIED": "",
			"NOTICE": "",
			"BODY":   renderFilesCards(v),
		})
	}
	return fill(filesPageTmpl, map[string]string{
		"DENIED": "",
		"NOTICE": "",
		"BODY":   renderFilesListing(v),
	})
}

// renderFilesCards makes the first screen: three buttons in one column at
// each width. A wide screen gets a narrower page, not three columns. There is
// thus one layout to build and to test.
func renderFilesCards(v filesPageView) string {
	var b strings.Builder
	b.WriteString(`<div class="files-cards">`)
	for _, c := range v.Cards {
		fmt.Fprintf(&b, `<a class="files-card %s" href="%s">`+
			`<i class="material-icons files-card-icon">%s</i>`+
			`<span class="files-card-text">`+
			`<span class="files-card-title">%s</span>`+
			`<span class="files-card-where">%s</span>`+
			`<span class="files-card-count">%s</span>`+
			`</span></a>`,
			escapeHTML(c.Class), escapeHTML(filesPageURL(c.Key, "", false)),
			escapeHTML(c.Icon), escapeHTML(c.Title), escapeHTML(c.Where),
			escapeHTML(c.Count))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// filesPageURL makes a link back to this page. It has three parameters, and
// only this function makes them. No request value can thus go into the link.
func filesPageURL(tree, dir string, all bool) string {
	out := "/OMNGoFiles.html"
	sep := "?"
	if tree != "" {
		out += sep + "tree=" + url.QueryEscape(tree)
		sep = "&"
	}
	if dir != "" {
		out += sep + "dir=" + url.QueryEscape(dir)
		sep = "&"
	}
	if all {
		out += sep + "all=1"
	}
	return out
}

func renderFilesListing(v filesPageView) string {
	var b strings.Builder

	// Write the crumb. Each label holds its own slash, and nothing goes
	// between two labels. The trail thus reads as the path: html/js/.
	b.WriteString(`<div class="files-crumbs">`)
	for i, c := range v.Crumbs {
		if i == len(v.Crumbs)-1 {
			fmt.Fprintf(&b, `<span class="files-crumb-here">%s</span>`, escapeHTML(c.Label))
			continue
		}
		fmt.Fprintf(&b, `<a href="%s">%s</a>`,
			escapeHTML(filesPageURL(v.Tree, c.Dir, false)), escapeHTML(c.Label))
	}
	b.WriteString(`</div>`)

	fmt.Fprintf(&b, `<p class="files-summary">%s</p>`, escapeHTML(v.Summary))

	// Fold the legend, and leave it out when this directory uses no word.
	// <details> needs no script, and it keeps its state while the page is
	// open.
	if len(v.Legend) > 0 {
		b.WriteString(`<details class="files-legend">` +
			`<summary>What the words mean</summary>`)
		for _, item := range v.Legend {
			fmt.Fprintf(&b, `<div><b class="%s">%s</b> — %s</div>`,
				escapeHTML(item.Color), escapeHTML(item.Word), escapeHTML(item.Text))
		}
		b.WriteString(`</details>`)
	}

	if v.Empty {
		b.WriteString(`<p class="files-empty">This directory holds nothing.</p>`)
		return b.String()
	}

	b.WriteString(`<ul class="files-list">`)
	for _, d := range v.Dirs {
		fmt.Fprintf(&b, `<li class="files-row files-dir">`+
			`<span class="files-name"><i class="material-icons files-kind">folder</i>`+
			`<a href="%s">%s</a></span>`,
			escapeHTML(filesPageURL(v.Tree, d.Dir, false)), escapeHTML(d.Name+"/"))
		if word, color := filesDirNote(v.Tree, d); word != "" {
			fmt.Fprintf(&b, `<span class="files-state %s">%s</span>`,
				escapeHTML(color), escapeHTML(word))
		}
		fmt.Fprintf(&b, `<span class="files-facts"><span class="files-size">%s · %s</span>`+
			`</span></li>`,
			escapeHTML(filesCountLabel(d.Files)), escapeHTML(filesSize(d.Bytes)))
	}
	for _, f := range v.Files {
		renderFilesRow(&b, f)
	}
	b.WriteString(`</ul>`)

	if v.Hidden > 0 {
		fmt.Fprintf(&b, `<p class="files-more">%s not shown `+
			`<a href="%s">show all %s &rarr;</a></p>`,
			escapeHTML(itoa(v.Hidden)),
			escapeHTML(filesPageURL(v.Tree, v.Dir, true)),
			escapeHTML(itoa(v.Total)))
	}
	return b.String()
}

// renderFilesRow writes one row: the name and the state on the first line,
// and the facts on the second. The name has the whole first line, thus it
// never goes into a narrow column.
func renderFilesRow(b *strings.Builder, f filesFileRow) {
	b.WriteString(`<li class="files-row">`)
	fmt.Fprintf(b, `<span class="files-name">`+
		`<i class="material-icons files-kind">%s</i><a href="%s">%s</a></span>`,
		escapeHTML(f.Kind), escapeHTML(f.URL), escapeHTML(f.Name))
	if f.State != "" {
		fmt.Fprintf(b, `<span class="files-state %s">%s</span>`,
			escapeHTML(f.StateColor), escapeHTML(f.State))
	}
	b.WriteString(`<span class="files-facts">`)
	fmt.Fprintf(b, `<span class="files-size">%s</span>`, escapeHTML(f.Size))
	if f.Mod != "" {
		// Show the date only, because the full time is too wide for a phone.
		// The title keeps the full time.
		fmt.Fprintf(b, `<span class="files-meta" title="%s">%s</span>`,
			escapeHTML(f.ModFull), escapeHTML(f.Mod))
	}
	// The ownership word is on the second line of each row that has it, in
	// each tree. The color is a hint, and the word is the fact.
	if f.AppOwned {
		fmt.Fprintf(b, `<span class="files-meta %s" title="%s">app-owned</span>`,
			escapeHTML(f.OwnerColor), escapeHTML(filesOwnerHint))
	}
	for _, extra := range f.Extra {
		fmt.Fprintf(b, `<span class="files-meta">%s</span>`, escapeHTML(extra))
	}
	if f.EditURL != "" {
		fmt.Fprintf(b, `<a class="files-edit" href="%s">edit</a>`, escapeHTML(f.EditURL))
	}
	b.WriteString(`</span></li>`)
}

// filesDirNote gives the one word that a directory row can show. The rule is
// the same as for a file row: speak only when the app is involved. A
// directory speaks when OMN-Go put files into it, and the count tells how
// many.
func filesDirNote(tree string, d filesDirRow) (word, color string) {
	if tree == filesTreeBundled || !d.anyShips {
		return "", ""
	}
	if d.everyShips && !d.anyDevice {
		return itoa(d.Files) + " " + filesFromTheApp + ", none extracted", filesColorPlain
	}
	return itoa(d.shipCount) + " " + filesFromTheApp, filesColorApp
}

// --- The search result page (search_page.html) ---

// searchPageView holds each value of renderSearchPage. Query is RAW, because
// it comes from a URL. The function escapes it for the attribute and for the
// text. The Results hold the same data as the API. renderSnippetHTML adds the
// <mark> tags and escapes the text.
type searchPageView struct {
	Query        string
	Results      []searchResult
	Total        int
	Truncated    bool
	IndexedKinds []string // what the index currently covers, for the empty state
	Highlight    []string // query terms, hung off every result link as ?hl=
	Disabled     bool     // global search is switched off: explain, do not search
}

// searchKindLabel answers the name of a kind for the group heading.
func searchKindLabel(kind string) string {
	switch kind {
	case SearchKindMD:
		return "Notes"
	case SearchKindBookmarks:
		return "Bookmarks"
	case SearchKindJS:
		return "Scripts"
	case SearchKindJSON:
		return "JSON"
	case SearchKindUserJSON:
		return "Uploaded JSON"
	default:
		return kind
	}
}

// renderSnippetHTML puts <mark> around each span of a snippet. The spans are
// RUNE offsets, thus the function walks the text as []rune, and it never cuts
// a Cyrillic letter. It escapes each segment. The only markup in the result
// is the <mark> tags that it writes.
func renderSnippetHTML(text string, spans [][2]int) string {
	runes := []rune(text)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		start, length := sp[0], sp[1]
		if start < at || length <= 0 || start+length > len(runes) {
			continue
		}
		if start > at {
			b.WriteString(escapeHTML(string(runes[at:start])))
		}
		b.WriteString(`<mark class="omn-search-hit">`)
		b.WriteString(escapeHTML(string(runes[start : start+length])))
		b.WriteString(`</mark>`)
		at = start + length
	}
	if at < len(runes) {
		b.WriteString(escapeHTML(string(runes[at:])))
	}
	return b.String()
}

// searchDisabledNotice is the page text when global search is off. A person
// can put a "Search" link on a note, thus this is a page and not a 404. It
// names the cause and the remedy. The markup is static, and no value from the
// request goes into it.
const searchDisabledNotice = `<div class="search-page-notice">` +
	`<h2>Global search is off</h2>` +
	`<p>Searching every note at once needs an index, and the index is held in ` +
	`memory for as long as the app runs - roughly a third of the size of the ` +
	`text it covers. That is a real cost on a small device, so it is off until ` +
	`you ask for it.</p>` +
	`<p><a class="search-page-cta" href="/Config.html#cfg-search">` +
	`Turn on global search in Settings</a></p>` +
	`<p class="search-page-note">There, <em>Enable global search</em> switches ` +
	`it on and the checkboxes under it choose what gets indexed - notes and ` +
	`bookmarks to begin with. It applies immediately; no restart.</p>` +
	`<p class="search-page-note">Searching the note you have open needs none of ` +
	`this and always works: the magnifier in the page header, or ` +
	`<kbd>Ctrl</kbd>+<kbd>K</kbd>.</p>` +
	`</div>`

func renderSearchPage(v searchPageView) string {
	if v.Disabled {
		// Each other slot stays empty. There is no form, because a submit
		// comes back here. There is no result section.
		return fill(searchPageTmpl, map[string]string{
			"DISABLED": " is-disabled",
			"NOTICE":   searchDisabledNotice,
			"QUERY":    "",
			"SUMMARY":  "",
			"GROUPS":   "",
			"EMPTY":    "",
		})
	}

	var groups strings.Builder

	// Group by kind, in a fixed order, thus the page has a stable shape.
	// Inside a group, the order of the server stays.
	for _, kind := range searchKindsAll {
		var inKind []searchResult
		for _, r := range v.Results {
			if r.Kind == kind {
				inKind = append(inKind, r)
			}
		}
		if len(inKind) == 0 {
			continue
		}
		fmt.Fprintf(&groups, "<h2 class=\"search-group\">%s <span class=\"search-group-count\">%d</span></h2>\n",
			escapeHTML(searchKindLabel(kind)), len(inKind))

		for _, r := range inKind {
			groups.WriteString("<div class=\"search-result\">\n")
			title := r.Title
			if title == "" {
				title = r.Name
			}
			// The link carries the query as ?hl=. The note then marks and
			// scrolls to the match. The client removes the parameters after
			// it uses them, thus a copied URL is plain.
			fmt.Fprintf(&groups, "  <a class=\"search-result-title\" href=\"%s\">%s</a>\n",
				escapeHTML(highlightURL(r.URL, v.Highlight)), escapeHTML(title))
			fmt.Fprintf(&groups, "  <div class=\"search-result-path\">%s</div>\n", escapeHTML(r.Name))

			if len(r.Tags) > 0 {
				groups.WriteString("  <div class=\"search-result-tags\">")
				for _, t := range r.Tags {
					// Use the same pill markup and the same anchor as the
					// page header. See renderIndexPage.
					fmt.Fprintf(&groups, "<a href=\"/OMNGoTags.html#%s\" class=\"taglink\"><span class=\"tagmark\">%s</span></a>",
						escapeHTML(tagSlug(t)), escapeHTML(t))
				}
				groups.WriteString("</div>\n")
			}

			lastSection := ""
			for _, m := range r.Matches {
				// Show the section heading one time for each run of hits in
				// that section. Several matches in one bookmark are one
				// place.
				if m.Section != nil && m.Section.Label != "" && m.Section.Label != lastSection {
					lastSection = m.Section.Label
					groups.WriteString("  <div class=\"search-section\">")
					if m.Section.ID != "" {
						// r.URL already ends with the anchor of the BEST hit.
						// This link needs the anchor of THIS section, thus
						// cut the anchor off first.
						base := r.URL
						if at := strings.IndexByte(base, '#'); at >= 0 {
							base = base[:at]
						}
						fmt.Fprintf(&groups, "<a href=\"%s#%s\">%s</a>",
							escapeHTML(highlightURL(base, v.Highlight)),
							escapeHTML(m.Section.ID), escapeHTML(m.Section.Label))
					} else {
						groups.WriteString(escapeHTML(m.Section.Label))
					}
					groups.WriteString("</div>\n")
				} else if m.Section == nil {
					lastSection = ""
				}
				// Each snippet line is a link that opens the note AT that
				// line. snippetURL puts the text of the line into the href as
				// ?hlt=. The text gets percent-encoding there and an HTML
				// escape here. With LAN sharing, an attacker can control the
				// content of a note.
				fmt.Fprintf(&groups, "  <a class=\"search-snippet\" href=\"%s\">",
					escapeHTML(snippetURL(r.URL, v.Highlight, m)))
				fmt.Fprintf(&groups, "<span class=\"search-snippet-line\">%d</span>", m.Line)
				if m.Context != "" {
					where := "inside a code block"
					if m.Context == "script" {
						where = "inside a <script> block"
					}
					fmt.Fprintf(&groups, "<span class=\"search-snippet-ctx\" title=\"%s\">&lsaquo;/&rsaquo;</span>",
						escapeHTML(where))
				}
				fmt.Fprintf(&groups, "<span class=\"search-snippet-text\">%s</span>",
					renderSnippetHTML(m.Text, m.Spans))
				groups.WriteString("</a>\n")
			}
			if r.Truncated {
				groups.WriteString("  <div class=\"search-result-note\">only the first 500 KiB of this file was searched</div>\n")
			}
			groups.WriteString("</div>\n")
		}
	}

	summary := ""
	empty := ""
	switch {
	case v.Query == "":
		summary = ""
	case v.Total == 0:
		// Name the kinds that the search covered. "No results" from a setting
		// that the reader forgot is a trap.
		var kinds []string
		for _, k := range v.IndexedKinds {
			kinds = append(kinds, searchKindLabel(k))
		}
		covered := "nothing"
		if len(kinds) > 0 {
			covered = strings.Join(kinds, ", ")
		}
		empty = fmt.Sprintf(`<div class="search-empty">`+
			`<p>No matches for <strong>%s</strong>.</p>`+
			`<p class="search-empty-hint">The index currently covers: %s. `+
			`<a href="/Config.html#cfg-search">Change what is searched</a>.</p>`+
			`</div>`, escapeHTML(v.Query), escapeHTML(covered))
	default:
		word := "results"
		if v.Total == 1 {
			word = "result"
		}
		summary = fmt.Sprintf("%d %s for <strong>%s</strong>", v.Total, word, escapeHTML(v.Query))
		if v.Truncated && len(v.Results) < v.Total {
			summary += fmt.Sprintf(" <span class=\"search-page-note\">(showing the first %d)</span>", len(v.Results))
		}
	}

	return fill(searchPageTmpl, map[string]string{
		"DISABLED": "",
		"NOTICE":   "",
		"QUERY":    escapeHTML(v.Query),
		"SUMMARY":  summary,
		"GROUPS":   groups.String(),
		"EMPTY":    empty,
	})
}

// --- The wait page of the external editor ---

type externalEditView struct {
	Cmd      string
	FileName string
	ViewURL  string
}

func renderExternalEditPage(v externalEditView) string {
	return fill(externalEditTmpl, map[string]string{
		"CMD":       escapeHTML(v.Cmd),
		"FILE_NAME": escapeHTML(v.FileName),
		// ViewURL is in a JS string, inside an HTML onclick attribute. Escape
		// for JS first, and then for HTML: the inner context first.
		"VIEW_URL_ATTR_JS": escapeHTML(escapeJS(v.ViewURL)),
	})
}

// --- The runtime values of a cached page ---

// runtimeVarsMarker is the placeholder that index.html writes one time, near
// the end of <head>. It stays in the cached .html files on disk.
const runtimeVarsMarker = `<meta id="omn-go-runtime-vars-marker">`

// modalsMarker is the empty slot of the modals that need the server.
// injectRuntimeVars puts modalsHTML there when the server sends the page. An
// exported page keeps an empty div.
const modalsMarker = `<div id="omn-go-modals-slot"></div>`

// injectRuntimeVars puts the values of NOW into the runtimeVarsMarker of a
// page: APP_VERSION, USE_INTERNAL_ED, OMN_THEME, OMN_SEARCH_GLOBAL,
// OMN_INCOMING_PAGE, OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS. The page
// cache on disk keeps the marker. A person can change each setting at any
// time, and a new compile of each page would defeat the cache.
//
// The script sets data-theme on <html>. The marker is in <head>, thus the
// theme applies before the body shows, and the wrong theme never flashes. The
// CSS does the rest: "light" or "dark" fixes the colors, and "auto" or no
// attribute uses prefers-color-scheme. An exported page has no attribute.
//
// The server controls each value, and no value is user input. APP_VERSION and
// OMN_INCOMING_PAGE are constants, four values are booleans, and
// normalizeTheme and normalizeLogTags allow only known values. fmt can thus
// put them in safely.
func (a *App) injectRuntimeVars(page []byte) []byte {
	cfg := a.GetConfig()
	script := fmt.Sprintf(
		`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t; var OMN_THEME = %q; var OMN_SEARCH_GLOBAL = %t; var OMN_INCOMING_PAGE = %q; var OMN_LOG_DEBUG = %t; var OMN_LOG_INFO = %t; var OMN_LOG_TAGS = %q; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>`,
		APP_VERSION, cfg.UseInternalEd, normalizeTheme(cfg.Theme), a.globalSearchAvailable(), incomingIndexName,
		cfg.LogDebug, cfg.LogInfo, strings.Join(normalizeLogTags(cfg.LogTags), ","))
	page = bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
	// Put the modals into the slot. The editor page has no slot, and nothing
	// changes there.
	page = bytes.Replace(page, []byte(modalsMarker), []byte(modalsHTML), 1)
	return page
}
