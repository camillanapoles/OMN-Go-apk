package backend

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

// pageHeader is the header block of a page that the server makes.
func pageHeader(title, category string) []byte {
	return []byte("Title: " + title + "\nCategory: " + category + "\n\n")
}

// renderPage writes one page in the shell of a note page: the content type,
// the status code, the compiled page and the runtime values. name is the page
// name of compilePageWithBody.
func (a *App) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	compiled := a.compilePageWithBody(name, header, body)
	writeHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(a.injectRuntimeVars(compiled))
}

// --- The page shell (index.html) ---

// index.html loads omn-go-custom.css and omn-go-custom.js last, thus a user
// rule wins at the same specificity. Keep this note out of the template,
// because the template goes to each page.
var indexPageTmpl = loadTemplate("index.html")

// modalsHTML holds the modals that need the server. injectRuntimeVars puts
// them into modalsMarker when the server sends the page, thus an exported
// page stays small.
var modalsHTML = loadTemplate("modals.html")

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

// --- The runtime values of a cached page ---

// runtimeVarsMarker is the placeholder that index.html writes one time, near
// the end of <head>. It stays in the cached .html files on disk.
const runtimeVarsMarker = `<meta id="omn-go-runtime-vars-marker">`

// modalsMarker is the empty slot of the modals that need the server.
// injectRuntimeVars puts modalsHTML there when the server sends the page. An
// exported page keeps an empty div.
const modalsMarker = `<div id="omn-go-modals-slot"></div>`

// pageFacts gives injectRuntimeVars two values of other groups. connectGroups
// sets them, thus the page shell names no search or exchange code. A nil
// searchGlobal gives false.
type pageFacts struct {
	searchGlobal func() bool // OMN_SEARCH_GLOBAL
	incomingPage string      // OMN_INCOMING_PAGE
}

// injectRuntimeVars puts the values of NOW into the runtimeVarsMarker of a
// page: APP_VERSION, USE_INTERNAL_ED, OMN_THEME, OMN_SEARCH_GLOBAL,
// OMN_INCOMING_PAGE, OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS. The cache
// on disk keeps the marker, thus a change of a setting needs no new compile.
//
// The marker is in <head>, thus data-theme applies before the body shows. The
// server controls each value, and normalizeTheme and normalizeLogTags allow
// only known values, thus fmt can put them in.
func (a *App) injectRuntimeVars(page []byte) []byte {
	cfg := a.config.get()
	searchGlobal := a.pages.searchGlobal != nil && a.pages.searchGlobal()
	script := fmt.Sprintf(
		`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t; var OMN_THEME = %q; var OMN_SEARCH_GLOBAL = %t; var OMN_INCOMING_PAGE = %q; var OMN_LOG_DEBUG = %t; var OMN_LOG_INFO = %t; var OMN_LOG_TAGS = %q; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>`,
		APP_VERSION, cfg.UseInternalEd, normalizeTheme(cfg.Theme), searchGlobal, a.pages.incomingPage,
		cfg.LogDebug, cfg.LogInfo, strings.Join(normalizeLogTags(cfg.LogTags), ","))
	page = bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
	// Put the modals into the slot. The editor page has no slot, and nothing
	// changes there.
	page = bytes.Replace(page, []byte(modalsMarker), []byte(modalsHTML), 1)
	return page
}

// writeHTMLHeader is the ONE place that sets the type of a page, with the
// charset: a page that the server renders has no <meta charset>.
// pageCacheWriter in middleware.go reads the prefix "text/html". See
// TestConnectionMiddlewareUsesNoStoreForAPage.
func writeHTMLHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", htmlContentType)
}

const htmlContentType = "text/html; charset=utf-8"
