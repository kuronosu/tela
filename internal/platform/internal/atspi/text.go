//go:build linux

package atspi

import (
	"slices"
	"unicode"
)

// Text granularities of the Text interface, over a node's text as code
// points (AT-SPI offsets count characters). The tree has no layout of
// the text (where lines wrap), so the rules are simple and textual:
//
//   - A word is a run of letters, digits, marks and underscores, an
//     apostrophe or a period between two of them included ("don't",
//     "3.14"). A word segment runs from a word's start to the next word's
//     start (trailing spaces and punctuation included), the "word start"
//     boundary; "word end" runs from one word's end to the next's.
//   - A sentence ends after ., ! or ? (and the closing quotes or brackets
//     after it) followed by white space, and at a line break. Sentence
//     start segments run from the first character after that white space;
//     end segments from the terminator's end.
//   - A line is what runs up to and including a line feed: soft wraps
//     are not known, so a paragraph that wraps on screen is one line.
//     Paragraphs are lines.

// AtspiTextGranularity values (GetStringAtOffset).
const (
	granChar      = 0
	granWord      = 1
	granSentence  = 2
	granLine      = 3
	granParagraph = 4
)

// AtspiTextBoundaryType values (GetTextAtOffset and its neighbors).
const (
	boundaryChar          = 0
	boundaryWordStart     = 1
	boundaryWordEnd       = 2
	boundarySentenceStart = 3
	boundarySentenceEnd   = 4
	boundaryLineStart     = 5
	boundaryLineEnd       = 6
)

// isWordRune reports whether text[i] belongs to a word.
func isWordRune(text []rune, i int) bool {
	r := text[i]
	if wordRune(r) {
		return true
	}
	switch r {
	case '\'', '’', '.':
		return i > 0 && i+1 < len(text) && wordRune(text[i-1]) && wordRune(text[i+1])
	}
	return false
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

// boundaries returns the sorted offsets that delimit text's segments of
// boundary type typ (one of the boundary constants), 0 and len(text)
// included.
func boundaries(text []rune, typ uint32) []int {
	n := len(text)
	b := []int{0}
	switch typ {
	case boundaryChar:
		for i := 1; i < n; i++ {
			b = append(b, i)
		}
	case boundaryWordStart:
		for i := 1; i < n; i++ {
			if isWordRune(text, i) && !isWordRune(text, i-1) {
				b = append(b, i)
			}
		}
	case boundaryWordEnd:
		for i := 1; i < n; i++ {
			if isWordRune(text, i-1) && !isWordRune(text, i) {
				b = append(b, i)
			}
		}
	case boundarySentenceStart, boundarySentenceEnd:
		for i := 0; i < n; i++ {
			end := sentenceEnd(text, i)
			if end < 0 {
				continue
			}
			if typ == boundarySentenceEnd {
				b = append(b, end)
				continue
			}
			j := end
			for j < n && unicode.IsSpace(text[j]) {
				j++
			}
			b = append(b, j)
			i = j - 1
		}
	case boundaryLineStart:
		for i := 1; i < n; i++ {
			if text[i-1] == '\n' {
				b = append(b, i)
			}
		}
	case boundaryLineEnd:
		for i := 0; i < n; i++ {
			if text[i] == '\n' {
				b = append(b, i)
			}
		}
	}
	b = append(b, n)
	slices.Sort(b)
	return slices.Compact(b)
}

// sentenceEnd returns the offset after the sentence that ends at text[i]
// (a terminator followed by closing punctuation and white space, or a
// line feed), or -1 when no sentence ends there.
func sentenceEnd(text []rune, i int) int {
	switch text[i] {
	case '\n':
		return i
	case '.', '!', '?', '…':
	default:
		return -1
	}
	j := i + 1
	for j < len(text) && closing(text[j]) {
		j++
	}
	if j < len(text) && !unicode.IsSpace(text[j]) {
		return -1 // "3.14", "e.g.x": not an end
	}
	return j
}

func closing(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '”', '’', '»':
		return true
	}
	return false
}

// segment returns the segment of text of boundary type typ around offset
// (which = 0), the one before it (-1) or the one after it (1), as
// [start, end) offsets. Outside the text it returns an empty segment at
// the nearest end. An offset at the end of the text is in the last
// segment.
func segment(text []rune, offset int, typ uint32, which int) (start, end int) {
	n := len(text)
	if n == 0 {
		return 0, 0
	}
	offset = min(max(offset, 0), n)
	if offset == n {
		offset = n - 1
	}
	b := boundaries(text, typ)
	// b[i] <= offset < b[i+1]
	i, found := slices.BinarySearch(b, offset)
	if !found {
		i--
	}
	i += which
	if i < 0 {
		return 0, 0
	}
	if i+1 >= len(b) {
		return n, n
	}
	return b[i], b[i+1]
}

// stringAt implements GetStringAtOffset: the segment of granularity gran
// around offset.
func stringAt(text []rune, offset int, gran uint32) (start, end int) {
	switch gran {
	case granChar:
		return segment(text, offset, boundaryChar, 0)
	case granWord:
		return segment(text, offset, boundaryWordStart, 0)
	case granSentence:
		return segment(text, offset, boundarySentenceStart, 0)
	case granLine, granParagraph:
		return segment(text, offset, boundaryLineStart, 0)
	}
	return 0, 0
}

// clampRange returns [start, end) inside text, end -1 meaning the end.
func clampRange(text []rune, start, end int) (int, int) {
	n := len(text)
	if end < 0 || end > n {
		end = n
	}
	start = min(max(start, 0), n)
	if end < start {
		start, end = end, start
	}
	return start, end
}
