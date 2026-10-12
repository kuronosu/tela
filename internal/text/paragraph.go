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

// Paragraph is the input of NewLayout: a UTF-8 string, styled spans
// (byte offsets) and inline boxes.
//
// Spans must be sorted by Start, must not overlap and must start and end on
// rune boundaries; text that no span covers uses Default. Default is also
// the style of the block (the CSS root inline box): its first available
// font and line height form the strut that gives every line its minimum
// height, it styles the ellipsis of truncated lines, and tab stops are
// measured with its space. Invalid UTF-8 is laid out as U+FFFD, one per
// invalid byte.
type Paragraph struct {
	Text    string
	Style   ParagraphStyle
	Default TextStyle
	Spans   []Span
	Boxes   []InlineBox
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
}

func runeBoundary(s string, i int) bool {
	return i == len(s) || i == 0 || (i > 0 && i < len(s) && utf8.RuneStart(s[i]))
}
