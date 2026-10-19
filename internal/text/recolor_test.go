package text

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestPaintLineRecolored: the glyphs of a recolored style and its
// currentColor decorations take the new paint; other styles, and
// decorations with a color of their own, keep theirs; nothing moves.
func TestPaintLineRecolored(t *testing.T) {
	red, blue, green := gfx.RGB8(255, 0, 0), gfx.RGB8(0, 0, 255), gfx.RGB8(0, 160, 0)
	under := TextDecoration{Lines: DecorationUnderline}
	p := &Paragraph{
		Text:    "plain styled other",
		Default: TextStyle{Size: 16, Paint: red, Decoration: under},
		Spans: []Span{
			{Start: 6, End: 12, Style: TextStyle{Size: 16, Paint: blue, Decoration: TextDecoration{Lines: DecorationUnderline, Paint: green}}},
		},
	}
	l := NewLayout(layoutCollection(t), p)
	paints := func(over []gfx.Paint) (glyphs, rects []gfx.Paint, origins []gfx.Point) {
		b := gfx.NewBuilder()
		l.PaintLineRecolored(b, gfx.Point{}, 0, 1, over)
		for _, c := range b.Finish().All() {
			switch c := c.(type) {
			case gfx.DrawGlyphRun:
				glyphs = append(glyphs, c.Paint)
				origins = append(origins, c.Origin)
			case gfx.FillRect:
				rects = append(rects, c.Paint)
			}
		}
		return
	}
	g0, r0, o0 := paints(nil)
	if len(g0) != 3 || g0[0] != red || g0[1] != blue || g0[2] != red {
		t.Fatalf("glyph runs %v", g0)
	}
	black := gfx.Color{A: 1}
	g1, r1, o1 := paints([]gfx.Paint{black})
	if len(g1) != 3 || g1[0] != black || g1[1] != blue || g1[2] != black {
		t.Errorf("recolored style 0: %v", g1)
	}
	// The default style's underline follows its text; the span's green one
	// does not.
	if len(r1) != len(r0) {
		t.Fatalf("decorations %v, before %v", r1, r0)
	}
	for i := range r0 {
		want := r0[i]
		if r0[i] == red {
			want = black
		}
		if r1[i] != want {
			t.Errorf("decoration %d: %v, want %v", i, r1[i], want)
		}
	}
	for i := range o0 {
		if o0[i] != o1[i] {
			t.Errorf("run %d moved: %v -> %v", i, o0[i], o1[i])
		}
	}
	// Merging runs: recolor the span like its neighbors and the three runs
	// of the same font become one.
	if g2, _, _ := paints([]gfx.Paint{nil, red}); len(g2) != 1 || g2[0] != red {
		t.Errorf("runs of one paint are not merged: %v", g2)
	}
}
