package search

// ----------------------------------------------------------------------
// Search: the query layer
// ----------------------------------------------------------------------
//
// PAGE search (scope=page) reads the open note and keeps nothing. GLOBAL
// search (scope=all) uses the index of index.go, and it needs
// search_enabled. Both use the matcher of package textmatch and the same
// response shape.

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"net.basov.omngo/backend/internal/textmatch"
)

// These are the limits and defaults of one request.
const (
	searchDefaultSnippets = 3
	searchMaxSnippets     = 10
	searchDefaultLimit    = 50
	searchMaxLimit        = 200

	// maxIndexFileBytes limits the search of one file to the first 500 KiB,
	// cut at a line end. The search does NOT skip a larger file. It marks the
	// result as truncated.
	maxIndexFileBytes = 500 << 10

	// snippetMaxRunes and snippetLead shape a snippet around its first hit.
	snippetMaxRunes = 160
	snippetLead     = 60
)

// The field weights are times 10, thus the arithmetic uses integers. A title
// hit is worth three content hits. The tests of this file hold the exact
// numbers.
const (
	weightTitle   = 30
	weightTags    = 25
	weightPath    = 20
	weightHeader  = 15
	weightContent = 10
)

// The kind weights are times 100. A note wins against a JSON file at an equal
// score.
var kindWeight = map[string]int{
	"md":        100,
	"bookmarks": 100,
	"json":      90,
	"user_json": 90,
	"js":        85,
}

// queryTerm is one folded term of the query, with its character mask and its
// field.
type queryTerm struct {
	runes []rune
	mask  uint64
	field string // "" = any field; otherwise "title", "tag", "path"

	// raw is the term as typed, without the field prefix. Only the highlight
	// reads it, because the client marks the literal text: the folded "еж"
	// marks nothing on a page that says "ёж".
	raw string
}

// parsedQuery holds the terms that must ALL match, and the kind filter of a
// "kind:" prefix.
type parsedQuery struct {
	terms []queryTerm
	kinds []string
}

// parseQuery splits a raw query into terms and reads the field prefixes.
// "tag:hydro title:manual json" limits the first two terms. An unknown prefix
// is NOT a prefix, thus "http://x" stays a search for "http://x".
func parseQuery(q string) parsedQuery {
	var out parsedQuery
	for _, f := range strings.Fields(q) {
		field := ""
		if k, v, found := strings.Cut(f, ":"); found && v != "" {
			switch strings.ToLower(k) {
			case "title", "tag", "tags", "path", "name":
				field = normalizeQueryField(strings.ToLower(k))
				f = v
			case "kind":
				for _, kind := range strings.Split(v, ",") {
					if kind = strings.TrimSpace(strings.ToLower(kind)); kind != "" {
						out.kinds = append(out.kinds, kind)
					}
				}
				continue
			}
		}
		runes := textmatch.Fold(f)
		if len(runes) == 0 {
			continue
		}
		out.terms = append(out.terms, queryTerm{
			runes: runes, mask: textmatch.RuneMask(runes), field: field, raw: f,
		})
	}
	return out
}

// highlightMinRunes is the same as OMN_HL_MIN in omn-go-core.js. A term of
// one character marks half the page, thus both ends drop it.
const highlightMinRunes = 2

// highlightTerms answers the terms that an opened result marks: as typed,
// without prefixes, with no duplicate. They are text and not spans, because a
// span points into the SOURCE, and the page shows the RENDERED text.
func highlightTerms(q parsedQuery) []string {
	var out []string
	seen := make(map[string]bool, len(q.terms))
	for _, t := range q.terms {
		if utf8.RuneCountInString(t.raw) < highlightMinRunes {
			continue
		}
		key := strings.ToLower(t.raw)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t.raw)
	}
	return out
}

// highlightURL adds the terms to a document URL: /Note.html?hl=fetch&hl=json.
// Each term gets its own parameter, because a term can hold a comma.
func highlightURL(base string, terms []string) string {
	if base == "" || len(terms) == 0 {
		return base
	}
	// A result URL can already hold a section anchor, and the query string
	// goes BEFORE the fragment: "/Bookmarks.html?hl=cats#2026-06-15-200000".
	// Behind the '#', "?hl=" would become part of the id.
	frag := ""
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base, frag = base[:i], base[i:]
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	var b strings.Builder
	b.WriteString(base)
	for _, t := range terms {
		b.WriteString(sep)
		sep = "&"
		b.WriteString("hl=")
		b.WriteString(url.QueryEscape(t))
	}
	b.WriteString(frag)
	return b.String()
}

// snippetURL makes the link for ONE matching line. It holds the terms as
// ?hl=, the text of the line as ?hlt=, and the section as the fragment. The
// link carries the TEXT, because a source line number does not match the
// compiled HTML. omnMarkNear in omn-go-core.js finds the text. The fragment
// is the section of THIS line, not of the best hit.
func snippetURL(base string, terms []string, m searchMatch) string {
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base = base[:i]
	}
	frag := ""
	if m.Section != nil && m.Section.ID != "" {
		frag = "#" + m.Section.ID
	}
	out := highlightURL(base, terms)
	// The page does not show a hit inside a <script> block. Such a line keeps
	// its terms and its section, and nothing more.
	if m.Text == "" || m.Context == "script" || len(terms) == 0 {
		return out + frag
	}
	sep := "?"
	if strings.Contains(out, "?") {
		sep = "&"
	}
	return out + sep + "hlt=" + url.QueryEscape(m.Text) + frag
}

func normalizeQueryField(k string) string {
	switch k {
	case "tags":
		return "tag"
	case "name":
		return "path"
	default:
		return k
	}
}

// readCapped reads at most max bytes, cut at the last complete line, thus no
// snippet is half a line. truncated tells whether the file had more.
func readCapped(p string, max int) (data []byte, truncated bool, err error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	// Read max+1 bytes, thus a file of exactly max bytes is not truncated.
	data, err = io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) <= max {
		return data, false, nil
	}
	cut := data[:max]
	if nl := bytes.LastIndexByte(cut, '\n'); nl > 0 {
		cut = cut[:nl]
	}
	return cut, true, nil
}

// isBinary rejects a file with a NUL byte in its first 8 KiB. The test is
// cheap, and it is wrong only for text that does not belong in a notes
// directory.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8<<10 {
		n = 8 << 10
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// assetKind maps a storage-relative html/ path to its search kind.
func assetKind(rel string) string {
	switch {
	case strings.HasPrefix(rel, "js/"):
		return "js"
	case strings.HasPrefix(rel, "user_json/"):
		return "user_json"
	case strings.HasPrefix(rel, "json/"):
		return "json"
	default:
		return "asset"
	}
}

// kindAllowed applies the kind filters: the query parameter and a "kind:"
// prefix in the query. An empty filter allows each kind.
func kindAllowed(kind string, filters ...[]string) bool {
	for _, f := range filters {
		if len(f) == 0 {
			continue
		}
		found := false
		for _, k := range f {
			if k == kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
