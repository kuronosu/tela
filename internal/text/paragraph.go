package text

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/gfx"
)

// This file holds the input model of paragraph layout: a Paragraph is a
// UTF-8 string with styled spans (byte ranges) and inline boxes. See
// NewLayout and the "Paragraph layout" section of the package
// documentation.

// Direction is the base direction of a paragraph: CSS direction, or HTML
// dir="auto".
type Direction uint8

const (
	// DirectionLTR is left to right, CSS direction: ltr. It is the zero
	// value, like CSS's initial value.
	DirectionLTR Direction = iota
	// DirectionRTL is right to left, CSS direction: rtl.
	DirectionRTL
	// DirectionAuto takes the direction of the first strong character
	// (UAX #9 rules P2 and P3, like HTML dir="auto"), left to right when
	// there is none.
	DirectionAuto
)

// TextAlign is the CSS text-align of a paragraph.
type TextAlign uint8

const (
	// AlignStart aligns lines to the start edge: left in a left-to-right
	// paragraph, right in a right-to-left one. It is the zero value.
	AlignStart TextAlign = iota
	// AlignEnd aligns lines to the end edge.
	AlignEnd
	// AlignLeft aligns lines to the left edge.
	AlignLeft
	// AlignRight aligns lines to the right edge.
	AlignRight
	// AlignCenter centers lines.
	AlignCenter
	// AlignJustify stretches word separators (and gaps between CJK
	// characters) so that lines fill the width, except the last line and
	// lines that end in a forced break, which are start-aligned
	// (text-align-last: auto).
	AlignJustify
)

// WhiteSpace is the CSS white-space property (CSS Text 3, section 3).
type WhiteSpace uint8

const (
	// WhiteSpaceNormal collapses spaces, tabs and newlines and wraps lines.
	WhiteSpaceNormal WhiteSpace = iota
	// WhiteSpaceNowrap collapses like normal but never wraps.
	WhiteSpaceNowrap
	// WhiteSpacePre preserves spaces, tabs and newlines and never wraps.
	WhiteSpacePre
	// WhiteSpacePreWrap preserves spaces, tabs and newlines and wraps;
	// spaces at the end of a line hang.
	WhiteSpacePreWrap
	// WhiteSpacePreLine collapses spaces and tabs, preserves newlines and
	// wraps.
	WhiteSpacePreLine
	// WhiteSpaceBreakSpaces is pre-wrap where preserved spaces never hang
	// and every space is a wrap opportunity.
	WhiteSpaceBreakSpaces
)

func (w WhiteSpace) collapses() bool {
	return w == WhiteSpaceNormal || w == WhiteSpaceNowrap || w == WhiteSpacePreLine
}

func (w WhiteSpace) wraps() bool { return w != WhiteSpaceNowrap && w != WhiteSpacePre }

// TextWrapStyle is the CSS text-wrap-style property (CSS Text 4): how the
// lines of a paragraph that wraps are chosen. It has no
// effect when WhiteSpace does not wrap. Balancing and prettifying follow
// Blink (wrapstyle.go): they move line breaks only, so the lines keep being
// aligned in the full width the paragraph is broken at, and they do not
// change the min- and max-content widths.
type TextWrapStyle uint8

const (
	// TextWrapAuto breaks lines greedily: each line takes as much content
	// as fits. It is the zero value, like CSS's initial value.
	TextWrapAuto TextWrapStyle = iota
	// TextWrapBalance makes the lines about equally long (Tailwind
	// text-balance), as Chromium does: up to 6 lines up to the first
	// forced break (or up to LineClamp lines), with the same number of
	// lines as greedy breaking; longer paragraphs wrap greedily.
	TextWrapBalance
	// TextWrapPretty avoids a last line of a single short word (Tailwind
	// text-pretty), as Chromium does: when the last line of a paragraph
	// (or of a part ended by a forced break) holds one word and less than
	// a third of the width, the breaks of its last 4 lines are chosen
	// again, with the same number of lines.
	TextWrapPretty
)

// WordBreak is the CSS word-break property.
type WordBreak uint8

const (
	// WordBreakNormal breaks lines at the opportunities of UAX #14.
	WordBreakNormal WordBreak = iota
	// WordBreakBreakAll also allows breaks between any two letters or
	// digits (Tailwind break-all).
	WordBreakBreakAll
	// WordBreakKeepAll forbids breaks between letters, digits and
	// ideographs: CJK text then breaks only at spaces and punctuation
	// (Tailwind break-keep).
	WordBreakKeepAll
)

// OverflowWrap is the CSS overflow-wrap property.
type OverflowWrap uint8

const (
	// OverflowWrapNormal lets words that do not fit overflow the line.
	OverflowWrapNormal OverflowWrap = iota
	// OverflowWrapBreakWord breaks a word that does not fit at any
	// grapheme boundary, when the line has no other opportunity; it does
	// not change the min-content width (Tailwind break-words).
	OverflowWrapBreakWord
	// OverflowWrapAnywhere is break-word whose opportunities also count for
	// the min-content width.
	OverflowWrapAnywhere
)

// TextOverflow is the CSS text-overflow property.
type TextOverflow uint8

const (
	// TextOverflowClip leaves overflowing lines as they are (the caller
	// clips them).
	TextOverflowClip TextOverflow = iota
	// TextOverflowEllipsis truncates every line wider than the layout's
	// maximum width and ends it with an ellipsis (Tailwind truncate, with
	// WhiteSpaceNowrap).
	TextOverflowEllipsis
)

// ParagraphStyle holds the properties that apply to a whole paragraph (the
// block container in CSS terms).
type ParagraphStyle struct {
	Direction    Direction
	Align        TextAlign
	WhiteSpace   WhiteSpace
	WordBreak    WordBreak
	OverflowWrap OverflowWrap
	// WrapStyle is text-wrap-style: greedy, balanced or pretty lines.
	WrapStyle TextWrapStyle
	// TabSize is the CSS tab-size in multiples of the advance of a space
	// (letter- and word-spacing included). Zero means 8; negative values
	// are treated as zero (tabs then take no space).
	TabSize float32
	// TextOverflow truncates overflowing lines with an ellipsis.
	TextOverflow TextOverflow
	// LineClamp, when positive, keeps only the first LineClamp lines and
	// ends the last one with an ellipsis when more lines follow (CSS
	// line-clamp, Tailwind line-clamp-N).
	LineClamp int
}

func (p ParagraphStyle) tabSize() float32 {
	switch {
	case p.TabSize == 0 || p.TabSize != p.TabSize:
		return 8
	case p.TabSize < 0:
		return 0
	}
	return p.TabSize
}

// LineHeightKind selects how a LineHeight is resolved.
type LineHeightKind uint8

const (
	// LineHeightNormal derives the line height from the font metrics
	// (ascent + descent + line gap). It is the zero value.
	LineHeightNormal LineHeightKind = iota
	// LineHeightScale multiplies the font size (CSS unitless numbers).
	LineHeightScale
	// LineHeightPx is a length in logical pixels.
	LineHeightPx
)

// LineHeight is the CSS line-height of a span. The zero value is normal.
type LineHeight struct {
	Kind  LineHeightKind
	Value float32
}

// LineHeightMultiple returns a line height of v times the font size.
func LineHeightMultiple(v float32) LineHeight { return LineHeight{LineHeightScale, v} }

// LineHeightPixels returns a line height of v logical pixels.
func LineHeightPixels(v float32) LineHeight { return LineHeight{LineHeightPx, v} }

// Decoration is a set of CSS text-decoration-line values.
type Decoration uint8

const (
	// DecorationUnderline draws a line below the baseline.
	DecorationUnderline Decoration = 1 << iota
	// DecorationOverline draws a line at the top of the font's ascent.
	DecorationOverline
	// DecorationLineThrough draws a line through the text.
	DecorationLineThrough
)

// TextDecoration is the CSS text-decoration of a span. Only solid lines are
// drawn for now.
type TextDecoration struct {
	Lines Decoration
	// Paint is the decoration color; nil means the text's paint
	// (currentColor).
	Paint gfx.Paint
	// Thickness is the line thickness in logical pixels; zero or negative
	// means auto: the font's underline (or strikeout) thickness.
	Thickness float32
}

// TextTransform is the CSS text-transform property.
type TextTransform uint8

const (
	// TransformNone leaves the text as it is.
	TransformNone TextTransform = iota
	// TransformUppercase maps letters to upper case (full case mapping:
	// "ß" becomes "SS").
	TransformUppercase
	// TransformLowercase maps letters to lower case (with the final sigma
	// rule).
	TransformLowercase
	// TransformCapitalize maps the first letter of each word (UAX #29) to
	// title case.
	TransformCapitalize
)

// NumericVariant is a set of CSS font-variant-numeric values, mapped to
// OpenType features.
type NumericVariant uint16

const (
	NumericOrdinal           NumericVariant = 1 << iota // ordn
	NumericSlashedZero                                  // zero
	NumericLining                                       // lnum
	NumericOldstyle                                     // onum
	NumericProportional                                 // pnum
	NumericTabular                                      // tnum (Tailwind tabular-nums)
	NumericDiagonalFractions                            // frac
	NumericStackedFractions                             // afrc
)

var numericTags = [...]string{"ordn", "zero", "lnum", "onum", "pnum", "tnum", "frac", "afrc"}

// Feature is an OpenType feature setting (CSS font-feature-settings), such
// as {"tnum", 1} or {"liga", 0}.
type Feature struct {
	Tag   string // four ASCII characters
	Value uint32
}

// TextStyle is the style of a span of text. The zero value is a valid
// style: sans-serif, 16 px, weight 400, line-height normal, black.
type TextStyle struct {
	// Families are font family names or CSS generic families, in order of
	// preference; empty means "sans-serif".
	Families []string
	// Size is the font size in logical pixels; zero or less means 16.
	Size    float32
	Weight  float32 // CSS font-weight; zero means 400
	Style   Style   // CSS font-style
	Stretch float32 // CSS font-stretch percentage; zero means 100
	// LetterSpacing is added after every grapheme cluster (visually to its
	// right, like Chromium), in logical pixels. A non-zero value disables
	// optional ligatures (liga, clig, dlig, hlig, calt), as in Chromium.
	LetterSpacing float32
	// WordSpacing is added to every word separator, in logical pixels.
	WordSpacing float32
	LineHeight  LineHeight
	// Paint is the text color or gradient; nil means opaque black.
	Paint      gfx.Paint
	Decoration TextDecoration
	Transform  TextTransform
	// Features are applied after the ones Numeric implies.
	Features []Feature
	Numeric  NumericVariant
	// Language is a BCP 47 tag, used by shaping (locl) and by
	// text-transform (Turkish and Azeri dotted i).
	Language string
}

func (s *TextStyle) size() float32 {
	if !(s.Size > 0) || s.Size != s.Size {
		return 16
	}
	return min(s.Size, 1e6)
}

func (s *TextStyle) query() Query {
	fams := s.Families
	if len(fams) == 0 {
		fams = []string{"sans-serif"}
	}
	return Query{Families: fams, Weight: s.Weight, Style: s.Style, Stretch: s.Stretch}
}

func (s *TextStyle) paint() gfx.Paint {
	if s.Paint == nil {
		return gfx.Black
	}
	return s.Paint
}

// shapesLike reports whether text in styles s and t can be shaped together:
// everything that changes the glyphs or their advances is equal.
func (s *TextStyle) shapesLike(t *TextStyle) bool {
	return slices.Equal(s.Families, t.Families) && s.size() == t.size() &&
		s.Weight == t.Weight && s.Style == t.Style && s.Stretch == t.Stretch &&
		s.LetterSpacing == t.LetterSpacing && slices.Equal(s.Features, t.Features) &&
		s.Numeric == t.Numeric && s.Language == t.Language
}

// Span applies a style to the bytes [Start, End) of the paragraph text.
type Span struct {
	Start, End int
	Style      TextStyle
}

// BoxAlign is the CSS vertical-align of an inline box.
type BoxAlign uint8

const (
	// BoxBaseline puts the box's baseline (Descent above its bottom) on the
	// baseline of the text. It is the zero value.
	BoxBaseline BoxAlign = iota
	// BoxMiddle centers the box on the baseline raised by half the
	// x-height of its span's font.
	BoxMiddle
	// BoxTextTop aligns the box's top with the top of its span's font
	// (ascent).
	BoxTextTop
	// BoxTextBottom aligns the box's bottom with the bottom of its span's
	// font (descent).
	BoxTextBottom
	// BoxTop aligns the box's top with the top of the line.
	BoxTop
	// BoxBottom aligns the box's bottom with the bottom of the line.
	BoxBottom
)

// InlineBox is an atomic placeholder in a paragraph, such as an inline
// icon: the layout reserves its size and reports its position; the caller
// paints it. Like a CSS atomic inline, it is a bidi neutral (U+FFFC) and a
// line break opportunity on both sides.
type InlineBox struct {
	// ID is the caller's identifier, reported back with the position.
	ID int
	// At is the byte offset in the text where the box sits (before the
	// character at that offset). Boxes with the same offset keep their
	// order.
	At            int
	Width, Height float32
	// Descent is the distance from the box's bottom up to its baseline;
	// zero puts the bottom on the baseline, like an image.
	Descent float32
	Align   BoxAlign
}

// Inline is a CSS inline box (CSS Inline 3): an inline element such as a
// span, a link or inline code, whose content is the text [Start, End) and
// the inline boxes [BoxStart, BoxEnd) of Paragraph.Boxes, nested inlines
// included. The layout splits it into fragments, one per line and per bidi
// run that separates its content (InlineFragments), which the caller
// decorates (backgrounds, borders): the layout itself paints nothing of
// it.
//
// Left and Right are the space its left and right edges take on the line
// (margin, border and padding in the inline axis, logical px): the start
// edge (left in a left-to-right paragraph, right in a right-to-left one)
// sits on the first line the inline is on, before its first character,
// and the end edge on its last line, after its last character
// (box-decoration-break: slice). An edge counts for line breaking with
// the character next to it, and makes text shaping break at the inline's
// boundaries (CSS Text 3, section 7.3). An inline that has no content left
// after white-space processing still has a fragment (CSS 2.1, section
// 10.8: empty inline boxes have margins, padding and borders), which never
// adds a line break opportunity: it stays with the content before it.
//
// Style is the inline's own text style: like every inline box, its first
// available font and line height contribute to the height of each line it
// is on (its strut), and the font's quantized ascent and descent are the
// content area of its fragments.
type Inline struct {
	// ID is the caller's identifier.
	ID               int
	Start, End       int
	BoxStart, BoxEnd int
	// Depth is the number of inlines that contain this one: its parent
	// is the nearest inline before it at Depth-1 (empty inlines with equal
	// ranges are told apart this way).
	Depth       int
	Style       TextStyle
	Left, Right float32
}

// Paragraph is the input of NewLayout: a UTF-8 string, styled spans
// (byte offsets), inline boxes and inlines.
//
// Spans must be sorted by Start, must not overlap and must start and end on
// rune boundaries; text that no span covers uses Default. Default is also
// the style of the block (the CSS root inline box): its first available
// font and line height form the strut that gives every line its minimum
// height, it styles the ellipsis of truncated lines, and tab stops are
// measured with its space. Invalid UTF-8 is laid out as U+FFFD, one per
// invalid byte.
//
// Inlines are in tree order (an inline before the inlines it contains,
// siblings in text order) and properly nested: an inline that starts
// inside another one (in text and in boxes) ends inside it.
//
// Scale is the display scale (device pixels per logical pixel) the line
// boxes are quantized for: font ascents and descents round to whole device
// pixels and half-leadings round down to whole device pixels, as Blink does
// with the font scaled by the device scale factor (ADR-159). Zero, negative
// and non-finite values mean 1. Layout.SetScale changes it without shaping
// again.
type Paragraph struct {
	Text    string
	Style   ParagraphStyle
	Default TextStyle
	Spans   []Span
	Boxes   []InlineBox
	Inlines []Inline
	Scale   float32

	open []int // the inlines OpenInline opened and CloseInline did not close
}

// AddText appends s to the text with style st.
func (p *Paragraph) AddText(s string, st TextStyle) {
	start := len(p.Text)
	p.Text += s
	if len(s) > 0 {
		p.Spans = append(p.Spans, Span{Start: start, End: len(p.Text), Style: st})
	}
}

// AddBox appends an inline box at the end of the text.
func (p *Paragraph) AddBox(b InlineBox) {
	b.At = len(p.Text)
	p.Boxes = append(p.Boxes, b)
}

// OpenInline starts an inline at the end of the text and returns its index
// in Inlines; CloseInline(index) ends it after the text and boxes added in
// between.
func (p *Paragraph) OpenInline(in Inline) int {
	in.Start, in.End = len(p.Text), len(p.Text)
	in.BoxStart, in.BoxEnd = len(p.Boxes), len(p.Boxes)
	in.Depth = len(p.open)
	p.Inlines = append(p.Inlines, in)
	p.open = append(p.open, len(p.Inlines)-1)
	return len(p.Inlines) - 1
}

// CloseInline ends inline i, the innermost open one, at the end of the
// text.
func (p *Paragraph) CloseInline(i int) {
	if n := len(p.open); n == 0 || p.open[n-1] != i {
		panic(fmt.Sprintf("text: CloseInline(%d) does not close the innermost open inline", i))
	}
	p.open = p.open[:len(p.open)-1]
	p.Inlines[i].End = len(p.Text)
	p.Inlines[i].BoxEnd = len(p.Boxes)
}

// validate panics with an actionable message on malformed input.
func (p *Paragraph) validate() {
	end := 0
	for i, s := range p.Spans {
		switch {
		case s.Start < end:
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Spans[%d] starts at byte %d, before the end of the previous span (%d); spans must be sorted and must not overlap", i, s.Start, end))
		case s.End < s.Start || s.End > len(p.Text):
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Spans[%d] = [%d, %d) is outside the text (%d bytes)", i, s.Start, s.End, len(p.Text)))
		case !runeBoundary(p.Text, s.Start) || !runeBoundary(p.Text, s.End):
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Spans[%d] = [%d, %d) does not start and end on rune boundaries", i, s.Start, s.End))
		}
		end = s.End
	}
	for i, b := range p.Boxes {
		if b.At < 0 || b.At > len(p.Text) || !runeBoundary(p.Text, b.At) {
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Boxes[%d].At = %d is not a rune boundary of the text (%d bytes)", i, b.At, len(p.Text)))
		}
	}
	var stack []int // the open inlines: stack[d] is the parent at depth d+1
	prevEnd, prevBoxEnd := 0, 0
	for i, in := range p.Inlines {
		switch {
		case in.End < in.Start || in.Start < 0 || in.End > len(p.Text) || !runeBoundary(p.Text, in.Start) || !runeBoundary(p.Text, in.End):
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Inlines[%d] = [%d, %d) is not a range of rune boundaries of the text (%d bytes)", i, in.Start, in.End, len(p.Text)))
		case in.BoxEnd < in.BoxStart || in.BoxStart < 0 || in.BoxEnd > len(p.Boxes):
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Inlines[%d] holds boxes [%d, %d), outside the %d boxes", i, in.BoxStart, in.BoxEnd, len(p.Boxes)))
		case in.Depth < 0 || in.Depth > len(stack):
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Inlines[%d].Depth = %d, but the inline before it is at depth %d; inlines must be in tree order", i, in.Depth, len(stack)-1))
		}
		stack = stack[:in.Depth]
		if in.Depth > 0 {
			o := &p.Inlines[stack[in.Depth-1]]
			if !(o.Start <= in.Start && in.End <= o.End && o.BoxStart <= in.BoxStart && in.BoxEnd <= o.BoxEnd) {
				panic(fmt.Sprintf("text.NewLayout: Paragraph.Inlines[%d] is not inside its parent Inlines[%d]", i, stack[in.Depth-1]))
			}
		}
		if in.Start < prevEnd && in.Depth == 0 || in.BoxStart < prevBoxEnd && in.Depth == 0 {
			panic(fmt.Sprintf("text.NewLayout: Paragraph.Inlines[%d] overlaps the inline before it; inlines must be nested and in tree order", i))
		}
		if in.Depth == 0 {
			prevEnd, prevBoxEnd = in.End, in.BoxEnd
		}
		stack = append(stack, i)
	}
}

func runeBoundary(s string, i int) bool {
	return i == len(s) || i == 0 || (i > 0 && i < len(s) && utf8.RuneStart(s[i]))
}
