package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Box shadows (DrawShadow), CSS Backgrounds 3 section 7.
//
// # Shape
//
// The shadow shape is the box translated by the offset and grown by the
// spread (outer) or shrunk by it (inset). Corner radii follow the spec's
// spread rule (shadowShape): each radius component grows by the spread, but
// when it is smaller than the spread, the spread is first multiplied by
// 1 + (r-1)³ with r = radius/spread, so small radii stay sharp; shrinking
// subtracts and floors at zero. For an inset shadow the shape grows by a
// negative spread and shrinks by a positive one.
//
// An outer shadow is the shape blurred, painted only outside the box: its
// coverage is multiplied by 1 - (box coverage). An inset shadow is the
// complement of the shape blurred (1 - the blurred shape), painted only
// inside the box (times the box coverage). The box coverage is the
// rasterizer's exact area coverage, so the clip edge matches a FillRRect of
// the same box.
//
// # Analytic blur (Evan Wallace's closed form, extended)
//
// The value of a pixel is the shape convolved with the Gaussian of standard
// deviation Blur/2 and averaged over the pixel's square (the same box filter
// as the rasterizer's coverage, so that Blur -> 0 converges to the anti-
// aliased fill). Both filters are separable, so for a rectangle the value is
// a product of two 1D integrals in closed form: averaging Phi((x - a)/s) over
// a pixel gives s*(I(t1) - I(t0)) with I(t) = t*Phi(t) + phi(t), Phi the
// standard normal CDF (via erf) and phi its density.
//
// A rounded rectangle is its rectangle minus, at each corner, the region D
// between the corner box and the ellipse. The blurred deficit of D is
// subtracted from the rectangle's closed form. D is split at the point of
// the arc where its slope is 1 (in coordinates scaled by the standard
// deviations): a rectangle at the box corner (closed form), a piece along
// the box's horizontal edge whose height is a function of x with slope at
// most 1, and a piece along the vertical edge whose width is a function of y
// with slope at most 1. Each curved piece is integrated in closed form
// across (erf) and numerically along its axis, with Gauss-Legendre panels at
// most two standard deviations long (2 to 4 points, fewer for short panels,
// whose integrand barely varies). Wallace's shader samples the whole
// shape along one axis (4 midpoint samples); restricting the numerical part
// to the corner pieces, whose boundary is never steep, and integrating each
// pixel's square exactly makes the result accurate to a fraction of an 8-bit
// level (TestShadowExact compares it with a brute-force reference).
// Elliptical corners are exact too: the split point is taken on the ellipse.
// Corners are handled per quadrant with their own radii (as GPUI does), and
// their deficits add up exactly because the regions D of a normalized
// rounded rectangle never overlap.
//
// # Transforms
//
//   - Scale/translate (flips and non-uniform scales included): the shape is
//     mapped to device space and blurred there with the per-axis standard
//     deviations. The 1D factors are computed once per column and row, so
//     the cost is one multiplication per pixel plus a few terms near the
//     corners.
//   - Rotation with a uniform scale: a Gaussian is rotation invariant, so the
//     value is evaluated in local space at each pixel's center, with the
//     pixel's square approximated by a local square of the same area (both
//     have the same second moments).
//   - Anything else (skew, non-uniform scale with rotation), and pathological
//     geometry that would need too many quadrature nodes: the shape is
//     rasterized into a mask and blurred with the separable Gaussian of the
//     layer filters, using the marginal device standard deviations.
//
// A device standard deviation below sigmaMin is a hard shadow: the shape's
// coverage from the rasterizer, with the same clip rules.

// shadowK is the cutoff of the Gaussian, in standard deviations: beyond it
// the tails (below 3.2e-5, or 0.008 levels of 255) are dropped exactly,
// which also keeps every value a function of the geometry and the pixel
// only.
const shadowK = 4

// nodeBudget bounds the quadrature nodes of one corner piece; geometry
// needing more (huge radii with tiny blurs) uses the mask path.
const nodeBudget = 1 << 15

// panelLen is the longest quadrature panel in standard deviations. With
// 4-point Gauss-Legendre, 1, 2 and 3 give a largest error of 0.02, 0.02 and
// 0.1 levels of 255 in TestShadowExact (the first two are the reference's
// own error). Panels up to 1 standard deviation use 3 points and up to 0.5
// use 2, with the same measured error.
const panelLen = 2

// Gauss-Legendre nodes and weights on [-1, 1], with 2, 3 and 4 points.
var (
	glX = [3][]float64{
		{-0.5773502691896257, 0.5773502691896257},
		{-0.7745966692414834, 0, 0.7745966692414834},
		{-0.8611363115940526, -0.3399810435848563, 0.3399810435848563, 0.8611363115940526},
	}
	glW = [3][]float64{
		{1, 1},
		{0.5555555555555556, 0.8888888888888888, 0.5555555555555556},
		{0.3478548451374538, 0.6521451548625461, 0.6521451548625461, 0.3478548451374538},
	}
)

// invSqrt2Pi is 1/sqrt(2*pi).
const invSqrt2Pi = 0.3989422804014327

// stepAvg returns the average over [p, p+w] of Phi((x-a)/s): a blurred step
// at a, box-filtered. It is exactly 0 or 1 beyond shadowK standard
// deviations.
func stepAvg(p, w, a, s float64) float64 {
	ks := float64(shadowK * s)
	switch {
	case p+w <= a-ks:
		return 0
	case p >= a+ks:
		return 1
	}
	inv := 1 / s
	return float64(s*(gaussI(float64((p+w-a)*inv))-gaussI(float64((p-a)*inv)))) / w
}

// densAvg returns the average over [p, p+w] of the Gaussian density of
// standard deviation s centered at v. It is exactly 0 beyond shadowK
// standard deviations.
func densAvg(p, w, v, s float64) float64 {
	ks := float64(shadowK * s)
	if p+w <= v-ks || p >= v+ks {
		return 0
	}
	inv := 1 / s
	return (normCDF(float64((p+w-v)*inv)) - normCDF(float64((p-v)*inv))) / w
}

// spreadRadius applies a spread e to a radius component r (CSS Backgrounds
// 3, section 7.1.1): r + e when growing, but with e scaled by 1 + (r/e - 1)³
// when r < e; r + e floored at 0 when shrinking.
func spreadRadius(r, e float32) float32 {
	rf, ef := float64(r), float64(e)
	if ef < 0 {
		return float32(max(rf+ef, 0))
	}
	if rf < ef {
		c := rf/ef - 1
		ef = float64(ef * (1 + float64(float64(c*c)*c)))
	}
	return float32(rf + ef)
}

// shadowShape returns the shadow shape of s in local space: the box
// translated by Offset and grown by Spread (outer) or shrunk by it (inset),
// with the CSS spread rule for the radii. ok is false when it is empty.
func shadowShape(s gfx.DrawShadow) (shape gfx.RRect, ok bool) {
	e := s.Spread
	if s.Inset {
		e = -e
	}
	rect := s.RRect.Rect.Translate(s.Offset.X, s.Offset.Y).Outset(e)
	if rect.Empty() || !rect.IsFinite() {
		return gfx.RRect{}, false
	}
	shape.Rect = rect
	for i, rad := range s.RRect.Radii {
		shape.Radii[i] = gfx.Pt(spreadRadius(rad.X, e), spreadRadius(rad.Y, e))
	}
	return shape.Normalize(), true
}

// shadowEval produces the blurred shape's value for the pixels of one row
// of the zone.
type shadowEval interface {
	// row sets v[i], for the pixels (zone.Min.X+i, y) with want[i] != 0, to
	// the blurred shape's value in [0, 1] (unclamped).
	row(y int, v []float64, want []uint8)
}

// drawShadow paints a DrawShadow command.
func (r *Renderer) drawShadow(c gfx.DrawShadow) {
	shape, ok := shadowShape(c)
	if !ok && !c.Inset {
		return
	}
	b := &r.blit
	m := &r.ctm
	sigma := float64(c.Blur) / 2

	// The pixels the shadow may paint, as in the Builder's bounds: the shape
	// grown by 3 sigma (outer) or the box (inset). Pixels that merely touch
	// the bounds are included; Visit grows regions by a pixel, so they are
	// never culled.
	var zone image.Rectangle
	if c.Inset {
		zone = touchBounds(m, rect64(c.RRect.Rect, 0))
	} else {
		zone = touchBounds(m, rect64(shape.Rect, float64(3*sigma)))
	}
	zone = zone.Intersect(b.clip)
	if zone.Empty() {
		return
	}
	sc := &r.shd
	sc.reset()

	// Box coverage over the zone.
	boxRect := r.rectBounds(c.RRect.Rect).Intersect(zone)
	var box []uint8
	if !boxRect.Empty() {
		box = r.maskRRect(boxRect, c.RRect)
	}
	if c.Inset && box == nil {
		return
	}

	ev, release := r.shadowEvaluator(c, shape, ok, sigma, zone)
	if release != nil {
		defer r.putBuf(release)
	}
	defer func() {
		if box != nil {
			r.putBuf(box)
		}
	}()

	b.setShader(r.solidFor(c.Color))
	n := zone.Dx()
	v := sc.vrow(n)
	want := sc.wantRow(n)
	cov := sc.covRow(n)
	for y := zone.Min.Y; y < zone.Max.Y; y++ {
		// The box coverage of this row of the zone, and the pixels to
		// evaluate: outside the box (outer) or inside it (inset).
		clear(cov)
		if y >= boxRect.Min.Y && y < boxRect.Max.Y {
			off := (y - boxRect.Min.Y) * boxRect.Dx()
			copy(cov[boxRect.Min.X-zone.Min.X:], box[off:off+boxRect.Dx()])
		}
		any := false
		if c.Inset {
			for i, bc := range cov {
				want[i] = bc
				any = any || bc != 0
			}
		} else {
			for i, bc := range cov {
				want[i] = ^bc
				any = any || bc != 255
			}
		}
		if !any {
			continue
		}
		ev.row(y, v, want)
		lo, hi := n, 0
		for i := range cov {
			if want[i] == 0 {
				cov[i] = 0
				continue
			}
			s := min(max(v[i], 0), 1)
			var a float64
			if c.Inset {
				a = float64((1 - s) * float64(cov[i]))
			} else {
				a = float64(s * float64(255-cov[i]))
			}
			cov[i] = uint8(math.Floor(a + 0.5)) // a is in [0, 255]
			if cov[i] != 0 {
				lo, hi = min(lo, i), i+1
			}
		}
		if lo < hi {
			b.mask(y, zone.Min.X+lo, cov[lo:hi])
		}
	}
}

// rect64 returns r grown by e, in float64.
func rect64(r gfx.Rect, e float64) [4]float64 {
	return [4]float64{float64(r.X0) - e, float64(r.Y0) - e, float64(r.X1) + e, float64(r.Y1) + e}
}

// applyF maps a local point given in float64 to device space, with every
// product rounded explicitly.
func applyF(m *gfx.Affine, x, y float64) (float64, float64) {
	return float64(float64(m[0])*x) + float64(float64(m[2])*y) + float64(m[4]),
		float64(float64(m[1])*x) + float64(float64(m[3])*y) + float64(m[5])
}

// touchBounds returns the device pixels whose square touches the transformed
// local rectangle r (x0, y0, x1, y1), clamped to ±coordLimit.
func touchBounds(m *gfx.Affine, r [4]float64) image.Rectangle {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4][2]float64{{r[0], r[1]}, {r[2], r[1]}, {r[2], r[3]}, {r[0], r[3]}} {
		x, y := applyF(m, p[0], p[1])
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if !(x0 <= x1 && y0 <= y1) {
		return image.Rectangle{}
	}
	return image.Rect(
		int(math.Floor(clampLim(x0))), int(math.Floor(clampLim(y0))),
		int(math.Ceil(clampLim(x1))), int(math.Ceil(clampLim(y1))),
	)
}

// maskRRect rasterizes the coverage of a local rounded rectangle under the
// current transform into a new buffer covering rect (released with putBuf),
// ignoring the clip mask.
func (r *Renderer) maskRRect(rect image.Rectangle, rr gfx.RRect) []uint8 {
	b := &r.blit
	buf := r.getBuf(rect.Dx() * rect.Dy())
	clip, cm := b.clip, b.cm
	b.clip, b.cm = rect, clipMask{}
	b.toMask, b.a8, b.a8Stride, b.a8Origin = true, buf, rect.Dx(), rect.Min
	r.coverRRect(rr)
	b.toMask, b.a8 = false, nil
	b.clip, b.cm = clip, cm
	return buf
}

// similarity reports whether the linear part of m is a rotation (possibly
// with a flip) times a uniform scale, and returns the scale.
func similarity(m *gfx.Affine) (float64, bool) {
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	tol := 1e-6 * max(math.Abs(a), math.Abs(b), math.Abs(c), math.Abs(d))
	if (math.Abs(a-d) <= tol && math.Abs(b+c) <= tol) || (math.Abs(a+d) <= tol && math.Abs(b-c) <= tol) {
		return math.Sqrt(math.Abs(a*d - b*c)), true // exact products of float32 values
	}
	return 0, false
}

// shadowEvaluator picks the evaluation method for a shadow (see the file
// comment). release, if not nil, is a buffer to free afterwards.
func (r *Renderer) shadowEvaluator(c gfx.DrawShadow, shape gfx.RRect, ok bool, sigma float64, zone image.Rectangle) (shadowEval, []uint8) {
	sc := &r.shd
	if !ok { // inset with an empty inner shape: the whole box is shadowed
		return zeroEval{}, nil
	}
	m := &r.ctm
	sx, sy := deviceSigmas(m, sigma)
	if max(sx, sy) < sigmaMin {
		return r.maskEval(shape, zone, nil, nil)
	}
	if m.IsScaleTranslate() && min(sx, sy) >= sigmaMin {
		g := &sc.g
		if g.setDevice(m, shape, sx, sy) && g.build(sc, zoneF(zone)) {
			return sc.separable(g, zone), nil
		}
	} else if s, sim := similarity(m); sim && sigma*s >= sigmaMin {
		g := &sc.g
		inv, okInv := invert(*m)
		if okInv && g.setLocal(shape, sigma, 1/s) {
			// The zone's corners in local space bound the nodes needed.
			w := localBox(&inv, zone)
			if g.build(sc, w) {
				sc.local = localEval{g: g, sc: sc, inv: inv, zone: zone, hw: 0.5 / s}
				return &sc.local, nil
			}
		}
	}
	// Mask and blur.
	kx, ky := r.shadowKernels(sx, sy)
	return r.maskEval(shape, zone, &kx, &ky)
}

// shadowKernels returns blur kernels for the mask path, with weights in a
// scratch arena of the shadow (the filter arena is in use by open layers).
func (r *Renderer) shadowKernels(sx, sy float64) (gaussKernel, gaussKernel) {
	sc := &r.shd
	sc.kw = sc.kw[:0]
	var kx, ky gaussKernel
	kx, sc.kw = newKernel(sx, 0, sc.kw)
	ky, sc.kw = newKernel(sy, 0, sc.kw)
	return kx, ky
}

// zoneF returns the pixels of zone as a float rectangle (x0, y0, x1, y1).
func zoneF(z image.Rectangle) [4]float64 {
	return [4]float64{float64(z.Min.X), float64(z.Min.Y), float64(z.Max.X), float64(z.Max.Y)}
}

// localBox returns the local-space bounding box of the device rectangle z
// under the inverse transform inv.
func localBox(inv *inverse, z image.Rectangle) [4]float64 {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4][2]float64{
		{float64(z.Min.X), float64(z.Min.Y)}, {float64(z.Max.X), float64(z.Min.Y)},
		{float64(z.Max.X), float64(z.Max.Y)}, {float64(z.Min.X), float64(z.Max.Y)},
	} {
		x, y := inv.apply(p[0], p[1])
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return [4]float64{x0, y0, x1, y1}
}

// apply maps a device point to local space, with the products rounded
// explicitly.
func (inv *inverse) apply(X, Y float64) (float64, float64) {
	return float64(inv.a*X) + float64(inv.c*Y) + inv.e, float64(inv.b*X) + float64(inv.d*Y) + inv.f
}

// zeroEval is the blurred value of an empty shape.
type zeroEval struct{}

func (zeroEval) row(y int, v []float64, want []uint8) { clear(v) }

// maskShadow evaluates a shadow from its rasterized shape, blurred with the
// separable Gaussian kernels (none for a hard shadow).
type maskShadow struct {
	zone, rect image.Rectangle
	buf        []uint8
}

// maskEval rasterizes the shape and blurs it. The buffer covers every
// pixel within the kernels' support of the zone where any pass of the blur
// can be nonzero, so that the zone's values do not depend on the zone.
func (r *Renderer) maskEval(shape gfx.RRect, zone image.Rectangle, kx, ky *gaussKernel) (shadowEval, []uint8) {
	sb := r.rectBounds(shape.Rect)
	q := zone.Intersect(sb)
	if kx != nil {
		dx, dy := kx.support(), ky.support()
		q = growXY(zone, dx, dy).Intersect(growXY(sb, dx, dy))
	}
	if q.Empty() {
		return zeroEval{}, nil
	}
	buf := r.maskRRect(q, shape)
	if kx != nil {
		r.blurS.blurPlane(buf, q.Dx(), q.Dx(), q.Dy(), 1, q.Min.X, q.Min.Y, kx, ky)
	}
	r.shd.mask = maskShadow{zone: zone, rect: q, buf: buf}
	return &r.shd.mask, buf
}

func (e *maskShadow) row(y int, v []float64, want []uint8) {
	clear(v)
	q := e.rect
	if y < q.Min.Y || y >= q.Max.Y {
		return
	}
	row := e.buf[(y-q.Min.Y)*q.Dx() : (y-q.Min.Y+1)*q.Dx()]
	for x := max(q.Min.X, e.zone.Min.X); x < min(q.Max.X, e.zone.Max.X); x++ {
		v[x-e.zone.Min.X] = float64(row[x-q.Min.X]) / 255
	}
}

// ---------------------------------------------------------------------------
// Analytic geometry
// ---------------------------------------------------------------------------

// rrBlur is a rounded rectangle to be blurred, in a frame where the Gaussian
// is axis-aligned with standard deviations sx, sy and a pixel is a box of
// wx by wy (device space for scale/translate transforms, local space for
// similarities).
type rrBlur struct {
	x0, y0, x1, y1 float64
	sx, sy         float64
	wx, wy         float64
	rad            [4][2]float64 // TL, TR, BR, BL
	c              [4]rrCorner
}

// rrCorner is one corner's deficit region D, split into a rectangle and two
// curved pieces.
type rrCorner struct {
	on                 bool
	cx, cy, rx, ry     float64
	gx, gy             float64 // ±1: from the center toward the box corner
	bx, by             float64 // the box corner
	px, py             float64 // the split point on the arc
	zx0, zy0, zx1, zy1 float64 // where the deficit can be nonzero
	// Quadrature nodes: top along x (the piece at the horizontal edge),
	// side along y; indices into shadowScratch.nodes.
	top, side [2]int
}

// rrNode is a quadrature node: its position along the piece's axis, its
// weight, and the arc's coordinate on the other axis.
type rrNode struct{ p, w, arc float64 }

// setDevice sets g to the device-space image of shape under the
// scale/translate transform m, blurred with sx, sy. It returns false if the
// shape is out of the range the analytic path handles.
func (g *rrBlur) setDevice(m *gfx.Affine, shape gfx.RRect, sx, sy float64) bool {
	x0, y0 := apply(m, gfx.Pt(shape.Rect.X0, shape.Rect.Y0))
	x1, y1 := apply(m, gfx.Pt(shape.Rect.X1, shape.Rect.Y1))
	ax, ay := math.Abs(float64(m[0])), math.Abs(float64(m[3]))
	var rad [4][2]float64
	for i, p := range shape.Radii {
		rad[i] = [2]float64{float64(p.X) * ax, float64(p.Y) * ay} // exact
	}
	tl, tr, br, bl := gfx.TopLeft, gfx.TopRight, gfx.BottomRight, gfx.BottomLeft
	if x0 > x1 {
		x0, x1 = x1, x0
		rad[tl], rad[tr], rad[br], rad[bl] = rad[tr], rad[tl], rad[bl], rad[br]
	}
	if y0 > y1 {
		y0, y1 = y1, y0
		rad[tl], rad[tr], rad[br], rad[bl] = rad[bl], rad[br], rad[tr], rad[tl]
	}
	*g = rrBlur{x0: x0, y0: y0, x1: x1, y1: y1, sx: sx, sy: sy, wx: 1, wy: 1, rad: rad}
	return g.limits()
}

// setLocal sets g to shape in local space, blurred with s, with pixels
// approximated by local squares of side w.
func (g *rrBlur) setLocal(shape gfx.RRect, s, w float64) bool {
	var rad [4][2]float64
	for i, p := range shape.Radii {
		rad[i] = [2]float64{float64(p.X), float64(p.Y)}
	}
	*g = rrBlur{
		x0: float64(shape.Rect.X0), y0: float64(shape.Rect.Y0), x1: float64(shape.Rect.X1), y1: float64(shape.Rect.Y1),
		sx: s, sy: s, wx: w, wy: w, rad: rad,
	}
	return g.limits()
}

// limits reports whether the frame values are small enough for the
// analytic path (huge geometry goes through the mask path, which clips it).
func (g *rrBlur) limits() bool {
	const lim = 1 << 40
	for _, v := range [...]float64{g.x0, g.y0, g.x1, g.y1, g.sx, g.sy} {
		if !(math.Abs(v) < lim) {
			return false
		}
	}
	return g.x0 < g.x1 && g.y0 < g.y1
}

// build sets up the corners and their quadrature nodes for the frame
// rectangle win (x0, y0, x1, y1) that bounds the pixels to evaluate. It
// returns false if a corner would need more than nodeBudget nodes.
func (g *rrBlur) build(sc *shadowScratch, win [4]float64) bool {
	sc.nodes = sc.nodes[:0]
	type spec struct{ cx, cy, gx, gy float64 }
	r := &g.rad
	specs := [4]spec{
		{g.x0 + r[0][0], g.y0 + r[0][1], -1, -1},
		{g.x1 - r[1][0], g.y0 + r[1][1], 1, -1},
		{g.x1 - r[2][0], g.y1 - r[2][1], 1, 1},
		{g.x0 + r[3][0], g.y1 - r[3][1], -1, 1},
	}
	kx, ky := float64(shadowK*g.sx)+g.wx, float64(shadowK*g.sy)+g.wy
	for i, s := range specs {
		c := &g.c[i]
		rx, ry := r[i][0], r[i][1]
		*c = rrCorner{}
		if !(rx > 0 && ry > 0) {
			continue
		}
		c.on = true
		c.cx, c.cy, c.rx, c.ry, c.gx, c.gy = s.cx, s.cy, rx, ry, s.gx, s.gy
		c.bx, c.by = s.cx+float64(s.gx*rx), s.cy+float64(s.gy*ry)
		// The split point: slope 1 in coordinates scaled by the standard
		// deviations, at angle t with tan t = (ry/sy)/(rx/sx).
		nx, ny := rx/g.sx, ry/g.sy
		hyp := math.Sqrt(float64(nx*nx) + float64(ny*ny))
		cosT, sinT := nx/hyp, ny/hyp
		c.px, c.py = s.cx+float64(float64(s.gx*rx)*cosT), s.cy+float64(float64(s.gy*ry)*sinT)
		c.zx0, c.zx1 = min(c.bx, c.cx)-kx, max(c.bx, c.cx)+kx
		c.zy0, c.zy1 = min(c.by, c.cy)-ky, max(c.by, c.cy)+ky
		// Top piece: nodes along x between the split point and the center.
		var ok bool
		c.top[0] = len(sc.nodes)
		sc.nodes, ok = addNodes(sc.nodes, min(c.px, c.cx), max(c.px, c.cx), g.sx, win[0]-kx, win[2]+kx, func(u float64) float64 {
			q := clamp01(math.Abs(u-c.cx) / c.rx)
			return c.cy + float64(float64(c.gy*c.ry)*math.Sqrt(float64((1-q)*(1+q))))
		})
		if !ok {
			return false
		}
		c.top[1] = len(sc.nodes)
		// Side piece: nodes along y between the split point and the center.
		c.side[0] = len(sc.nodes)
		sc.nodes, ok = addNodes(sc.nodes, min(c.py, c.cy), max(c.py, c.cy), g.sy, win[1]-ky, win[3]+ky, func(v float64) float64 {
			q := clamp01(math.Abs(v-c.cy) / c.ry)
			return c.cx + float64(float64(c.gx*c.rx)*math.Sqrt(float64((1-q)*(1+q))))
		})
		if !ok {
			return false
		}
		c.side[1] = len(sc.nodes)
	}
	return true
}

// addNodes appends the Gauss-Legendre nodes of [lo, hi], split into equal
// panels at most panelLen standard deviations s long, keeping only the panels
// that meet [wlo, whi]. arc gives the curved boundary at a node. ok is false
// beyond nodeBudget.
func addNodes(nodes []rrNode, lo, hi, s, wlo, whi float64, arc func(float64) float64) ([]rrNode, bool) {
	l := hi - lo
	if !(l > 0) {
		return nodes, true
	}
	np := math.Ceil(l / (panelLen * s))
	if np*4 > nodeBudget {
		return nodes, false
	}
	n := max(int(np), 1)
	ph := l / float64(n)
	k0 := max(0, int(math.Floor((wlo-lo)/ph)))
	k1 := min(n, int(math.Ceil((whi-lo)/ph)))
	half := ph / 2
	// Panels much shorter than the standard deviation need fewer points:
	// the integrand barely varies across them.
	order := 2
	switch {
	case ph <= 0.5*s:
		order = 0
	case ph <= s:
		order = 1
	}
	gx, gw := glX[order], glW[order]
	for k := k0; k < k1; k++ {
		mid := lo + float64(float64(k)*ph) + half
		for i := range gx {
			p := mid + float64(half*gx[i])
			nodes = append(nodes, rrNode{p: p, w: float64(half * gw[i]), arc: arc(p)})
		}
	}
	return nodes, true
}

// interval returns the box-filtered blurred indicator of the interval
// between a and b (in either order) for the pixel box [p, p+w].
func interval(p, w, a, b, s float64) float64 {
	if a > b {
		a, b = b, a
	}
	return stepAvg(p, w, a, s) - stepAvg(p, w, b, s)
}

// at returns the blurred value for the pixel box [p, p+wx] x [q, q+wy].
func (g *rrBlur) at(sc *shadowScratch, p, q float64) float64 {
	fx := stepAvg(p, g.wx, g.x0, g.sx) - stepAvg(p, g.wx, g.x1, g.sx)
	if fx == 0 {
		return 0
	}
	fy := stepAvg(q, g.wy, g.y0, g.sy) - stepAvg(q, g.wy, g.y1, g.sy)
	v := float64(fx * fy)
	for i := range g.c {
		c := &g.c[i]
		if !c.on || p+g.wx <= c.zx0 || p >= c.zx1 || q+g.wy <= c.zy0 || q >= c.zy1 {
			continue
		}
		d := float64(interval(p, g.wx, c.bx, c.px, g.sx) * interval(q, g.wy, c.by, c.py, g.sy))
		for _, n := range sc.nodes[c.top[0]:c.top[1]] {
			if dn := densAvg(p, g.wx, n.p, g.sx); dn != 0 {
				d += float64(float64(n.w*dn) * interval(q, g.wy, c.by, n.arc, g.sy))
			}
		}
		for _, n := range sc.nodes[c.side[0]:c.side[1]] {
			if dn := densAvg(q, g.wy, n.p, g.sy); dn != 0 {
				d += float64(float64(n.w*dn) * interval(p, g.wx, c.bx, n.arc, g.sx))
			}
		}
		v -= d
	}
	return v
}

// localEval evaluates a shadow under a similarity transform at each pixel's
// center, mapped to local space.
type localEval struct {
	g    *rrBlur
	sc   *shadowScratch
	inv  inverse
	zone image.Rectangle
	hw   float64 // half the side of a pixel in local units
}

func (e *localEval) row(y int, v []float64, want []uint8) {
	Y := float64(y) + 0.5
	for i := range v {
		if want[i] == 0 {
			continue
		}
		lx, ly := e.inv.apply(float64(e.zone.Min.X+i)+0.5, Y)
		v[i] = e.g.at(e.sc, lx-e.hw, ly-e.hw)
	}
}

// ---------------------------------------------------------------------------
// Separable evaluation (scale/translate transforms)
// ---------------------------------------------------------------------------

// stepTab holds stepAvg for whole pixels p in [lo, lo+len(v)); pixels below
// are 0 and pixels from lo+len(v) on are 1 (within the zone it was built
// for).
type stepTab struct {
	lo int
	v  []float64
}

func (t *stepTab) at(p int) float64 {
	i := p - t.lo
	switch {
	case i < 0:
		return 0
	case i >= len(t.v):
		return 1
	}
	return t.v[i]
}

// densTab holds w*densAvg for whole pixels p in [lo, lo+len(v)); 0
// elsewhere.
type densTab struct {
	lo int
	v  []float64
}

func (t *densTab) at(p int) float64 {
	i := p - t.lo
	if i < 0 || i >= len(t.v) {
		return 0
	}
	return t.v[i]
}

// fint converts a pixel coordinate to int, clamped far beyond any zone.
func fint(v float64) int {
	return int(min(max(v, -1<<40), 1<<40))
}

// makeStep returns the step table of edge a (standard deviation s) for the
// pixels [z0, z1). Consecutive pixels share their boundary's I value.
func (sc *shadowScratch) makeStep(a, s float64, z0, z1 int) stepTab {
	ks := float64(shadowK * s)
	tlo := fint(math.Floor(a-ks-1)) + 1 // first p with p+1 > a-ks
	thi := fint(math.Ceil(a + ks))      // first p with p >= a+ks
	lo := max(tlo, z0)
	hi := max(min(thi, z1), lo)
	t := stepTab{lo: lo, v: sc.floats(hi - lo)}
	if hi > lo {
		inv := 1 / s
		prev := gaussI(float64((float64(lo) - a) * inv))
		for p := lo; p < hi; p++ {
			next := gaussI(float64((float64(p+1) - a) * inv))
			t.v[p-lo] = float64(s * (next - prev))
			prev = next
		}
	}
	return t
}

// makeDens returns w times the density table of a node at v (standard
// deviation s) for the pixels [z0, z1).
func (sc *shadowScratch) makeDens(v, w, s float64, z0, z1 int) densTab {
	ks := float64(shadowK * s)
	lo := max(fint(math.Floor(v-ks-1))+1, z0)
	hi := max(min(fint(math.Ceil(v+ks)), z1), lo)
	t := densTab{lo: lo, v: sc.floats(hi - lo)}
	if hi > lo {
		inv := 1 / s
		prev := normCDF(float64((float64(lo) - v) * inv))
		for p := lo; p < hi; p++ {
			next := normCDF(float64((float64(p+1) - v) * inv))
			t.v[p-lo] = float64(w * (next - prev))
			prev = next
		}
	}
	return t
}

// sepNode is a quadrature node with its tables.
type sepNode struct {
	dens densTab // along the node's axis, times the node weight
	arc  stepTab // the arc's step along the other axis
}

// sepCorner is a corner's deficit, tabulated over the zone.
type sepCorner struct {
	x0, y0, x1, y1 int // pixels where the deficit can be nonzero
	rectX, rectY   []float64
	boxX, boxY     stepTab   // steps of the box corner's edges
	top, side      []sepNode // top: nodes along x; side: along y
	topLo, topHi   []int32   // per column: the top nodes whose density reaches it
	sideLo, sideHi []int32   // per row: the same for the side nodes
}

// sepEval evaluates a shadow under a scale/translate transform.
type sepEval struct {
	zone         image.Rectangle
	fx, fy       []float64
	corners      []sepCorner
	sideD, topIV []float64
}

// separable tabulates g over zone.
func (sc *shadowScratch) separable(g *rrBlur, zone image.Rectangle) *sepEval {
	e := &sc.sep
	*e = sepEval{zone: zone, corners: e.corners[:0], sideD: e.sideD[:0], topIV: e.topIV[:0]}
	zx0, zy0, zx1, zy1 := zone.Min.X, zone.Min.Y, zone.Max.X, zone.Max.Y
	e.fx = sc.floats(zone.Dx())
	e.fy = sc.floats(zone.Dy())
	{
		a, b := sc.makeStep(g.x0, g.sx, zx0, zx1), sc.makeStep(g.x1, g.sx, zx0, zx1)
		for i := range e.fx {
			e.fx[i] = a.at(zx0+i) - b.at(zx0+i)
		}
		a, b = sc.makeStep(g.y0, g.sy, zy0, zy1), sc.makeStep(g.y1, g.sy, zy0, zy1)
		for i := range e.fy {
			e.fy[i] = a.at(zy0+i) - b.at(zy0+i)
		}
	}
	for i := range g.c {
		c := &g.c[i]
		if !c.on {
			continue
		}
		x0, x1 := max(fint(math.Floor(c.zx0)), zx0), min(fint(math.Ceil(c.zx1)), zx1)
		y0, y1 := max(fint(math.Floor(c.zy0)), zy0), min(fint(math.Ceil(c.zy1)), zy1)
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		var k sepCorner
		k.x0, k.y0, k.x1, k.y1 = x0, y0, x1, y1
		k.rectX = sc.intervalTab(c.bx, c.px, g.sx, x0, x1)
		k.rectY = sc.intervalTab(c.by, c.py, g.sy, y0, y1)
		k.boxX = sc.makeStep(c.bx, g.sx, x0, x1)
		k.boxY = sc.makeStep(c.by, g.sy, y0, y1)
		// The interval of a node is [box edge, arc] for the top-left
		// corner and [arc, box edge] when the corner faces the other way:
		// the evaluation computes step(box) - step(arc) and the sign is
		// folded into the weights.
		topW, sideW := 1.0, 1.0
		if c.gy > 0 {
			topW = -1
		}
		if c.gx > 0 {
			sideW = -1
		}
		k.top = sc.sepNodes(len(sc.nodes[c.top[0]:c.top[1]]))
		for j, n := range sc.nodes[c.top[0]:c.top[1]] {
			k.top[j] = sepNode{dens: sc.makeDens(n.p, topW*n.w, g.sx, x0, x1), arc: sc.makeStep(n.arc, g.sy, y0, y1)}
		}
		k.side = sc.sepNodes(len(sc.nodes[c.side[0]:c.side[1]]))
		for j, n := range sc.nodes[c.side[0]:c.side[1]] {
			k.side[j] = sepNode{dens: sc.makeDens(n.p, sideW*n.w, g.sy, y0, y1), arc: sc.makeStep(n.arc, g.sx, x0, x1)}
		}
		k.topLo, k.topHi = nodeRanges(sc, k.top, x0, x1)
		k.sideLo, k.sideHi = nodeRanges(sc, k.side, y0, y1)
		e.corners = append(e.corners, k)
	}
	return e
}

// intervalTab returns the box-filtered blurred indicator of the interval
// between a and b for the pixels [z0, z1).
func (sc *shadowScratch) intervalTab(a, b, s float64, z0, z1 int) []float64 {
	if a > b {
		a, b = b, a
	}
	out := sc.floats(z1 - z0)
	ta, tb := sc.makeStep(a, s, z0, z1), sc.makeStep(b, s, z0, z1)
	for i := range out {
		out[i] = ta.at(z0+i) - tb.at(z0+i)
	}
	return out
}

// nodeRanges returns, for each pixel p in [z0, z1), the range of nodes
// whose density table is nonzero there. Nodes are sorted by position, so
// the ranges are monotonic.
func nodeRanges(sc *shadowScratch, nodes []sepNode, z0, z1 int) (lo, hi []int32) {
	lo, hi = sc.ints(z1-z0), sc.ints(z1-z0)
	a, b := 0, 0
	for p := z0; p < z1; p++ {
		for a < len(nodes) && nodes[a].dens.lo+len(nodes[a].dens.v) <= p {
			a++
		}
		b = max(b, a)
		for b < len(nodes) && nodes[b].dens.lo <= p {
			b++
		}
		lo[p-z0], hi[p-z0] = int32(a), int32(b)
	}
	return lo, hi
}

func (e *sepEval) row(y int, v []float64, want []uint8) {
	z := e.zone
	fy := e.fy[y-z.Min.Y]
	for i := range v {
		v[i] = float64(e.fx[i] * fy)
	}
	for ci := range e.corners {
		k := &e.corners[ci]
		if y < k.y0 || y >= k.y1 {
			continue
		}
		ry := k.rectY[y-k.y0]
		by := k.boxY.at(y)
		// The side nodes reaching this row, with their densities, and the
		// intervals of all top nodes in this row.
		sl, sh := k.sideLo[y-k.y0], k.sideHi[y-k.y0]
		e.sideD = e.sideD[:0]
		for _, n := range k.side[sl:sh] {
			e.sideD = append(e.sideD, n.dens.at(y))
		}
		e.topIV = e.topIV[:0]
		for _, n := range k.top {
			e.topIV = append(e.topIV, by-n.arc.at(y))
		}
		for x := k.x0; x < k.x1; x++ {
			i := x - z.Min.X
			if want[i] == 0 {
				continue
			}
			d := float64(k.rectX[x-k.x0] * ry)
			tl, th := int(k.topLo[x-k.x0]), int(k.topHi[x-k.x0])
			for j := tl; j < th; j++ {
				dn := &k.top[j].dens // its window contains x
				d += float64(dn.v[x-dn.lo] * e.topIV[j])
			}
			bx := k.boxX.at(x)
			for j := range e.sideD {
				d += float64(e.sideD[j] * (bx - k.side[sl+int32(j)].arc.at(x)))
			}
			v[i] -= d
		}
	}
}

// ---------------------------------------------------------------------------
// Scratch
// ---------------------------------------------------------------------------

// shadowScratch holds the buffers of drawShadow, reused between calls.
type shadowScratch struct {
	g     rrBlur
	sep   sepEval
	local localEval
	mask  maskShadow
	nodes []rrNode
	kw    []int64

	// Arenas: slices handed out stay valid when an arena grows (the old
	// array stays alive), and are reused from the next shadow on.
	f   []float64
	i32 []int32
	sn  []sepNode

	want, cov []uint8
	vf        []float64
}

func (sc *shadowScratch) reset() {
	sc.mask = maskShadow{} // drop the buffer
	sc.f, sc.i32 = sc.f[:0], sc.i32[:0]
	clear(sc.sn)
	sc.sn = sc.sn[:0]
}

func (sc *shadowScratch) floats(n int) []float64 {
	if cap(sc.f)-len(sc.f) < n {
		sc.f = make([]float64, 0, max(2*cap(sc.f), n+1024))
	}
	out := sc.f[len(sc.f) : len(sc.f)+n : len(sc.f)+n]
	sc.f = sc.f[:len(sc.f)+n]
	return out
}

func (sc *shadowScratch) ints(n int) []int32 {
	if cap(sc.i32)-len(sc.i32) < n {
		sc.i32 = make([]int32, 0, max(2*cap(sc.i32), n+1024))
	}
	out := sc.i32[len(sc.i32) : len(sc.i32)+n : len(sc.i32)+n]
	sc.i32 = sc.i32[:len(sc.i32)+n]
	return out
}

func (sc *shadowScratch) sepNodes(n int) []sepNode {
	if cap(sc.sn)-len(sc.sn) < n {
		sc.sn = make([]sepNode, 0, max(2*cap(sc.sn), n+64))
	}
	out := sc.sn[len(sc.sn) : len(sc.sn)+n : len(sc.sn)+n]
	sc.sn = sc.sn[:len(sc.sn)+n]
	return out
}

func (sc *shadowScratch) vrow(n int) []float64 {
	if cap(sc.vf) < n {
		sc.vf = make([]float64, n)
	}
	return sc.vf[:n]
}

func (sc *shadowScratch) wantRow(n int) []uint8 {
	if cap(sc.want) < n {
		sc.want = make([]uint8, n)
	}
	return sc.want[:n]
}

func (sc *shadowScratch) covRow(n int) []uint8 {
	if cap(sc.cov) < n {
		sc.cov = make([]uint8, n)
	}
	return sc.cov[:n]
}
