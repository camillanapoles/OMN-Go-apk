package backend

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

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

// loadPageDocument reads the one file of a page query. resolvePageName
// decides what name means. A missing file, or one outside the storage
// directory, gives nil and no error, thus a query cannot probe the file
// system.
func (a *App) loadPageDocument(name string) (*searchDocument, error) {
	if name == "" {
		return nil, nil
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	filePath := htmlPath
	if isPage {
		filePath = mdPath
	}
	if !a.layout().contains(filePath) {
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

// parseMarkdown fills a document from a note. The header block gives weighted
// fields, and only the BODY gives content lines. The line numbers count the
// header too.
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

// classifyContexts marks each line as prose, "code" or "script". The mark
// does not lower the score, because a person can search FOR code. The fence
// state wins over the tag state, the same as in markdown.go. An inline `code`
// span gets no mark.
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
