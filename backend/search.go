package backend

// ----------------------------------------------------------------------
// Search: the query layer
// ----------------------------------------------------------------------
//
// This file changes a query into results. It holds TWO searches that share
// the matcher of search_match.go:
//
//	PAGE search (scope=page)   The open note alone. It reads one file,
//	                           scores it, and keeps nothing.
//	GLOBAL search (scope=all)  Each file, through the index of
//	                           search_index.go. It needs search_enabled.
//
// Both use the same scoring and the same response shape, thus a result means
// the same thing in each scope. Page search has no setting, because it has no
// cost when nobody uses it.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

	// raw is the term as the user typed it, without the field prefix. Only
	// the highlight reads it. The client marks the LITERAL text, and the fold
	// maps 'ё' to 'е'. The folded "еж" would thus mark nothing on a page that
	// says "ёж".
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
		runes := fold(f)
		if len(runes) == 0 {
			continue
		}
		out.terms = append(out.terms, queryTerm{
			runes: runes, mask: runeMask(runes), field: field, raw: f,
		})
	}
	return out
}

// highlightMinRunes is the same as OMN_HL_MIN in omn-go-core.js. A term of
// one character marks half the page, thus both ends drop it.
const highlightMinRunes = 2

// highlightTerms answers the terms that an opened result marks: the terms as
// typed, without field prefixes, with no duplicate. These are not the spans
// of the matcher. A span is an offset in the SOURCE, and the page shows the
// RENDERED text. "**fetch** the json" renders as "fetch the json". A term
// that only matched fuzzily finds nothing, and the page marks nothing. The
// result list already showed the matching lines.
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
// Each term gets its own parameter, because a term can hold a comma. The
// client removes the parameters from the address bar, thus a copied URL is
// plain.
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

// snippetURL makes the link for ONE matching line. The link holds the query
// terms as ?hl=, the text of the line as ?hlt=, and the section of the line
// as the fragment.
//
// ?hlt= points the link at THIS line. The line number cannot do that. It
// counts lines of the markdown SOURCE, and the page shows compiled HTML
// without the <script> blocks and the link URLs. omnMarkNear in
// omn-go-core.js thus finds the TEXT. The snippet has at most snippetMaxRunes
// runes, thus the URL stays short.
//
// The fragment is the section of THIS line. base ends with the anchor of the
// BEST hit, and that is another line. A section with no id gives no fragment.
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

// docField is one short weighted field: a title, a tag or the path.
type docField struct {
	name   string
	text   []rune // folded
	weight int    // x10, see the weight constants
}

// docLine is one content line. It holds the line number in the file, the raw
// text for the snippet, the folded text for the matcher, and the mask. The
// mask rejects most lines with no look at the text.
type docLine struct {
	no      int
	raw     string
	fold    []rune
	mask    uint64
	context string // "", "code" or "script" - see classifyContexts
}

// searchDocument is one searchable thing. Page search makes one for each
// request. The index makes one for each file and keeps a reduced form.
type searchDocument struct {
	Path      string // storage-relative, slash form ("md/Test/OMN-Go/Fetch.md")
	Kind      string
	Name      string // page name for md, file path for an asset
	Title     string
	Tags      []string
	URL       string
	fields    []docField
	lines     []docLine
	sections  []docSection // may be nil: a flat document has no parts
	truncated bool
}

// loadPageDocument reads the one file of a page query. name is what the page
// shows, for example "Note", "Note.html", "Note.md" or "js/thing.js".
// resolvePageName decides what it means. The function answers nil, and no
// error, for a file that does not exist or that is outside the storage
// directory. A query thus cannot probe the file system.
func (a *App) loadPageDocument(name string) (*searchDocument, error) {
	if name == "" {
		return nil, nil
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	filePath := htmlPath
	if isPage {
		filePath = mdPath
	}
	if !a.withinStorage(filePath) {
		return nil, nil
	}

	data, truncated, err := readCapped(filePath, maxIndexFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if isBinary(data) {
		return nil, nil
	}

	if isPage {
		return newMarkdownDocument(baseName, string(data), truncated), nil
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(name)), "/")
	return newAssetDocument(rel, string(data), truncated), nil
}

// newMarkdownDocument makes the search form of a note. Page search and the
// index both call it, thus both see the same fields, the same line numbers
// and the same context labels.
func newMarkdownDocument(baseName, content string, truncated bool) *searchDocument {
	doc := &searchDocument{
		Path:      "md/" + baseName + ".md",
		Kind:      SearchKindMD,
		Name:      baseName,
		URL:       "/" + baseName + ".html",
		truncated: truncated,
	}
	// Page search and the index must agree that Bookmarks.html is a bookmarks
	// document, thus the decision is here.
	if baseName == bookmarksNote {
		doc.Kind = SearchKindBookmarks
	}
	doc.parseMarkdown(content)
	return doc
}

// newAssetDocument is the same for a file with no header block, for example a
// script or a JSON file. rel is the path below html/, for example
// "js/mine.js".
func newAssetDocument(rel, content string, truncated bool) *searchDocument {
	doc := &searchDocument{
		Path:      "html/" + rel,
		Kind:      assetKind(rel),
		Name:      rel,
		URL:       "/" + rel,
		truncated: truncated,
	}
	doc.parsePlain(content)
	return doc
}

// parseMarkdown fills a document from the source of a note. The header block
// gives weighted fields, and only the BODY gives content lines. A header
// "Category: Notes" thus never gives a content hit. The line numbers count
// the header too, as the file shows them.
func (d *searchDocument) parseMarkdown(content string) {
	hb := parseHeaderBlock(content)
	title, tags := extractTitleTags(content)
	if title == "" {
		title = d.Name
	}
	d.Title = title
	d.Tags = tags

	d.fields = append(d.fields,
		docField{name: "title", text: fold(title), weight: weightTitle},
		docField{name: "path", text: fold(d.Name), weight: weightPath})
	for _, t := range tags {
		d.fields = append(d.fields, docField{name: "tag", text: fold(t), weight: weightTags})
	}
	if hb.HasHeader {
		for _, h := range strings.Split(hb.Header, "\n") {
			k, v, found := strings.Cut(h, ":")
			if !found {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "title", "tags":
				continue // already weighted above, at their own weights
			}
			if v = strings.TrimSpace(v); v != "" {
				d.fields = append(d.fields, docField{
					name: strings.ToLower(strings.TrimSpace(k)), text: fold(v), weight: weightHeader})
			}
		}
	}

	// The body line numbers continue after the header.
	firstBodyLine := 1 + strings.Count(content[:hb.BodyOffset], "\n")

	if d.Kind == SearchKindBookmarks {
		d.addBookmarks(hb.Body, firstBodyLine)
		return
	}
	raw, contexts := d.addLines(hb.Body, firstBodyLine)
	d.sections = sectionsFromHeadings(raw, contexts, firstBodyLine)
}

// parsePlain fills a document from a file with no header block. Each line is
// content, and the path is the only field.
func (d *searchDocument) parsePlain(content string) {
	d.Title = path.Base(d.Name)
	d.fields = append(d.fields,
		docField{name: "path", text: fold(d.Name), weight: weightPath})
	d.addLines(content, 1)
}

// addLines answers the split lines and their context labels.
// sectionsFromHeadings reads them, and it does not split the body a second
// time.
func (d *searchDocument) addLines(content string, firstLineNo int) ([]string, []string) {
	raw := strings.Split(content, "\n")
	contexts := classifyContexts(raw)
	for i, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue // an empty line can never match, and costs memory to keep
		}
		f := fold(line)
		d.lines = append(d.lines, docLine{
			no:      firstLineNo + i,
			raw:     line,
			fold:    f,
			mask:    runeMask(f),
			context: contexts[i],
		})
	}
	return raw, contexts
}

// classifyContexts marks each line as prose, "code" or "script". A hit in the
// JavaScript of a note is a different answer from a hit in prose, and the
// result list shows the difference. The mark does not lower the score,
// because a person can search FOR code.
//
// The fence state wins over the tag state. A "<script>" inside a fenced
// example must not mark the rest of the file. markdown.go has the same rule
// for the renderer. The marks are for each line, thus an inline `code` span
// gets no mark.
func classifyContexts(lines []string) []string {
	out := make([]string, len(lines))
	inFence, inScript, inPre := false, false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			out[i] = "code"
			inFence = !inFence
			continue
		}
		if inFence {
			out[i] = "code"
			continue
		}
		if inScript {
			out[i] = "script"
			if strings.Contains(lower, "</script>") {
				inScript = false
			}
			continue
		}
		if inPre {
			out[i] = "code"
			if strings.Contains(lower, "</pre>") {
				inPre = false
			}
			continue
		}
		if strings.Contains(lower, "<script") {
			out[i] = "script"
			if !strings.Contains(lower, "</script>") {
				inScript = true
			}
			continue
		}
		if strings.Contains(lower, "<pre") {
			out[i] = "code"
			if !strings.Contains(lower, "</pre>") {
				inPre = true
			}
			continue
		}
	}
	return out
}

// lineHit is one matching content line, with the spans of each term merged.
type lineHit struct {
	line  *docLine
	score int
	tier  matchTier
	spans []span
}

// scoreDocument applies AND: each term must hit the document, or the document
// is not a result. For each term, the best tier and weighted score of all
// fields and lines wins. See betterMatch.
//
// THE PHRASE RUNG. A document that holds the whole query, in order and side
// by side, gets tierPhrase. It then ranks above each other document, whatever
// the sums say. The score of a document is a SUM over the terms, and a field
// has a weight. Five loose query words in one title scored 2001, and the note
// with the sentence scored 718. A bonus that closes that gap is too large for
// the next query. TestPhraseTierBeatsAHigherScore holds the rule.
//
// The check costs almost nothing. The loop counts the DISTINCT terms that hit
// each line and each field. Only a line with each term can hold the phrase,
// thus the substring test runs on few lines. The measured time was 0.94 to
// 1.05 of the loop without the count.
//
// The rule is narrow. The words must be side by side, with one space between
// them, in the folded text. A query with a field prefix is never a phrase.
func scoreDocument(q parsedQuery, d *searchDocument) (int, matchTier, []lineHit, bool) {
	if len(q.terms) == 0 {
		return 0, tierNone, nil, false
	}

	total := 0
	worst := tierSubstring // the document's tier is its WEAKEST term's tier
	hits := map[int]*lineHit{}

	// The phrase counters are a slice, because a map hash for each hit costs
	// a third of the scoring time.
	phraseWanted := len(q.terms) > 1
	for _, term := range q.terms {
		if term.field != "" {
			phraseWanted = false
			break
		}
	}
	var perLine, perField []uint16
	if phraseWanted {
		perLine = make([]uint16, len(d.lines))
		perField = make([]uint16, len(d.fields))
	}

	for _, term := range q.terms {
		bestScore, bestTier := 0, tierNone

		for fi, f := range d.fields {
			if term.field != "" && term.field != f.name {
				continue
			}
			s, _, tier, ok := scoreTerm(term.runes, f.text)
			if !ok {
				continue
			}
			if phraseWanted && tier == tierSubstring {
				perField[fi]++
			}
			weighted := s * f.weight / 10
			if betterMatch(tier, weighted, bestTier, bestScore) {
				bestScore, bestTier = weighted, tier
			}
		}

		if term.field == "" {
			for i := range d.lines {
				ln := &d.lines[i]
				if maskRejects(term.mask, ln.mask) {
					continue // no rune loop, no allocation
				}
				s, spans, tier, ok := scoreTerm(term.runes, ln.fold)
				if !ok {
					continue
				}
				if phraseWanted && tier == tierSubstring {
					perLine[i]++
				}
				weighted := s * weightContent / 10
				if betterMatch(tier, weighted, bestTier, bestScore) {
					bestScore, bestTier = weighted, tier
				}
				h := hits[ln.no]
				if h == nil {
					h = &lineHit{line: ln}
					hits[ln.no] = h
				}
				h.spans = append(h.spans, spans...)
				// SUM the distinct terms of a line, and do not keep the best
				// one. The line with each term of the query is the line to
				// show, also when another line matches one term better.
				// "await fetch('/json/test.json')" wins against a heading
				// "fetch". Each term adds to a line one time.
				h.score += weighted
				if tier > h.tier {
					h.tier = tier // a line is only as good as its weakest term
				}
			}
		}

		if bestTier == tierNone && term.field == "" {
			// Nothing matched as a substring or a subsequence. Try the typo
			// rung before the code drops the document. It finds "fetch" for
			// "fecth". It cannot use the mask, thus it runs last.
			if s, spans, ok := scoreTypoInDocument(term.runes, d); ok {
				bestScore, bestTier = s.score, tierTypo
				for _, lh := range spans {
					h := hits[lh.line.no]
					if h == nil {
						h = &lineHit{line: lh.line}
						hits[lh.line.no] = h
					}
					h.spans = append(h.spans, lh.spans...)
					h.score += lh.score
					if lh.tier > h.tier {
						h.tier = lh.tier
					}
				}
			}
		}
		if bestTier == tierNone {
			return 0, tierNone, nil, false // AND: one miss drops the document
		}
		if bestTier > worst {
			worst = bestTier
		}
		total += bestScore
	}

	// Check the phrase now. Only a field or a line that took EACH term can
	// hold it, thus the check reads nothing else.
	if phraseWanted {
		full := uint16(len(q.terms))
		want := queryPhrase(q)
		found := false
		for fi, n := range perField {
			if n == full {
				if _, _, ok := scoreSubstring(want, d.fields[fi].text); ok {
					found = true
					break
				}
			}
		}
		for i, n := range perLine {
			if n != full {
				continue
			}
			if _, _, ok := scoreSubstring(want, d.lines[i].fold); ok {
				found = true
				// The line takes the rung too, thus the panel shows the line
				// that the reader typed first.
				if h := hits[d.lines[i].no]; h != nil {
					h.tier = tierPhrase
				}
			}
		}
		if found {
			worst = tierPhrase
		}
	}

	kw, ok := kindWeight[d.Kind]
	if !ok {
		kw = 100
	}
	total = total * kw / 100

	ordered := make([]lineHit, 0, len(hits))
	for _, h := range hits {
		h.spans = mergeSpans(h.spans)
		ordered = append(ordered, *h)
	}
	sortLineHits(ordered)
	return total, worst, ordered, true
}

// queryPhrase joins the folded terms with one space. The result compares with
// the folded text of a field or a line.
func queryPhrase(q parsedQuery) []rune {
	out := make([]rune, 0, 32)
	for i, t := range q.terms {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, t.runes...)
	}
	return out
}

// typoResult is the best token match of a term in a document.
type typoResult struct {
	score int
	token string
}

// scoreTypoInDocument runs the edit-distance rung over the tokens of the
// document. It makes the tokens here, because a token dictionary in the index
// grows with the vocabulary. See the banner of search_index.go. The trigram
// signature first narrows the candidates to few documents.
//
// The spans come from a substring scan for the matched TOKEN. The token is
// the real text, thus the highlight is correct. A highlight of the misspelled
// query would not be.
func scoreTypoInDocument(term []rune, d *searchDocument) (typoResult, []lineHit, bool) {
	if typoBudget(len(term)) == 0 {
		return typoResult{}, nil, false
	}

	best := typoResult{}
	bestWeight := 0
	seen := map[string]bool{}

	consider := func(text string, weight int) {
		for _, tok := range tokenize(text) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			s, _, ok := scoreTypo(term, []rune(tok))
			if !ok {
				continue
			}
			if weighted := s * weight / 10; weighted > bestWeight {
				bestWeight = weighted
				best = typoResult{score: weighted, token: tok}
			}
		}
	}

	for _, f := range d.fields {
		consider(string(f.text), f.weight)
	}
	for i := range d.lines {
		consider(d.lines[i].raw, weightContent)
	}
	if best.token == "" {
		return typoResult{}, nil, false
	}

	var hits []lineHit
	needle := []rune(best.token)
	for i := range d.lines {
		ln := &d.lines[i]
		if _, spans, ok := scoreSubstring(needle, ln.fold); ok {
			hits = append(hits, lineHit{line: ln, score: best.score, tier: tierTypo, spans: spans})
		}
	}
	return best, hits, true
}

// sortLineHits orders by tier and score, and then by line number. The order
// is thus stable, and a tie reads from top to bottom.
func sortLineHits(hits []lineHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j], hits[j-1]
			if betterMatch(a.tier, a.score, b.tier, b.score) ||
				(a.tier == b.tier && a.score == b.score && a.line.no < b.line.no) {
				hits[j], hits[j-1] = hits[j-1], hits[j]
				continue
			}
			break
		}
	}
}

// snippetFor cuts a line to fit a result row, and it moves the spans to
// match. It removes the white space at each end. When the rest is too long,
// it takes a window around the first hit and adds an ellipsis. It drops a
// span outside the window, because a span that does not cover its match is
// worse than none.
func snippetFor(raw string, spans []span) (string, []span) {
	runes := []rune(raw)

	lead := 0
	for lead < len(runes) && isSpace(runes[lead]) {
		lead++
	}
	end := len(runes)
	for end > lead && isSpace(runes[end-1]) {
		end--
	}
	runes = runes[lead:end]
	shifted := make([]span, 0, len(spans))
	for _, s := range spans {
		s.Start -= lead
		if s.Start >= 0 && s.Start+s.Len <= len(runes) {
			shifted = append(shifted, s)
		}
	}

	if len(runes) <= snippetMaxRunes {
		return string(runes), shifted
	}

	start := 0
	if len(shifted) > 0 && shifted[0].Start > snippetLead {
		start = shifted[0].Start - snippetLead
	}
	stop := start + snippetMaxRunes
	if stop > len(runes) {
		stop = len(runes)
		if start = stop - snippetMaxRunes; start < 0 {
			start = 0
		}
	}

	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if stop < len(runes) {
		suffix = "…"
	}

	out := make([]span, 0, len(shifted))
	for _, s := range shifted {
		if s.Start < start || s.Start+s.Len > stop {
			continue
		}
		s.Start += len([]rune(prefix)) - start
		out = append(out, s)
	}
	return prefix + string(runes[start:stop]) + suffix, out
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
}

// Only GLOBAL search has a setting. Page search has no cost when nobody uses
// it. A switch that saves nothing only gives one more way to break the
// feature.

// globalSearchAvailable answers "can this server search each file now": the
// user asked for it AND an index exists. injectRuntimeVars gives it to each
// page as OMN_SEARCH_GLOBAL, thus the dialog never offers a scope that fails.
func (a *App) globalSearchAvailable() bool {
	return a.GetConfig().SearchEnabled && a.searchIndexBuilt()
}

// defaultSearchScope is the scope of a request without one. It follows the
// setting, but it falls back to page scope when global search cannot answer.
// A request with no preference must not get a scope that fails.
func (a *App) defaultSearchScope() string {
	if a.globalSearchAvailable() {
		return normalizeSearchScope(a.GetConfig().SearchScope)
	}
	return SearchScopePage
}

// searchMatch is one snippet of the response. Spans are [start, len] pairs in
// RUNE offsets into Text. See the banner of search_match.go.
type searchMatch struct {
	Line    int            `json:"line"`
	Context string         `json:"context,omitempty"`
	Section *searchSection `json:"section,omitempty"`
	Text    string         `json:"text"`
	Spans   [][2]int       `json:"spans"`
}

// searchSection names the part of a document that a hit is in: a bookmark
// entry, a quick note, or the section of a heading. It is absent for a flat
// document, and for a hit above the first heading. ID is the anchor in the
// compiled HTML, and it can be empty while Label is not. The UI always shows
// the label, and it makes a link only for an id. See search_sections.go.
type searchSection struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
}

type searchResult struct {
	Path      string        `json:"path"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name"`
	Title     string        `json:"title"`
	Tags      []string      `json:"tags,omitempty"`
	Score     int           `json:"score"`
	URL       string        `json:"url"`
	Truncated bool          `json:"truncated,omitempty"`
	Matches   []searchMatch `json:"matches,omitempty"`
}

type searchResponse struct {
	Query     string         `json:"query"`
	Scope     string         `json:"scope"`
	TookMS    int64          `json:"took_ms"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated"`
	Results   []searchResult `json:"results"`
	Status    string         `json:"status,omitempty"`
	Error     string         `json:"error,omitempty"`

	// Highlight holds the terms of the query as typed. The client marks them
	// in an opened result. The terms belong to the QUESTION, thus they are
	// here one time and not on each result.
	Highlight []string `json:"highlight,omitempty"`
}

// handleSearch answers GET /api/search. It has NO authMiddleware, the same as
// /api/note and each page route. See doc/API.md. A search shows nothing that
// a LAN guest cannot fetch file by file. A login would thus protect nothing,
// and it would stop the guest.
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	qs := r.URL.Query()

	scope := strings.ToLower(strings.TrimSpace(qs.Get("scope")))
	if scope == "" {
		scope = a.defaultSearchScope()
	}

	resp := searchResponse{
		Query:   qs.Get("q"),
		Scope:   scope,
		Results: []searchResult{},
	}

	switch scope {
	case SearchScopePage:
		a.searchPage(&resp, qs)
	case SearchScopeAll:
		if !a.GetConfig().SearchEnabled {
			// This is not an empty result. "Nothing matched" is about the
			// notes, and this answer is about the settings.
			resp.Status = "disabled"
			resp.Error = "global search is off (Settings -> Search)"
			a.writeSearchJSON(w, http.StatusServiceUnavailable, &resp, started)
			return
		}
		if !a.ensureSearchIndex() {
			resp.Status = "unavailable"
			resp.Error = "the search index is not ready"
			a.writeSearchJSON(w, http.StatusServiceUnavailable, &resp, started)
			return
		}
		a.searchGlobal(&resp, qs)
	default:
		resp.Status = "error"
		resp.Error = "unknown scope " + strconv.Quote(scope)
		a.writeSearchJSON(w, http.StatusBadRequest, &resp, started)
		return
	}

	resp.Highlight = highlightTerms(parseQuery(resp.Query))
	a.writeSearchJSON(w, http.StatusOK, &resp, started)
}

// searchPage fills resp from the one document that "on" names.
func (a *App) searchPage(resp *searchResponse, qs map[string][]string) {
	get := func(k string) string {
		if v, ok := qs[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	q := parseQuery(resp.Query)
	if len(q.terms) == 0 {
		return // an empty query is an empty result, not an error
	}

	doc, err := a.loadPageDocument(get("on"))
	if err != nil {
		a.logErrf(logSearch, "%s: %v", get("on"), err)
		return
	}
	if doc == nil {
		return // missing, outside storage, or binary: nothing to say
	}
	// The kind filters of the query apply. Config.SearchKinds does NOT apply.
	// That setting tells what the INDEX holds in memory. It says nothing
	// about the file on the screen. Page search must work on a note also when
	// the index does not cover notes.
	if !kindAllowed(doc.Kind, splitCSV(get("kind")), q.kinds) {
		return
	}

	score, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		return
	}

	limit := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)
	hits = cutSnippets(q, hits, limit, nil)

	res := searchResult{
		Path: doc.Path, Kind: doc.Kind, Name: doc.Name, Title: doc.Title,
		Tags: doc.Tags, Score: score, URL: doc.URL, Truncated: doc.truncated,
	}
	res.Matches, _ = buildMatches(doc, hits)
	// Page scope gives no anchor. The reader is already on the page, and the
	// dialog marks the text there.

	resp.Results = append(resp.Results, res)
	resp.Total = 1
	resp.Truncated = doc.truncated
}

// cutSnippets answers the lines that a result shows: the first limit lines,
// less each line that carries no word of the query. A long note can match a
// common query on hundreds of lines. The last rows of the panel then showed
// lines with only the article "a".
//
// THE ORDER OF THE TWO STEPS IS THE RULE. The window comes first, and the
// drop second. A drop first would let a weaker line from a worse rung move UP
// into the window. With the window first, a line can only leave.
// TestCutSnippetsNeverPromotes holds the rule.
//
// A line leaves when the query has several terms and each term that hits the
// line is short or common. See isShortTerm and commonWords.
// doc/decisions/0009-show-only-the-search-rows-that-carry-a-word-of-the-query.md
// gives the measurements and the rejected alternatives.
func cutSnippets(q parsedQuery, hits []lineHit, limit int, common map[string]bool) []lineHit {
	if len(hits) > limit {
		hits = hits[:limit] // the window first
	}
	if len(q.terms) < 2 || len(hits) == 0 {
		return hits
	}
	kept := make([]lineHit, 0, len(hits))
	for _, h := range hits {
		carries := false
		for _, term := range q.terms {
			if isShortTerm(term.runes) || common[string(term.runes)] {
				continue
			}
			_, _, tier, ok := scoreTerm(term.runes, h.line.fold)
			if !ok || tier != tierSubstring {
				continue
			}
			carries = true
			break
		}
		if carries {
			kept = append(kept, h)
		}
	}
	// A matching document has something to say. When no line of the window
	// carries a word, the window is the answer.
	if len(kept) == 0 {
		return hits
	}
	return kept
}

// searchGlobal answers a query of scope=all from the index. It tests each
// document against the masks and the trigram signature of the query, with no
// I/O. It reads from disk only the documents that can match. The code of page
// search scores them. The index only decides WHICH files to read, thus it
// stays small.
func (a *App) searchGlobal(resp *searchResponse, qs map[string][]string) {
	get := func(k string) string {
		if v, ok := qs[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	q := parseQuery(resp.Query)
	if len(q.terms) == 0 {
		return
	}

	kindFilter := splitCSV(get("kind"))
	limit := clampInt(atoiOr(get("limit"), searchDefaultLimit), 1, searchMaxLimit)
	snippets := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)

	type scored struct {
		doc   *searchDocument
		index *indexedDoc
		score int
		tier  matchTier
		hits  []lineHit
	}
	var found []scored
	common := a.commonWords()
	read := 0

	for _, d := range a.snapshotDocs() {
		if !kindAllowed(d.Kind, kindFilter, q.kinds) {
			continue
		}
		feasible := true
		for _, term := range q.terms {
			if !d.couldMatchTerm(term) {
				feasible = false
				break
			}
		}
		if !feasible {
			continue
		}

		// Only now does the search open a file.
		doc := a.reloadDocument(d)
		if doc == nil {
			continue
		}
		read++
		score, tier, hits, ok := scoreDocument(q, doc)
		if !ok {
			continue
		}
		found = append(found, scored{doc: doc, index: d, score: score, tier: tier, hits: hits})
	}

	// Order the documents by tier and then by score, the same rule as for one
	// match. A document with each term as it is thus wins against one that
	// needed a typo match. A tie orders the newest first, and then by path.
	sortScored := func(i, j int) bool {
		a1, b1 := found[i], found[j]
		if a1.tier != b1.tier || a1.score != b1.score {
			return betterMatch(a1.tier, a1.score, b1.tier, b1.score)
		}
		if !a1.index.ModTime.Equal(b1.index.ModTime) {
			return a1.index.ModTime.After(b1.index.ModTime)
		}
		return a1.index.Path < b1.index.Path
	}
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && sortScored(j, j-1); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}

	resp.Total = len(found)
	if len(found) > limit {
		found = found[:limit]
		resp.Truncated = true
	}

	for _, f := range found {
		hits := cutSnippets(q, f.hits, snippets, common)
		res := searchResult{
			Path: f.doc.Path, Kind: f.doc.Kind, Name: f.doc.Name, Title: f.doc.Title,
			Tags: f.doc.Tags, Score: f.score, URL: f.doc.URL, Truncated: f.doc.truncated,
		}
		var anchor string
		res.Matches, anchor = buildMatches(f.doc, hits)
		res.URL += anchor
		if f.doc.truncated {
			resp.Truncated = true
		}
		resp.Results = append(resp.Results, res)
	}
	if read > 0 {
		a.logInfof(logSearch, "%q: %d candidates read, %d matched", resp.Query, read, resp.Total)
	}
}

// buildMatches makes the snippets of the response from the line hits, with
// the section of each hit. Both scopes use it. The URL of the document gets
// the fragment of the BEST hit. The reader sees the ranked order, thus a jump
// to a weaker match higher on the page would make no sense.
func buildMatches(doc *searchDocument, hits []lineHit) ([]searchMatch, string) {
	var out []searchMatch
	anchor := ""
	for _, h := range hits {
		text, spans := snippetFor(h.line.raw, h.spans)
		m := searchMatch{Line: h.line.no, Context: h.line.context, Text: text}
		for _, sp := range spans {
			m.Spans = append(m.Spans, [2]int{sp.Start, sp.Len})
		}
		if sec := sectionFor(doc.sections, h.line.no); sec != nil && (sec.id != "" || sec.label != "") {
			m.Section = &searchSection{ID: sec.id, Label: sec.label}
			if anchor == "" && sec.id != "" {
				anchor = "#" + sec.id
			}
		}
		out = append(out, m)
	}
	return out, anchor
}

func (a *App) writeSearchJSON(w http.ResponseWriter, status int, resp *searchResponse, started time.Time) {
	resp.TookMS = time.Since(started).Milliseconds()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.logErrf(logSearch, "encode: %v", err)
	}
}

// serveSearchPage renders /OMNGoSearch.html for each request, the same as the
// Config page. It has no md/ source and no html/ cache, and ?refresh does
// nothing.
//
// The page is for GLOBAL search only, because it shows a ranked list of all
// files. With global search off, the page says so and names the setting. Page
// search lives in the dialog.
func (a *App) serveSearchPage(w http.ResponseWriter, r *http.Request) {
	cfg := a.GetConfig()

	// A note can link to the Search page, thus a person can reach it when
	// search is off. The page then says why it can do nothing, and where to
	// change that.
	if !cfg.SearchEnabled {
		body := renderSearchPage(searchPageView{Disabled: true})
		compiled := a.compilePageWithBody("Search",
			[]byte("Title: Search\nCategory: System\n\n"), body)
		writeHTMLHeader(w)
		w.Write(a.injectRuntimeVars(compiled))
		return
	}

	query := r.URL.Query().Get("q")
	view := searchPageView{
		Query:        query,
		IndexedKinds: normalizeSearchKinds(cfg.SearchKinds),
		Highlight:    highlightTerms(parseQuery(query)),
	}

	if strings.TrimSpace(query) != "" && a.ensureSearchIndex() {
		// The page and the API use the same code, thus the page and the
		// dialog always agree.
		resp := searchResponse{Query: query, Scope: SearchScopeAll, Results: []searchResult{}}
		a.searchGlobal(&resp, map[string][]string{"q": {query}})
		view.Results = resp.Results
		view.Total = resp.Total
		view.Truncated = resp.Truncated
	}

	title := "Search"
	if query != "" {
		title = "Search: " + query
	}
	body := renderSearchPage(view)
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars(compiled))
}

// withinStorage reports whether p stays inside StorageDir. resolvePageName
// already cleans its input, thus this is a second guard. notFoundSuggestion
// holds a copy of the same test.
func (a *App) withinStorage(p string) bool {
	root, err := filepath.Abs(a.StorageDir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
