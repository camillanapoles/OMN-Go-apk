package exchange

// ----------------------------------------------------------------------
// Send a note to another person, and receive one
// ----------------------------------------------------------------------
//
// A note leaves as its Markdown source, and it arrives under md/incoming/.
// Each transport delivers a flat file name, thus THE PATH TRAVELS INSIDE THE
// FILE as "FileName:". AN EXPORT IS A READ: it adds the line to the copy that
// leaves, and it never writes the stored note.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/noteheader"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

const (
	// incomingDirName is the one place under md/ where an arriving note can
	// land. An arriving note thus cannot overwrite a note of the user, also
	// when the sanitizer has a fault.
	incomingDirName = "incoming"

	// incomingIndexBase is the note that lists the arrivals:
	// md/incoming/incoming.md.
	incomingIndexBase = "incoming"

	// IncomingIndexName is the NAME of that note, as a URL and the page use
	// it. injectRuntimeVars gives it to the page as OMN_INCOMING_PAGE, thus
	// the JavaScript needs no copy of the name.
	IncomingIndexName = incomingDirName + "/" + incomingIndexBase

	// incomingListMarker marks the list. A new line goes directly after it,
	// newest first, and a person can keep text above it.
	incomingListMarker = "<!-- omn-go-incoming-list -->"

	headerKeyFileName = "FileName"
	headerKeyImported = "Imported"

	// headerDescription carries the description of a note to the Android
	// message. The value is BASE64, because an HTTP header is not UTF-8, and
	// a newline would end it.
	headerDescription = "X-OMN-Description"

	// descriptionMaxRunes limits that header. Telegram refuses a caption over
	// 1024 characters as a whole, and it does not shorten it. 1000 stays
	// below that limit.
	descriptionMaxRunes = 1000

	// exportNameMaxRunes limits the attachment name. 100 is below each file
	// system limit in use.
	exportNameMaxRunes = 100

	// These three limits bound what an arriving FileName: can ask for. A path
	// from another device promises nothing.
	importSegmentMaxRunes = 64
	importPathMaxRunes    = 200
	importMaxSegments     = 8
)

// ExportNoteSource answers the Markdown of a note, ready to send, and the
// name of the attachment. It SETS FileName:, and it does not add a second
// line. An imported note already has one. See noteheader.SetKey.
func (svc Service) ExportNoteSource(name string) (data []byte, filename string, err error) {
	mdPath, _, baseName, isPage := storage.ResolvePageName(svc.Layout, svc.MimeTypes, name)
	if !isPage {
		return nil, "", fmt.Errorf("%q is not a note", name)
	}
	src, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, "", err
	}
	out := noteheader.SetKey(normalizeNewlines(string(src)), headerKeyFileName, baseName)
	return []byte(out), flattenExportName(baseName) + ".md", nil
}

// flattenExportName makes ONE file name for a recipient:
//
//	project/Sub/WeeklyPlan  ->  project-Sub-WeeklyPlan.md
//
// The name is A LABEL FOR A PERSON. The importer reads FileName:, and it uses
// this name only when that line is missing. The name uses only characters
// that Windows and Android can both store.
func flattenExportName(noteName string) string {
	out := make([]rune, 0, len(noteName))
	lastDash := false
	for _, r := range strings.ReplaceAll(noteName, "/", "-") {
		keep := r == '.' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z')
		if !keep {
			r = '-'
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		out = append(out, r)
	}
	name := strings.Trim(string(out), "-.")
	if len([]rune(name)) > exportNameMaxRunes {
		name = string([]rune(name)[:exportNameMaxRunes])
		name = strings.Trim(name, "-.")
	}
	if name == "" {
		name = "note"
	}
	return name
}

// descriptionRe finds the description block of a note, an HTML comment that
// the page does not show:
//
//	<!--- DESCRIPTION:
//	There is some
//	description
//	--->
//
// It accepts each number of dashes at each end, each case of DESCRIPTION, and
// an optional colon. HTML ends a comment at the first "-->", thus a line of
// dashes ends the description.
var descriptionRe = regexp.MustCompile(`(?is)<!--+\s*DESCRIPTION\b\s*:?\s*(.*?)\s*--+>`)

// noteDescription answers the text of the FIRST description block in the
// whole note, or "" when the note has none.
func noteDescription(src string) string {
	m := descriptionRe.FindStringSubmatch(normalizeNewlines(src))
	if m == nil {
		return ""
	}
	text := strings.TrimSpace(m[1])
	if r := []rune(text); len(r) > descriptionMaxRunes {
		text = strings.TrimSpace(string(r[:descriptionMaxRunes]))
	}
	return text
}

// importResult tells where an arriving note landed.
type importResult struct {
	// Name is the note name under md/, for example
	// "incoming/project/Sub/WeeklyPlan-2". A URL and resolvePageName use it.
	Name string
	// Rel is the path under md/incoming/, for example
	// "project/Sub/WeeklyPlan-2". The incoming index is in that directory,
	// thus it links to Rel.
	Rel string
	// Base is the file name as saved, with the collision suffix when the
	// import needed one, for example "WeeklyPlan-2".
	Base string
	// Label is the text of the index link. See incomingLabel.
	Label string
}

// ImportNote writes an arriving note under md/incoming/ and adds a line to
// the incoming index. displayName is the attachment name, the fallback for a
// note with no FileName: line. The caller gives now for the tests.
func (svc Service) ImportNote(content []byte, displayName string, now time.Time) (importResult, error) {
	src := normalizeNewlines(string(content))
	if strings.TrimSpace(src) == "" {
		return importResult{}, fmt.Errorf("the note is empty")
	}

	// Keep FileName:, because it tells where this copy came from. An export
	// sets the line again with noteheader.SetKey.
	original, _ := headerValue(src, headerKeyFileName)

	rel := sanitizeImportPath(original)
	if rel == "" {
		rel = sanitizeImportPath(strings.TrimSuffix(displayName, ".md"))
	}
	if rel == "" {
		title, _ := headerValue(src, "Title")
		rel = sanitizeImportPath(title)
	}
	if rel == "" {
		rel = "note-" + now.UTC().Format("2006-01-02-150405")
	}

	dir, base := path.Split(rel)
	fullDir, ok := svc.incomingPath(dir)
	if !ok {
		return importResult{}, fmt.Errorf("the name %q leaves the incoming directory", original)
	}
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return importResult{}, err
	}

	wanted := base
	base = freeNoteBase(fullDir, base)
	rel = path.Join(dir, base)

	// The link text is the Title of the note, and the file name is the
	// fallback. The label keeps the collision suffix.
	title, _ := headerValue(src, "Title")
	index := ""
	if base != wanted {
		// freeNoteBase adds "-2", "-3" and so on to the name that it got.
		index = strings.TrimPrefix(base, wanted+"-")
	}
	label := incomingLabel(title, base, index)

	// Record when the note arrived. Date: and Modified: are facts of the
	// sender, and they stay as they are.
	src = noteheader.SetKey(src, headerKeyImported, noteheader.Stamp(now))

	if err := os.WriteFile(filepath.Join(fullDir, base+".md"), []byte(src), 0644); err != nil {
		return importResult{}, err
	}

	res := importResult{
		Name:  path.Join(incomingDirName, rel),
		Rel:   rel,
		Base:  base,
		Label: label,
	}
	if err := svc.addIncomingIndexLine(res, now); err != nil {
		// The note is on disk. Only its index line is missing. Report that,
		// and do not fail an import that worked.
		return res, fmt.Errorf("the note was saved, but the incoming index was not updated: %w", err)
	}
	return res, nil
}

// sanitizeImportPath changes a FileName: from another device into a path that
// is safe under md/incoming/, or "". THIS IS THE ONLY PATH THAT AN ATTACKER
// CONTROLS. incomingPath checks the resolved path after it, thus two guards
// protect md/.
func sanitizeImportPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A Windows sender writes "project\Sub\Note". Treat the backslash as a
	// separator. The segments then get the same rules as each other segment.
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.TrimSuffix(raw, ".md")
	// A drive letter means nothing on this device. Remove it.
	if len(raw) > 1 && raw[1] == ':' {
		raw = raw[2:]
	}

	var segs []string
	for _, seg := range strings.Split(raw, "/") {
		seg = sanitizeImportSegment(seg)
		if seg == "" {
			continue // an empty, ".", ".." or all-punctuation segment
		}
		segs = append(segs, seg)
		if len(segs) >= importMaxSegments {
			break
		}
	}
	if len(segs) == 0 {
		return ""
	}

	// When the path is too long, drop folders from the FRONT. The name of the
	// file is the part that a reader needs most.
	for len([]rune(path.Join(segs...))) > importPathMaxRunes && len(segs) > 1 {
		segs = segs[1:]
	}
	out := path.Join(segs...)
	if len([]rune(out)) > importPathMaxRunes {
		out = string([]rune(out)[:importPathMaxRunes])
		out = strings.Trim(out, " .-")
	}
	return out
}

// sanitizeImportSegment cleans ONE path segment. It answers "" for a segment
// that must not exist: empty, ".", "..", or empty after the character rules.
func sanitizeImportSegment(seg string) string {
	seg = strings.TrimSpace(seg)
	if seg == "" || seg == "." || seg == ".." {
		return ""
	}
	out := make([]rune, 0, len(seg))
	lastDash := false
	for _, r := range seg {
		// A control character does not belong in a file name. A newline would
		// also break the header line.
		if r < 0x20 || r == 0x7f {
			continue
		}
		keep := r == ' ' || r == '.' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z')
		if !keep {
			r = '-'
		}
		// Join a run of dashes into one, or "Note (2)" becomes "Note -2-".
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		out = append(out, r)
	}
	// A leading dot hides the file. Windows cannot store a trailing dot or
	// space.
	seg = strings.Trim(string(out), " .-")
	if len([]rune(seg)) > importSegmentMaxRunes {
		seg = string([]rune(seg)[:importSegmentMaxRunes])
		seg = strings.Trim(seg, " .-")
	}
	return seg
}

// incomingPath joins rel under md/incoming/ and reports whether the resolved
// result stays inside. filepath.Join resolves a "..", and it does not refuse
// it.
func (svc Service) incomingPath(rel string) (string, bool) {
	root := svc.Layout.MD(incomingDirName)
	full := filepath.Join(root, filepath.FromSlash(rel))
	if _, ok := storage.RelInside(root, full); !ok {
		return "", false
	}
	return full, true
}

// freeNoteBase answers base, or base-2, base-3 and so on: the first name that
// no note in dir has. The form "-2" is safe in a URL, a heading id, git and
// Windows.
func freeNoteBase(dir, base string) string {
	if !storage.FileExists(filepath.Join(dir, base+".md")) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !storage.FileExists(filepath.Join(dir, candidate+".md")) {
			return candidate
		}
	}
}

// hrefEscapePath percent-encodes a note path for an href, and it keeps "/",
// which url.PathEscape escapes. It encodes each byte outside the unreserved
// set.
func hrefEscapePath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

// incomingLabelMaxRunes limits the link text on the incoming index. A long
// title would change the list into prose.
const incomingLabelMaxRunes = 80

// incomingLabelUnsafe lists the characters that a Title: must not carry into
// a Markdown link label:
//
//	[ ]    End the label early.
//	< > &  Raw HTML and entities pass through.
//	\      Escapes the next character.
//	` * ~  A code span, emphasis and strikethrough. "_" stays, because it
//	       does not emphasize inside a word.
//	|      A table cell.
//
// incomingLabel changes each one to a space.
const incomingLabelUnsafe = "[]<>&\\`*~|"

// incomingLabel answers the link text of one line on the incoming index: the
// Title of the note, or base as the fallback. index is the collision suffix,
// for example "2", or "". The title comes from another device, and nothing
// escapes the Markdown line later. The function thus makes it one line of
// plain text.
func incomingLabel(title, base, index string) string {
	clean := make([]rune, 0, len(title))
	space := true // leading whitespace is dropped by starting "inside" a run
	for _, r := range title {
		if r == '\t' || r == '\n' || r == '\r' || r == ' ' ||
			(r < 0x80 && strings.ContainsRune(incomingLabelUnsafe, r)) {
			if !space {
				clean = append(clean, ' ')
				space = true
			}
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue // a control character has no business on the page
		}
		clean = append(clean, r)
		space = false
	}
	label := strings.TrimSpace(string(clean))
	if len([]rune(label)) > incomingLabelMaxRunes {
		label = strings.TrimSpace(string([]rune(label)[:incomingLabelMaxRunes])) + "\u2026"
	}
	if label == "" {
		return base
	}
	if index != "" {
		label += " (" + index + ")"
	}
	return label
}

// addIncomingIndexLine puts one line at the top of the list in
// md/incoming/incoming.md: below the marker, or first in the body. The link
// target is relative to the index directory.
func (svc Service) addIncomingIndexLine(res importResult, now time.Time) error {
	dir := svc.Layout.MD(incomingDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	indexPath := filepath.Join(dir, incomingIndexBase+".md")

	content := ""
	if data, err := os.ReadFile(indexPath); err == nil {
		content = normalizeNewlines(string(data))
	} else if !os.IsNotExist(err) {
		return err
	} else {
		content = incomingIndexStarter(now)
	}

	label := res.Label
	if label == "" {
		label = res.Base
	}
	// incomingLabel already cleaned the label. hrefEscapePath encodes the
	// target, because a note name can hold a space.
	line := "* <span class=\"omn-incoming-when\">" + now.UTC().Format("2006-01-02 15:04") +
		"</span> · [" + label + "](" + hrefEscapePath(res.Rel) + ")"

	header, sep, body := noteheader.SplitRegion(content)
	if header == "" {
		return os.WriteFile(indexPath, []byte(line+"\n\n"+content), 0644)
	}

	// Put the line below the marker, when the note has one. The receive box
	// is above the marker.
	if at := strings.Index(body, incomingListMarker); at >= 0 {
		at += len(incomingListMarker)
		if at < len(body) && body[at] == '\n' {
			at++
		}
		return os.WriteFile(indexPath,
			[]byte(header+sep+body[:at]+line+"\n"+body[at:]), 0644)
	}

	// Without the marker, put an empty line before the line. "* 2026-08-09
	// 12:34 · [x](y)" holds a colon. After one newline, isHeaderFirstLine
	// would read it as a header line.
	if body == "" {
		return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"), 0644)
	}
	return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"+body), 0644)
}

// incomingIndexTmpl is the incoming index as the app first writes it: a
// header block, the receive box and the list marker.
var incomingIndexTmpl = render.LoadTemplate("incoming_index.md")

// incomingIndexStarter answers the incoming index as the app first writes it.
// It is a template, because initStorage extracts frontend/md FLAT into md/.
// The app writes it one time, and then it belongs to the user.
func incomingIndexStarter(now time.Time) string {
	return normalizeNewlines(render.Fill(incomingIndexTmpl, map[string]string{
		"DATE": noteheader.Stamp(now),
	}))
}

// EnsureIncomingIndex writes the incoming index when it is absent. The start
// and each import call it. On the desktop, the receive box is how the first
// note arrives, thus the page must exist before the first import.
func (svc Service) EnsureIncomingIndex(now time.Time) error {
	dir := svc.Layout.MD(incomingDirName)
	indexPath := filepath.Join(dir, incomingIndexBase+".md")
	if storage.FileExists(indexPath) {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(indexPath, []byte(incomingIndexStarter(now)), 0644)
}

// normalizeNewlines changes CRLF and CR to LF. A mail client or a Windows
// machine can change the line ends, and each rule in this file counts lines.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// headerValue reads one header key, and it does not remove it.
func headerValue(content, key string) (string, bool) {
	header, _, _ := noteheader.SplitRegion(content)
	if header == "" {
		return "", false
	}
	for _, l := range strings.Split(header, "\n") {
		if strings.EqualFold(noteheader.KeyOf(l), key) {
			if c := strings.IndexByte(l, ':'); c >= 0 {
				return strings.TrimSpace(l[c+1:]), true
			}
		}
	}
	return "", false
}
