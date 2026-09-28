package backend

import (
	"fmt"
	"net/url"
	"strings"
)

// --- The file index page (files_page.html, see files_index.go) ---

var filesPageTmpl = loadTemplate("files_page.html")

// filesCrumb is one step of the breadcrumb. Dir is the new value of ?dir=.
type filesCrumb struct {
	Label string
	Dir   string
}

// filesTreeCard is one button of the first screen.
type filesTreeCard struct {
	Key   string
	Icon  string
	Title string
	Where string
	Count string
	Class string
}

// filesLegendItem is one line of the legend under the crumb.
type filesLegendItem struct {
	Color string
	Word  string
	Text  string
}

// filesDirRow is one directory below the directory in view. Files and Bytes
// are RECURSIVE totals. See (*filesDirRow).note in files_state.go.
type filesDirRow struct {
	Name        string
	Dir         string
	Files       int
	Bytes       int64
	anyShips    bool
	shipCount   int
	anyDevice   bool
	everyShips  bool
	everyDevice bool
}

// filesFileRow is one NAME of the tree in view. Each field is raw, and
// renderFilesPage escapes it. See the banner of files_index.go for the word
// and the colors.
type filesFileRow struct {
	Name       string
	Path       string
	URL        string
	EditURL    string // "" when the row offers no edit link
	Kind       string // a Material Icons ligature
	Size       string
	Mod        string // "" for a file that is not on the device
	ModFull    string
	State      string
	StateColor string
	AppOwned   bool
	OwnerColor string
	Extra      []string
}

type filesPageView struct {
	Tree       string // "" on the first screen
	Dir        string
	Crumbs     []filesCrumb
	Cards      []filesTreeCard
	Legend     []filesLegendItem
	Summary    string
	Dirs       []filesDirRow
	Files      []filesFileRow
	Total      int // files directly in this directory, before the cap
	Hidden     int // ... how many of them are not shown
	Empty      bool
	ShowingAll bool
}

// filesOwnerHint is the tooltip of the app-owned mark.
const filesOwnerHint = "The next version of OMN-Go backs up your copy and replaces it"

func renderFilesPage(v filesPageView) string {
	if v.Tree == "" {
		return fill(filesPageTmpl, map[string]string{"BODY": renderFilesCards(v)})
	}
	return fill(filesPageTmpl, map[string]string{"BODY": renderFilesListing(v)})
}

// renderFilesCards makes the first screen: three buttons in one column at
// each width. A wide screen gets a narrower page, not three columns. There is
// thus one layout to build and to test.
func renderFilesCards(v filesPageView) string {
	var b strings.Builder
	b.WriteString(`<div class="files-cards">`)
	for _, c := range v.Cards {
		fmt.Fprintf(&b, `<a class="files-card %s" href="%s">`+
			`<i class="material-icons files-card-icon">%s</i>`+
			`<span class="files-card-text">`+
			`<span class="files-card-title">%s</span>`+
			`<span class="files-card-where">%s</span>`+
			`<span class="files-card-count">%s</span>`+
			`</span></a>`,
			escapeHTML(c.Class), escapeHTML(filesPageURL(c.Key, "", false)),
			escapeHTML(c.Icon), escapeHTML(c.Title), escapeHTML(c.Where),
			escapeHTML(c.Count))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// filesPageURL makes a link back to this page. It has three parameters, and
// only this function makes them. No request value can thus go into the link.
func filesPageURL(tree, dir string, all bool) string {
	out := "/OMNGoFiles.html"
	sep := "?"
	if tree != "" {
		out += sep + "tree=" + url.QueryEscape(tree)
		sep = "&"
	}
	if dir != "" {
		out += sep + "dir=" + url.QueryEscape(dir)
		sep = "&"
	}
	if all {
		out += sep + "all=1"
	}
	return out
}

func renderFilesListing(v filesPageView) string {
	var b strings.Builder

	// Write the crumb. Each label holds its own slash, and nothing goes
	// between two labels. The trail thus reads as the path: html/js/.
	b.WriteString(`<div class="files-crumbs">`)
	for i, c := range v.Crumbs {
		if i == len(v.Crumbs)-1 {
			fmt.Fprintf(&b, `<span class="files-crumb-here">%s</span>`, escapeHTML(c.Label))
			continue
		}
		fmt.Fprintf(&b, `<a href="%s">%s</a>`,
			escapeHTML(filesPageURL(v.Tree, c.Dir, false)), escapeHTML(c.Label))
	}
	b.WriteString(`</div>`)

	fmt.Fprintf(&b, `<p class="files-summary">%s</p>`, escapeHTML(v.Summary))

	// Fold the legend, and leave it out when this directory uses no word.
	// <details> needs no script, and it keeps its state while the page is
	// open.
	if len(v.Legend) > 0 {
		b.WriteString(`<details class="files-legend">` +
			`<summary>What the words mean</summary>`)
		for _, item := range v.Legend {
			fmt.Fprintf(&b, `<div><b class="%s">%s</b> — %s</div>`,
				escapeHTML(item.Color), escapeHTML(item.Word), escapeHTML(item.Text))
		}
		b.WriteString(`</details>`)
	}

	if v.Empty {
		b.WriteString(`<p class="files-empty">This directory holds nothing.</p>`)
		return b.String()
	}

	b.WriteString(`<ul class="files-list">`)
	for _, d := range v.Dirs {
		fmt.Fprintf(&b, `<li class="files-row files-dir">`+
			`<span class="files-name"><i class="material-icons files-kind">folder</i>`+
			`<a href="%s">%s</a></span>`,
			escapeHTML(filesPageURL(v.Tree, d.Dir, false)), escapeHTML(d.Name+"/"))
		if word, color := filesDirNote(v.Tree, d); word != "" {
			fmt.Fprintf(&b, `<span class="files-state %s">%s</span>`,
				escapeHTML(color), escapeHTML(word))
		}
		fmt.Fprintf(&b, `<span class="files-facts"><span class="files-size">%s · %s</span>`+
			`</span></li>`,
			escapeHTML(filesCountLabel(d.Files)), escapeHTML(filesSize(d.Bytes)))
	}
	for _, f := range v.Files {
		renderFilesRow(&b, f)
	}
	b.WriteString(`</ul>`)

	if v.Hidden > 0 {
		fmt.Fprintf(&b, `<p class="files-more">%s not shown `+
			`<a href="%s">show all %s &rarr;</a></p>`,
			escapeHTML(itoa(v.Hidden)),
			escapeHTML(filesPageURL(v.Tree, v.Dir, true)),
			escapeHTML(itoa(v.Total)))
	}
	return b.String()
}

// renderFilesRow writes one row: the name and the state on the first line,
// and the facts on the second. The name has the whole first line, thus it
// never goes into a narrow column.
func renderFilesRow(b *strings.Builder, f filesFileRow) {
	b.WriteString(`<li class="files-row">`)
	fmt.Fprintf(b, `<span class="files-name">`+
		`<i class="material-icons files-kind">%s</i><a href="%s">%s</a></span>`,
		escapeHTML(f.Kind), escapeHTML(f.URL), escapeHTML(f.Name))
	if f.State != "" {
		fmt.Fprintf(b, `<span class="files-state %s">%s</span>`,
			escapeHTML(f.StateColor), escapeHTML(f.State))
	}
	b.WriteString(`<span class="files-facts">`)
	fmt.Fprintf(b, `<span class="files-size">%s</span>`, escapeHTML(f.Size))
	if f.Mod != "" {
		// Show the date only, because the full time is too wide for a phone.
		// The title keeps the full time.
		fmt.Fprintf(b, `<span class="files-meta" title="%s">%s</span>`,
			escapeHTML(f.ModFull), escapeHTML(f.Mod))
	}
	// The ownership word is on the second line of each row that has it, in
	// each tree. The color is a hint, and the word is the fact.
	if f.AppOwned {
		fmt.Fprintf(b, `<span class="files-meta %s" title="%s">app-owned</span>`,
			escapeHTML(f.OwnerColor), escapeHTML(filesOwnerHint))
	}
	for _, extra := range f.Extra {
		fmt.Fprintf(b, `<span class="files-meta">%s</span>`, escapeHTML(extra))
	}
	if f.EditURL != "" {
		fmt.Fprintf(b, `<a class="files-edit" href="%s">edit</a>`, escapeHTML(f.EditURL))
	}
	b.WriteString(`</span></li>`)
}

// filesDirNote gives the one word of a directory row. A directory speaks only
// when OMN-Go put files into it, and the count tells how many.
func filesDirNote(tree string, d filesDirRow) (word, color string) {
	if tree == filesTreeBundled || !d.anyShips {
		return "", ""
	}
	if d.everyShips && !d.anyDevice {
		return itoa(d.Files) + " " + filesFromTheApp + ", none extracted", filesColorPlain
	}
	return itoa(d.shipCount) + " " + filesFromTheApp, filesColorApp
}
