package backend

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/textmatch"
)

// searchRoot tells where one kind of file lives.
type searchRoot struct {
	kind string
	dir  string // absolute
	exts []string
}

func (a *App) searchRoots(kinds []string) []searchRoot {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var roots []searchRoot
	if want[config.SearchKindMD] || want[config.SearchKindBookmarks] {
		roots = append(roots, searchRoot{
			kind: config.SearchKindMD,
			dir:  a.layout().md(),
			exts: []string{".md"},
		})
	}
	if want[config.SearchKindJS] {
		roots = append(roots, searchRoot{
			kind: config.SearchKindJS,
			dir:  a.layout().html("js"),
			exts: []string{".js"},
		})
	}
	if want[config.SearchKindJSON] {
		roots = append(roots, searchRoot{
			kind: config.SearchKindJSON,
			dir:  a.layout().html("json"),
			exts: []string{".json"},
		})
	}
	if want[config.SearchKindUserJSON] {
		roots = append(roots, searchRoot{
			kind: config.SearchKindUserJSON,
			dir:  a.layout().html("user_json"),
			exts: []string{".json", ".jsonl"},
		})
	}
	return roots
}

// rebuildSearchIndex walks the enabled roots and replaces the index. It makes
// the new map first, and swaps it in under the write lock. A query thus sees
// the whole old index or the whole new one.
func (a *App) rebuildSearchIndex() {
	cfg := a.config.Get()
	if !cfg.SearchEnabled {
		a.dropSearchIndex()
		return
	}
	if a.search == nil {
		return
	}

	started := time.Now()
	kinds := config.NormalizeSearchKinds(cfg.SearchKinds)
	wantBookmarks := false
	wantMD := false
	for _, k := range kinds {
		switch k {
		case config.SearchKindBookmarks:
			wantBookmarks = true
		case config.SearchKindMD:
			wantMD = true
		}
	}

	docs := map[string]*indexedDoc{}
	docFreq := map[string]int{}
	var lines int
	var bytes int64
	capped := false

	for _, root := range a.searchRoots(kinds) {
		_ = filepath.WalkDir(root.dir, func(p string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if e.IsDir() {
				// md/local is the ignored scratch tree, and buildTagIndex
				// skips it too.
				if root.kind == config.SearchKindMD && p == filepath.Join(root.dir, "local") {
					return fs.SkipDir
				}
				return nil
			}
			if !hasExt(e.Name(), root.exts) {
				return nil
			}
			rel, err := filepath.Rel(root.dir, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)

			kind := root.kind
			if root.kind == config.SearchKindMD {
				name := strings.TrimSuffix(rel, ".md")
				switch name {
				case "OMNGoTags":
					return nil // generated FROM the notes; indexing it duplicates them
				case "Bookmarks":
					if !wantBookmarks {
						return nil
					}
					kind = config.SearchKindBookmarks
				default:
					if !wantMD {
						return nil
					}
				}
			}

			bundled := isBundledAsset(root.kind, rel)
			if bundled && !cfg.SearchBundled {
				return nil
			}
			if bytes >= maxIndexBytes {
				capped = true
				return nil
			}

			info, err := e.Info()
			if err != nil {
				return nil
			}
			doc := a.indexFile(root.kind, kind, rel, p, info, bundled, docFreq)
			if doc == nil {
				return nil
			}
			docs[doc.Path] = doc
			lines += len(doc.LineMasks)
			bytes += doc.Size
			return nil
		})
	}

	// Keep only the words above the line. See commonWordShare.
	common := map[string]bool{}
	for w, n := range docFreq {
		if n*commonWordShare > len(docs) {
			common[w] = true
		}
	}

	stamp := a.searchStamp(kinds)

	a.search.mu.Lock()
	a.search.docs = docs
	a.search.common = common
	a.search.lines = lines
	a.search.bytes = bytes
	a.search.stamp = stamp
	a.search.checked = time.Now()
	a.search.built = time.Now()
	a.search.dirty = false
	a.search.kinds = strings.Join(kinds, ",")
	a.search.bundled = cfg.SearchBundled
	a.search.mu.Unlock()

	if capped {
		a.log(logx.Search).Errf("index capped at %d MB of text; some files were left out", maxIndexBytes>>20)
	}
	a.log(logx.Search).Infof("Indexed %d files (%d lines, %.1f MB) in %s",
		len(docs), lines, float64(bytes)/(1<<20), time.Since(started).Round(time.Millisecond))
}

// indexFile reads one file and keeps what the index needs. docFreq counts the
// DOCUMENTS that hold a word. A word that appears twenty times in this file
// thus adds one.
func (a *App) indexFile(rootKind, kind, rel, path string, info fs.FileInfo, bundled bool, docFreq map[string]int) *indexedDoc {
	data, truncated, err := readCapped(path, maxIndexFileBytes)
	if err != nil || isBinary(data) {
		return nil
	}

	var doc *searchDocument
	if rootKind == config.SearchKindMD {
		doc = newMarkdownDocument(strings.TrimSuffix(rel, ".md"), string(data), truncated)
		doc.Kind = kind // Bookmarks.md is its own kind
	} else {
		doc = newAssetDocument(rootKind+"/"+rel, string(data), truncated)
	}

	out := &indexedDoc{
		Path:      doc.Path,
		Kind:      doc.Kind,
		Name:      doc.Name,
		Title:     doc.Title,
		Tags:      doc.Tags,
		URL:       doc.URL,
		Bundled:   bundled,
		ModTime:   info.ModTime(),
		Size:      int64(len(data)),
		Truncated: truncated,
		LineMasks: make([]uint64, 0, len(doc.lines)),
	}
	for _, f := range doc.fields {
		out.FieldMask |= textmatch.RuneMask(f.text)
		addTrigrams(&out.Tri, f.text)
	}
	for i := range doc.lines {
		out.LineMasks = append(out.LineMasks, doc.lines[i].mask)
		addTrigrams(&out.Tri, doc.lines[i].fold)
	}

	if docFreq != nil {
		words := map[string]bool{}
		for _, f := range doc.fields {
			indexWords(f.text, words)
		}
		for i := range doc.lines {
			indexWords(doc.lines[i].fold, words)
		}
		for w := range words {
			docFreq[w]++
		}
	}
	return out
}

func addTrigrams(sig *[8]uint64, s []rune) {
	for _, h := range trigrams(s) {
		sig[(h>>6)&7] |= 1 << (h & 63)
	}
}

// isBundledAsset reports whether OMN-Go ships a file. It reads
// versionDependentAssets in assets.go, thus a new bundled file needs no
// second list.
func isBundledAsset(rootKind, rel string) bool {
	if rootKind != config.SearchKindJS && rootKind != config.SearchKindJSON {
		return false
	}
	full := "html/" + rootKind + "/" + rel
	for _, v := range versionDependentAssets {
		if v == full {
			return true
		}
	}
	base := filepath.Base(rel)
	return strings.Contains(base, ".min.")
}

func hasExt(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	return false
}

// searchStamp walks with stat only. It opens no file.
func (a *App) searchStamp(kinds []string) indexStamp {
	var st indexStamp
	consider := func(e fs.DirEntry, countSize bool) {
		info, err := e.Info()
		if err != nil {
			return
		}
		if info.ModTime().After(st.newest) {
			st.newest = info.ModTime()
		}
		if countSize {
			st.bytes += info.Size()
		}
	}
	for _, root := range a.searchRoots(kinds) {
		_ = filepath.WalkDir(root.dir, func(p string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if e.IsDir() {
				if root.kind == config.SearchKindMD && p == filepath.Join(root.dir, "local") {
					return fs.SkipDir
				}
				consider(e, false) // a rename shows up here and nowhere else
				return nil
			}
			if !hasExt(e.Name(), root.exts) {
				return nil
			}
			st.files++
			consider(e, true)
			return nil
		})
	}
	return st
}

// ensureSearchIndex runs before each global query. It builds the index when
// none exists. It checks the files at most once for each
// indexStaleCheckEvery, unless a write inside the process marked the index
// dirty.
func (a *App) ensureSearchIndex() bool {
	cfg := a.config.Get()
	if !cfg.SearchEnabled || a.search == nil {
		return false
	}
	kinds := strings.Join(config.NormalizeSearchKinds(cfg.SearchKinds), ",")

	a.search.mu.RLock()
	built := a.search.docs != nil
	checked := a.search.checked
	dirty := a.search.dirty
	stamp := a.search.stamp
	sameSettings := a.search.kinds == kinds && a.search.bundled == cfg.SearchBundled
	a.search.mu.RUnlock()

	// A change of the settings changes what the index covers. Rebuild it,
	// whatever the file times say.
	if !built || !sameSettings {
		a.rebuildSearchIndex()
		return a.searchIndexBuilt()
	}
	// A write inside the process changed the CONTENT, and the process knows
	// it. A stat check would add nothing, and it can be wrong. An mtime can
	// have a precision of one second, thus the stamp can miss an edit.
	if dirty {
		a.rebuildSearchIndex()
		return a.searchIndexBuilt()
	}
	if time.Since(checked) < indexStaleCheckEvery {
		return true
	}

	fresh := a.searchStamp(config.NormalizeSearchKinds(cfg.SearchKinds))
	if fresh == stamp {
		a.search.mu.Lock()
		a.search.checked = time.Now()
		a.search.dirty = false
		a.search.mu.Unlock()
		return true
	}
	a.rebuildSearchIndex()
	return a.searchIndexBuilt()
}
