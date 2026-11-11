package text

import (
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestStyleRunInsideGrapheme documents what a style run that starts inside
// a grapheme cluster does (Span's rule), against Chromium 155: the same
// markup (e<span>&#x301;</span>x and 👩<span>&#x200D;💻</span>x in Inter
// and Noto Color Emoji at 48 px) measured with Range.getBoundingClientRect
// per character and looked at in a screenshot.
//
//   - A span that only changes the paint (a color) does not split the
//     shaping item: "é" and the ZWJ sequence 👩‍💻 are shaped whole, one
//     glyph each, as one cluster with the style of its first character.
//     Chromium also shapes across the element boundary and paints the
//     glyph with the first fragment's color; the span's own fragment is
//     empty (width 0).
//   - A span that changes the shaping (weight, size) splits the item:
//     the run that starts inside the grapheme is shaped alone, a lone
//     combining mark (zero advance, drawn over the "e") or the rest of a
//     broken ZWJ sequence (ZWJ and 💻 at 24 px), as in Chromium, with the
//     same advances. The split is a cluster boundary, so the caret can
//     stop there (Chromium's caret moves by grapheme across elements:
//     a difference, recorded as a known limitation).
//
// Nothing panics: painting, selection, hit-testing and caret movement walk
// every cluster.
func TestStyleRunInsideGrapheme(t *testing.T) {
	base := TextStyle{Families: []string{"Inter", "Noto Color Emoji"}, Size: 48, Weight: 400, Paint: gfx.Black}
	red := base
	red.Paint = gfx.RGB8(255, 0, 0)
	bold := red
	bold.Weight = 800
	small := base
	small.Size = 24
	type clus struct {
		start, end int
		adv        float32 // Chromium's
		style      uint16
	}
	for _, c := range []struct {
		name       string
		text       string
		start, end int
		st         TextStyle
		want       []clus
		stops      []int // MoveForward from 0
	}{
		{"color on a combining mark", "éx", 1, 3, red,
			[]clus{{0, 3, 24.33, 0}, {3, 4, 24.33, 0}}, []int{3, 4}},
		{"weight on a combining mark", "éx", 1, 3, bold,
			[]clus{{0, 1, 25.73, 0}, {1, 3, 0, 1}, {3, 4, 24.33, 0}}, []int{1, 3, 4}},
		{"color on a diaeresis", "äx", 1, 3, red,
			[]clus{{0, 3, 24.84, 0}, {3, 4, 24.33, 0}}, []int{3, 4}},
		{"color inside a ZWJ sequence", "👩‍💻x", 4, 11, red,
			[]clus{{0, 11, 59.77, 0}, {11, 12, 24.33, 0}}, []int{11, 12}},
		{"size inside a ZWJ sequence", "👩‍💻x", 4, 11, small,
			[]clus{{0, 4, 59.77, 0}, {4, 11, 29.9, 1}, {11, 12, 24.33, 0}}, []int{4, 11, 12}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := &Paragraph{Text: c.text, Default: base, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap},
				Spans: []Span{{Start: c.start, End: c.end, Style: c.st}}}
			l := NewLayout(layoutCollection(t), p)
			l.BreakLines(float32(math.Inf(1)))
			if l.NumClusters() != len(c.want) {
				t.Fatalf("%d clusters, want %d", l.NumClusters(), len(c.want))
			}
			for i, w := range c.want {
				cl := l.Cluster(i)
				if cl.TextStart != w.start || cl.TextEnd != w.end || l.clusters[i].style != w.style {
					t.Errorf("cluster %d: text [%d,%d) style %d, want [%d,%d) style %d",
						i, cl.TextStart, cl.TextEnd, l.clusters[i].style, w.start, w.end, w.style)
				}
				if math.Abs(float64(cl.Advance-w.adv)) > 0.02 {
					t.Errorf("cluster %d: advance %.3f, Chromium's %.2f", i, cl.Advance, w.adv)
				}
			}
			var stops []int
			s := Selection{}
			for range len(c.stops) {
				s = l.Move(s, MoveForward, false)
				stops = append(stops, s.Focus.Offset)
			}
			for i := range stops {
				if stops[i] != c.stops[i] {
					t.Errorf("caret stops %v, want %v", stops, c.stops)
					break
				}
			}
			b := gfx.NewBuilder()
			l.PaintScaled(b, gfx.Pt(0, 0), 2)
			b.Finish()
			l.SelectionRects(l.SelectAll(), nil)
			for x := float32(-5); x < l.Width()+5; x += 3 {
				l.HitTest(x, 20)
			}
		})
	}
}
