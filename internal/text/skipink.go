package text

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

// Skip-ink (CSS Text Decoration 4, text-decoration-skip-ink: auto, the
// initial value; Tailwind has no utility for it, so it is always on).
// Underlines and overlines leave a gap where a glyph's outline crosses
// them; line-throughs do not. Tela follows Blink
// (TextPainter::ClipDecorationsStripe and DecorationsStripeIntercepts,
// with Skia's SkFont::getIntercepts):
//
//   - the band tested starts at the line's top and is as tall as its
//     resolved thickness before it is floored to device pixels (the
//     style's, or a tenth of the font size, at least 1 px: Blink's
//     ResolvedThickness), inset by half a device pixel at the top and the
//     bottom (Blink ignores intersections of less than half a pixel);
//   - each glyph that crosses the band gives one interval, from the
//     leftmost to the rightmost x of its outline inside the band (Skia's
//     per-glyph intercept: the loop of a "g" is one interval, its gap
//     included);
//   - the interval grows on both sides by that resolved thickness (at
//     most 13 device pixels), and the line is not drawn there. Blink clips
//     the line out with a rectangle that is not anti-aliased, so the gaps
//     end on whole device pixels.
//
// Measured against Chromium 155 (TestChromiumSkipInk: 16 cells at x1 and
// x2, the band and the dilation with the unrounded thickness fit best):
// on the row through each line's middle, 0 to 4 columns differ (12 to 103
// without skip-ink).
//
// Only the glyphs of the clusters that carry the decoration cut it. Color
// glyphs (emoji) cut it by their outline glyph, if their font has one, as
// in Skia.

// span32 is an interval [a, b] of x.
type span32 struct{ a, b float32 }

// inkSkipMaxDilation is Blink's kDecorationClipMaxDilation, in device px.
const inkSkipMaxDilation = 13

// inkGaps appends to gaps the intervals of x, relative to the paint
// origin, where the decoration of cluster c whose top is at y (from the
// baseline at by, absolute) is not drawn, at display scale k (0: 1).
func (l *Layout) inkGaps(gaps []span32, c int32, by, y, k float32) []span32 {
	if !(k > 0) {
		k = 1
	}
	cl := &l.clusters[c]
	if cl.g0 == cl.g1 || int(cl.run) >= len(l.runs) {
		return gaps
	}
	r := &l.runs[cl.run]
	if r.kind != runText || r.face == nil {
		return gaps
	}
	th := l.styles[cl.style].Decoration.Thickness
	if !(th > 0) || !finite32(th) {
		th = max(1, l.sinfo[cl.style].size/10)
	}
	half := 0.5 / k
	y0 := by + y + half
	y1 := max(y0, by+y+th-half)
	dil := min(th, inkSkipMaxDilation/k)
	x := l.cx[c]
	for g := cl.g0; g < cl.g1; g++ {
		gd := &l.glyphs[g]
		if a, b, ok := glyphIntercept(r.face, gd.id, r.size, x+gd.dx, by+gd.dy, y0, y1, k); ok {
			gaps = append(gaps, span32{a - dil, b + dil})
		}
		x += gd.adv
	}
	return gaps
}

// glyphIntercept returns the leftmost and rightmost x of glyph id's
// outline, drawn at size with its pen at (px, py), between the lines y0
// and y1 (y0 <= y1, inclusive), or false when the outline does not reach
// the band. The outline is flattened to a tenth of a device pixel at
// display scale k.
func glyphIntercept(f *Face, id gfx.GlyphID, size, px, py, y0, y1, k float32) (a, b float32, ok bool) {
	bd := f.GlyphBounds(id)
	if bd.Empty() || py+float32(bd.Y1*size) < y0 || py+float32(bd.Y0*size) > y1 {
		return 0, 0, false
	}
	path, has := f.GlyphOutline(id)
	if !has {
		return 0, 0, false
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	add := func(x float32) { lo, hi = min(lo, x), max(hi, x) }
	var start, prev gfx.Point
	edge := func(p, q gfx.Point) {
		if p.Y >= y0 && p.Y <= y1 {
			add(p.X)
		}
		for _, yl := range [2]float32{y0, y1} {
			if (p.Y < yl) != (q.Y < yl) {
				t := (yl - p.Y) / (q.Y - p.Y)
				add(p.X + float32(t*(q.X-p.X))) // explicit rounding: no fused multiply-add (ADR-041)
			}
		}
	}
	tol := 0.1 / (size * k) // em units
	path.Flatten(tol, func(v gfx.Verb, pt gfx.Point) {
		q := gfx.Point{X: px + float32(pt.X*size), Y: py + float32(pt.Y*size)}
		switch v {
		case gfx.MoveTo:
			start, prev = q, q
		case gfx.LineTo:
			edge(prev, q)
			prev = q
		case gfx.Close:
			edge(prev, start)
			prev = start
		}
	})
	if !(lo <= hi) {
		return 0, 0, false
	}
	return lo, hi, true
}

// fillSkipping fills the decoration rectangle from x0 to x1 (relative to
// origin) and from top to bottom (absolute) with paint, but for the gaps,
// whose ends are snapped to whole device pixels at scale k (0: none).
func fillSkipping(b *gfx.Builder, origin gfx.Point, x0, x1, top, bottom float32, gaps []span32, k float32, paint gfx.Paint) {
	slices.SortFunc(gaps, func(p, q span32) int {
		switch {
		case p.a < q.a:
			return -1
		case p.a > q.a:
			return 1
		}
		return 0
	})
	snap := func(v float32) float32 {
		if !(k > 0) {
			return v
		}
		return float32(gfx.RoundDevice(float64(origin.X+v)*float64(k))/float64(k)) - origin.X
	}
	at := x0
	for _, g := range gaps {
		a, e := snap(g.a), snap(g.b)
		if a > at {
			b.FillRect(gfx.Rect{X0: origin.X + at, Y0: top, X1: origin.X + min(a, x1), Y1: bottom}, paint)
		}
		at = max(at, e)
		if at >= x1 {
			return
		}
	}
	if at < x1 {
		b.FillRect(gfx.Rect{X0: origin.X + at, Y0: top, X1: origin.X + x1, Y1: bottom}, paint)
	}
}
