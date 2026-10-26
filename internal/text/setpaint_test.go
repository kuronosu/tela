package text

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestSetPaintMatchesNewLayout checks that recoloring a layout's styles
// (Layout.SetPaint: the default, a span with an underline whose color is
// currentColor, a span with a decoration color of its own) paints exactly
// what NewLayout paints for the paragraph with the new paints, without
// changing its lines (ADR-161).
func TestSetPaintMatchesNewLayout(t *testing.T) {
	c := NewDefaultCollection()
	red, blue, green := gfx.Color{R: 1, A: 1}, gfx.Color{B: 1, A: 1}, gfx.Color{G: 0.5, A: 1}
	para := func(def, span1, span2, deco2 gfx.Paint) *Paragraph {
		return &Paragraph{
			Text:    "Recolored text keeps its lines and its underlines.",
			Default: TextStyle{Size: 14, Paint: def},
			Spans: []Span{
				{Start: 10, End: 14, Style: TextStyle{Size: 14, Paint: span1, Decoration: TextDecoration{Lines: DecorationUnderline}}},
				{Start: 37, End: 50, Style: TextStyle{Size: 14, Paint: span2, Decoration: TextDecoration{Lines: DecorationUnderline, Paint: deco2}}},
			},
			Scale: 2,
		}
	}
	paint := func(l *Layout) *gfx.DisplayList {
		b := gfx.NewBuilder()
		l.PaintScaled(b, gfx.Point{X: 3, Y: 5}, 2)
		return b.Finish()
	}
	l := NewLayout(c, para(red, red, red, nil))
	l.BreakLines(120)
	lines, width := l.NumLines(), l.Width()
	l.SetPaint(0, blue, nil)
	l.SetPaint(1, green, nil)
	l.SetPaint(2, red, blue)
	l.SetPaint(9, blue, nil) // out of range: nothing
	want := NewLayout(c, para(blue, green, red, blue))
	want.BreakLines(120)
	if l.NumLines() != lines || l.Width() != width {
		t.Fatalf("SetPaint changed the lines: %d lines, width %g; were %d, %g", l.NumLines(), l.Width(), lines, width)
	}
	got, exp := paint(l), paint(want)
	if got.Len() != exp.Len() {
		t.Fatalf("recolored layout paints %d commands, a new layout %d", got.Len(), exp.Len())
	}
	for i := range got.Len() {
		if !gfx.Equal(got.At(i), exp.At(i)) {
			t.Errorf("command %d: recolored %#v, new layout %#v", i, got.At(i), exp.At(i))
		}
	}
}
