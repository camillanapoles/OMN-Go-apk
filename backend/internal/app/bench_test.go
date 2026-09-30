package app

// ----------------------------------------------------------------------
// The benchmarks
// ----------------------------------------------------------------------
//
// These benchmarks measure the paths that a person waits for:
//
//   - the compile of a note, and the answer for a page,
//   - the search of one page, the global search and the index build,
//   - a SQL query, and a database backup with its restore.
//
// The refactor of the backend into packages must not make these paths
// slower. Run the benchmarks on master before a refactor patch and again
// after it. Then compare the two results.
//
//	go test -run '^$' -bench . -benchmem -count 5 ./backend/ > before.txt
//
// The normal gate does not run a benchmark. TestBenchmarkFixtures runs in
// the gate. It checks that each fixture below still gives the data that
// the benchmarks need. A benchmark that measures an empty corpus gives a
// fast number with no meaning.

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/search"
)

// benchCopies is the number of copies of the bundled notes in the search
// corpus. The bundled notes are about 20 files. With ten copies, the
// corpus is near the size of a real note tree.
const benchCopies = 10

// benchApp makes an application of a fresh install in a temporary
// directory. It is newTestApp for a benchmark.
func benchApp(tb testing.TB) *App {
	tb.Helper()
	a := &App{StorageDir: tb.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			tb.Fatal(err)
		}
	}
	a.loadConfig(a.layout().Config())
	a.Router = http.NewServeMux()
	a.registerRoutes(a.Router)
	return a
}

// benchNotes answers each bundled note, by its name without ".md".
func benchNotes(tb testing.TB) map[string][]byte {
	tb.Helper()
	notes := map[string][]byte{}
	err := fs.WalkDir(frontend.Static, "md", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		data, err := frontend.Static.ReadFile(p)
		if err != nil {
			return err
		}
		notes[strings.TrimSuffix(strings.TrimPrefix(p, "md/"), ".md")] = data
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return notes
}

// benchCorpus writes benchCopies copies of each bundled note into the md
// directory. Copy n of a note goes to copyN/<name>.md.
func benchCorpus(tb testing.TB, a *App) int {
	tb.Helper()
	count := 0
	for name, data := range benchNotes(tb) {
		for n := 0; n < benchCopies; n++ {
			p := filepath.Join(a.StorageDir, "md", fmt.Sprintf("copy%d", n), filepath.FromSlash(name)+".md")
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				tb.Fatal(err)
			}
			if err := os.WriteFile(p, data, 0644); err != nil {
				tb.Fatal(err)
			}
			count++
		}
	}
	return count
}

// benchSearchApp makes an application with the corpus and a built global
// search index.
func benchSearchApp(tb testing.TB) *App {
	tb.Helper()
	a := benchApp(tb)
	benchCorpus(tb, a)
	a.search = &search.Index{}
	a.config.Update(func(c *config.Config) {
		c.SearchEnabled = true
		c.SearchKinds = []string{config.SearchKindMD, config.SearchKindBookmarks}
	})
	a.rebuildSearchIndex()
	return a
}

// benchGet sends one GET through the route table and answers the
// recorder.
func benchGet(a *App, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.Router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// benchSQL sends one batch to /api/sql and answers the recorder.
func benchSQL(a *App, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/sql", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.handleSQL(rec, req)
	return rec
}

// benchDatabase makes the database "bench" with 2000 rows in table t.
func benchDatabase(tb testing.TB, a *App) {
	tb.Helper()
	if rec := benchSQL(a, `{"db":"bench","statements":[{"sql":"CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT, score REAL)"}]}`); rec.Code != http.StatusOK {
		tb.Fatalf("create table: %d %s", rec.Code, rec.Body.String())
	}
	var sb strings.Builder
	sb.WriteString(`{"db":"bench","statements":[`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"sql":"INSERT INTO t(name, score) VALUES(?, ?)","args":["name %d", %d.5]}`, i%17, i)
	}
	sb.WriteString(`]}`)
	for i := 0; i < 20; i++ {
		if rec := benchSQL(a, sb.String()); rec.Code != http.StatusOK {
			tb.Fatalf("insert: %d %s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkCompileBundledNotes compiles each bundled note to HTML in
// memory. This is the work of each save and of the first open of a note.
func BenchmarkCompileBundledNotes(b *testing.B) {
	a := benchApp(b)
	notes := benchNotes(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for name, data := range notes {
			rd := a.renderer()
			rd.CompilePage(name, data)
		}
	}
}

// BenchmarkServeCachedPage answers GET /UserManual.html from the cache on
// disk. This is the work of each open of a note after the first one.
func BenchmarkServeCachedPage(b *testing.B) {
	a := benchApp(b)
	if rec := benchGet(a, "/UserManual.html"); rec.Code != http.StatusOK {
		b.Fatalf("first GET: %d", rec.Code)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGet(a, "/UserManual.html")
	}
}

// BenchmarkSearchPage searches one note. This is the search of the page
// that is open, which needs no index.
func BenchmarkSearchPage(b *testing.B) {
	a := benchApp(b)
	q := "/api/search?" + url.Values{"q": {"sync conflict"}, "scope": {"page"}, "on": {"UserManual.html"}}.Encode()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGet(a, q)
	}
}

// BenchmarkSearchGlobal searches the whole corpus through the index. The
// query holds a typo, thus the fuzzy matcher also runs.
func BenchmarkSearchGlobal(b *testing.B) {
	a := benchSearchApp(b)
	q := "/api/search?" + url.Values{"q": {"databse backup"}, "scope": {"all"}}.Encode()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGet(a, q)
	}
}

// BenchmarkSearchIndexBuild builds the global index of the corpus. This
// is the work after the start of the application and after a pull.
func BenchmarkSearchIndexBuild(b *testing.B) {
	a := benchSearchApp(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.rebuildSearchIndex()
	}
}

// BenchmarkSQLQuery runs two SELECT statements over 2000 rows through
// /api/sql. A note script uses this path.
func BenchmarkSQLQuery(b *testing.B) {
	a := benchApp(b)
	benchDatabase(b, a)
	query := `{"db":"bench","statements":[` +
		`{"sql":"SELECT name, sum(score), count(*) FROM t GROUP BY name ORDER BY 2 DESC"},` +
		`{"sql":"SELECT * FROM t WHERE id % 7 = 3 LIMIT 200"}]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSQL(a, query)
	}
}

// BenchmarkDBBackupRestore makes a backup of 2000 rows and restores it.
//
// Each loop removes its backup at the end. The backup directory thus
// stays small, and each loop does the same work.
func BenchmarkDBBackupRestore(b *testing.B) {
	a := benchApp(b)
	benchDatabase(b, a)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rel, _, err := a.databases().CreateBackup("bench")
		if err != nil {
			b.Fatal(err)
		}
		file := path.Base(filepath.ToSlash(rel))
		err = a.databases().Restore("bench", file)
		if err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := os.Remove(filepath.Join(a.databases().BackupDir("bench"), file)); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

// TestBenchmarkFixtures checks the data of each benchmark in the normal
// gate. A change of a route, of the search or of the SQL answer can make
// a benchmark measure a fault page or an empty result. This test then
// fails, and the benchmark numbers stay true.
func TestBenchmarkFixtures(t *testing.T) {
	a := benchApp(t)
	if n := len(benchNotes(t)); n < 10 {
		t.Errorf("only %d bundled notes, want 10 or more", n)
	}
	if rec := benchGet(a, "/UserManual.html"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h") {
		t.Errorf("GET /UserManual.html: status %d, not a compiled page", rec.Code)
	}
	q := "/api/search?" + url.Values{"q": {"sync conflict"}, "scope": {"page"}, "on": {"UserManual.html"}}.Encode()
	if rec := benchGet(a, q); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"results":[{`) {
		t.Errorf("the page search has no result: %d %.200s", rec.Code, rec.Body.String())
	}

	s := benchSearchApp(t)
	q = "/api/search?" + url.Values{"q": {"databse backup"}, "scope": {"all"}}.Encode()
	if rec := benchGet(s, q); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"results":[{`) {
		t.Errorf("the global search has no result: %d %.200s", rec.Code, rec.Body.String())
	}

	benchDatabase(t, a)
	rec := benchSQL(a, `{"db":"bench","statements":[{"sql":"SELECT count(*) AS n FROM t"}]}`)
	if !strings.Contains(rec.Body.String(), "2000") {
		t.Errorf("the database does not hold 2000 rows: %s", rec.Body.String())
	}
}
