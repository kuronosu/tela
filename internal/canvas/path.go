package canvas

import (
	"math"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// Paths: the current path of a Canvas, whose points are in the canvas's
// coordinates (each one is transformed by the transform current when it was
// added, so a transform changed afterwards does not move it), and a Path2D,
// whose points are its own. The methods that build both are the same, with
// the matrix that the points go through.
//
// A path keeps what it is given, in float64. What is drawn is its rounding to
// float32 (path.gfx), without the segments that have no length there: the
// browser removes them before it strokes a path ("trace a path"), where a
// stroke of gfx would draw a dot for a subpath made of nothing else. Filling
// them changes nothing.

// vec is a point or a vector.
type vec struct{ x, y float64 }

// path is a list of subpaths.
type path struct {
	verbs []gfx.Verb
	pts   []vec
	start vec // of the last subpath
	// box is the whole path, when hasBox: one upright rectangle, maybe with
	// rounded corners, which is drawn as such; or, when oval, an ellipse:
	// only the shadow of a circle takes it for one.
	box          box
	hasBox, oval bool
	// detail is by how much more than at its own size the arcs of a Path2D
	// can be scaled (0 for 1).
	detail float64
}

// pathDetail is the detail of a Path2D.
const pathDetail = 64

// box is an upright rectangle with the radii of its corners, in gfx's order
// (top left, top right, bottom right, bottom left).
type box struct {
	x0, y0, x1, y1 float64
	r              [4]vec
}

func (p *path) reset() {
	p.verbs, p.pts, p.hasBox = p.verbs[:0], p.pts[:0], false
}

// last returns the last point, and false for a path without one.
func (p *path) last() (vec, bool) {
	n := len(p.verbs)
	switch {
	case n == 0:
		return vec{}, false
	case p.verbs[n-1] == gfx.Close:
		return p.start, true
	}
	return p.pts[len(p.pts)-1], true
}

// moveTo starts a subpath. (After a box, a point alone draws nothing.)
func (p *path) moveTo(v vec) {
	if n := len(p.verbs); n > 0 && p.verbs[n-1] == gfx.MoveTo {
		p.pts[len(p.pts)-1] = v
	} else {
		p.verbs = append(p.verbs, gfx.MoveTo)
		p.pts = append(p.pts, v)
	}
	p.start = v
}

// lineTo adds a line, or starts a subpath when there is none.
func (p *path) lineTo(v vec) {
	if len(p.verbs) == 0 {
		p.moveTo(v)
		return
	}
	p.hasBox = false
	p.verbs = append(p.verbs, gfx.LineTo)
	p.pts = append(p.pts, v)
}

// quadTo and cubicTo add a curve, after its first control point when there
// is no subpath.
func (p *path) quadTo(c, v vec) {
	if len(p.verbs) == 0 {
		p.moveTo(c)
	}
	p.hasBox = false
	p.verbs = append(p.verbs, gfx.QuadTo)
	p.pts = append(p.pts, c, v)
}

func (p *path) cubicTo(c1, c2, v vec) {
	if len(p.verbs) == 0 {
		p.moveTo(c1)
	}
	p.hasBox = false
	p.verbs = append(p.verbs, gfx.CubicTo)
	p.pts = append(p.pts, c1, c2, v)
}

func (p *path) close() {
	if n := len(p.verbs); n > 0 && p.verbs[n-1] != gfx.Close {
		p.verbs = append(p.verbs, gfx.Close)
	}
}

// rect adds a rectangle of m's coordinates.
func (p *path) rect(m *matrix, x, y, w, h float64) {
	whole := len(p.verbs) == 0
	p.moveTo(m.vec(x, y))
	p.lineTo(m.vec(x+w, y))
	p.lineTo(m.vec(x+w, y+h))
	p.lineTo(m.vec(x, y+h))
	p.close()
	if whole && m.axisAligned() {
		b := box{x0: x, y0: y, x1: x + w, y1: y + h}
		p.box, p.hasBox, p.oval = b.in(m), true, false
	}
}

// roundRect adds a rectangle of m's coordinates with rounded corners, whose
// radii are up to four numbers, in the browser's order. It adds nothing when
// one of them is not finite or is negative.
//
// With a negative width or height the path goes the other way around, as in
// Chromium: the first radius is always that of the corner at x, y.
func (p *path) roundRect(m *matrix, x, y, w, h float64, radii []float64) {
	if len(radii) > 4 {
		radii = radii[:4]
	}
	for _, r := range radii {
		if !finite(r) || r < 0 {
			return
		}
	}
	var ul, ur, lr, ll float64
	switch len(radii) {
	case 1:
		ul, ur, lr, ll = radii[0], radii[0], radii[0], radii[0]
	case 2:
		ul, ur, lr, ll = radii[0], radii[1], radii[0], radii[1]
	case 3:
		ul, ur, lr, ll = radii[0], radii[1], radii[2], radii[1]
	case 4:
		ul, ur, lr, ll = radii[0], radii[1], radii[2], radii[3]
	}
	// The corners must not overlap.
	k := 1.0
	aw, ah := math.Abs(w), math.Abs(h)
	for _, side := range [4][2]float64{{aw, ul + ur}, {ah, ur + lr}, {aw, lr + ll}, {ah, ll + ul}} {
		if side[1] > side[0] {
			k = min(k, side[0]/side[1])
		}
	}
	ul, ur, lr, ll = float64(ul*k), float64(ur*k), float64(lr*k), float64(ll*k)
	if ul == 0 && ur == 0 && lr == 0 && ll == 0 {
		p.rect(m, x, y, w, h)
		return
	}
	whole := len(p.verbs) == 0
	sx, sy := math.Copysign(1, w), math.Copysign(1, h)
	x1, y1 := x+w, y+h
	// corner goes to the point a and turns to b, around the center c of the
	// corner of radius r, from the angle by a quarter. (A mirrored rectangle
	// has mirrored angles.)
	corner := func(r float64, c, a, b vec, angle float64) {
		first, end := m.vec(a.x, a.y), m.vec(b.x, b.y)
		p.lineTo(first)
		if r != 0 {
			p.arc(m, c.x, c.y, float64(r*sx), float64(r*sy), 0, 1, angle, math.Pi/2, end)
		}
	}
	ulx, uly := x+float64(ul*sx), y+float64(ul*sy)
	urx, ury := x1-float64(ur*sx), y+float64(ur*sy)
	lrx, lry := x1-float64(lr*sx), y1-float64(lr*sy)
	llx, lly := x+float64(ll*sx), y1-float64(ll*sy)
	p.moveTo(m.vec(ulx, y))
	corner(ur, vec{urx, ury}, vec{urx, y}, vec{x1, ury}, -math.Pi/2)
	corner(lr, vec{lrx, lry}, vec{x1, lry}, vec{lrx, y1}, 0)
	corner(ll, vec{llx, lly}, vec{llx, y1}, vec{x, lly}, math.Pi/2)
	corner(ul, vec{ulx, uly}, vec{x, uly}, vec{ulx, y}, math.Pi)
	p.close()
	p.moveTo(m.vec(x, y))
	if whole && m.axisAligned() {
		b := box{x0: x, y0: y, x1: x1, y1: y1, r: [4]vec{{ul, ul}, {ur, ur}, {lr, lr}, {ll, ll}}}
		p.box, p.hasBox, p.oval = b.in(m), true, false
	}
}

// in returns b transformed by m, which must keep rectangles upright, made
// upright.
func (b *box) in(m *matrix) box {
	c := [4]vec{m.vec(b.x0, b.y0), m.vec(b.x1, b.y0), m.vec(b.x1, b.y1), m.vec(b.x0, b.y1)}
	out := box{
		x0: min(c[0].x, c[2].x), y0: min(c[0].y, c[2].y),
		x1: max(c[0].x, c[2].x), y1: max(c[0].y, c[2].y),
	}
	for i, v := range c {
		j := 0
		switch left, top := v.x == out.x0, v.y == out.y0; {
		case !left && top:
			j = 1
		case !left && !top:
			j = 2
		case left && !top:
			j = 3
		}
		out.r[j] = vec{
			math.Abs(float64(m.a*b.r[i].x) + float64(m.c*b.r[i].y)),
			math.Abs(float64(m.b*b.r[i].x) + float64(m.d*b.r[i].y)),
		}
	}
	return out
}

// rrect returns b as gfx draws it.
func (b *box) rrect() gfx.RRect {
	rr := gfx.RRect{Rect: gfx.Rect{X0: float32(b.x0), Y0: float32(b.y0), X1: float32(b.x1), Y1: float32(b.y1)}}
	for i, r := range b.r {
		rr.Radii[i] = gfx.Point{X: float32(r.x), Y: float32(r.y)}
	}
	return rr
}

// arcSweep returns the angle an arc turns from start to end: at most once
// around, and in its direction, whichever angle is larger.
func arcSweep(start, end float64, ccw bool) float64 {
	const turn = 2 * math.Pi
	if ccw {
		return -arcSweep(-start, -end, false)
	}
	switch d := end - start; {
	case d >= turn:
		return turn
	case d < 0:
		return turn - math.Mod(-d, turn)
	default:
		return d
	}
}

// ellipse adds an arc of the ellipse of m's coordinates around x, y, with the
// radii rx and ry along axes turned by rotation, that starts at angle start
// and turns by sweep, after a line to its start.
func (p *path) ellipse(m *matrix, x, y, rx, ry, rotation, start, sweep float64) {
	sr, cr := 0.0, 1.0
	if rotation != 0 {
		sr, cr = fmath.Sincos(rotation)
	}
	at := func(angle float64) vec {
		s, c := fmath.Sincos(angle)
		ex, ey := float64(rx*c), float64(ry*s)
		return m.vec(x+float64(ex*cr)-float64(ey*sr), y+float64(ex*sr)+float64(ey*cr))
	}
	first := at(start)
	p.lineTo(first)
	if sweep == 0 || rx == 0 && ry == 0 {
		return
	}
	end := first // a whole turn ends exactly where it starts
	if math.Abs(sweep) != 2*math.Pi {
		end = at(start + sweep)
	}
	if rx == 0 || ry == 0 {
		// A line there and back: its ends are corners, which the cubics of an
		// arc would only be nearly.
		turn := 0.0
		if rx == 0 {
			turn = math.Pi / 2
		}
		dir := math.Copysign(1, sweep)
		// Within a turn: an angle so large that adding to it changes nothing
		// would never reach the end of the sweep.
		from := math.Mod(start, 2*math.Pi)
		for i := math.Floor(float64((from-turn)*dir)/math.Pi) + 1; ; i++ {
			a := turn + float64(float64(i*math.Pi)*dir)
			if !(float64((a-from)*dir) < math.Abs(sweep)) {
				break
			}
			p.lineTo(at(a))
		}
		p.lineTo(end)
		return
	}
	whole := len(p.verbs) == 1 && end == first && rotation == 0 && m.axisAligned()
	p.arc(m, x, y, rx, ry, sr, cr, start, sweep, end)
	if whole {
		b := box{x0: x - rx, y0: y - ry, x1: x + rx, y1: y + ry, r: [4]vec{{rx, ry}, {rx, ry}, {rx, ry}, {rx, ry}}}
		p.box, p.hasBox, p.oval = b.in(m), true, true
	}
}

// arcTolerance is how far, in logical pixels, the curves of an arc may be
// from it.
const arcTolerance = 0.005

// arcError is the distance between a quarter of the unit circle and its
// cubic Bezier; each halving of the angle divides it by 64.
const arcError = 2.73e-4

// arc adds the curves of an arc of the ellipse of m's coordinates around x, y
// with radii rx and ry, turned by the angle of sine sr and cosine cr, from
// angle start, where the path is, over sweep. end is its end, transformed.
func (p *path) arc(m *matrix, x, y, rx, ry, sr, cr, start, sweep float64, end vec) {
	p.hasBox = false
	// Quarters of the circle, halved until the curves are close enough.
	step := math.Pi / 2
	size := float64(max(math.Abs(rx), math.Abs(ry)) * m.extent())
	if p.detail != 0 {
		size = float64(size * p.detail)
	}
	for e := float64(size * arcError); e > arcTolerance && step > math.Pi/1024; e /= 64 {
		step /= 2
	}
	n := max(int(math.Ceil(math.Abs(sweep)/step-1e-9)), 1)
	step = sweep / float64(n)
	// The control points are on the tangents at the ends of each curve: at
	// this much of the derivative.
	arm := float64(4.0 / 3 * fmath.Tan(step/4))
	// at returns the point of the arc at the angle of sine s and cosine c,
	// moved by k times the derivative there.
	at := func(s, c, k float64) vec {
		ex, ey := float64(rx*(c-float64(k*s))), float64(ry*(s+float64(k*c)))
		return m.vec(x+float64(ex*cr)-float64(ey*sr), y+float64(ex*sr)+float64(ey*cr))
	}
	s0, c0 := fmath.Sincos(start)
	for i := 1; i <= n; i++ {
		s1, c1 := fmath.Sincos(start + float64(step*float64(i)))
		v := end
		if i < n {
			v = at(s1, c1, 0)
		}
		p.verbs = append(p.verbs, gfx.CubicTo)
		p.pts = append(p.pts, at(s0, c0, arm), at(s1, c1, -arm), v)
		s0, c0 = s1, c1
	}
}

// arcTo adds the arc of the radius that touches the line from the last point
// to x1, y1 and the line from there to x2, y2, after a line to its start.
func (p *path) arcTo(m *matrix, x1, y1, x2, y2, radius float64) {
	if radius < 0 {
		return
	}
	if len(p.verbs) == 0 {
		p.moveTo(m.vec(x1, y1))
	}
	last, _ := p.last()
	inv, ok := m.invert()
	if !ok {
		return
	}
	x0, y0 := inv.at(last.x, last.y)
	ax, ay, bx, by := x0-x1, y0-y1, x2-x1, y2-y1
	la, lb := math.Sqrt(float64(ax*ax)+float64(ay*ay)), math.Sqrt(float64(bx*bx)+float64(by*by))
	// The last point went through the transform and back.
	eps := float64(1e-12 * (math.Abs(x1) + math.Abs(y1) + math.Abs(x0) + math.Abs(y0)))
	if la <= eps || lb == 0 || radius == 0 {
		p.lineTo(m.vec(x1, y1))
		return
	}
	ax, ay, bx, by = ax/la, ay/la, bx/lb, by/lb
	cross := float64(ax*by) - float64(ay*bx)
	dot := float64(ax*bx) + float64(ay*by)
	if math.Abs(cross) < 1e-12 { // the points are on a line
		p.lineTo(m.vec(x1, y1))
		return
	}
	// The arc touches the lines at the distance radius / tan(angle / 2) of
	// the corner, where tan(angle / 2) is sin / (1 + cos).
	d := float64(radius*(1+dot)) / math.Abs(cross)
	tx0, ty0 := x1+float64(ax*d), y1+float64(ay*d)
	tx1, ty1 := x1+float64(bx*d), y1+float64(by*d)
	// Its center is on the normal of the first line, to the side of the
	// second.
	nx, ny := -ay, ax
	if cross < 0 {
		nx, ny = ay, -ax
	}
	cx, cy := tx0+float64(nx*radius), ty0+float64(ny*radius)
	start := fmath.Atan2(ty0-cy, tx0-cx)
	sweep := fmath.Atan2(ty1-cy, tx1-cx) - start
	switch { // the shorter way around
	case sweep > math.Pi:
		sweep -= 2 * math.Pi
	case sweep < -math.Pi:
		sweep += 2 * math.Pi
	}
	p.lineTo(m.vec(tx0, ty0))
	p.arc(m, cx, cy, radius, radius, 0, 1, start, sweep, m.vec(tx1, ty1))
}

// in makes dst the path transformed by m.
func (p *path) in(dst *path, m *matrix) {
	dst.reset()
	dst.verbs = append(dst.verbs, p.verbs...)
	for _, v := range p.pts {
		dst.pts = append(dst.pts, m.vec(v.x, v.y))
	}
	dst.start = m.vec(p.start.x, p.start.y)
}

// append adds the subpaths of q, whose last point then starts a new one.
func (p *path) append(q *path) {
	last, ok := q.last()
	if !ok {
		return
	}
	start, b, hasBox, oval := q.start, q.box, q.hasBox && len(p.verbs) == 0, q.oval
	p.verbs = append(p.verbs, q.verbs...)
	p.pts = append(p.pts, q.pts...)
	p.start, p.box, p.hasBox, p.oval = start, b, hasBox, oval
	p.moveTo(last)
}

// gfx makes dst the path transformed by m (nil for none), without the
// segments that have no length.
func (p *path) gfx(dst *gfx.Path, m *matrix) {
	dst.Reset()
	var cur, start gfx.Point
	line := false // the last subpath of dst has a segment
	pts := p.pts
	at := func(i int) gfx.Point {
		if v := pts[i]; m == nil {
			return gfx.Point{X: float32(v.x), Y: float32(v.y)}
		} else {
			return m.point(v.x, v.y)
		}
	}
	for _, verb := range p.verbs {
		switch verb {
		case gfx.MoveTo:
			cur = at(0)
			start, line = cur, false
			dst.MoveTo(cur)
		case gfx.LineTo:
			if a := at(0); a != cur {
				dst.LineTo(a)
				cur, line = a, true
			}
		case gfx.QuadTo:
			if c, a := at(0), at(1); c != cur || a != cur {
				dst.QuadTo(c, a)
				cur, line = a, true
			}
		case gfx.CubicTo:
			if c1, c2, a := at(0), at(1), at(2); c1 != cur || c2 != cur || a != cur {
				dst.CubicTo(c1, c2, a)
				cur, line = a, true
			}
		case gfx.Close:
			if line { // else its one point, where the next subpath starts too
				dst.Close()
			}
			cur, line = start, false
		}
		pts = pts[verb.NumPoints():]
	}
}

// setGfx makes p the path src.
func (p *path) setGfx(src gfx.Path) {
	p.reset()
	for el := range src.Elements() {
		at := func(i int) vec { return vec{float64(el.Pts[i].X), float64(el.Pts[i].Y)} }
		switch el.Verb {
		case gfx.MoveTo:
			p.moveTo(at(0))
		case gfx.LineTo:
			p.lineTo(at(0))
		case gfx.QuadTo:
			p.quadTo(at(0), at(1))
		case gfx.CubicTo:
			p.cubicTo(at(0), at(1), at(2))
		case gfx.Close:
			p.close()
		}
	}
}

// contains reports whether the point (x, y) is inside the path, whose open
// subpaths count as closed, or on it. It only reads the path.
func (p *path) contains(x, y float64, rule gfx.FillRule) bool {
	if !finite(x, y) {
		return false
	}
	h := hit{x: x, y: y}
	// A point this close to the path is on it: its rounding.
	size := math.Abs(x) + math.Abs(y)
	for _, v := range p.pts {
		size = max(size, math.Abs(v.x)+math.Abs(v.y))
	}
	h.eps = float64(size * 1e-10)
	var cur, start vec
	pts := p.pts
	for _, verb := range p.verbs {
		switch verb {
		case gfx.MoveTo:
			h.line(cur, start)
			cur, start = pts[0], pts[0]
		case gfx.LineTo:
			h.line(cur, pts[0])
			cur = pts[0]
		case gfx.QuadTo:
			c, e := pts[0], pts[1]
			toward := func(a vec) vec {
				return vec{a.x + float64((c.x-a.x)*(2.0/3)), a.y + float64((c.y-a.y)*(2.0/3))}
			}
			h.cubic(cur, toward(cur), toward(e), e, 0)
			cur = e
		case gfx.CubicTo:
			h.cubic(cur, pts[0], pts[1], pts[2], 0)
			cur = pts[2]
		case gfx.Close:
			h.line(cur, start)
			cur = start
		}
		pts = pts[verb.NumPoints():]
	}
	h.line(cur, start)
	if rule == gfx.EvenOdd {
		return h.on || h.winding&1 != 0
	}
	return h.on || h.winding != 0
}

// hit counts how often a path turns around a point, by what crosses the ray
// from the point to the right.
type hit struct {
	x, y, eps float64
	winding   int
	on        bool // the point is on the path
}

// cross counts a piece from a to b, all of which is to the right of the
// point.
func (h *hit) cross(a, b vec) {
	switch {
	case a.y <= h.y && b.y > h.y:
		h.winding++
	case b.y <= h.y && a.y > h.y:
		h.winding--
	}
}

func (h *hit) line(a, b vec) {
	if a == b || h.y < min(a.y, b.y)-h.eps || h.y > max(a.y, b.y)+h.eps || h.x > max(a.x, b.x)+h.eps {
		return
	}
	if h.x < min(a.x, b.x)-h.eps {
		h.cross(a, b)
		return
	}
	dx, dy := b.x-a.x, b.y-a.y
	// side is above 0 for a point to the left of a line that goes down.
	side := float64(dx*(h.y-a.y)) - float64(dy*(h.x-a.x))
	if math.Abs(side) <= float64(h.eps*math.Sqrt(float64(dx*dx)+float64(dy*dy))) {
		h.on = true
		return
	}
	if (side > 0) == (dy > 0) {
		h.cross(a, b)
	}
}

func (h *hit) cubic(p0, p1, p2, p3 vec, depth int) {
	x0, x1 := min(p0.x, p1.x, p2.x, p3.x), max(p0.x, p1.x, p2.x, p3.x)
	y0, y1 := min(p0.y, p1.y, p2.y, p3.y), max(p0.y, p1.y, p2.y, p3.y)
	if h.y < y0-h.eps || h.y > y1+h.eps || h.x > x1+h.eps {
		return
	}
	if h.x < x0-h.eps {
		h.cross(p0, p3)
		return
	}
	if depth == 48 || x1-x0 <= float64(h.eps*8) && y1-y0 <= float64(h.eps*8) {
		h.on = true
		return
	}
	mid := func(a, b vec) vec { return vec{float64((a.x + b.x) / 2), float64((a.y + b.y) / 2)} }
	a, b, c := mid(p0, p1), mid(p1, p2), mid(p2, p3)
	d, e := mid(a, b), mid(b, c)
	f := mid(d, e)
	h.cubic(p0, a, d, f, depth+1)
	h.cubic(f, e, c, p3, depth+1)
}
