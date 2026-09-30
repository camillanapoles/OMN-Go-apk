package search

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

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/textmatch"
)

const (
	// indexStaleCheckEvery limits how often a query pays for the stat walk.
	// At 10 000 files, the walk took 24 ms, too much for each keystroke. A
	// write inside the process does not wait. See MarkDirty. The
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
	if !textmatch.MaskRejects(t.mask, d.FieldMask) {
		return true
	}
	for _, m := range d.LineMasks {
		if !textmatch.MaskRejects(t.mask, m) {
			return true
		}
	}
	return d.couldMatchTypo(t.runes)
}

// couldMatchTypo applies the trigram bound. A term of length L within k edits
// still shares at least L-2-3k of its trigrams with the text.
func (d *indexedDoc) couldMatchTypo(term []rune) bool {
	k := textmatch.TypoBudget(len(term))
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
// edit through the app does not use the stamp. See EnsureIndex.
type indexStamp struct {
	files  int
	newest time.Time
	bytes  int64
}

type Index struct {
	mu        sync.RWMutex
	docs      map[string]*indexedDoc
	stamp     indexStamp
	checked   time.Time // when stamp was last verified against disk
	built     time.Time // when the current contents were assembled
	dirty     bool      // an in-process write happened; re-check without waiting
	lines     int
	bytes     int64
	kinds     string // the SearchKinds the current contents were built for
	bundled   bool   // ... and the SearchBundled setting
	commonSet map[string]bool
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
		if textmatch.IsWordRune(r) {
			w = append(w, r)
			continue
		}
		flush()
	}
	flush()
}

// CommonWords answers the set of words that carry little in this collection.
// It answers nil when no index exists, and cutSnippets then counts no word as
// common.
func (ix *Index) CommonWords() map[string]bool {
	if ix == nil {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.commonSet
}

// connectGroups sets MarkDirty as the hook onPageWritten.
// renderAndCache writes each change of a note inside the process, and it
// calls the hook. Examples are a save, a quick note, a bookmark, a sync and a
// precompile.
func (ix *Index) MarkDirty() {
	if ix == nil {
		return
	}
	ix.mu.Lock()
	ix.dirty = true
	ix.mu.Unlock()
}

// DropIndex releases the index and its memory when a person turns
// global search off. A device that is short of memory needs that at once, and
// not after a restart.
func (svc Service) DropIndex() {
	if svc.Index == nil {
		return
	}
	svc.Index.mu.Lock()
	had := len(svc.Index.docs)
	svc.Index.docs = nil
	svc.Index.commonSet = nil
	svc.Index.lines = 0
	svc.Index.bytes = 0
	svc.Index.built = time.Time{}
	svc.Index.mu.Unlock()
	if had > 0 {
		svc.Log(logx.Search).Infof("index dropped (%d documents released)", had)
	}
}

// Built reports whether an index exists. GlobalAvailable
// reads it, thus the dialog never offers a scope that this server cannot
// serve.
func (ix *Index) Built() bool {
	if ix == nil {
		return false
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.docs != nil
}

// IndexStatus is the line on the Config page. A person must see how
// much memory the index uses before the person turns it off.
func (svc Service) IndexStatus() string {
	cfg := svc.Config
	if !cfg.SearchEnabled {
		return "Off - page search still works, and costs nothing."
	}
	if svc.Index == nil {
		return "Not built yet."
	}
	svc.Index.mu.RLock()
	defer svc.Index.mu.RUnlock()
	if svc.Index.docs == nil {
		return "Not built yet."
	}
	// The measured memory is about 0.53 times the indexed text.
	resident := float64(svc.Index.bytes) / (1 << 20) * 0.53
	return fmtIndexStatus(len(svc.Index.docs), svc.Index.lines,
		float64(svc.Index.bytes)/(1<<20), resident, svc.Index.built)
}

// Stats is a copy of the counters of the index, for the Status page.
type Stats struct {
	Docs    int
	Lines   int
	Bytes   int64
	Dirty   bool
	Built   time.Time
	Checked time.Time
	// BytesEstimate is an ESTIMATE of the memory of the index. Go cannot
	// measure a live object graph. The count covers one 8-byte mask for each
	// line, the 64-byte signature and the strings. A flat value covers the
	// struct and its map entry.
	BytesEstimate int64
}

// Stats answers the counters of the index under the read lock. A nil index
// answers zero values.
func (ix *Index) Stats() Stats {
	if ix == nil {
		return Stats{}
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	st := Stats{
		Docs:    len(ix.docs),
		Lines:   ix.lines,
		Bytes:   ix.bytes,
		Dirty:   ix.dirty,
		Built:   ix.built,
		Checked: ix.checked,
	}
	const perDocOverhead = 160
	for path, doc := range ix.docs {
		st.BytesEstimate += int64(perDocOverhead + len(path))
		st.BytesEstimate += int64(len(doc.Path) + len(doc.Kind) + len(doc.Name) + len(doc.Title) + len(doc.URL))
		for _, t := range doc.Tags {
			st.BytesEstimate += int64(len(t) + 16)
		}
		st.BytesEstimate += int64(8 * len(doc.LineMasks))
		st.BytesEstimate += 8 + 64 // FieldMask + Tri
	}
	return st
}

// MarkChecked records a check of the files at time at, and it clears the
// dirty flag. The tests use it to put the index inside or outside the window
// of indexStaleCheckEvery.
func (ix *Index) MarkChecked(at time.Time) {
	ix.mu.Lock()
	ix.checked = at
	ix.dirty = false
	ix.mu.Unlock()
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

// Docs answers the documents under a read lock. A query then reads
// files without the lock.
func (ix *Index) Docs() []*indexedDoc {
	if ix == nil {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make([]*indexedDoc, 0, len(ix.docs))
	for _, d := range ix.docs {
		out = append(out, d)
	}
	return out
}

// StoragePath maps a storage-relative path of a document to disk.
func (svc Service) StoragePath(rel string) string {
	return svc.Layout.File(filepath.FromSlash(rel))
}

// ReloadDocument reads an indexed document again, and answers its full search
// form. It answers nil when the file is gone, thus a deleted note does not
// fail a query.
func (svc Service) ReloadDocument(d *indexedDoc) *searchDocument {
	data, truncated, err := readCapped(svc.StoragePath(d.Path), maxIndexFileBytes)
	if err != nil {
		if !os.IsNotExist(err) {
			svc.Log(logx.Search).Errf("%s: %v", d.Path, err)
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
