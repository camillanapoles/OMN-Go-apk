package backend

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fileGroups puts each production file in one group. A later split makes
// each group a package, and the compiler then holds the layers.
var fileGroups = map[string]string{
	"log_levels.go": "logx", "logger.go": "logx",
	"search_match.go": "textmatch",
	"header_block.go": "noteheader",

	"config.go": "config", "config_fields.go": "config",
	"config_store.go": "config", "hostname.go": "config", "version.go": "config",

	"markdown.go": "render", "templates.go": "render", "pages.go": "render",
	"render_cache.go": "render", "tags.go": "render",
	"editor_page.go": "render", "json_response.go": "render",
	"storage_layout.go": "storage", "assets.go": "storage",
	"note_files.go": "storage", "paths.go": "storage",

	"files_index.go": "files", "files_page.go": "files", "files_state.go": "files",
	"note_exchange.go": "exchange", "note_exchange_http.go": "exchange",
	"status.go": "status", "status_collect.go": "status", "status_render.go": "status",
	"search.go": "search", "search_document.go": "search",
	"search_http.go": "search", "search_index.go": "search",
	"search_index_build.go": "search", "search_page.go": "search",
	"search_score.go": "search", "search_sections.go": "search",
	"git_commit.go": "gitsync", "git_fs.go": "gitsync",
	"git_handlers.go": "gitsync", "git_pull.go": "gitsync",
	"git_push.go": "gitsync", "git_repo.go": "gitsync",
	"git_sync.go": "gitsync", "host_keys.go": "gitsync",
	"sqlite.go": "db", "db_backup.go": "db", "db_backup_create.go": "db",
	"db_backup_http.go": "db", "db_backup_restore.go": "db",

	"server.go": "app", "middleware.go": "app", "session.go": "app",
	"request_guard.go": "app", "handlers.go": "app", "page_access.go": "app",
	"note_handlers.go": "app", "config_handlers.go": "app",
	"config_page.go": "app", "upload_handlers.go": "app",
	"serving.go": "app", "storage.go": "app",
}

// declGroups names a declaration that sits in the file of another group.
// The two embed variables belong to the frontend group. They stay in
// server.go until the split.
var declGroups = map[string]string{
	"staticFS":    "frontend",
	"templatesFS": "frontend",
}

// groupLayers gives the layer of each group. A group can use a group of a
// lower layer only. Two groups of the same layer cannot use each other. Thus
// the groups have no cycle.
var groupLayers = map[string]int{
	"logx": 0, "textmatch": 0, "noteheader": 0, "frontend": 0,
	"config": 1,
	"render": 2, "storage": 2,
	"files": 3, "exchange": 3, "status": 3, "search": 3, "gitsync": 3, "db": 3,
	"app": 4,
}

// knownGroupLinks lists each link that breaks the layer table today. An
// entry is "file uses name". The list must shrink to empty. Do not add an
// entry. Remove the link, or connect the two groups with a hook.
var knownGroupLinks = []string{
	"config.go uses configFilename",
	"config.go uses fallbackPort",
	"config.go uses file",
	"config.go uses layout",
	"files_index.go uses itoa",
	"files_page.go uses itoa",
	"files_state.go uses isLocalOnlyPath",
	"files_state.go uses resolveContentType",
	"logger.go uses Config",
	"logger.go uses loadTemplate",
	"logger.go uses normalizeLogTags",
	"logger.go uses pageHeader",
	"logger.go uses renderPage",
	"logger.go uses writeJSON",
	"markdown.go uses hasKnownAssetExtension",
	"note_exchange_http.go uses maxUploadBytes",
	"paths.go uses hasKnownAssetExtension",
	"paths.go uses pageHTMLPath",
	"render_cache.go uses containedName",
	"render_cache.go uses html",
	"render_cache.go uses layout",
	"status.go uses fallbackPort",
	"status_collect.go uses ActiveConnCount",
	"status_collect.go uses Chroot",
	"status_collect.go uses NoLockFS",
	"status_collect.go uses boundAddress",
	"status_collect.go uses redactGitURL",
	"status_collect.go uses slotRemoteName",
	"status_collect.go uses stableMtimeFS",
	"tags.go uses layout",
	"tags.go uses md",
}

// groupLinkUses answers each use of a package-level name in another file, as
// "file uses name", with the file of the name. It type-checks the production
// files. A package outside the standard library becomes an empty package.
// The check then reports errors for it, and the scan ignores them. A name of
// this package still resolves.
func groupLinkUses(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: stdlibOnly{importer.Default()}, Error: func(error) {}}
	pkg, _ := conf.Check("net.basov.omngo/backend", fset, files, info)

	// The App struct holds the state of each group until Phase 3. A use of
	// App or of one of its fields is thus no link.
	app := pkg.Scope().Lookup("App")
	uses := map[string]string{}
	for id, obj := range info.Uses {
		if obj == nil || obj.Pkg() != pkg || obj == app {
			continue
		}
		if v, ok := obj.(*types.Var); ok && v.IsField() {
			continue
		}
		if _, ok := obj.(*types.Func); !ok && obj.Parent() != pkg.Scope() {
			continue
		}
		from := fset.Position(id.Pos()).Filename
		to := fset.Position(obj.Pos()).Filename
		if from != to {
			uses[filepath.Base(from)+" uses "+obj.Name()] = filepath.Base(to)
		}
	}
	return uses
}

// stdlibOnly imports each package of the standard library, and an empty
// package for each other path. The scan thus needs no module download.
type stdlibOnly struct{ std types.Importer }

func (s stdlibOnly) Import(path string) (*types.Package, error) {
	if !strings.Contains(strings.Split(path, "/")[0], ".") {
		return s.std.Import(path)
	}
	p := types.NewPackage(path, filepath.Base(path))
	p.MarkComplete()
	return p, nil
}

// groupOf answers the group of a use: the group of its declaration name, or
// of its file.
func groupOf(file, name string) string {
	if g, ok := declGroups[name]; ok {
		return g
	}
	return fileGroups[file]
}

func TestEachFileHasAGroup(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		g, ok := fileGroups[n]
		if !ok {
			t.Errorf("%s has no group. Add it to fileGroups.", n)
			continue
		}
		if _, ok := groupLayers[g]; !ok {
			t.Errorf("the group %q of %s has no layer in groupLayers", g, n)
		}
	}
	for n := range fileGroups {
		if _, err := os.Stat(n); err != nil {
			t.Errorf("fileGroups names %s, and the file does not exist", n)
		}
	}
}

// A group uses only groups of a lower layer. The test fails on a new link,
// and it fails when a known link is gone and its entry stays.
func TestGroupsUseOnlyLowerLayers(t *testing.T) {
	uses := groupLinkUses(t)
	if uses["config_handlers.go uses renderPage"] != "pages.go" {
		t.Fatal("the scan did not find a use of renderPage. The scan is broken.")
	}
	var found []string
	for use, to := range uses {
		from, name, _ := strings.Cut(use, " uses ")
		gFrom, gTo := fileGroups[from], groupOf(to, name)
		if gFrom == gTo || groupLayers[gTo] < groupLayers[gFrom] {
			continue
		}
		found = append(found, use)
		if !slices.Contains(knownGroupLinks, use) {
			t.Errorf("%s of %s. The group %s (layer %d) cannot use the group %s (layer %d). "+
				"Remove the link, or connect the two groups with a hook.",
				use, to, gFrom, groupLayers[gFrom], gTo, groupLayers[gTo])
		}
	}
	for _, k := range knownGroupLinks {
		if !slices.Contains(found, k) {
			t.Errorf("the link %q is gone. Remove it from knownGroupLinks.", k)
		}
	}
}
