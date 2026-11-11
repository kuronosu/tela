package text

import (
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// skipInkText is a paragraph whose underline the descenders of many
// glyphs cross.
var skipInkText = strings.Repeat("Typography gives quick jumping glyphs plenty of gaps: jq gyp pqg. ", 12)

// skipInkPara is skipInkText underlined (or not), in lines of 22.4 px, so
// that the baselines fall on several device-pixel phases at scale 1.5.
func skipInkPara(lines Decoration) *Paragraph {
	st := TextStyle{Families: []string{"Inter"}, Size: 16, Weight: 400, LineHeight: LineHeightPixels(22.4), Paint: gfx.Black,
		Decoration: TextDecoration{Lines: lines}}
	return &Paragraph{Text: skipInkText, Default: st}
}

// skipInkDump paints p in coll at scale k from origin and returns the
// dump of the display list.
func skipInkDump(coll *Collection, p *Paragraph, k float32, origin gfx.Point) string {
	l := NewLayout(coll, p)
	l.BreakLines(420)
	return paintDump(l, k, origin)
}

// paintDump paints l at scale k from origin and returns the dump of the
// display list.
func paintDump(l *Layout, k float32, origin gfx.Point) string {
	b := gfx.NewBuilder()
	l.PaintScaled(b, origin, k)
	return b.Finish().String()
}

type skipInkPaint struct {
	k      float32
	origin gfx.Point
	lines  Decoration
}

var skipInkPaints = []skipInkPaint{
	{1, gfx.Pt(0, 0), DecorationUnderline},
	{1.5, gfx.Pt(0, 0), DecorationUnderline},
	{2, gfx.Pt(0, 0), DecorationUnderline},
	{1.5, gfx.Pt(3.3, 7.7), DecorationUnderline},
	{1.25, gfx.Pt(0.4, 100.6), DecorationUnderline | DecorationOverline},
}

// The skip-ink gaps are a function of the glyphs, the band and the scale
// only: painting again (the intercepts cached) or in another order on
// another collection gives the same display list, bit for bit.
func TestSkipInkCache(t *testing.T) {
	cold := testCollection(t)
	want := make([]string, len(skipInkPaints))
	for i, c := range skipInkPaints {
		want[i] = skipInkDump(cold, skipInkPara(c.lines), c.k, c.origin)
		if n := strings.Count(want[i], "FillRect"); n < 100 {
			t.Fatalf("%v: %d rectangles: the underlines have no gaps", c, n)
		}
	}
	for i, c := range skipInkPaints {
		if got := skipInkDump(cold, skipInkPara(c.lines), c.k, c.origin); got != want[i] {
			t.Errorf("%v: painted again, the display list differs", c)
		}
	}
	other := testCollection(t)
	for i := len(skipInkPaints) - 1; i >= 0; i-- {
		c := skipInkPaints[i]
		if got := skipInkDump(other, skipInkPara(c.lines), c.k, c.origin); got != want[i] {
			t.Errorf("%v: painted in another order, the display list differs", c)
		}
	}
	// Painting the same lines again, or moved sideways, flattens nothing:
	// every intercept (an underline's and an overline's per glyph) is
	// kept, so no glyph's is replaced.
	l := NewLayout(cold, skipInkPara(DecorationUnderline|DecorationOverline))
	l.BreakLines(420)
	l.PaintScaled(gfx.NewBuilder(), gfx.Pt(5, 5), 1.5)
	m := l.inkHits.Load()
	if m == nil {
		t.Fatal("no intercepts kept")
	}
	kept := make([]*inkKept, len(l.glyphs))
	n := 0
	for g := range kept {
		if kept[g] = m.slot(g).Load(); kept[g] != nil {
			n++
		}
	}
	if n < 50 {
		t.Fatalf("%d glyphs keep intercepts", n)
	}
	for _, o := range []gfx.Point{gfx.Pt(5, 5), gfx.Pt(37.3, 5)} {
		l.PaintScaled(gfx.NewBuilder(), o, 1.5)
		for g := range kept {
			if m.slot(g).Load() != kept[g] {
				t.Fatalf("painting the same lines again at %v computed the intercept of glyph %d", o, g)
			}
		}
	}
}

// Paint is read-only: goroutines paint the same layouts at once, at
// several scales, keeping their intercepts in the same slots (run with the
// race detector).
func TestSkipInkConcurrent(t *testing.T) {
	ref := testCollection(t)
	want := make([]string, len(skipInkPaints))
	for i, c := range skipInkPaints {
		want[i] = skipInkDump(ref, skipInkPara(c.lines), c.k, c.origin)
	}
	coll := testCollection(t)
	layouts := map[Decoration]*Layout{}
	for _, c := range skipInkPaints {
		if layouts[c.lines] == nil {
			l := NewLayout(coll, skipInkPara(c.lines))
			l.BreakLines(420)
			layouts[c.lines] = l
		}
	}
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 2 * len(skipInkPaints) {
				i := (w + j) % len(skipInkPaints)
				c := skipInkPaints[i]
				if got := paintDump(layouts[c.lines], c.k, c.origin); got != want[i] {
					t.Errorf("worker %d: %v: the display list differs", w, c)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// BenchmarkSkipInkPaint paints a laid-out paragraph of 15 underlined lines
// again, as a layer records it again on every pointer move of a selection
// drag; "first" paints it as for the first time (no intercept kept), and
// "plain" is the same paragraph without the underline.
func BenchmarkSkipInkPaint(b *testing.B) {
	coll := layoutCollection(b)
	for _, c := range []struct {
		name  string
		lines Decoration
		first bool
	}{{"underline", DecorationUnderline, false}, {"underline-first", DecorationUnderline, true}, {"plain", 0, false}} {
		b.Run(c.name, func(b *testing.B) {
			l := NewLayout(coll, skipInkPara(c.lines))
			l.BreakLines(420)
			l.PaintScaled(gfx.NewBuilder(), gfx.Pt(0, 0), 2)
			b.ReportAllocs()
			for b.Loop() {
				if c.first {
					l.inkHits.Store(nil)
				}
				bd := gfx.NewBuilder()
				l.PaintScaled(bd, gfx.Pt(0, 0), 2)
				bd.Finish()
			}
		})
	}
}
