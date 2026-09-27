# 0009. Show only the search rows that carry a word of the query

* Status: accepted
* Version: 26.08.81, 26.09.40
* Code: `cutSnippets` in `backend/search.go`, `commonWords` and
  `commonWordShare` in `backend/search_index.go`

## Context

The search panel shows ten rows of each note that matches. A long note
matches a query of several words on hundreds of lines. Most of those
lines match one short or common word and nothing else.

Two measurements showed the problem:

* The query "also makes a local connection" matched 564 lines of the User
  Manual. Rows 1 to 8 carried two to five words of the query. Rows 9 and
  10 carried the letter "a" alone.
* The query "the tag and the level" gave 50 rows. 33 of them carried no
  content word, and the panel drew 1360 marks.

## Decision

`cutSnippets` takes the first ten rows, and then removes each row that
carries no word of the query. A row carries a word when a query term
meets all three conditions:

* The term is longer than one rune. A term of one Han, Hiragana, Katakana
  or Hangul rune is a whole word, thus it counts as long.
* The term is not common in this collection. A word is common when more
  than half of the indexed documents hold it. `commonWords` gives the set.
* The term matches the row verbatim, and not as scattered letters.

The order is part of the rule. The window of ten comes first, and the cut
comes second. With the cut first, a weaker row from below the ten would
take the place of each removed row.

When no row of the window carries a word, the window is the answer. A
note that matched always shows something.

## Rejected alternatives

* A score floor of the top score divided by four. It repaired one query of
  two words, and it did nothing for a query of three words.
* A cut at the first change of rung. It reduced a list to one row.
* The character mask of the index as the measure of a common word. It
  saturates: "cat" seemed to be in 96 percent of the documents of a
  corpus that never says it.
* The share of the lines of one document that a term matches. The
  subsequence rung makes "cat" match half the lines of a document.
* The score of a row against the best row of its note. A noise row scores
  93 to 100 percent of the best row in a note that holds no answer.
* The count of distinct query terms in a row. The subsequence rung
  inflates that count as well.

## Consequences

* All three conditions are necessary. Without the common-word test, the
  second query keeps 75 percent of its empty rows. Without the verbatim
  test, it keeps 89 percent.
* The set of common words is relative to the collection. A person who
  writes about one subject only pushes the words of that subject over the
  half. The cut then keeps no row, and the panel shows the window. The
  worst case is thus the same as no cut.
* With global search off, no index exists and the set is empty. The cut
  then uses the first two conditions alone.
* The tests of `backend/search_test.go` and `backend/search_index_test.go`
  break each condition in turn.
