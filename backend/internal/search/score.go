package search

import "net.basov.omngo/backend/internal/textmatch"

// lineHit is one matching content line, with the spans of each term merged.
type lineHit struct {
	line  *docLine
	score int
	tier  textmatch.Tier
	spans []textmatch.Span
}

// scoreDocument applies AND: each term must hit the document. For each term,
// the best tier and weighted score wins. See textmatch.BetterMatch.
//
// THE PHRASE RUNG. A document that holds the whole query, in order and side
// by side in the folded text, gets textmatch.TierPhrase above each sum. A bonus cannot
// do that: five loose words in one title scored 2001, and the sentence scored
// 718. TestPhraseTierBeatsAHigherScore holds the rule. The loop counts the
// DISTINCT terms of each line and field, thus the phrase test reads few
// lines. A query with a field prefix is never a phrase.
func scoreDocument(q parsedQuery, d *searchDocument) (int, textmatch.Tier, []lineHit, bool) {
	if len(q.terms) == 0 {
		return 0, textmatch.TierNone, nil, false
	}

	total := 0
	worst := textmatch.TierSubstring // the document's tier is its WEAKEST term's tier
	hits := map[int]*lineHit{}

	// The phrase counters are a slice, because a map hash for each hit costs
	// a third of the scoring time.
	phraseWanted := len(q.terms) > 1
	for _, term := range q.terms {
		if term.field != "" {
			phraseWanted = false
			break
		}
	}
	var perLine, perField []uint16
	if phraseWanted {
		perLine = make([]uint16, len(d.lines))
		perField = make([]uint16, len(d.fields))
	}

	for _, term := range q.terms {
		bestScore, bestTier := 0, textmatch.TierNone

		for fi, f := range d.fields {
			if term.field != "" && term.field != f.name {
				continue
			}
			s, _, tier, ok := textmatch.ScoreTerm(term.runes, f.text)
			if !ok {
				continue
			}
			if phraseWanted && tier == textmatch.TierSubstring {
				perField[fi]++
			}
			weighted := s * f.weight / 10
			if textmatch.BetterMatch(tier, weighted, bestTier, bestScore) {
				bestScore, bestTier = weighted, tier
			}
		}

		if term.field == "" {
			for i := range d.lines {
				ln := &d.lines[i]
				if textmatch.MaskRejects(term.mask, ln.mask) {
					continue // no rune loop, no allocation
				}
				s, spans, tier, ok := textmatch.ScoreTerm(term.runes, ln.fold)
				if !ok {
					continue
				}
				if phraseWanted && tier == textmatch.TierSubstring {
					perLine[i]++
				}
				weighted := s * weightContent / 10
				if textmatch.BetterMatch(tier, weighted, bestTier, bestScore) {
					bestScore, bestTier = weighted, tier
				}
				h := hits[ln.no]
				if h == nil {
					h = &lineHit{line: ln}
					hits[ln.no] = h
				}
				h.spans = append(h.spans, spans...)
				// SUM the distinct terms of a line. The line with each term
				// is the line to show, also when another line matches one
				// term better.
				h.score += weighted
				if tier > h.tier {
					h.tier = tier // a line is only as good as its weakest term
				}
			}
		}

		if bestTier == textmatch.TierNone && term.field == "" {
			// Nothing matched as a substring or a subsequence. Try the typo
			// rung before the code drops the document. It finds "fetch" for
			// "fecth". It cannot use the mask, thus it runs last.
			if s, spans, ok := scoreTypoInDocument(term.runes, d); ok {
				bestScore, bestTier = s.score, textmatch.TierTypo
				for _, lh := range spans {
					h := hits[lh.line.no]
					if h == nil {
						h = &lineHit{line: lh.line}
						hits[lh.line.no] = h
					}
					h.spans = append(h.spans, lh.spans...)
					h.score += lh.score
					if lh.tier > h.tier {
						h.tier = lh.tier
					}
				}
			}
		}
		if bestTier == textmatch.TierNone {
			return 0, textmatch.TierNone, nil, false // AND: one miss drops the document
		}
		if bestTier > worst {
			worst = bestTier
		}
		total += bestScore
	}

	// Check the phrase now. Only a field or a line that took EACH term can
	// hold it, thus the check reads nothing else.
	if phraseWanted {
		full := uint16(len(q.terms))
		want := queryPhrase(q)
		found := false
		for fi, n := range perField {
			if n == full {
				if _, _, ok := textmatch.ScoreSubstring(want, d.fields[fi].text); ok {
					found = true
					break
				}
			}
		}
		for i, n := range perLine {
			if n != full {
				continue
			}
			if _, _, ok := textmatch.ScoreSubstring(want, d.lines[i].fold); ok {
				found = true
				// The line takes the rung too, thus the panel shows the line
				// that the reader typed first.
				if h := hits[d.lines[i].no]; h != nil {
					h.tier = textmatch.TierPhrase
				}
			}
		}
		if found {
			worst = textmatch.TierPhrase
		}
	}

	kw, ok := kindWeight[d.Kind]
	if !ok {
		kw = 100
	}
	total = total * kw / 100

	ordered := make([]lineHit, 0, len(hits))
	for _, h := range hits {
		h.spans = textmatch.MergeSpans(h.spans)
		ordered = append(ordered, *h)
	}
	sortLineHits(ordered)
	return total, worst, ordered, true
}

// queryPhrase joins the folded terms with one space. The result compares with
// the folded text of a field or a line.
func queryPhrase(q parsedQuery) []rune {
	out := make([]rune, 0, 32)
	for i, t := range q.terms {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, t.runes...)
	}
	return out
}

// typoResult is the best token match of a term in a document.
type typoResult struct {
	score int
	token string
}

// scoreTypoInDocument runs the edit-distance rung over the tokens of the
// document. The index keeps no tokens. See the banner of index.go. The
// spans mark the matched TOKEN, which is the real text, and not the
// misspelled query.
func scoreTypoInDocument(term []rune, d *searchDocument) (typoResult, []lineHit, bool) {
	if textmatch.TypoBudget(len(term)) == 0 {
		return typoResult{}, nil, false
	}

	best := typoResult{}
	bestWeight := 0
	seen := map[string]bool{}

	consider := func(text string, weight int) {
		for _, tok := range textmatch.Tokenize(text) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			s, _, ok := textmatch.ScoreTypo(term, []rune(tok))
			if !ok {
				continue
			}
			if weighted := s * weight / 10; weighted > bestWeight {
				bestWeight = weighted
				best = typoResult{score: weighted, token: tok}
			}
		}
	}

	for _, f := range d.fields {
		consider(string(f.text), f.weight)
	}
	for i := range d.lines {
		consider(d.lines[i].raw, weightContent)
	}
	if best.token == "" {
		return typoResult{}, nil, false
	}

	var hits []lineHit
	needle := []rune(best.token)
	for i := range d.lines {
		ln := &d.lines[i]
		if _, spans, ok := textmatch.ScoreSubstring(needle, ln.fold); ok {
			hits = append(hits, lineHit{line: ln, score: best.score, tier: textmatch.TierTypo, spans: spans})
		}
	}
	return best, hits, true
}

// sortLineHits orders by tier and score, and then by line number. The order
// is thus stable, and a tie reads from top to bottom.
func sortLineHits(hits []lineHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j], hits[j-1]
			if textmatch.BetterMatch(a.tier, a.score, b.tier, b.score) ||
				(a.tier == b.tier && a.score == b.score && a.line.no < b.line.no) {
				hits[j], hits[j-1] = hits[j-1], hits[j]
				continue
			}
			break
		}
	}
}

// snippetFor cuts a line to fit a result row, and it moves the spans. A long
// line gets a window around the first hit. It drops a span outside the
// window, because a span that misses its match is worse than none.
func snippetFor(raw string, spans []textmatch.Span) (string, []textmatch.Span) {
	runes := []rune(raw)

	lead := 0
	for lead < len(runes) && isSpace(runes[lead]) {
		lead++
	}
	end := len(runes)
	for end > lead && isSpace(runes[end-1]) {
		end--
	}
	runes = runes[lead:end]
	shifted := make([]textmatch.Span, 0, len(spans))
	for _, s := range spans {
		s.Start -= lead
		if s.Start >= 0 && s.Start+s.Len <= len(runes) {
			shifted = append(shifted, s)
		}
	}

	if len(runes) <= snippetMaxRunes {
		return string(runes), shifted
	}

	start := 0
	if len(shifted) > 0 && shifted[0].Start > snippetLead {
		start = shifted[0].Start - snippetLead
	}
	stop := start + snippetMaxRunes
	if stop > len(runes) {
		stop = len(runes)
		if start = stop - snippetMaxRunes; start < 0 {
			start = 0
		}
	}

	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if stop < len(runes) {
		suffix = "…"
	}

	out := make([]textmatch.Span, 0, len(shifted))
	for _, s := range shifted {
		if s.Start < start || s.Start+s.Len > stop {
			continue
		}
		s.Start += len([]rune(prefix)) - start
		out = append(out, s)
	}
	return prefix + string(runes[start:stop]) + suffix, out
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
}
