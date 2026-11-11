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
//
// The intercepts are computed in em units, from the outline and the band
// relative to the glyph's pen, and each glyph of a layout keeps its last
// ones (inkHits): a line painted again (a layer recorded again on every
// pointer move of a selection drag) flattens no outline, and a kept
// intercept is the one computed, bit for bit. The cache is the layout's,
// not the face's: the band of a glyph relative to its pen is the line's,
// which depends on the device-pixel phase of the baseline (and on the
// float32 rounding of its position), so a glyph shared by many lines has
// a band per line, more than a small cache per glyph can hold.

// span32 is an interval [a, b] of x.
type span32 struct{ a, b float32 }

// inkSkipMaxDilation is Blink's kDecorationClipMaxDilation, in device px.
const inkSkipMaxDilation = 13

// inkGaps appends to gaps the intervals of x, relative to the paint
// origin, where the decoration of cluster c whose top is at y below the
// line's baseline is not drawn, at display scale k (0: 1).
func (l *Layout) inkGaps(gaps []span32, c int32, y, k float32) []span32 {
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
	y0 := y + half
	y1 := max(y0, y+th-half)
	dil := min(th, inkSkipMaxDilation/k)
	dx, dy := l.clusterOff(c)
	x := l.cx[c] + dx
	for g := cl.g0; g < cl.g1; g++ {
		gd := &l.glyphs[g]
		pen := dy + gd.dy
		if a, b, ok := l.glyphIntercept(g, r.face, r.size, x+gd.dx, y0-pen, y1-pen, k); ok {
			gaps = append(gaps, span32{a - dil, b + dil})
		}
		x += gd.adv
	}
	return gaps
}

// glyphIntercept returns the leftmost and rightmost x of the outline of
// glyph g of the layout, drawn from face f at size with its pen at x = px,
// between the lines y0 and y1 below the pen (y0 <= y1, inclusive), or
// false when the outline does not reach the band. The outline is
// flattened to a tenth of a device pixel at display scale k.
func (l *Layout) glyphIntercept(g int32, f *Face, size, px, y0, y1, k float32) (a, b float32, ok bool) {
	if !(size > 0) || !finite32(size) {
		return 0, 0, false
	}
	e0, e1 := y0/size, y1/size // em
	if !finite32(e0) || !finite32(e1) {
		return 0, 0, false
	}
	id := l.glyphs[g].id
	bd := f.GlyphBounds(id)
	if bd.Empty() || bd.Y1 < e0 || bd.Y0 > e1 {
		return 0, 0, false
	}
	path, has := f.GlyphOutline(id)
	if !has {
		return 0, 0, false
	}
	lo, hi := l.inkIntercept(g, path, e0, e1, 0.1/(size*k))
	if !(lo <= hi) {
		return 0, 0, false
	}
	// explicit rounding: no fused multiply-add (ADR-041)
	return px + float32(lo*size), px + float32(hi*size), true
}

// inkBands is the number of bands whose intercepts a glyph of a layout
// keeps: its underline's and its overline's.
const inkBands = 2

// inkKept is the intercepts of a glyph with the last bands it was tested
// against, the most recent first. It is immutable and replaced whole, so
// the goroutines that paint a layout at once (Paint is read-only) read it
// without locks.
type inkKept struct {
	n  int
	at [inkBands]inkHit
}

// inkHit is the intercept of a glyph's outline with a band: the band and
// the flattening tolerance, keys compared bit for bit, and the leftmost
// and rightmost x inside the band (lo > hi when there is none), all in em
// units.
type inkHit struct {
	y0, y1, tol uint32 // float32 bits
	lo, hi      float32
}

// inkIntercept returns the leftmost and rightmost x of path, the outline
// of glyph g, between the lines y0 and y1 (em, y0 <= y1, inclusive),
// flattened to tol (em); lo > hi when the outline does not reach the
// band. The result is kept for the band and tolerance.
func (l *Layout) inkIntercept(g int32, path gfx.Path, y0, y1, tol float32) (lo, hi float32) {
	m := l.inkHits.Load()
	if m == nil {
		// Pages of slots are made for the glyphs that need one: a short
		// link in a long paragraph pays for its own glyphs.
		nm := newGlyphSlots[inkKept](len(l.glyphs))
		if l.inkHits.CompareAndSwap(nil, &nm) {
			m = &nm
		} else {
			m = l.inkHits.Load()
		}
	}
	slot := m.slot(int(g))
	key := inkHit{y0: math.Float32bits(y0), y1: math.Float32bits(y1), tol: math.Float32bits(tol)}
	old := slot.Load()
	if old != nil {
		for i := range old.n {
			if h := &old.at[i]; h.y0 == key.y0 && h.y1 == key.y1 && h.tol == key.tol {
				return h.lo, h.hi
			}
		}
	}
	key.lo, key.hi = outlineIntercept(path, y0, y1, tol)
	kept := &inkKept{n: 1}
	kept.at[0] = key
	if old != nil {
		kept.n += copy(kept.at[1:], old.at[:old.n])
	}
	// A lost race drops this band, computed again next time.
	slot.CompareAndSwap(old, kept)
	return key.lo, key.hi
}

// outlineIntercept returns the leftmost and rightmost x of path between
// the lines y0 and y1 (y0 <= y1, inclusive), flattened to tol, all in the
// path's units; lo > hi when the path does not reach the band.
func outlineIntercept(path gfx.Path, y0, y1, tol float32) (lo, hi float32) {
	lo, hi = float32(math.Inf(1)), float32(math.Inf(-1))
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
	path.Flatten(tol, func(v gfx.Verb, q gfx.Point) {
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
	return lo, hi
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
