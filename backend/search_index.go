package backend

// ----------------------------------------------------------------------
// The global search index
// ----------------------------------------------------------------------
//
// The index does not hold the notes. For each file it keeps the metadata of a
// result (path, title, tags), one 64-bit character mask for each line, and a
// 512-bit trigram signature. A query reads the text again from disk, for the
// few documents that it reaches. TestIndexHoldsNoText holds the rule.
//
// A measurement at about 10 000 notes and 25 MB chose this shape:
//
//	raw + folded text + mask for each line ...  2.35x the notes
//	folded text + mask for each line .........  1.90x the notes
//	masks only, text read on demand ..........  0.53x the notes   <- this
//
// A token dictionary for each document grew with the vocabulary, at about 44
// thousand tokens for each MB. The trigram signature does the same job for
// the typo rung in 64 bytes for each document. See scoreTypoInDocument.
//
// The cost is a disk read for each candidate document. A note is small, and
// the page cache of the OS serves the next query. The masks keep the set of
// candidates small.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// indexStaleCheckEvery limits how often a query pays for the stat walk.
	// At 10 000 files, the walk took 24 ms, too much for each keystroke. A
	// write inside the process does not wait. See markSearchIndexDirty. The
	// limit thus only delays an edit from an external editor or a git
	// checkout.
	indexStaleCheckEvery = 2 * time.Second

	// maxIndexBytes limits the TEXT that the index reads. The memory is about
	// half of it. The limit is about 2.5 times the expected size, thus a hit
	// means that something unexpected is in the notes directory.
	maxIndexBytes = 64 << 20
)

// indexedDoc is one document as the index keeps it: enough to reject a query
// and to describe a result, but not the text.
type indexedDoc struct {
	Path      string // storage-relative, slash form
	Kind      string
	Name      string
	Title     string
	Tags      []string
	URL       string
	Bundled   bool
	ModTime   time.Time
	Size      int64
	Truncated bool

	// FieldMask covers the title, the tags, the path and the header.
	// LineMasks has one entry for each content line that is not empty. A term
	// can match a field OR a line, thus the two stay separate.
	FieldMask uint64
	LineMasks []uint64

	// Tri is the 512-bit trigram signature of the whole document. Only the
	// typo rung uses it, because a character mask cannot filter a typo.
	Tri [8]uint64
}

// couldMatchTerm reports whether this document can satisfy one term, and it
// reads no byte of the file. False means "certainly not", and that keeps the
// candidate set small. The mask test gives no false negative for rungs 1 and
// 2, because both need each rune. The trigram test covers the typo rung.
func (d *indexedDoc) couldMatchTerm(t queryTerm) bool {
	if !maskRejects(t.mask, d.FieldMask) {
		return true
	}
	for _, m := range d.LineMasks {
		if !maskRejects(t.mask, m) {
			return true
		}
	}
	return d.couldMatchTypo(t.runes)
}

// couldMatchTypo applies the trigram bound. A term of length L within k edits
// still shares at least L-2-3k of its trigrams with the text.
func (d *indexedDoc) couldMatchTypo(term []rune) bool {
	k := typoBudget(len(term))
	if k == 0 {
		return false
	}
	tris := trigrams(term)
	need := len(term) - 2 - 3*k
	if need <= 0 {
		return true // too short for the bound to say anything useful
	}
	found := 0
	for _, h := range tris {
		if d.Tri[(h>>6)&7]&(1<<(h&63)) != 0 {
			found++
			if found >= need {
				return true
			}
		}
	}
	return false
}

// trigrams hashes each 3-rune window of s with a cheap rolling hash. A
// collision costs one extra document read, because the signature only
// REJECTS.
func trigrams(s []rune) []uint32 {
	if len(s) < 3 {
		return nil
	}
	out := make([]uint32, 0, len(s)-2)
	for i := 0; i+3 <= len(s); i++ {
		out = append(out, uint32(s[i])*961+uint32(s[i+1])*31+uint32(s[i+2]))
	}
	return out
}

// indexStamp is the cheap fingerprint of the files on disk. It holds the
// count, the newest mtime of the files AND of their directories, and the
// total size.
//
// An add, a delete or a rename changes the mtime of the directory, and maybe
// of no file. Some filesystems round an mtime to the second, and the external
// media of Android is one of them. Two edits in one second thus keep the same
// newest mtime, but the size changes for almost each real edit. An external
// edit that keeps the size in the same second waits for the next change. An
// edit through the app does not use the stamp. See ensureSearchIndex.
type indexStamp struct {
	files  int
	newest time.Time
	bytes  int64
}

type searchIndex struct {
	mu      sync.RWMutex
	docs    map[string]*indexedDoc
	stamp   indexStamp
	checked time.Time // when stamp was last verified against disk
	built   time.Time // when the current contents were assembled
	dirty   bool      // an in-process write happened; re-check without waiting
	lines   int
	bytes   int64
	kinds   string // the SearchKinds the current contents were built for
	bundled bool   // ... and the SearchBundled setting
	common  map[string]bool
}

// ----------------------------------------------------------------------
// The common words of this collection
// ----------------------------------------------------------------------
//
// A query of five words with "the" two times gives rows that say nothing.
// cutSnippets skips a term of ONE rune. A rule by length cannot go further,
// because "cat", "git" and "log" are real queries. The measure is thus how
// common a word is. See
// doc/decisions/0009-show-only-the-search-rows-that-carry-a-word-of-the-query.md.
//
// commonWordShare is the share of the documents above which a word carries
// little. It is a half. A collection of 66 documents had 4536 words, and 22
// of them were above the half. The index keeps only these words.
//
// The rebuild counts each word, and that large map lives only during the
// rebuild. The set is relative to the collection. When one subject fills the
// notes, its words go above the line, and cutSnippets keeps the window. The
// worst case is thus the same as no rule.
const commonWordShare = 2 // one part in two, thus above a half

// indexWords adds each word of one folded string to a set. It is not
// tokenize, because tokenize drops a short word, and "the", "of" and "to" are
// the words that this set needs.
func indexWords(rs []rune, into map[string]bool) {
	var w []rune
	flush := func() {
		if len(w) > 0 {
			into[string(w)] = true
			w = w[:0]
		}
	}
	for _, r := range rs {
		if isWordRune(r) {
			w = append(w, r)
			continue
		}
		flush()
	}
	flush()
}

// commonWords answers the set of words that carry little in this collection.
// It answers nil when no index exists, and cutSnippets then counts no word as
// common.
func (a *App) commonWords() map[string]bool {
	if a.search == nil {
		return nil
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	return a.search.common
}

// renderAndCache calls markSearchIndexDirty. Each change of a note inside the
// process goes through that one writer. Examples are a save, a quick note, a
// bookmark, a sync and a precompile.
func (a *App) markSearchIndexDirty() {
	if a.search == nil {
		return
	}
	a.search.mu.Lock()
	a.search.dirty = true
	a.search.mu.Unlock()
}

// dropSearchIndex releases the index and its memory when a person turns
// global search off. A device that is short of memory needs that at once, and
// not after a restart.
func (a *App) dropSearchIndex() {
	if a.search == nil {
		return
	}
	a.search.mu.Lock()
	had := len(a.search.docs)
	a.search.docs = nil
	a.search.common = nil
	a.search.lines = 0
	a.search.bytes = 0
	a.search.built = time.Time{}
	a.search.mu.Unlock()
	if had > 0 {
		a.logInfof(logSearch, "index dropped (%d documents released)", had)
	}
}

// searchIndexBuilt reports whether an index exists. globalSearchAvailable
// reads it, thus the dialog never offers a scope that this server cannot
// serve.
func (a *App) searchIndexBuilt() bool {
	if a.search == nil {
		return false
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	return a.search.docs != nil
}

// searchIndexStatus is the line on the Config page. A person must see how
// much memory the index uses before the person turns it off.
func (a *App) searchIndexStatus() string {
	cfg := a.GetConfig()
	if !cfg.SearchEnabled {
		return "Off - page search still works, and costs nothing."
	}
	if a.search == nil {
		return "Not built yet."
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	if a.search.docs == nil {
		return "Not built yet."
	}
	// The measured memory is about 0.53 times the indexed text.
	resident := float64(a.search.bytes) / (1 << 20) * 0.53
	return fmtIndexStatus(len(a.search.docs), a.search.lines,
		float64(a.search.bytes)/(1<<20), resident, a.search.built)
}

func fmtIndexStatus(docs, lines int, mb, residentMB float64, built time.Time) string {
	var b strings.Builder
	b.WriteString(plural(docs, "document", "documents"))
	b.WriteString(", ")
	b.WriteString(plural(lines, "line", "lines"))
	b.WriteString(", ")
	b.WriteString(oneDecimal(mb))
	b.WriteString(" MB indexed, about ")
	b.WriteString(oneDecimal(residentMB))
	b.WriteString(" MB in memory")
	if !built.IsZero() {
		b.WriteString(", built ")
		b.WriteString(built.Format("15:04:05"))
	}
	b.WriteString(".")
	return b.String()
}

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
	if want[SearchKindMD] || want[SearchKindBookmarks] {
		roots = append(roots, searchRoot{
			kind: SearchKindMD,
			dir:  filepath.Join(a.StorageDir, "md"),
			exts: []string{".md"},
		})
	}
	if want[SearchKindJS] {
		roots = append(roots, searchRoot{
			kind: SearchKindJS,
			dir:  filepath.Join(a.StorageDir, "html", "js"),
			exts: []string{".js"},
		})
	}
	if want[SearchKindJSON] {
		roots = append(roots, searchRoot{
			kind: SearchKindJSON,
			dir:  filepath.Join(a.StorageDir, "html", "json"),
			exts: []string{".json"},
		})
	}
	if want[SearchKindUserJSON] {
		roots = append(roots, searchRoot{
			kind: SearchKindUserJSON,
			dir:  filepath.Join(a.StorageDir, "html", "user_json"),
			exts: []string{".json", ".jsonl"},
		})
	}
	return roots
}

// rebuildSearchIndex walks the enabled roots and replaces the index. It makes
// the new map first, and swaps it in under the write lock. A query thus sees
// the whole old index or the whole new one.
func (a *App) rebuildSearchIndex() {
	cfg := a.GetConfig()
	if !cfg.SearchEnabled {
		a.dropSearchIndex()
		return
	}
	if a.search == nil {
		return
	}

	started := time.Now()
	kinds := normalizeSearchKinds(cfg.SearchKinds)
	wantBookmarks := false
	wantMD := false
	for _, k := range kinds {
		switch k {
		case SearchKindBookmarks:
			wantBookmarks = true
		case SearchKindMD:
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
				if root.kind == SearchKindMD && p == filepath.Join(root.dir, "local") {
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
			if root.kind == SearchKindMD {
				name := strings.TrimSuffix(rel, ".md")
				switch name {
				case "OMNGoTags":
					return nil // generated FROM the notes; indexing it duplicates them
				case "Bookmarks":
					if !wantBookmarks {
						return nil
					}
					kind = SearchKindBookmarks
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
		a.logErrf(logSearch, "index capped at %d MB of text; some files were left out", maxIndexBytes>>20)
	}
	a.logInfof(logSearch, "Indexed %d files (%d lines, %.1f MB) in %s",
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
	if rootKind == SearchKindMD {
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
		out.FieldMask |= runeMask(f.text)
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
	if rootKind != SearchKindJS && rootKind != SearchKindJSON {
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
				if root.kind == SearchKindMD && p == filepath.Join(root.dir, "local") {
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
	cfg := a.GetConfig()
	if !cfg.SearchEnabled || a.search == nil {
		return false
	}
	kinds := strings.Join(normalizeSearchKinds(cfg.SearchKinds), ",")

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

	fresh := a.searchStamp(normalizeSearchKinds(cfg.SearchKinds))
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

// snapshotDocs answers the documents under a read lock. A query then reads
// files without the lock.
func (a *App) snapshotDocs() []*indexedDoc {
	if a.search == nil {
		return nil
	}
	a.search.mu.RLock()
	defer a.search.mu.RUnlock()
	out := make([]*indexedDoc, 0, len(a.search.docs))
	for _, d := range a.search.docs {
		out = append(out, d)
	}
	return out
}

// storagePath maps a storage-relative path of a document to disk.
func (a *App) storagePath(rel string) string {
	return filepath.Join(a.StorageDir, filepath.FromSlash(rel))
}

// reloadDocument reads an indexed document again, and answers its full search
// form. It answers nil when the file is gone, thus a deleted note does not
// fail a query.
func (a *App) reloadDocument(d *indexedDoc) *searchDocument {
	data, truncated, err := readCapped(a.storagePath(d.Path), maxIndexFileBytes)
	if err != nil {
		if !os.IsNotExist(err) {
			a.logErrf(logSearch, "%s: %v", d.Path, err)
		}
		return nil
	}
	if isBinary(data) {
		return nil
	}
	if strings.HasPrefix(d.Path, "md/") {
		doc := newMarkdownDocument(strings.TrimSuffix(strings.TrimPrefix(d.Path, "md/"), ".md"),
			string(data), truncated)
		doc.Kind = d.Kind
		return doc
	}
	return newAssetDocument(strings.TrimPrefix(d.Path, "html/"), string(data), truncated)
}

func plural(n int, one, many string) string {
	word := many
	if n == 1 {
		word = one
	}
	return itoa(n) + " " + word
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func oneDecimal(f float64) string {
	if f < 0 {
		return "0.0"
	}
	whole := int(f)
	frac := int((f-float64(whole))*10 + 0.5)
	if frac >= 10 {
		whole++
		frac = 0
	}
	return itoa(whole) + "." + itoa(frac)
}
