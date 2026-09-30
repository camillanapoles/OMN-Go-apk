package render

import (
	"log"
	"strings"

	"net.basov.omngo/backend/frontend"
)

// ----------------------------------------------------------------------
// Why the pages do NOT use html/template
// ----------------------------------------------------------------------
//
// html/template calls reflect.Value.MethodByName, and that stops the
// dead-code elimination of methods. See
// doc/decisions/0008-render-the-pages-without-html-template.md. Each render
// function is in the file of its page. It escapes each value for its place:
//
//	EscapeHTML(v)            HTML text, or a quoted HTML attribute.
//	EscapeJS(v)              A quoted JS string in an inline <script>.
//	EscapeHTML(EscapeJS(v))  A JS string inside an HTML attribute.
//	trusted HTML             The markdown body or a fragment from a render
//	                         function. It never gets a second escape.

// EscapeHTML escapes a value for HTML text or a double-quoted HTML attribute.
func EscapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// EscapeJS escapes a value for a quoted JavaScript string in an inline
// <script>. It writes '<' and '>' as hex escapes, thus no value can close the
// </script> block.
func EscapeJS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '<':
			b.WriteString(`\x3c`)
		case '>':
			b.WriteString(`\x3e`)
		case '&':
			b.WriteString(`\x26`)
		case '\u2028':
			b.WriteString(`\u2028`)
		case '\u2029':
			b.WriteString(`\u2029`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LoadTemplate reads one page fragment from frontend.Templates. That embed
// stays separate from frontend.Static, because the app extracts frontend.Static
// as files that a person can edit. A missing file shows at the first render.
func LoadTemplate(filename string) string {
	data, err := frontend.Templates.ReadFile("templates/" + filename)
	if err != nil {
		log.Printf("[templates] (error) failed to read embedded %s: %v", filename, err)
		return "<p>Missing embedded template: " + EscapeHTML(filename) + "</p>"
	}
	return string(data)
}

// Fill replaces the %%NAME%% placeholders in tmpl. Each value MUST already
// have the escape for its place. See the banner of this file. Fill itself
// escapes nothing, thus trusted HTML can also pass through it.
func Fill(tmpl string, pairs map[string]string) string {
	oldnew := make([]string, 0, len(pairs)*2)
	for k, v := range pairs {
		oldnew = append(oldnew, "%%"+k+"%%", v)
	}
	return strings.NewReplacer(oldnew...).Replace(tmpl)
}
