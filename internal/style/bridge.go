package style

import "github.com/kuronosu/tela/internal/text"

// TextStyle returns the style of a span of text in this element: the
// inherited text properties in the form internal/text expects. Core uses it
// for the runs of a paragraph and for the paragraph's default style.
func (s *ComputedStyle) TextStyle() text.TextStyle {
	t := &s.Text
	st := text.TextStyle{
		Families:      t.FontFamily,
		Size:          t.FontSize,
		Weight:        t.FontWeight,
		Style:         t.FontStyle,
		Stretch:       t.FontStretch,
		LetterSpacing: t.LetterSpacing,
		WordSpacing:   t.WordSpacing,
		LineHeight:    t.LineHeight,
		Paint:         t.Color,
		Transform:     t.TextTransform,
		Numeric:       t.Numeric,
	}
	if t.Decoration.Lines != 0 {
		st.Decoration = text.TextDecoration{
			Lines:     t.Decoration.Lines,
			Paint:     t.Decoration.Color,
			Thickness: t.Decoration.Thickness,
		}
	}
	return st
}

// ParagraphStyle returns the properties that apply to a whole paragraph in
// this element: base direction, alignment, white-space, wrap style,
// breaking, truncation and clamping.
func (s *ComputedStyle) ParagraphStyle() text.ParagraphStyle {
	t := &s.Text
	return text.ParagraphStyle{
		Direction:    t.Direction,
		Align:        t.TextAlign,
		WhiteSpace:   t.WhiteSpace,
		WrapStyle:    t.WrapStyle,
		WordBreak:    t.WordBreak,
		OverflowWrap: t.OverflowWrap,
		TabSize:      t.TabSize,
		TextOverflow: s.TextOverflow,
		LineClamp:    s.LineClamp,
	}
}
