package text

import (
	"fmt"
	"unicode"

	"github.com/go-text/typesetting/bidi"
)

// Bidi levels come from go-text's implementation of UAX #9, which passes
// the Unicode conformance tests (BidiTest.txt, BidiCharacterTest.txt).
// Its runs end at every level change since go-text commit cf60e2d (before,
// they ended only where the direction changed, so levels 0 and 2 shared a
// run and an Arabic-Indic number after Latin text could not be reordered
// correctly).

// bidiLevels resolves the embedding level of every rune (UAX #9 up to rule
// L1 at paragraph separators and the end of the text; L1 at line ends is
// applied per line). The text is split into bidi paragraphs at paragraph
// separators (newlines), which keep the base level, and all paragraphs use
// base. The returned slice is appended to out.
func bidiLevels(p *bidi.Paragraph, text []rune, base uint8, out []uint8) []uint8 {
	dir := bidi.LeftToRight
	if base == 1 {
		dir = bidi.RightToLeft
	}
	start := 0
	for i := 0; i <= len(text); i++ {
		if i < len(text) && !isParagraphSeparator(text[i]) {
			continue
		}
		if i > start {
			runs := p.Segment(text[start:i], dir)
			n := len(out)
			for k := range runs.NumRuns() {
				r := runs.Run(k)
				for range r.End - r.Start {
					out = append(out, uint8(r.Level))
				}
			}
			if len(out)-n != i-start {
				// Cannot happen: the segment holds no paragraph separator.
				panic(fmt.Sprintf("internal/text: bidi returned %d levels for %d runes", len(out)-n, i-start))
			}
		}
		if i < len(text) {
			out = append(out, base)
		}
		start = i + 1
	}
	return out
}

// isParagraphSeparator reports the characters of bidi class B.
func isParagraphSeparator(r rune) bool {
	switch r {
	case '\n', '\r', 0x1C, 0x1D, 0x1E, 0x85, 0x2029:
		return true
	}
	return false
}

// firstStrongDirection implements rules P2 and P3 of UAX #9 for
// DirectionAuto: the level of the first character of bidi class L, R or AL,
// skipping isolated text; 0 when there is none. The classes are
// approximated from scripts and general categories: letters of
// right-to-left scripts (and RLM, ALM) are R or AL, other letters (and LRM)
// are L.
func firstStrongDirection(text []rune) uint8 {
	depth := 0
	for _, r := range text {
		switch {
		case r == 0x2066 || r == 0x2067 || r == 0x2068: // LRI, RLI, FSI
			depth++
			continue
		case r == 0x2069: // PDI
			if depth > 0 {
				depth--
			}
			continue
		case depth > 0:
			continue
		case isParagraphSeparator(r):
			continue
		}
		if d, ok := strongDirection(r); ok {
			return d
		}
	}
	return 0
}

// StrongDirection returns the direction of the first strong character of
// s that is not inside an isolate (UAX #9 rules P2 and P3, with the same
// approximation as DirectionAuto), and false when s has none. Core uses
// it for HTML's dir="auto", which looks at an element's text.
func StrongDirection(s string) (Direction, bool) {
	depth := 0
	for _, r := range s {
		switch {
		case r == 0x2066 || r == 0x2067 || r == 0x2068: // LRI, RLI, FSI
			depth++
			continue
		case r == 0x2069: // PDI
			if depth > 0 {
				depth--
			}
			continue
		case depth > 0:
			continue
		}
		if d, ok := strongDirection(r); ok {
			if d == 1 {
				return DirectionRTL, true
			}
			return DirectionLTR, true
		}
	}
	return DirectionLTR, false
}

// strongDirection returns 1 for characters of bidi class R or AL, 0 for L,
// and false for the others (approximated, see firstStrongDirection).
func strongDirection(r rune) (uint8, bool) {
	switch r {
	case 0x200E: // LRM
		return 0, true
	case 0x200F, 0x061C: // RLM, ALM
		return 1, true
	}
	if !unicode.IsLetter(r) {
		return 0, false
	}
	if isRTLLetter(r) {
		return 1, true
	}
	return 0, true
}

func isRTLLetter(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x08FF, // Hebrew, Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic, Arabic Extended
		r >= 0xFB1D && r <= 0xFDFF, // Hebrew and Arabic presentation forms A
		r >= 0xFE70 && r <= 0xFEFF, // Arabic presentation forms B
		r >= 0x10800 && r <= 0x10FFF, r >= 0x1E800 && r <= 0x1EFFF:
		return true
	}
	return false
}

// bidiWhitespace reports the characters that rule L1 resets to the
// paragraph level at the end of a line: white space (WS), segment and
// paragraph separators (S, B), boundary neutrals (BN, approximated by
// format and control characters) and explicit formatting characters.
func bidiWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', 0x0B, 0x0C, '\r', 0x1C, 0x1D, 0x1E, 0x1F, 0x85,
		0x1680, 0x2028, 0x2029, 0x205F, 0x3000:
		return true
	}
	switch {
	case r >= 0x2000 && r <= 0x200A:
		return true
	case r == 0x00A0, r == 0x2007, r == 0x202F: // no-break spaces are CS, not WS
		return false
	case r == 0x200E || r == 0x200F || r == 0x061C: // marks are strong
		return false
	}
	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cc, r)
}

// reorder implements rule L2 of UAX #9 on the items of one line: from the
// highest level down to the lowest odd level, every maximal sequence of
// items at that level or higher is reversed. levels[i] is the level of
// order[i]; both are permuted in place.
func reorder[T any](levels []uint8, order []T) {
	if len(levels) == 0 {
		return
	}
	hi, lowOdd := uint8(0), uint8(255)
	for _, l := range levels {
		hi = max(hi, l)
		if l%2 == 1 {
			lowOdd = min(lowOdd, l)
		}
	}
	if lowOdd == 255 {
		return // all even: nothing to reverse
	}
	for lvl := hi; lvl >= lowOdd; lvl-- {
		for i := 0; i < len(levels); {
			if levels[i] < lvl {
				i++
				continue
			}
			j := i
			for j < len(levels) && levels[j] >= lvl {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				levels[a], levels[b] = levels[b], levels[a]
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
		if lvl == 0 {
			break
		}
	}
}
