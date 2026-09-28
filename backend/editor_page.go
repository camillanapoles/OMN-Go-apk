package backend

// --- The editor page (editor.html) ---

// editor.html loads no custom CSS or JavaScript file. A bad custom file thus
// cannot break the editor.
var editorPageTmpl = loadTemplate("editor.html")

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

// --- The wait page of the external editor ---

var externalEditTmpl = loadTemplate("external_edit.html")

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
