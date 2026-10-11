package raster

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// fillRRect fills a rounded rectangle in local space: with the analytic
// fast path under scale/translate transforms, otherwise as a path.
func (r *Renderer) fillRRect(rr gfx.RRect, paint gfx.Paint) {
	r.blit.setShader(r.shaderFor(paint, r.ctm))
	if rr.IsRect() {
		r.fillRect(rr.Rect, paint)
		return
	}
	if r.ctm.IsScaleTranslate() && r.fillRRectFast(rr) {
		return
	}
	r.fillPath(rr.Path(), gfx.NonZero)
}

// corner is one elliptical corner of a device-space rounded rectangle.
type corner struct {
	on                 bool    // both radii are positive
	cx, cy             float64 // center of the ellipse
	rx, ry             float64
	area               float64 // rx*ry
	sx, sy             float64 // ±1: u = sx*(x-cx)/rx is in [0, 1] inside the corner box
	bx0, by0, bx1, by1 float64 // the corner box
}

func newCorner(cx, cy, rx, ry, sx, sy float64) corner {
	c := corner{on: rx > 0 && ry > 0, cx: cx, cy: cy, rx: rx, ry: ry, area: float64(rx * ry), sx: sx, sy: sy}
	c.bx0, c.bx1 = min(cx, cx+sx*rx), max(cx, cx+sx*rx) // sx*rx is exact (sign change)
	c.by0, c.by1 = min(cy, cy+sy*ry), max(cy, cy+sy*ry)
	return c
}

// fillRRectFast fills rr with the analytic fast path under the current
// scale/translate transform. It returns false, having drawn nothing, when
// the rectangle reaches beyond ±coordLimit device pixels (the caller then
// uses the path rasterizer, which clips the outline exactly).
//
// The coverage of a pixel is the coverage of the rectangle (computed exactly
// like fillRectFixed, on the 1/256 grid) minus, for every corner box the
// pixel meets, the area of pixel ∩ box that lies outside the corner ellipse
// (corner.addOutside, in closed form). This is the exact area of pixel ∩
// rrect, because no point is subtracted twice:
//
//   - the boxes of adjacent corners do not overlap, since normalized radii
//     fit along each side;
//   - the boxes of opposite corners can overlap (for example top-left and
//     bottom-right radii both larger than half the size), but their outside
//     regions cannot. In normalized coordinates a point of the top-left
//     outside region has α²+β² > 1, so α+β > 1, where α = (cx-x)/rx and
//     β = (cy-y)/ry; for the bottom-right corner likewise α'+β' > 1. Inside
//     both boxes α*rx + α'*rx' = rx + rx' - w <= min(rx, rx'), which gives
//     α + α' <= 1, and β + β' <= 1 in the same way: a contradiction.
//
// TestRRectFastPathExact checks the result against supersampling.
func (r *Renderer) fillRRectFast(rr gfx.RRect) bool {
	m := &r.ctm
	sx, sy := float64(m[0]), float64(m[3])
	x0, y0 := apply(m, gfx.Pt(rr.Rect.X0, rr.Rect.Y0))
	x1, y1 := apply(m, gfx.Pt(rr.Rect.X1, rr.Rect.Y1))
	var rad [4][2]float64 // TL, TR, BR, BL; exact products of float32 values
	for i, p := range rr.Radii {
		rad[i] = [2]float64{float64(p.X) * math.Abs(sx), float64(p.Y) * math.Abs(sy)}
	}
	tl, tr, br, bl := gfx.TopLeft, gfx.TopRight, gfx.BottomRight, gfx.BottomLeft
	if x0 > x1 { // horizontal flip: left and right corners swap
		x0, x1 = x1, x0
		rad[tl], rad[tr], rad[br], rad[bl] = rad[tr], rad[tl], rad[bl], rad[br]
	}
	if y0 > y1 { // vertical flip: top and bottom corners swap
		y0, y1 = y1, y0
		rad[tl], rad[tr], rad[br], rad[bl] = rad[bl], rad[br], rad[tr], rad[tl]
	}
	const lim = coordLimit
	if !(x0 >= -lim && x1 <= lim && y0 >= -lim && y1 <= lim) {
		return false
	}
	X0, Y0, X1, Y1 := fix(x0), fix(y0), fix(x1), fix(y1)
	if X0 >= X1 || Y0 >= Y1 {
		return true // snapped to nothing, like the same rectangle as a path
	}
	// The snapped rectangle, exactly, in pixels.
	qx0, qy0 := float64(X0)/one, float64(Y0)/one
	qx1, qy1 := float64(X1)/one, float64(Y1)/one
	w, h := qx1-qx0, qy1-qy0

	// Snapping can shrink a side below the sum of its radii by up to 1/256
	// pixel: scale the radii down again like gfx.RRect.Normalize.
	for i := range rad {
		if !(rad[i][0] > 0 && rad[i][1] > 0) {
			rad[i] = [2]float64{}
		}
	}
	f := 1.0
	for _, s := range [4][2]float64{
		{w, rad[tl][0] + rad[tr][0]}, {h, rad[tr][1] + rad[br][1]},
		{w, rad[bl][0] + rad[br][0]}, {h, rad[tl][1] + rad[bl][1]},
	} {
		if s[1] > s[0] {
			f = min(f, s[0]/s[1])
		}
	}
	if f < 1 {
		for i := range rad {
			rad[i] = [2]float64{float64(rad[i][0] * f), float64(rad[i][1] * f)}
		}
	}

	corners := [4]corner{
		newCorner(qx0+rad[tl][0], qy0+rad[tl][1], rad[tl][0], rad[tl][1], -1, -1),
		newCorner(qx1-rad[tr][0], qy0+rad[tr][1], rad[tr][0], rad[tr][1], 1, -1),
		newCorner(qx1-rad[br][0], qy1-rad[br][1], rad[br][0], rad[br][1], 1, 1),
		newCorner(qx0+rad[bl][0], qy1-rad[bl][1], rad[bl][0], rad[bl][1], -1, 1),
	}

	b := &r.blit
	px0 := max(int(X0>>fracBits), b.clip.Min.X)
	px1 := min(int((X1+one-1)>>fracBits), b.clip.Max.X)
	py0 := max(int(Y0>>fracBits), b.clip.Min.Y)
	py1 := min(int((Y1+one-1)>>fracBits), b.clip.Max.Y)
	if px0 >= px1 || py0 >= py1 {
		return true
	}
	n := px1 - px0
	if cap(r.rowMask) < n {
		r.rowMask = make([]uint8, n)
		r.rowSub = make([]float64, n)
	}
	mask, sub := r.rowMask[:n], r.rowSub[:n]

	type span struct {
		x0, x1 int
		c      *corner
	}
	for y := py0; y < py1; y++ {
		top := int64(y) << fracBits
		covy := min(Y1, top+one) - max(Y0, top)
		fy0 := float64(y)
		// The columns of this row where a corner cuts the pixel: at most
		// one span per corner, sorted by x (insertion sort of <= 4).
		var spans [4]span
		ns := 0
		for k := range corners {
			c := &corners[k]
			x0, x1 := c.columns(fy0)
			x0, x1 = max(x0, px0), min(x1, px1)
			if x0 >= x1 {
				continue
			}
			i := ns
			for i > 0 && spans[i-1].x0 > x0 {
				spans[i] = spans[i-1]
				i--
			}
			spans[i] = span{x0, x1, c}
			ns++
		}
		// Walk the row: plain rectangle runs between the corner spans,
		// exact masks over (merged) corner spans.
		x := px0
		for i := 0; i < ns; {
			lo, hi := spans[i].x0, spans[i].x1
			j := i + 1
			for j < ns && spans[j].x0 < hi { // merge overlapping spans
				hi = max(hi, spans[j].x1)
				j++
			}
			if x < lo {
				b.rectRow(y, X0, X1, covy, x, lo)
			}
			lo = max(lo, x)
			m, sb := mask[:hi-lo], sub[:hi-lo]
			clear(sb)
			for _, s := range spans[i:j] {
				x0 := max(s.x0, lo)
				s.c.addOutside(x0, s.x1, fy0, sb[x0-lo:])
			}
			for k := range m {
				a := rectCovX(lo+k, X0, X1) * covy
				if sb[k] == 0 {
					m[k] = cov8(a)
				} else {
					m[k] = unitTo8(float64(a)/(one*one) - sb[k])
				}
			}
			b.mask(y, lo, m)
			x, i = hi, j
		}
		if x < px1 {
			b.rectRow(y, X0, X1, covy, x, px1)
		}
	}
	return true
}

// columns returns the range of pixel columns [x0, x1) of the row starting
// at py where the corner may change the coverage: the pixels of the corner
// box that are not entirely inside the ellipse. It is conservative by one
// column; corner.addOutside adds 0 for the extra pixels.
func (c *corner) columns(py float64) (x0, x1 int) {
	if !c.on {
		return 0, 0
	}
	ya, yb := max(py, c.by0), min(py+1, c.by1)
	if !(ya < yb) {
		return 0, 0
	}
	// The largest v of the row; the arc is at u = sqrt(1-v²) there, and
	// pixels with all u <= uIn are inside the ellipse.
	var vh float64
	if c.sy < 0 {
		vh = (c.cy - ya) / c.ry
	} else {
		vh = (yb - c.cy) / c.ry
	}
	vh = clamp01(vh)
	uIn := math.Sqrt(float64((1 - vh) * (1 + vh)))
	bx0, bx1 := int(math.Floor(c.bx0)), int(math.Ceil(c.bx1))
	if c.sx < 0 { // u grows to the left: inside is to the right of xt
		xt := c.cx - float64(uIn*c.rx)
		return bx0, min(bx1, int(math.Ceil(xt))+1)
	}
	xt := c.cx + float64(uIn*c.rx)
	return max(bx0, int(math.Floor(xt))-1), bx1
}

// unitTo8 converts a coverage in [0, 1] (clamped) to 8 bits, rounding to
// nearest. For a = k/65536 it equals cov8(k).
func unitTo8(a float64) uint8 {
	v := math.Floor(float64(a*255) + 0.5)
	switch {
	case !(v > 0):
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v)
}

// addOutside adds to out[i], for the pixels (x0+i, py) with x0+i < x1, the
// area of the pixel that lies in the corner box but outside the corner
// ellipse, in square pixels.
//
// The pixel maps to the box [ul, uh] x [vl, vh] of the unit quarter disk
// (u, v in [0, 1], the arc is u²+v² = 1, the box corner is (1, 1)), and the
// area inside the disk is G(uh,vh) - G(ul,vh) - G(uh,vl) + G(ul,vl) with
// G = diskArea. Neighboring pixels share a column boundary, so G is
// evaluated once per boundary and row.
func (c *corner) addOutside(x0, x1 int, py float64, out []float64) {
	ya, yb := max(py, c.by0), min(py+1, c.by1)
	if !(ya < yb) {
		return
	}
	var vl, vh float64
	if c.sy < 0 {
		vl, vh = (c.cy-yb)/c.ry, (c.cy-ya)/c.ry
	} else {
		vl, vh = (ya-c.cy)/c.ry, (yb-c.cy)/c.ry
	}
	vl, vh = clamp01(vl), clamp01(vh)
	if !(vl < vh) {
		return
	}
	vl2, vh2 := float64(vl*vl), float64(vh*vh)
	// u of the column boundary X, clamped to the box.
	uOf := func(X float64) float64 {
		X = min(max(X, c.bx0), c.bx1)
		if c.sx < 0 {
			return clamp01((c.cx - X) / c.rx)
		}
		return clamp01((X - c.cx) / c.rx)
	}
	// boundary holds u at a column boundary and, once computed, G(u, vl)
	// and G(u, vh).
	type boundary struct {
		u, gl, gh float64
		ok        bool
	}
	eval := func(b *boundary) {
		if !b.ok {
			b.gl, b.gh, b.ok = diskArea(b.u, vl), diskArea(b.u, vh), true
		}
	}
	a := boundary{u: uOf(float64(x0))}
	for x := x0; x < x1; x++ {
		bnd := boundary{u: uOf(float64(x + 1))}
		lo, hi := &a, &bnd // lo has the smaller u
		if lo.u > hi.u {
			lo, hi = hi, lo
		}
		ul, uh := lo.u, hi.u
		switch {
		case !(ul < uh), float64(uh*uh)+vh2 <= 1: // empty, or entirely inside
		case float64(ul*ul)+vl2 >= 1: // entirely outside
			out[x-x0] += float64(float64((uh-ul)*(vh-vl)) * c.area)
		default:
			eval(lo)
			eval(hi)
			box := float64((uh - ul) * (vh - vl))
			in := hi.gh - lo.gh - hi.gl + lo.gl
			out[x-x0] += float64((box - in) * c.area)
		}
		a = bnd
	}
}

// clamp01 clamps v to [0, 1]. NaN cannot occur.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// diskArea returns the area of [0, u] x [0, v] inside the unit disk, for u, v
// in [0, 1]:
//
//	G(u, v) = ∫_0^u min(v, sqrt(1-s²)) ds
//	        = u*v                                   if u²+v² <= 1
//	        = (su*v + u*sqrt(1-u²) + asin u + asin v - π/2) / 2   otherwise,
//
// where su = sqrt(1-v²) is where the arc crosses height v (the second form
// is su*v + F(u) - F(su) with F(s) = (s*sqrt(1-s²) + asin s)/2 and
// asin su = π/2 - asin v).
func diskArea(u, v float64) float64 {
	if !(u > 0 && v > 0) {
		return 0
	}
	if float64(u*u)+float64(v*v) <= 1 {
		return u * v
	}
	su := math.Sqrt(float64((1 - v) * (1 + v)))
	cu := math.Sqrt(float64((1 - u) * (1 + u)))
	return (float64(su*v) + float64(u*cu) + asin01(u) + asin01(v) - math.Pi/2) / 2
}
