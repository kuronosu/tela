package text

import (
	"math"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/kuronosu/tela/internal/gfx"
)

// goTextPath converts a go-text outline to an em-space path, for comparing
// with the deterministic glyf loader.
func goTextPath(o font.GlyphOutline, upem float64) gfx.Path {
	var p gfx.Path
	em := func(q ot.SegmentPoint) gfx.Point {
		return gfx.Point{X: float32(float64(q.X) / upem), Y: float32(-float64(q.Y) / upem)}
	}
	for _, s := range o.Segments {
		switch s.Op {
		case ot.SegmentOpMoveTo:
			p.MoveTo(em(s.Args[0]))
		case ot.SegmentOpLineTo:
			p.LineTo(em(s.Args[0]))
		case ot.SegmentOpQuadTo:
			p.QuadTo(em(s.Args[0]), em(s.Args[1]))
		case ot.SegmentOpCubeTo:
			p.CubicTo(em(s.Args[0]), em(s.Args[1]), em(s.Args[2]))
		}
	}
	return p
}

func countMoves(p gfx.Path) int {
	n := 0
	for _, v := range p.Verbs() {
		if v == gfx.MoveTo {
			n++
		}
	}
	return n
}

// TestOutlinesMatchGoText compares every glyph of the variable embedded
// fonts, at default and non-default coordinates, with go-text's outlines:
// same contours, same bounds (go-text computes in float32, so variable
// outlines may differ by rounding).
func TestOutlinesMatchGoText(t *testing.T) {
	c := NewDefaultCollection()
	for _, q := range []struct {
		q    Query
		size float32
	}{
		{Query{Families: []string{"Inter"}}, 0},
		{Query{Families: []string{"Inter"}, Weight: 650}, 20},
		{Query{Families: []string{"Inter"}, Weight: 200, Style: StyleItalic}, 32},
		{Query{Families: []string{"JetBrains Mono"}, Weight: 800}, 0},
		{Query{Families: []string{"Noto Sans Arabic"}, Weight: 300, Stretch: 70}, 0},
	} {
		f := c.Face(q.q, q.size)
		gt := font.NewFace(f.tf.ft)
		gt.SetCoords(f.coords)
		tol := float32(1e-6)
		if f.coords != nil {
			tol = 0.01 / float32(f.upem) // a hundredth of a font unit
		}
		bad := 0
		for g := 0; g < f.tf.nGlyphs; g++ {
			ours, _ := f.GlyphOutline(gfx.GlyphID(g))
			o, _ := gt.GlyphDataOutline(font.GID(g))
			theirs := goTextPath(o, f.upem)
			a, b := ours.ControlBounds(), theirs.ControlBounds()
			d := max(abs32(a.X0-b.X0), abs32(a.Y0-b.Y0), abs32(a.X1-b.X1), abs32(a.Y1-b.Y1))
			if countMoves(ours) != countMoves(theirs) || d > tol {
				if bad++; bad <= 5 {
					t.Errorf("%s glyph %d: %d contours, bounds %v; go-text: %d contours, bounds %v",
						f.FontName(), g, countMoves(ours), a, countMoves(theirs), b)
				}
			}
			// Advances: HarfBuzz rounds variable advances to font units;
			// go-text does not.
			want := gt.HorizontalAdvance(font.GID(g))
			if got := f.Advance(gfx.GlyphID(g)) * float32(f.upem); math.Abs(float64(got-want)) > 0.5+1e-3 {
				if bad++; bad <= 5 {
					t.Errorf("%s glyph %d: advance %g font units, go-text %g", f.FontName(), g, got, want)
				}
			}
		}
	}
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

// TestOutlineKnownGlyphs checks two glyphs of Inter's default instance
// against the values in the font: 'O' has two contours and 'H' sits on the
// baseline with the cap height (1490 units) as its top.
func TestOutlineKnownGlyphs(t *testing.T) {
	f := NewDefaultCollection().Match("Inter", Query{}).Face(Instance{})
	if f.Coords() != nil {
		t.Fatalf("default instance has coordinates %v", f.Coords())
	}
	o, _ := f.GlyphIndex('O')
	p, ok := f.GlyphOutline(o)
	if !ok || countMoves(p) != 2 {
		t.Errorf("'O': ok=%v, %d contours, want 2", ok, countMoves(p))
	}
	h, _ := f.GlyphIndex('H')
	b := f.GlyphBounds(h)
	if b.Y1 != 0 || b.Y0 != -1490.0/2048 {
		t.Errorf("'H' bounds %v: want Y0 = %g (cap height), Y1 = 0 (baseline)", b, -1490.0/2048)
	}
	if b.X0 <= 0 || b.X1 >= f.Advance(h) {
		t.Errorf("'H' bounds %v are not inside its advance %g", b, f.Advance(h))
	}
	if sp, _ := f.GlyphIndex(' '); !f.GlyphBounds(sp).Empty() {
		t.Errorf("space has bounds %v", f.GlyphBounds(sp))
	} else if _, ok := f.GlyphOutline(sp); ok {
		t.Error("space has an outline")
	}
	if f.ColorGlyph(h, gfx.Black) != nil {
		t.Error("'H' of Inter is a color glyph")
	}
	// Out-of-range glyphs are empty, not a panic.
	if _, ok := f.GlyphOutline(1 << 20); ok || !f.GlyphBounds(1<<20).Empty() {
		t.Error("out-of-range glyph is not empty")
	}
}

// TestCFFOutlines covers CFF fonts (the CJK test subset).
func TestCFFOutlines(t *testing.T) {
	c := testCollection(t)
	f := c.Face(Query{Families: []string{"Noto Sans JP"}}, 16)
	if f.tf.glyf != nil {
		t.Fatal("the CJK subset should be a CFF font")
	}
	g, ok := f.GlyphIndex('語')
	if !ok {
		t.Fatal("no glyph for 語")
	}
	p, ok := f.GlyphOutline(g)
	b := f.GlyphBounds(g)
	if !ok || countMoves(p) < 5 || b.Empty() || b.Y0 > -0.7 || b.Y1 < 0 {
		t.Errorf("語: ok=%v, %d contours, bounds %v", ok, countMoves(p), b)
	}
	if a := f.Advance(g); a != 1 {
		t.Errorf("語 advance %g em, want 1", a)
	}
}

// TestMetricsInter checks the metrics of Inter against its tables: hhea
// ascender 1984 and descender -494 (USE_TYPO_METRICS is not set), line gap
// 0, cap height 1490, x-height 1118 at the default instance, and the MVAR
// adjusted values that go-text computes at other coordinates.
func TestMetricsInter(t *testing.T) {
	c := NewDefaultCollection()
	f := c.Match("Inter", Query{}).Face(Instance{})
	m := f.Metrics()
	want := Metrics{UnitsPerEm: 2048, Ascent: 1984.0 / 2048, Descent: 494.0 / 2048, CapHeight: 1490.0 / 2048, XHeight: 1118.0 / 2048}
	if m.UnitsPerEm != want.UnitsPerEm || m.Ascent != want.Ascent || m.Descent != want.Descent || m.LineGap != 0 ||
		m.CapHeight != want.CapHeight || m.XHeight != want.XHeight {
		t.Errorf("Inter metrics %+v, want %+v", m, want)
	}
	if !(m.UnderlineOffset > 0 && m.UnderlineThickness > 0 && m.StrikeoutOffset < 0 && m.StrikeoutThickness > 0) {
		t.Errorf("decorations %+v: want the underline below and the strikeout above the baseline", m)
	}
	if s := m.Scale(16); s.Ascent != m.Ascent*16 || s.UnitsPerEm != 2048 {
		t.Errorf("Scale(16) = %+v", s)
	}

	for _, q := range []Query{{Families: []string{"Inter"}, Weight: 800}, {Families: []string{"Inter"}, Weight: 100, Style: StyleItalic}} {
		for _, size := range []float32{14, 32} {
			f := c.Face(q, size)
			gt := font.NewFace(f.tf.ft)
			gt.SetCoords(f.coords)
			ext, _ := gt.FontHExtents()
			m := f.Metrics()
			u := float32(f.upem)
			check := func(name string, got, want float32) {
				if abs32(got-want) > 1e-3 {
					t.Errorf("%s: %s = %g, go-text %g", f.FontName(), name, got, want)
				}
			}
			check("ascent", m.Ascent*u, ext.Ascender)
			check("descent", -m.Descent*u, ext.Descender)
			check("line gap", m.LineGap*u, ext.LineGap)
			check("x-height", m.XHeight*u, gt.LineMetric(font.XHeight))
			check("cap height", m.CapHeight*u, gt.LineMetric(font.CapHeight))
			check("underline", -m.UnderlineOffset*u, gt.LineMetric(font.UnderlinePosition))
			check("strikeout", -m.StrikeoutOffset*u, gt.LineMetric(font.StrikethroughPosition))
		}
	}
}

// TestFaceInterning checks that equal coordinates give the same Face.
func TestFaceInterning(t *testing.T) {
	c := NewDefaultCollection()
	a := c.Face(Query{Families: []string{"Inter"}, Weight: 700}, 16)
	b := c.Face(Query{Families: []string{"sans-serif"}, Weight: 700}, 16.3) // same opsz bucket
	if a != b {
		t.Errorf("same instance, different faces: %s and %s", a.FontName(), b.FontName())
	}
	if c.Face(Query{Families: []string{"Inter"}, Weight: 700}, 17) == a {
		t.Error("opsz 17 and 16 share a face")
	}
	w := c.Face(Query{Families: []string{"Inter"}, Weight: 1000}, 16) // clamped to the axis maximum
	if w != c.Face(Query{Families: []string{"Inter"}, Weight: 900}, 16) {
		t.Error("weight 1000 is not clamped to Inter's maximum 900")
	}
	// Collections share the embedded typefaces and so their faces.
	if NewDefaultCollection().Face(Query{Families: []string{"Inter"}, Weight: 700}, 16) != a {
		t.Error("a second default collection made a new face")
	}
	keys := map[uint64]bool{}
	for _, f := range []*Face{a, w, c.Face(Query{}, 16), c.Face(Query{Families: []string{"monospace"}}, 16)} {
		if keys[f.FontKey()] {
			t.Errorf("%s: duplicate font key", f.FontName())
		}
		keys[f.FontKey()] = true
	}
	if got := a.FontName(); got != "Inter (opsz 16, wght 700)" {
		t.Errorf("FontName = %q", got)
	}
}
