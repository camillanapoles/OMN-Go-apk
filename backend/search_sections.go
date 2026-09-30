package backend

// ----------------------------------------------------------------------
// Sections: the address of a PART of a document
// ----------------------------------------------------------------------
//
// QuickNotes is a list of entries, each with a "---" line and a "#####
// <timestamp>" heading. Bookmarks.md holds a JSON array that Bookmarker.js
// shows as a list. The compiled page gives each entry an anchor id. A result
// can thus link to the entry that it matched, and not to the top of a long
// page.
//
// A section is a line range, a label and the anchor id of the compiled HTML.
// It does not change the scoring. There are two sectionizers:
// sectionsFromHeadings for each markdown document, and addBookmarks for the
// bookmarks array.
//
// The hard part is the anchor id. goldmark makes it when it compiles, and
// Bookmarker.js makes it when it shows the page. This file predicts both, and
// it reads neither. Each rule below prevents a WRONG prediction. A missing
// anchor is safe, because the result then links to the page. A wrong anchor
// sends the reader to another entry.

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"net.basov.omngo/backend/internal/textmatch"
)

// docSection is one part of a document, from line start to line end, both
// included. id is the anchor in the compiled HTML. It is "" for the text
// before the first heading, and for a heading with an id that this file
// cannot predict. Such a section still labels its hits, and it links to the
// page.
type docSection struct {
	start, end int
	id         string
	label      string
}

// sectionFor answers the section that holds a line, or nil. A linear search
// is sufficient, because a document has few sections. It runs only over the
// snippets of the final results, and never in the scoring loop.
func sectionFor(sections []docSection, line int) *docSection {
	for i := range sections {
		if line >= sections[i].start && line <= sections[i].end {
			return &sections[i]
		}
	}
	return nil
}

// timestampAnchor is the rule of Bookmarker.js:
//
//	li.setAttribute('id', bm.date.replaceAll(':','').replaceAll(' ','-'))
//
// "2026-06-15 20:00:00" thus gives "2026-06-15-200000". The heading rule of
// goldmark gives the same string for the same timestamp. The code computes
// the two separately, because that agreement is an accident.
func timestampAnchor(date string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(date), ":", ""), " ", "-")
}

// headingUnsafe lists the characters that make a heading id impossible to
// predict from the markdown source. goldmark makes an id from the RENDERED
// text. With these characters, the rendered text differs from the source:
//
//	[ ]    A link. "[text](http://x)" renders as "text", without the URL.
//	& < >  An entity or inline HTML. "&amp;" renders as "&".
//	`      Inline code. renderMarkdownToHTML puts a placeholder there first.
//	$      KaTeX math, with the same placeholder.
//	\      An escape changes the meaning of the next character.
//	_      An emphasis marker, and also a character that the rule keeps.
//	#      A closing run. "## Foo ##" has the text "Foo", but "C#" keeps its '#'.
//
// '*' and '~' are absent on purpose. As text, the rule drops them. As markup,
// goldmark removes them. Both readings give the same id.
const headingUnsafe = "[]`_$<>&#\\"

// headingSlug applies the id rule of goldmark (parser.WithAutoHeadingID) to
// the text of a heading. It lowercases and keeps an ASCII letter or digit. A
// space and a '-' become a '-'. It drops each other ASCII character and each
// non-ASCII rune. ok is false when the text holds a character of
// headingUnsafe.
func headingSlug(text string) (slug string, ok bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(text) {
		switch {
		case r > unicode.MaxASCII:
			// goldmark skips a multi-byte rune too.
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == ' ' || r == '\t' || r == '-':
			b.WriteByte('-')
		case strings.ContainsRune(headingUnsafe, r):
			return "", false
		default:
			// Dropped.
		}
	}
	return b.String(), true
}

// headingIDGen gives ids the way goldmark does, with the suffix for a
// collision. Two equal headings get "x" and "x-1", in document order. A
// heading that this code misses, or cannot predict, thus moves each id after
// it. After such a heading, headingIDGen gives no more ids for the document.
// The sections stay, and they link to the page.
type headingIDGen struct {
	taken     map[string]bool
	poisoned  bool
	anchorsOK bool
}

func newHeadingIDGen() *headingIDGen {
	return &headingIDGen{taken: map[string]bool{}, anchorsOK: anchorsPredictable()}
}

// next answers the anchor for a heading, or "" when it must not give one. A
// heading with no ASCII letter or digit, for example a Cyrillic heading, gets
// the id "heading" from goldmark. next records that id, thus the numbering
// stays correct. It does not return it, because "heading-2" means nothing to
// a reader.
func (h *headingIDGen) next(text string) string {
	if h.poisoned || !h.anchorsOK {
		return ""
	}
	slug, ok := headingSlug(text)
	if !ok {
		h.poisoned = true
		return ""
	}
	degenerate := slug == ""
	if degenerate {
		slug = "heading"
	}
	id := slug
	for i := 1; h.taken[id]; i++ {
		id = slug + "-" + itoa(i)
	}
	h.taken[id] = true
	if degenerate {
		return ""
	}
	return id
}

// poison stops the ids for the rest of the document. sectionsFromHeadings
// calls it for a setext heading. See reSetextRule.
func (h *headingIDGen) poison() { h.poisoned = true }

// headingIDAttrRe finds the ids in compiled HTML.
var headingIDAttrRe = regexp.MustCompile(`<h[1-6][^>]*\bid="([^"]*)"`)

// anchorProbe tests each rule of headingSlug: lowercase letters, a digit, a
// space and a '-', dropped punctuation, a Bookmarker.js timestamp, and a
// collision suffix.
var anchorProbe = []struct{ md, want string }{
	{"# Aa Bb 09", "aa-bb-09"},
	{"# a-b c", "a-b-c"},
	{"# x: y. z! (w)", "x-y-z-w"},
	{"# 2026-07-27 07:23:17", "2026-07-27-072317"},
	{"# dup name", "dup-name"},
	{"# dup name", "dup-name-1"},
}

var (
	anchorsOnce sync.Once
	anchorsGood bool
)

// anchorsPredictable compiles the probe document with the REAL renderer. It
// checks that the ids agree with headingSlug. goldmark makes the anchor, and
// this file makes the link. A new goldmark with another id rule would send
// the reader to the wrong section, with no error.
//
// TestHeadingIDsAgreeWithTheRenderer finds that in the build. This check
// finds it on a device, at run time. It then turns the anchors off, writes
// one log line, and each result links to the page. The cost is one compile of
// about 90 bytes, one time for each process.
func anchorsPredictable() bool {
	anchorsOnce.Do(func() { anchorsGood = probeAnchors() })
	return anchorsGood
}

func probeAnchors() bool {
	var src strings.Builder
	want := make([]string, 0, len(anchorProbe))
	for _, c := range anchorProbe {
		src.WriteString(c.md)
		src.WriteString("\n\n")
		want = append(want, c.want)
	}

	var got []string
	for _, m := range headingIDAttrRe.FindAllStringSubmatch(
		(&App{}).renderMarkdownToHTML([]byte(src.String())), -1) {
		got = append(got, m[1])
	}

	if len(got) != len(want) {
		logAnchorsOff("the renderer emitted " + itoa(len(got)) + " heading ids, expected " + itoa(len(want)))
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			logAnchorsOff("heading id " + itoa(i+1) + " is " + got[i] + ", expected " + want[i])
			return false
		}
	}
	return true
}

// reATXHeading matches the start of a heading. CommonMark needs the space
// after the '#' run, thus "#tag" is a word. A heading can be empty, for
// example "###".
var reATXHeading = regexp.MustCompile(`^ {0,3}(#{1,6})([ \t]+(.*))?$`)

// reSetextRule matches a line that can underline a setext heading. The code
// does not use it to find a heading. A run of '=' or '-' under a paragraph
// line is an h1 or h2. Its text depends on the rules of a paragraph, thus the
// code stops the ids of that document. QuickNotes always has an empty line
// before its "---", thus that line is a thematic break and never an
// underline.
var reSetextRule = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)

// sectionsFromHeadings splits a document at its headings. lines, contexts and
// firstLineNo describe the BODY, as addLines reads it. classifyContexts marks
// a heading inside a fence or a <script>, and the loop skips it. goldmark
// does not read such a line as a heading either, thus a count of it would
// move each suffix after it. The function answers nil for a document with no
// heading.
func sectionsFromHeadings(lines, contexts []string, firstLineNo int) []docSection {
	ids := newHeadingIDGen()
	var out []docSection

	for i, line := range lines {
		no := firstLineNo + i
		if contexts[i] != "" {
			continue // inside a fence, <pre> or <script>: not a heading
		}
		if i > 0 && reSetextRule.MatchString(line) && strings.TrimSpace(lines[i-1]) != "" &&
			!reATXHeading.MatchString(lines[i-1]) {
			ids.poison()
			continue
		}
		m := reATXHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		text := strings.TrimSpace(m[3])

		if n := len(out); n > 0 {
			out[n-1].end = no - 1
		}
		out = append(out, docSection{
			start: no,
			end:   1 << 30, // closed by the next heading, or left open
			id:    ids.next(text),
			label: text,
		})
	}

	if len(out) == 0 {
		return nil
	}
	// The lines above the first heading are the preamble. It has no label and
	// no id. It is a separate section, thus its hits do not go to the first
	// heading.
	if out[0].start > firstLineNo {
		out = append([]docSection{{start: firstLineNo, end: out[0].start - 1}}, out...)
	}
	return out
}

// bookmarkEntry is the struct that handleBookmark writes.
type bookmarkEntry struct {
	Date  string   `json:"date"`
	URL   string   `json:"url"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Notes []string `json:"notes"`
}

// reBookmarksArray finds the start of the array that Bookmarker.js reads.
var reBookmarksArray = regexp.MustCompile(`\bbookmarks\s*=\s*\[`)

// bookmarksBlock holds the array as valid JSON, and the lines of each entry
// and of the array. A hit thus gets a line that a reader can open.
type bookmarksBlock struct {
	json       string
	startLines []int // source line of each entry's '{', in array order
	firstLine  int   // the line the array opens on
	lastLine   int   // the line it closes on
}

// scanBookmarksArray answers the array as JSON that encoding/json can read.
// It removes two things that are not JSON:
//
//   - The "<!-- Don't edit body below this line -->" marker INSIDE the array.
//     handleBookmark puts each new entry after it. JavaScript accepts "<!--".
//   - A comma before the closing ']'. A file with no entries can have one.
//
// The scan knows each string, thus it does not change a "<!--" or a ",]"
// inside the text of a bookmark. Two regular expressions would change the
// data of the user.
func scanBookmarksArray(content string, firstLineNo int) (bookmarksBlock, bool) {
	loc := reBookmarksArray.FindStringIndex(content)
	if loc == nil {
		return bookmarksBlock{}, false
	}
	start := loc[1] - 1 // the '['

	var (
		out       []byte
		block     = bookmarksBlock{firstLine: firstLineNo + strings.Count(content[:start], "\n")}
		line      = block.firstLine
		depth     int
		inStr     bool
		esc       bool
		lastComma = -1 // index in out of a comma with only space since
	)

	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '\n' {
			line++
		}
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr, lastComma = true, -1
			out = append(out, c)
		case c == '<' && strings.HasPrefix(content[i:], "<!--"):
			end := strings.Index(content[i:], "-->")
			if end < 0 {
				return bookmarksBlock{}, false
			}
			line += strings.Count(content[i:i+end+3], "\n")
			i += end + 2
		case c == '[', c == '{':
			if c == '{' && depth == 1 {
				block.startLines = append(block.startLines, line)
			}
			depth++
			lastComma = -1
			out = append(out, c)
		case c == ']', c == '}':
			if lastComma >= 0 {
				out[lastComma] = ' ' // the trailing comma, neutralised in place
			}
			depth--
			lastComma = -1
			out = append(out, c)
			if depth == 0 {
				block.json, block.lastLine = string(out), line
				return block, true
			}
		case c == ',':
			out = append(out, c)
			lastComma = len(out) - 1
		case c == ' ', c == '\t', c == '\n', c == '\r':
			out = append(out, c) // whitespace does not end a trailing comma
		default:
			out = append(out, c)
			lastComma = -1
		}
	}
	return bookmarksBlock{}, false // unterminated array
}

// logAnchorsOff writes one error line. Without it, the only sign is that the
// results link to the top of a long note.
func logAnchorsOff(why string) {
	log.Printf("[search] (error) section anchors disabled - the renderer no longer "+
		"assigns heading ids the way this build predicts (%s). Results will "+
		"link at the page instead of the section.", why)
}

// bookmarksNote is the one note with its own parser. It is a base name,
// because resolvePageName and the index walk use the base name.
const bookmarksNote = "Bookmarks"

// bookmarkJoin separates the fields of an entry in its one searchable line.
// The UI uses the same middle dot, and no URL, tag or timestamp of this app
// holds it.
const bookmarkJoin = " · "

// addBookmarks indexes Bookmarks.md by ENTRIES, and not by lines.
// handleBookmark writes the array with json.MarshalIndent, and that writes
// '<', '>' and '&' as \u escapes. The source line of a bookmark "Cats & Dogs"
// thus does not hold "&". Only the decoded JSON holds the text that the user
// sees. TestBookmarkWithEscapedPunctuationIsFindable holds the rule.
//
// addBookmarks indexes the text outside the array as normal prose. A file
// that does not have the expected shape gets the normal line index.
func (d *searchDocument) addBookmarks(body string, firstLineNo int) {
	block, ok := scanBookmarksArray(body, firstLineNo)
	var entries []bookmarkEntry
	if ok {
		if err := json.Unmarshal([]byte(block.json), &entries); err != nil {
			ok = false
		} else if len(entries) != len(block.startLines) {
			// When the scan and the decoder count different numbers of
			// entries, the line of each entry would be wrong.
			ok = false
		}
	}
	if !ok {
		log.Printf("[search] (error) %s: not a readable bookmarks array, indexing it as plain text", d.Path)
		d.addLines(body, firstLineNo)
		return
	}

	// Replace the lines of the array with empty lines, and do not remove
	// them. addLines drops an empty line, thus the prose around the array
	// keeps its real line numbers.
	lines := strings.Split(body, "\n")
	for i := range lines {
		if no := firstLineNo + i; no >= block.firstLine && no <= block.lastLine {
			lines[i] = ""
		}
	}
	d.addLines(strings.Join(lines, "\n"), firstLineNo)

	for i, e := range entries {
		start := block.startLines[i]
		end := block.lastLine
		if i+1 < len(block.startLines) {
			end = block.startLines[i+1] - 1
		}

		label := strings.TrimSpace(e.Title)
		if label == "" {
			label = strings.TrimSpace(e.URL)
		}
		d.sections = append(d.sections, docSection{
			start: start,
			end:   end,
			id:    timestampAnchor(e.Date),
			label: label,
		})

		// Make ONE line for each entry, and not one for each field.
		// scoreDocument keys its hits by line number. Two lines with one
		// number would merge, and a span of the URL would mark the title.
		// snippetFor cuts the line to about 160 runes around the match.
		var parts []string
		for _, p := range []string{e.Title, e.URL, strings.Join(e.Tags, ", "), strings.Join(e.Notes, "; ")} {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			continue
		}
		text := strings.Join(parts, bookmarkJoin)
		f := textmatch.Fold(text)
		d.lines = append(d.lines, docLine{no: start, raw: text, fold: f, mask: textmatch.RuneMask(f)})
	}
}
