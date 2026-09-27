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
