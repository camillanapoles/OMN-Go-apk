package backend

import (
	"fmt"
	"strings"
)

// --- The search result page (search_page.html) ---

// searchPageView holds each value of renderSearchPage. Query comes from a
// URL, and the function escapes it. renderSnippetHTML escapes the snippets.
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

// renderSnippetHTML puts <mark> around each span. The spans are RUNE offsets,
// thus it walks the text as []rune. It escapes each segment.
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

// searchDisabledNotice is the page text when global search is off. A note can
// link here, thus it is a page and not a 404. Its markup is static.
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
				// Each snippet line opens the note AT that line. The text of
				// the line gets percent-encoding in snippetURL and an HTML
				// escape here, because the text of a note can come from
				// another person.
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
