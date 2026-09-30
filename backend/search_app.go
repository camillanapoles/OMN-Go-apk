package backend

import (
	"net/http"

	"net.basov.omngo/backend/internal/search"
)

// The App side of the search package. searchService gives the package the
// index of the App, the storage layout, a copy of the settings, the loggers
// and the page shell.

// searchService answers the search feature of the App for one call.
func (a *App) searchService() search.Service {
	return search.Service{
		Index:      a.search,
		Layout:     a.layout(),
		Config:     a.config.Get(),
		Log:        a.log,
		RenderPage: a.renderPage,
	}
}

// globalSearchAvailable is the OMN_SEARCH_GLOBAL value of each page. It reads
// only the index and the settings, thus it makes no loggers for each page.
func (a *App) globalSearchAvailable() bool {
	return search.Service{Index: a.search, Config: a.config.Get()}.GlobalAvailable()
}

// markSearchIndexDirty is the hook onPageWritten. See search.Index.MarkDirty.
func (a *App) markSearchIndexDirty() { a.search.MarkDirty() }

// rebuildSearchIndex builds the index again. See search.Service.RebuildIndex.
func (a *App) rebuildSearchIndex() { a.searchService().RebuildIndex() }

// dropSearchIndex releases the index. See search.Service.DropIndex.
func (a *App) dropSearchIndex() { a.searchService().DropIndex() }

// searchIndexStatus is the line of the Config page. See
// search.Service.IndexStatus.
func (a *App) searchIndexStatus() string { return a.searchService().IndexStatus() }

// handleSearch answers GET /api/search. See search.Service.HandleSearch.
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	a.searchService().HandleSearch(w, r)
}

// serveSearchPage sends the Search page. See
// search.Service.ServeSearchPage.
func (a *App) serveSearchPage(w http.ResponseWriter, r *http.Request) {
	a.searchService().ServeSearchPage(w, r)
}
