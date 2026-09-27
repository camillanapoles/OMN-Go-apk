package backend

import (
	"bytes"
	"fmt"
	"log"
	"strings"
)

// ----------------------------------------------------------------------
// Why this file does NOT use html/template
// ----------------------------------------------------------------------
//
// html/template calls reflect.Value.MethodByName, and that stops the
// dead-code elimination of methods. See
// doc/decisions/0008-render-the-pages-without-html-template.md. Each render
// function here escapes each value for its place:
//
//	escapeHTML(v)            HTML text, or a quoted HTML attribute.
//	escapeJS(v)              A quoted JS string in an inline <script>.
//	escapeHTML(escapeJS(v))  A JS string inside an HTML attribute.
//	trusted HTML             The markdown body or a fragment from here.
//	                         It never gets a second escape.

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

// loadTemplate reads one page fragment from templatesFS. That embed stays
// separate from staticFS, because the app extracts staticFS as files that a
// person can edit. A missing file shows at the first render.
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
	// index.html loads omn-go-custom.css and omn-go-custom.js last, thus a
	// user rule wins at the same specificity. editor.html loads neither, thus
	// a bad custom file never locks the user out of the editor. Keep these
	// notes out of the template, because it goes to each page.
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
	// modalsHTML holds the modals that need the server. injectRuntimeVars
	// puts them into modalsMarker when the server sends the page, thus an
	// exported page stays small.
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
		// Each pill links to OMNGoTags through AssetPrefix, thus it works at
		// each depth and through file://. The fragment is tagSlug(t), the
		// same as the section ids of tags.go. AssetPrefix holds only '.' and
		// '/'.
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

// editorPageView holds each value of renderEditorPage. The text of the note
// is absent on purpose: the editor fetches it from /api/note.
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

// safeLocalPath reports whether s can be an href: a path on this server, with
// no scheme and no "//host". A request header thus cannot become a live link
// out of the app.
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
// OMN_INCOMING_PAGE, OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS. The cache
// on disk keeps the marker, thus a change of a setting needs no new compile.
//
// The marker is in <head>, thus data-theme applies before the body shows. The
// server controls each value, and normalizeTheme and normalizeLogTags allow
// only known values, thus fmt can put them in.
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
