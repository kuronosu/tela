package text

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

func TestSynthesisRule(t *testing.T) {
	c := layoutCollection(t)
	for _, tc := range []struct {
		fam     string
		q       Query
		bold    bool
		oblique bool
	}{
		// A static Regular: bold from 600, oblique for italic and oblique.
		{"Noto Sans JP", Query{Weight: 400}, false, false},
		{"Noto Sans JP", Query{Weight: 500}, false, false},
		{"Noto Sans JP", Query{Weight: 600}, true, false},
		{"Noto Sans JP", Query{Weight: 900, Style: StyleItalic}, true, true},
		{"Noto Sans JP", Query{Style: StyleOblique}, false, true},
		// Variable fonts cover their weights; Inter has an italic face.
		{"Inter", Query{Weight: 900}, false, false},
		{"Inter", Query{Weight: 700, Style: StyleItalic}, false, false},
		// Noto Sans Arabic has no italic: slanted.
		{"Noto Sans Arabic", Query{Style: StyleItalic, Weight: 700}, false, true},
		// Color fonts are never synthesized.
		{"Noto Color Emoji", Query{Weight: 700, Style: StyleItalic}, false, false},
	} {
		tc.q.Families = []string{tc.fam}
		f := c.Face(tc.q, 20)
		if f.Typeface().Family() != tc.fam {
			t.Fatalf("%s: got %s", tc.fam, f.Typeface().Family())
		}
		bold, oblique := f.Synthetic()
		if (bold > 0) != tc.bold || oblique != tc.oblique {
			t.Errorf("%s %+v: synthetic bold %v oblique %v, want %v %v", tc.fam, tc.q, bold, oblique, tc.bold, tc.oblique)
		}
	}
}

func TestSynthesisKeepsLayout(t *testing.T) {
	// A synthetic face shapes and measures as its base: the same advances
	// and line metrics, so emboldening never rewraps text.
	text := synthText + " Hamburgefonstiv"
	st := TextStyle{Families: []string{"Noto Sans JP"}, Size: 20}
	regular := simple(t, text, ParagraphStyle{}, st, 1000)
	st.Weight, st.Style = 700, StyleItalic
	bold := simple(t, text, ParagraphStyle{}, st, 1000)
	if regular.Width() != bold.Width() || regular.Height() != bold.Height() {
		t.Errorf("synthetic bold italic %vx%v, regular %vx%v", bold.Width(), bold.Height(), regular.Width(), regular.Height())
	}
	c := layoutCollection(t)
	f := c.Face(Query{Families: []string{"Noto Sans JP"}, Weight: 700}, 20)
	if f.base == nil || f.FontKey() == f.base.FontKey() || f.Metrics() != f.base.Metrics() {
		t.Fatal("the synthetic face is not a face of its own over its base")
	}
	g, _ := f.GlyphIndex('H')
	if f.Advance(g) != f.base.Advance(g) {
		t.Error("synthetic bold changed an advance")
	}
	// Synthetic faces are shared by the sizes of one stroke width: 9 px
	// and below, 36 px and above.
	q := Query{Families: []string{"Noto Sans JP"}, Weight: 700}
	if c.Face(q, 40) != c.Face(q, 72) || c.Face(q, 6) != c.Face(q, 8) || c.Face(q, 12) == c.Face(q, 20) {
		t.Error("synthetic faces not shared by the sizes of one stroke width")
	}
}

func TestFakeBoldWidth(t *testing.T) {
	for _, c := range []struct{ size, want float32 }{{6, 1.0 / 24}, {9, 1.0 / 24}, {22.5, (1.0/24 + 1.0/32) / 2}, {36, 1.0 / 32}, {200, 1.0 / 32}} {
		if got := fakeBoldWidth(c.size); got-c.want > 1e-7 || c.want-got > 1e-7 {
			t.Errorf("size %g: %v, want %v", c.size, got, c.want)
		}
	}
}

func TestEmboldenGrowsEveryEdge(t *testing.T) {
	// A square with a square hole, in both windings: the outer edges move
	// out by h, the hole's edges in by h, the corners stay square.
	for _, flip := range []bool{false, true} {
		var p gfx.Path
		sq := func(x0, y0, x1, y1 float32, ccw bool) {
			pts := []gfx.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
			if ccw {
				pts[1], pts[3] = pts[3], pts[1]
			}
			p.MoveTo(pts[0])
			for _, q := range pts[1:] {
				p.LineTo(q)
			}
			p.Close()
		}
		sq(0, 0, 1, 1, flip)
		sq(0.25, 0.25, 0.75, 0.75, !flip)
		cs := contours(embolden(p, 0.05))
		if len(cs) != 2 {
			t.Fatalf("flip %v: %d contours", flip, len(cs))
		}
		bounds := func(c []pt2) [4]float64 {
			b := [4]float64{c[0].x, c[0].y, c[0].x, c[0].y}
			for _, q := range c {
				b = [4]float64{min(b[0], q.x), min(b[1], q.y), max(b[2], q.x), max(b[3], q.y)}
			}
			return b
		}
		near := func(a, b [4]float64) bool {
			for i := range a {
				if d := a[i] - b[i]; d > 1e-6 || d < -1e-6 {
					return false
				}
			}
			return true
		}
		if b := bounds(cs[0]); !near(b, [4]float64{-0.05, -0.05, 1.05, 1.05}) || len(cs[0]) != 4 {
			t.Errorf("flip %v: outer %v", flip, cs[0])
		}
		if b := bounds(cs[1]); !near(b, [4]float64{0.3, 0.3, 0.7, 0.7}) || len(cs[1]) != 4 {
			t.Errorf("flip %v: hole %v", flip, cs[1])
		}
	}
}

// synthDigest hashes the synthetic outlines of a few glyphs at several
// stroke widths, slanted and not: emboldening runs on the flattened
// outline in float64 with explicit rounding, so the outlines must be the
// same on every architecture (ADR-027).
func synthDigest(t testing.TB) uint64 {
	c := layoutCollection(t)
	d := newDigest()
	for _, fam := range []string{"Noto Sans JP", "Noto Sans Arabic"} {
		for _, size := range []float32{9, 14, 20, 40} {
			for _, style := range []Style{StyleNormal, StyleItalic} {
				f := c.Face(Query{Families: []string{fam}, Weight: 700, Style: style}, size)
				for _, r := range "HQgo&日語返ة" {
					if g, ok := f.GlyphIndex(r); ok {
						p, _ := f.GlyphOutline(g)
						d.path(p)
						b := f.GlyphBounds(g)
						d.f32(b.X0, b.Y0, b.X1, b.Y1)
					}
				}
			}
		}
	}
	return d.h.Sum64()
}

const wantSynthDigest uint64 = 0xc8defe5314fd4522

func TestSyntheticOutlinesDeterministic(t *testing.T) {
	if got := synthDigest(t); got != wantSynthDigest {
		t.Errorf("synthDigest = %#x, want %#x (the same on every architecture)", got, wantSynthDigest)
	}
}

func TestGoldenSynthetic(t *testing.T) {
	c := layoutCollection(t)
	b := gfx.NewBuilder()
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	y := float32(10)
	for _, row := range []struct {
		fam  string
		size float32
		text string
	}{
		{"Noto Sans JP", 28, "Synthetic 日本語"},
		{"Noto Sans JP", 14, "Bold and oblique at 14 px: 漢字とかな"},
		{"Noto Sans Arabic", 24, "مرحبا بالعالم"},
	} {
		x := float32(12)
		for _, s := range []struct {
			w float32
			s Style
		}{{400, StyleNormal}, {700, StyleNormal}, {400, StyleItalic}, {700, StyleItalic}} {
			st := TextStyle{Families: []string{row.fam}, Size: row.size, Weight: s.w, Style: s.s, Paint: ink, LineHeight: LineHeightPixels(row.size * 1.5)}
			l := NewLayout(c, &Paragraph{Text: row.text, Default: st, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}})
			l.BreakLines(1000)
			l.Paint(b, gfx.Pt(x, y))
			x += l.Width() + 28
		}
		y += row.size*1.5 + 8
	}
	const w, h = 1240, 140
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	raster.Clear(img, image.Rectangle{}, gfx.RGB8(0xf9, 0xfa, 0xfb))
	raster.Render(img, b.Finish(), raster.Options{Scale: 1})
	golden.Image(t, "synthetic_x1", img)
}
