package render

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

func TestEscapeHTML(t *testing.T) {
	tests := []struct{ in, want string }{
		{`plain`, `plain`},
		{`a & b`, `a &amp; b`},
		{`<script>`, `&lt;script&gt;`},
		{`say "hi"`, `say &quot;hi&quot;`},
		{`it's`, `it&#39;s`},
		// & must be escaped first or the others get double-escaped
		{`&lt;`, `&amp;lt;`},
	}
	for _, tt := range tests {
		if got := EscapeHTML(tt.in); got != tt.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeJS(t *testing.T) {
	tests := []struct{ in, want string }{
		{`plain`, `plain`},
		{`back\slash`, `back\\slash`},
		{`single'quote`, `single\'quote`},
		{`double"quote`, `double\"quote`},
		{"new\nline", `new\nline`},
		{"carriage\rreturn", `carriage\rreturn`},
		// critical: no value may ever assemble a literal "</script>"
		{`</script>`, `\x3c/script\x3e`},
		{`a&b`, `a\x26b`},
		{"line\u2028sep", `line\u2028sep`},
		{"para\u2029sep", `para\u2029sep`},
	}
	for _, tt := range tests {
		if got := EscapeJS(tt.in); got != tt.want {
			t.Errorf("EscapeJS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFill(t *testing.T) {
	got := Fill("a %%X%% b %%Y%% c %%X%%", map[string]string{"X": "1", "Y": "2"})
	want := "a 1 b 2 c 1"
	if got != want {
		t.Errorf("fill = %q, want %q", got, want)
	}
	// Unknown placeholders are left alone (they indicate a template/render
	// mismatch and should be visible, not silently vanish).
	got = Fill("keep %%UNKNOWN%%", map[string]string{"X": "1"})
	if got != "keep %%UNKNOWN%%" {
		t.Errorf("fill with unknown placeholder = %q", got)
	}
}

func TestRenderIndexPageEscaping(t *testing.T) {
	v := IndexPageView{
		Title:       `My "Quoted" <Title> & Co`,
		PackageName: "net.basov.omngo",
		PageName:    `Weird'Page"Name`,
		PageExt:     ".md",
		IsMarkdown:  true,
		MetaTags:    []MetaTagView{{Name: "author", Value: `A "quoted" <author>`}},
		Tags:        []string{`tag<1>`, "plain"},
		PreviewHTML: "<p>trusted <strong>html</strong></p>",
	}
	out := RenderIndexPage(v)

	// No placeholder may stay in the page.
	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	// HTML contexts escaped.
	if !strings.Contains(out, "My &quot;Quoted&quot; &lt;Title&gt; &amp; Co") {
		t.Error("title not HTML-escaped in output")
	}
	if !strings.Contains(out, `content="A &quot;quoted&quot; &lt;author&gt;"`) {
		t.Error("meta tag value not HTML-escaped")
	}
	if !strings.Contains(out, "tag&lt;1&gt;") {
		t.Error("tag pill not HTML-escaped")
	}
	// The rendered view page must NOT carry a copy of its own source. The
	// editor textarea is gone, and so is the old %%RAW_MD_HTML%%
	// placeholder. An edit is a separate page. Guard against a return of
	// the doubled content.
	if strings.Contains(out, "<textarea id=\"editor\"") {
		t.Error("rendered view page still embeds an #editor textarea (doubled content)")
	}
	// Trusted preview HTML is spliced unescaped.
	if !strings.Contains(out, "<p>trusted <strong>html</strong></p>") {
		t.Error("preview HTML was escaped or lost")
	}
	// JS string contexts escaped.
	if !strings.Contains(out, `var PageName = 'Weird\'Page\"Name';`) {
		t.Error("PageName not JS-escaped in inline script")
	}
	// currentNote moved from an end-of-body script into the page variables
	// block of the <head>. It is declared with var and single-quoted, like
	// its siblings. A classic note script that runs during the body parse
	// can thus see it.
	if !strings.Contains(out, `var currentNote = 'Weird\'Page\"Name';`) {
		t.Error("currentNote not JS-escaped in inline script")
	}
	if !strings.Contains(out, "var IS_MARKDOWN = true;") {
		t.Error("IS_MARKDOWN script missing for markdown page")
	}
	// The page must keep the marker of the runtime values, thus
	// InjectRuntimeVars can find it later. An older marker was an HTML
	// comment, and the render removed it with no message.
	if !strings.Contains(out, RuntimeVarsMarker) {
		t.Error("runtime vars marker missing from rendered page")
	}
	// The Generator meta that CompilePageWithBody injects is not part of
	// this view. Here we assert only that what we passed in came through.
}

// The user files must be LAST. The stylesheet loads after each stylesheet
// of the application, and the script after each script of it. That order
// is the whole feature. It is what lets a user rule win, and what lets a
// user function replace an application function. The editor page must NOT
// load them. A bad rule or a bad line can then never keep the user out of
// the editor that repairs it.
func TestRenderIndexPageLoadsCustomAssetsLast(t *testing.T) {
	out := RenderIndexPage(IndexPageView{
		Title:       "T",
		PageName:    "T",
		PageExt:     ".md",
		IsMarkdown:  true,
		AssetPrefix: "/",
		PreviewHTML: "<p>x</p>",
	})

	customCSS := strings.Index(out, "css/omn-go-custom.css")
	if customCSS < 0 {
		t.Fatal("page does not load css/omn-go-custom.css")
	}
	for _, sheet := range []string{"css/OMN-Go/omn-go-core.css", "css/OMN-Go/highlight.default.min.css", "css/OMN-Go/katex.min.css"} {
		if i := strings.Index(out, sheet); i < 0 || i > customCSS {
			t.Errorf("%s must load BEFORE css/omn-go-custom.css", sheet)
		}
	}

	customJS := strings.Index(out, "js/omn-go-custom.js")
	if customJS < 0 {
		t.Fatal("page does not load js/omn-go-custom.js")
	}
	for _, script := range []string{
		"js/OMN-Go/omn-go-core.js", "js/OMN-Go/omn-go-sse.js", "js/OMN-Go/highlight.min.js",
		"js/OMN-Go/katex.min.js", "js/OMN-Go/auto-render.min.js",
	} {
		if i := strings.Index(out, script); i < 0 || i > customJS {
			t.Errorf("%s must load BEFORE js/omn-go-custom.js", script)
		}
	}

	editor := RenderEditorPage(EditorPageView{Title: "T", Name: "T", PageExt: ".md", ViewURL: "/T.html"})
	if strings.Contains(editor, "omn-go-custom") {
		t.Error("the editor page must not load the user CSS or the user script")
	}
}

func TestRenderEditorPage(t *testing.T) {
	out := RenderEditorPage(EditorPageView{
		Title:   `Weird'Page"Name`,
		Name:    `Weird'Page"Name`,
		PageExt: ".md",
		ViewURL: "/Weird'Page\"Name.html",
	})

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder in editor page:\n%s", out)
	}
	// The source is fetched at runtime, never baked in.
	if strings.Contains(out, "OMN_EDIT_SOURCE") || strings.Contains(out, "textarea>Weird") {
		t.Error("editor page must not embed note source")
	}
	// The editor fetches from /api/note and loads its own script.
	if !strings.Contains(out, "/js/OMN-Go/omn-go-editor.js") {
		t.Error("editor page does not load omn-go-editor.js")
	}
	// Name is JS-escaped in the OMN_EDIT_NAME string literal.
	if !strings.Contains(out, `var OMN_EDIT_NAME = 'Weird\'Page\"Name';`) {
		t.Error("OMN_EDIT_NAME not JS-escaped")
	}
	// Title is HTML-escaped where it appears in text.
	if !strings.Contains(out, "Weird&#39;Page&quot;Name") {
		t.Error("editor title not HTML-escaped")
	}
	// Runtime marker present so the theme is injected (no flash).
	if !strings.Contains(out, RuntimeVarsMarker) {
		t.Error("editor page missing runtime-vars marker for theme injection")
	}
}

func TestRenderExternalEditPage(t *testing.T) {
	v := ExternalEditView{
		Cmd:      "subl",
		FileName: `note "x".md`,
		// A hostile ViewURL that tries to leave the JS string and the
		// onclick attribute at the same time.
		ViewURL: `x');alert("pwn`,
	}
	out := RenderExternalEditPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	if !strings.Contains(out, "note &quot;x&quot;.md") {
		t.Error("file name not HTML-escaped")
	}
	// The raw payload must not survive into the attribute.
	if strings.Contains(out, `x');alert("pwn`) {
		t.Error("hostile ViewURL not escaped in onclick attribute")
	}
	// JS-escaped then HTML-escaped: ' -> \' -> \&#39;
	if !strings.Contains(out, `x\&#39;)`) {
		t.Errorf("ViewURL escaping unexpected, got:\n%s", out)
	}
}

// templates.go holds only the helpers of each page. The view and the render
// function of a page go in the file of that page.
func TestTemplatesGoHoldsOnlyTheHelpers(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "templates.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			got = append(got, d.Name.Name)
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					got = append(got, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						got = append(got, n.Name)
					}
				}
			}
		}
	}
	slices.Sort(got)
	want := []string{"EscapeHTML", "EscapeJS", "Fill", "LoadTemplate"}
	if !slices.Equal(got, want) {
		t.Errorf("templates.go declares %v, want %v. Put the view and the "+
			"render function of a page in the file of that page.", got, want)
	}
}
