package render

// --- The editor page (editor.html) ---

// editor.html loads no custom CSS or JavaScript file. A bad custom file thus
// cannot break the editor.
var EditorPageTmpl = LoadTemplate("editor.html")

// EditorPageView holds each value of RenderEditorPage. The text of the note
// is absent on purpose: the editor fetches it from /api/note.
type EditorPageView struct {
	Title   string // display name (page/asset)
	Name    string // value for /api/note and /api/save
	PageExt string // e.g. ".md", ".js" (informational)
	ViewURL string // where to return after save/cancel
}

func RenderEditorPage(v EditorPageView) string {
	return Fill(EditorPageTmpl, map[string]string{
		"TITLE_HTML":  EscapeHTML(v.Title),
		"NAME_JS":     EscapeJS(v.Name),
		"PAGE_EXT_JS": EscapeJS(v.PageExt),
		// Only the JavaScript reads this, as OMN_EDIT_VIEW. The × button of
		// omn-go-editor.js goes to it.
		"VIEW_URL_JS": EscapeJS(v.ViewURL),
	})
}

// --- The wait page of the external editor ---

var externalEditTmpl = LoadTemplate("external_edit.html")

type ExternalEditView struct {
	Cmd      string
	FileName string
	ViewURL  string
}

func RenderExternalEditPage(v ExternalEditView) string {
	return Fill(externalEditTmpl, map[string]string{
		"CMD":       EscapeHTML(v.Cmd),
		"FILE_NAME": EscapeHTML(v.FileName),
		// ViewURL is in a JS string, inside an HTML onclick attribute. Escape
		// for JS first, and then for HTML: the inner context first.
		"VIEW_URL_ATTR_JS": EscapeHTML(EscapeJS(v.ViewURL)),
	})
}
