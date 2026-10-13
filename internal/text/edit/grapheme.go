package edit

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
)

// This file holds grapheme cluster boundaries (UAX #29, from go-text's
// segmenter) computed on the value without a layout, and the Backspace
// rule.
//
// Values never hold CR, and LF only in multi-line mode (see sanitize.go),
// so grapheme boundaries are always found on both sides of an LF (UAX #29
// rules GB4 and GB5): segmenting the paragraph (the text between LFs) that
// holds an offset gives the boundaries of the whole text around it.

// paragraph returns the range [ps, pe) of the text between line feeds
// that holds offset off (a line feed ends the paragraph before it).
func paragraph(s string, off int) (ps, pe int) {
	ps = strings.LastIndexByte(s[:off], '\n') + 1
	pe = len(s)
	if i := strings.IndexByte(s[off:], '\n'); i >= 0 {
		pe = off + i
	}
	return ps, pe
}

// appendBounds appends to dst the byte offsets of the grapheme cluster
// boundaries of s, shifted by base, from 0 to len(s) inclusive.
func appendBounds(dst []int, s string, base int) []int {
	dst = append(dst, base)
	if s == "" {
		return dst
	}
	var seg segmenter.Segmenter
	seg.Init([]rune(s))
	it := seg.GraphemeIterator()
	// The iterator reports byte offsets of its rune input re-encoded as
	// UTF-8, which are those of s: s is valid UTF-8 (sanitized values).
	for it.Next() {
		g := it.Grapheme()
		if end := g.OffsetInBytes + g.LengthInBytes; end > 0 {
			dst = append(dst, base+end)
		}
	}
	if dst[len(dst)-1] != base+len(s) {
		dst = append(dst, base+len(s))
	}
	return dst
}

// graphemeBounds returns the grapheme cluster boundaries of s, from 0 to
// len(s) inclusive. LFs split the work into paragraphs.
func graphemeBounds(s string) []int {
	out := make([]int, 0, len(s)+1)
	start := 0
	for {
		i := strings.IndexByte(s[start:], '\n')
		if i < 0 {
			out = appendBounds(out, s[start:], start)
			return out
		}
		out = appendBounds(out, s[start:start+i], start)
		start += i + 1 // the LF is a grapheme of its own
	}
}

// localBounds returns the grapheme boundaries of the paragraph of s that
// holds off.
func localBounds(s string, off int) []int {
	ps, pe := paragraph(s, off)
	return appendBounds(nil, s[ps:pe], ps)
}

// snapGrapheme moves off to a grapheme boundary of s: the start of the
// grapheme cluster that contains it, or its end when forward is set.
// Offsets are clamped to the text first.
func snapGrapheme(s string, off int, forward bool) int {
	off = max(0, min(off, len(s)))
	if off == 0 || off == len(s) || s[off-1] == '\n' || s[off] == '\n' {
		return off
	}
	b := localBounds(s, off)
	for i := 1; i < len(b); i++ {
		if b[i] == off {
			return off
		}
		if b[i] > off {
			if forward {
				return b[i]
			}
			return b[i-1]
		}
	}
	return off
}

// prevGrapheme returns the start of the grapheme cluster that ends at off
// (a boundary), or off at the start of the text.
func prevGrapheme(s string, off int) int {
	if off == 0 {
		return 0
	}
	if s[off-1] == '\n' {
		return off - 1
	}
	b := localBounds(s, off-1)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < off {
			return b[i]
		}
	}
	return 0
}

// nextGrapheme returns the end of the grapheme cluster that starts at off
// (a boundary), or off at the end of the text.
func nextGrapheme(s string, off int) int {
	if off >= len(s) {
		return len(s)
	}
	if s[off] == '\n' {
		return off + 1
	}
	for _, e := range localBounds(s, off) {
		if e > off {
			return e
		}
	}
	return len(s)
}

// backspaceStart returns where Backspace deletes back to from the
// boundary off, following Blink's BackspaceStateMachine: one code point,
// except that emoji sequences (flags, keycaps, skin tones, tag sequences
// and ZWJ sequences) go as a whole and a variation selector goes with the
// character it modifies. So Backspace after a decomposed "é" (e and
// U+0301) removes only the accent, and after a Devanagari or Thai cluster
// only its last code point, as in Chromium; Delete removes whole grapheme
// clusters (deleteForward). With whole set, the whole grapheme cluster is
// deleted (password fields, where the user cannot see code points).
func backspaceStart(s string, off int, whole bool) int {
	gs := prevGrapheme(s, off)
	g := s[gs:off]
	if whole || emojiSequence(g) {
		return gs
	}
	r, size := utf8.DecodeLastRuneInString(g)
	start := off - size
	if isVariationSelector(r) && start > gs {
		_, n := utf8.DecodeLastRuneInString(s[gs:start])
		start -= n
	}
	return start
}

// emojiSequence reports whether grapheme cluster g is an emoji sequence
// that Backspace deletes whole: it has a regional indicator pair, a skin
// tone modifier, a combining keycap, tag characters, or a ZWJ joining
// characters that are not letters (a ZWJ inside Indic letters is part of
// the spelling and goes code point by code point).
func emojiSequence(g string) bool {
	first, n := utf8.DecodeRuneInString(g)
	if n == len(g) {
		return false
	}
	for _, r := range g {
		switch {
		case r >= 0x1F1E6 && r <= 0x1F1FF, // regional indicators
			r >= 0x1F3FB && r <= 0x1F3FF, // skin tone modifiers
			r == 0x20E3,                  // combining enclosing keycap
			r >= 0xE0020 && r <= 0xE007F: // tags
			return true
		case r == 0x200D:
			if !unicode.IsLetter(first) && !unicode.IsMark(first) && !unicode.IsNumber(first) {
				return true
			}
		}
	}
	return false
}

// isVariationSelector reports whether r is a variation selector (VS1 to
// VS256, including the emoji and text presentation selectors).
func isVariationSelector(r rune) bool {
	return (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF)
}
