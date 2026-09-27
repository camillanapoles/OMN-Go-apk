package backend

import "net/http"

// systemPage is one row of the page-access table: the address, the handler
// and the role that a caller on another machine needs. A refused caller gets
// a page, because a person can open the address from a link.
type systemPage struct {
	path  string
	title string // the title of the refusal page
	admin bool   // a caller on another machine needs the admin role
	serve http.HandlerFunc
}

func (a *App) systemPages() []systemPage {
	return []systemPage{
		{"/Config.html", "Config", true, a.serveConfigPage},
		{"/OMNGoTags.html", "Tags", false, a.serveTagsPage},
		{"/OMNGoSearch.html", "Search", false, a.serveSearchPage},
		{"/OMNGoFiles.html", "Files", true, a.serveFilesPage},
		{"/OMNGoStatus.html", "Status", true, a.serveStatusPage},
		{"/OMNGoLogs.html", "Log", true, a.serveLogsPage},
		{"/db_backups", "Database Backups", true, a.serveDBBackupsPage},
	}
}

// pageHandler adds the role check of one row to its handler.
func (a *App) pageHandler(p systemPage) http.HandlerFunc {
	if !p.admin {
		return p.serve
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.hasRole(r) {
			a.serveRefusalPage(w, p.title)
			return
		}
		p.serve(w, r)
	}
}

// serveRefusalPage tells a caller without the role how to get it.
func (a *App) serveRefusalPage(w http.ResponseWriter, title string) {
	body := `<div class="config-panel">` +
		`<h2 class="config-title">` + title + `</h2>` +
		`<p class="config-hint">This page is for the admin of this device. ` +
		`Log in as admin on a note page, then open the page again.</p>` +
		`</div>`
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars(compiled))
}
