package text

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
)

// procText is the text that the layout shapes and breaks: the source after
// text-transform and white-space processing, with every inline box as a
// U+FFFC placeholder (a bidi neutral and a Common-script character, as
// CSS Writing Modes 3, section 2.4.3, and CSS Text 3 specify for atomic
// inlines).
//
// Mapping back to the source: src[i] is the byte offset in the source of the
// character that produced rune i, and src[len(runes)] is the source length.
// The source range of rune i is [src[i], src[i+1]): characters removed by
// white-space collapsing belong to the preceding kept rune, and runes that
// text-transform produced from one character (uppercase "ß" is "SS") share
// its offset, so the range of all but the last of them is empty. Layout
// never puts a cluster boundary where the offset does not grow, so every
// cluster maps to a non-empty source range, except inline boxes, whose
// range is the empty range at their offset.
type procText struct {
	runes []rune
	src   []int32
	style []uint16 // index into Layout.styles
	box   []int32  // inline box index, or -1
	// split marks the runes before which text shaping breaks: the
	// boundaries of inlines with margins, borders or padding (nil when
	// there is none).
	split []bool
	// first is the source offset of the character that produced rune 0
	// (src[0] is 0: the characters removed before it belong to it).
	first int32
}

const objectReplacement = '￼'

type rawRune struct {
	r     rune
	src   int32
	style uint16
	box   int32
}

// prepareText applies text-transform and white-space processing (CSS Text
// 3, section 4.1.1, phase I, plus the removal of collapsible spaces at the
// start of the paragraph and after preserved newlines from phase II).
// boxOrder is the order of the paragraph's boxes sorted by offset. Inlines
// left without content get a placeholder box (inline.go): the returned
// tags list their inlines, and the placeholder of tags[k] is box
// len(p.Boxes)+k.
func prepareText(p *Paragraph, styles []TextStyle, boxOrder []int) (procText, []int32) {
	text := p.Text
	needWords := false
	for i := range styles {
		if styles[i].Transform == TransformCapitalize {
			needWords = true
		}
	}

	// Source runes and their offsets (invalid bytes become U+FFFD).
	srcRunes := make([]rune, 0, len(text))
	srcOff := make([]int32, 0, len(text)+1)
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRuneInString(text[i:])
		srcRunes = append(srcRunes, r)
		srcOff = append(srcOff, int32(i))
		i += n
	}
	srcOff = append(srcOff, int32(len(text)))

	var wordStart []bool
	if needWords {
		wordStart = make([]bool, len(srcRunes))
		var seg segmenter.Segmenter
		seg.Init(srcRunes)
		it := seg.WordIterator()
		for it.Next() {
			w := it.Word()
			wordStart[w.Offset] = true
		}
	}

	raw := make([]rawRune, 0, len(srcRunes)+len(boxOrder))
	span, bi := 0, 0
	var buf [4]rune
	for k, r := range srcRunes {
		b := int(srcOff[k])
		for bi < len(boxOrder) && p.Boxes[boxOrder[bi]].At <= b {
			raw = append(raw, rawRune{objectReplacement, int32(p.Boxes[boxOrder[bi]].At), styleAt(p, &span, p.Boxes[boxOrder[bi]].At), int32(boxOrder[bi])})
			bi++
		}
		si := styleAt(p, &span, b)
		st := &styles[si]
		out := buf[:0]
		if st.Transform == TransformNone {
			out = append(out, r)
		} else {
			var prev, next rune
			if k > 0 {
				prev = srcRunes[k-1]
			}
			if k+1 < len(srcRunes) {
				next = srcRunes[k+1]
			}
			ws := wordStart != nil && wordStart[k]
			out = transformRune(out, r, st.Transform, st.Language, prev, next, ws)
		}
		for _, o := range out {
			raw = append(raw, rawRune{o, int32(b), si, -1})
		}
	}
	for ; bi < len(boxOrder); bi++ {
		at := p.Boxes[boxOrder[bi]].At
		raw = append(raw, rawRune{objectReplacement, int32(at), styleAt(p, &span, at), int32(boxOrder[bi])})
	}

	kept := collapseWhiteSpace(raw, p.Style.WhiteSpace)
	kept, tags := addPlaceholders(kept, p, len(styles)-len(p.Inlines))

	pt := procText{
		runes: make([]rune, len(kept)),
		src:   make([]int32, len(kept)+1),
		style: make([]uint16, len(kept)),
		box:   make([]int32, len(kept)),
	}
	for i, e := range kept {
		pt.runes[i], pt.src[i], pt.style[i], pt.box[i] = e.r, e.src, e.style, e.box
	}
	pt.src[len(kept)] = int32(len(text))
	// Characters removed at the start of the paragraph belong to the first
	// rune, unless it is an inline box (whose range stays empty).
	if len(kept) > 0 {
		pt.first = pt.src[0]
		if kept[0].box < 0 {
			pt.src[0] = 0
		}
	}
	pt.split = shapingSplits(kept, p)
	return pt, tags
}

// styleAt returns the style index of the byte at offset b (0 for Default,
// i+1 for Spans[i]); *span is a cursor into the sorted spans, advanced as b
// grows.
func styleAt(p *Paragraph, span *int, b int) uint16 {
	for *span < len(p.Spans) && p.Spans[*span].End <= b {
		*span++
	}
	if *span < len(p.Spans) && p.Spans[*span].Start <= b && b < p.Spans[*span].End {
		return uint16(*span + 1)
	}
	return 0
}

// collapseWhiteSpace normalizes CR LF and lone CR to LF, then, in the
// collapsing modes (normal, nowrap, pre-line), removes spaces and tabs
// around newlines, turns tabs into spaces, collapses runs of spaces to the
// first one, drops spaces at the start of the paragraph and after a
// preserved newline, and transforms newlines (normal, nowrap): removed
// next to U+200B or between two East Asian wide characters (none of them
// Hangul), else turned into a space (CSS Text 3, section 4.1.3).
func collapseWhiteSpace(raw []rawRune, ws WhiteSpace) []rawRune {
	out := make([]rawRune, 0, len(raw))
	collapse := ws.collapses()
	isSpace := func(e rawRune) bool { return e.box < 0 && (e.r == ' ' || e.r == '\t') }
	lineStart := true // at the start of the paragraph or after a preserved newline
	for i := 0; i < len(raw); i++ {
		e := raw[i]
		if e.box < 0 && e.r == '\r' {
			e.r = '\n'
			if i+1 < len(raw) && raw[i+1].box < 0 && raw[i+1].r == '\n' {
				i++
			}
		}
		if !collapse {
			out = append(out, e)
			continue
		}
		switch {
		case e.box < 0 && e.r == '\n':
			for len(out) > 0 && isSpace(out[len(out)-1]) {
				out = out[:len(out)-1]
			}
			j := i + 1
			for j < len(raw) && isSpace(raw[j]) {
				j++
			}
			if ws == WhiteSpacePreLine {
				out = append(out, e)
				lineStart = true
			} else {
				var prev, next rune
				if len(out) > 0 && out[len(out)-1].box < 0 {
					prev = out[len(out)-1].r
				}
				if j < len(raw) && raw[j].box < 0 {
					next = raw[j].r
					if next == '\r' {
						next = '\n'
					}
				}
				switch {
				case prev == '​' || next == '​':
				case eastAsianWide(prev) && eastAsianWide(next):
				case lineStart:
				default:
					e.r = ' '
					out = append(out, e)
				}
			}
			i = j - 1
		case isSpace(e):
			if lineStart || (len(out) > 0 && out[len(out)-1].box < 0 && out[len(out)-1].r == ' ') {
				continue
			}
			e.r = ' '
			out = append(out, e)
		default:
			out = append(out, e)
			lineStart = false
		}
	}
	return out
}

// eastAsianWide approximates East_Asian_Width F, W or H for the segment
// break transformation rules, excluding Hangul (CSS Text 3, section
// 4.1.3). Emoji are not included.
func eastAsianWide(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x303E, r >= 0x3041 && r <= 0x312F, r >= 0x3190 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF, r >= 0x4E00 && r <= 0x9FFF, r >= 0xA000 && r <= 0xA4CF,
		r >= 0xF900 && r <= 0xFAFF, r >= 0xFE10 && r <= 0xFE19, r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60, r >= 0xFF61 && r <= 0xFF9F, r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F200 && r <= 0x1F2FF, r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// Full case mappings that are not one rune to one rune (Unicode
// SpecialCasing.txt, unconditional entries), for the text-transform values.
var (
	upperSpecial = map[rune]string{
		'ß': "SS", 'ŉ': "ʼN", 'ǰ': "J̌", 'ΐ': "Ϊ́", 'ΰ': "Ϋ́", 'և': "ԵՒ",
		'ẖ': "H̱", 'ẗ': "T̈", 'ẘ': "W̊", 'ẙ': "Y̊", 'ẚ': "Aʾ",
		'ﬀ': "FF", 'ﬁ': "FI", 'ﬂ': "FL", 'ﬃ': "FFI", 'ﬄ': "FFL", 'ﬅ': "ST", 'ﬆ': "ST",
		'ﬓ': "ՄՆ", 'ﬔ': "ՄԵ", 'ﬕ': "ՄԻ", 'ﬖ': "ՎՆ", 'ﬗ': "ՄԽ",
	}
	titleSpecial = map[rune]string{
		'ß': "Ss", 'ŉ': "ʼN", 'ǰ': "J̌", 'և': "Եւ",
		'ﬀ': "Ff", 'ﬁ': "Fi", 'ﬂ': "Fl", 'ﬃ': "Ffi", 'ﬄ': "Ffl", 'ﬅ': "St", 'ﬆ': "St",
		'ﬓ': "Մն", 'ﬔ': "Մե", 'ﬕ': "Մի", 'ﬖ': "Վն", 'ﬗ': "Մխ",
	}
)

// transformRune appends the text-transform t of r to dst. prev and next
// are the neighboring source characters (0 at the ends), wordStart reports
// whether r starts a word (UAX #29) for capitalize.
func transformRune(dst []rune, r rune, t TextTransform, lang string, prev, next rune, wordStart bool) []rune {
	turkic := lang != "" && (strings.EqualFold(primaryTag(lang), "tr") || strings.EqualFold(primaryTag(lang), "az"))
	switch t {
	case TransformUppercase:
		if turkic {
			return append(dst, unicode.TurkishCase.ToUpper(r))
		}
		if s, ok := upperSpecial[r]; ok {
			return append(dst, []rune(s)...)
		}
		return append(dst, unicode.ToUpper(r))
	case TransformLowercase:
		if turkic {
			return append(dst, unicode.TurkishCase.ToLower(r))
		}
		switch {
		case r == 'İ':
			return append(dst, 'i', '̇')
		case r == 'Σ' && unicode.IsLetter(prev) && !unicode.IsLetter(next):
			return append(dst, 'ς') // final sigma
		}
		return append(dst, unicode.ToLower(r))
	case TransformCapitalize:
		if !wordStart || !unicode.IsLetter(r) {
			return append(dst, r)
		}
		if turkic {
			return append(dst, unicode.TurkishCase.ToTitle(r))
		}
		if s, ok := titleSpecial[r]; ok {
			return append(dst, []rune(s)...)
		}
		return append(dst, unicode.ToTitle(r))
	}
	return append(dst, r)
}

// primaryTag returns the language subtag of a BCP 47 tag ("tr" of "tr-TR").
func primaryTag(lang string) string {
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		return lang[:i]
	}
	return lang
}
