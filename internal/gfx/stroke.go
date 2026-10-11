// Copyright 2023 the Kurbo Authors
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Stroke expansion modeled on kurbo's kurbo/src/stroke.rs (stroke,
// StrokeCtx::do_join, finish, finish_closed, round_cap, square_cap and the
// PathSeg::tangents rule of bezpath.rs). Curves are flattened and offset as
// polylines instead of kurbo's cubic fitting, and inner joins are made
// winding-safe; see Path.Stroke. See THIRD_PARTY.md.

package gfx

import "math"

// Shares of the Stroke tolerance spent on each source of error. They add up
// to 1 along any part of the outline (see Path.Stroke).
const (
	strokeFlatShare = 0.5  // flattening the center line of curves
	strokeArcShare  = 0.25 // polygonizing round joins and caps
	strokeEndShare  = 0.25 // tangent errors at exposed curve ends; join classification
	// maxArcDepth bounds the polygonization of one round join or cap to
	// 2^maxArcDepth chords.
	maxArcDepth = 10
)

// Stroke returns the outline of the stroke of p with the given style, as a
// path to be filled with the NonZero rule. The returned path contains only
// MoveTo, LineTo and Close; curves, round joins and round caps are already
// flattened.
//
// The stroke shape is the one of SVG 2: the union of the sweep of a segment of
// length Width centered on the path and perpendicular to it, the join shapes
// at corners (the true tangents of the curves decide the miter and bevel
// geometry; the miter limit falls back to a bevel, as in SVG), and the cap
// shapes at the ends of open subpaths and of dashes. Closed subpaths join at
// their start point and get no caps. Zero-length segments are skipped for
// direction purposes. A subpath that has a drawing verb but zero length (such
// as MoveTo(p) followed by LineTo(p) or Close) draws a dot with round or
// square caps (a circle, or a square aligned with the x axis) and nothing with
// butt caps; a lone MoveTo draws nothing.
//
// Dashes (StrokeStyle.Dashes and DashOffset) are applied first, with kurbo's
// algorithm and SVG semantics: the pattern restarts at every subpath, a
// negative offset wraps around, a dash that runs through the start point of a
// closed subpath is joined with the dash that starts there, and a closed
// subpath covered by a single dash stays closed. Every dash gets caps; a
// zero-length dash is a dot aligned with the path direction. A pattern that
// would produce more than 65536 dashes is ignored and the stroke is solid.
//
// style is normalized like the Builder does (see StrokeStyle); a style that
// draws nothing (Width <= 0 or non-finite values), an empty path or a path
// with non-finite points give an empty path.
//
// tolerance is the maximum distance, in the coordinates of p, between the
// boundary of the returned outline and the boundary of the exact stroke.
// Values below 1e-4, and NaN, are raised to 1e-4, and values below the
// float32 resolution of the outline (about 2.4e-7 times its largest
// coordinate) are raised to it. A rasterizer that strokes in local
// coordinates and then transforms the outline by an Affine M must pass
// deviceTolerance / M.MaxScale(), so that the error stays within
// deviceTolerance on screen.
//
// # Fill rule and orientation
//
// The outline is the boundary of a union of pieces: one quadrilateral per
// flattened segment, one wedge per join and one shape per cap. Every piece is
// traced with the same orientation, and pieces only add winding, so the
// winding number is at least 1 everywhere inside the stroke and 0 outside:
// the output must be filled with NonZero, never EvenOdd. It self-overlaps in
// general (at inner joins, in tight curves, where dashes or subpaths cross).
// Inner joins are traced through the inner miter point when that point is
// provably covered by both adjacent pieces (this gives clean, non-overlapping
// outlines for ordinary corners) and through the center point otherwise,
// which keeps the winding non-zero for any width.
//
// # Curves
//
// kurbo offsets each curve by fitting cubic Beziers to the exact offset
// curve. Stroke instead flattens the center line (Path.Flatten's algorithm,
// with half the tolerance) and strokes the polyline, treating the vertices
// inside a curve as round joins. The union of the quadrilaterals of a
// polyline and of the round wedges at its vertices is exactly the set of
// points within Width/2 of the polyline (minus the cap regions at its ends),
// and that set moves by no more than the polyline's distance to the curve, so
// the error away from the ends is bounded by the flattening error plus the
// polygonization of the wedges. At the ends of a curve that meet a butt or
// square cap, or a miter or bevel join, the first or last piece is skewed so
// that its end edge lies exactly on the true normal of the curve; the skew
// thins that piece by Width/2*(1-cos d), where d is the angle between the
// chord and the true tangent, and the curve is split near its end until that
// is within a quarter of the tolerance.
func (p Path) Stroke(style StrokeStyle, tolerance float32) Path {
	var out Path
	style, ok := sanitizeStroke(style)
	if !ok || p.IsEmpty() || !p.IsFinite() {
		return out
	}
	st := newStroker(style, tolerance, pathMaxAbs(p), &out)
	subs := pathSubpaths(p)
	if len(style.Dashes) > 0 {
		if dashed, ok := dashSubpaths(subs, style.Dashes, style.DashOffset, st.tol); ok {
			subs = dashed
		}
	}
	for i := range subs {
		st.strokeSubpath(&subs[i])
	}
	return out
}

// v2 is a float64 vector. Its arithmetic converts every product explicitly
// so that the compiler cannot fuse multiply-adds: results are identical on
// every architecture.
type v2 struct{ x, y float64 }

func v2of(p Point) v2             { return v2{f64(p.X), f64(p.Y)} }
func (a v2) add(b v2) v2          { return v2{a.x + b.x, a.y + b.y} }
func (a v2) sub(b v2) v2          { return v2{a.x - b.x, a.y - b.y} }
func (a v2) neg() v2              { return v2{-a.x, -a.y} }
func (a v2) mul(s float64) v2     { return v2{float64(a.x * s), float64(a.y * s)} }
func (a v2) perp() v2             { return v2{-a.y, a.x} } // rotated by +90 degrees
func (a v2) norm() float64        { return math.Sqrt(dot2(a, a)) }
func dot2(a, b v2) float64        { return float64(a.x*b.x) + float64(a.y*b.y) }
func cross2(a, b v2) float64      { return float64(a.x*b.y) - float64(a.y*b.x) }
func lerp2(a, b v2, t float64) v2 { return a.add(b.sub(a).mul(t)) }

// unit returns a scaled to unit length and true, or false for a zero (or
// non-finite) vector.
func (a v2) unit() (v2, bool) {
	l := a.norm()
	if !(l > 0) || math.IsInf(l, 0) {
		return v2{}, false
	}
	return v2{a.x / l, a.y / l}, true
}

// point converts to a Point, clamping to the finite float32 range.
func (a v2) point() Point {
	const m = math.MaxFloat32
	return Point{float32(max(-m, min(m, a.x))), float32(max(-m, min(m, a.y)))}
}

// cseg is one drawing segment in float64: a line p[0] -> p[1], or a cubic
// p[0], p[1], p[2], p[3]. Quadratics are raised to cubics, which is exact.
type cseg struct {
	cubic bool
	p     [4]v2
}

func lineSeg(a, b v2) cseg { return cseg{p: [4]v2{a, b}} }

func cubicSeg(p0, p1, p2, p3 v2) cseg { return cseg{cubic: true, p: [4]v2{p0, p1, p2, p3}} }

func quadSeg(p0, p1, p2 v2) cseg {
	const k = 2.0 / 3.0
	return cubicSeg(p0, lerp2(p0, p1, k), lerp2(p2, p1, k), p2)
}

func (s *cseg) end() v2 {
	if s.cubic {
		return s.p[3]
	}
	return s.p[1]
}

// degenerate reports whether every point of s is (to within a relative
// 1e-12) its start point: the segment has no direction.
func (s *cseg) degenerate() bool {
	n := 2
	if s.cubic {
		n = 4
	}
	p0 := s.p[0]
	eps := 1e-12 * (1 + max(math.Abs(p0.x), math.Abs(p0.y)))
	for _, q := range s.p[1:n] {
		if math.Abs(q.x-p0.x) > eps || math.Abs(q.y-p0.y) > eps {
			return false
		}
	}
	return true
}

// tangents returns the unit tangents at the start and end of a
// non-degenerate segment, using kurbo's PathSeg::tangents rule: when a
// control point coincides with its end point, the next one gives the
// direction.
func (s *cseg) tangents() (t0, t1 v2) {
	if !s.cubic {
		t, _ := s.p[1].sub(s.p[0]).unit()
		return t, t
	}
	small := func(d v2) bool {
		eps := 1e-12 * (1 + max(math.Abs(s.p[0].x), math.Abs(s.p[0].y), math.Abs(s.p[3].x), math.Abs(s.p[3].y)))
		return math.Abs(d.x) <= eps && math.Abs(d.y) <= eps
	}
	d0 := s.p[1].sub(s.p[0])
	if small(d0) {
		d0 = s.p[2].sub(s.p[0])
		if small(d0) {
			d0 = s.p[3].sub(s.p[0])
		}
	}
	d1 := s.p[3].sub(s.p[2])
	if small(d1) {
		d1 = s.p[3].sub(s.p[1])
		if small(d1) {
			d1 = s.p[3].sub(s.p[0])
		}
	}
	t0, _ = d0.unit()
	t1, _ = d1.unit()
	return t0, t1
}

// eval returns the point at parameter t (de Casteljau).
func (s *cseg) eval(t float64) v2 {
	if !s.cubic {
		return lerp2(s.p[0], s.p[1], t)
	}
	a, b, c := lerp2(s.p[0], s.p[1], t), lerp2(s.p[1], s.p[2], t), lerp2(s.p[2], s.p[3], t)
	a, b = lerp2(a, b, t), lerp2(b, c, t)
	return lerp2(a, b, t)
}

// deriv returns the derivative at parameter t.
func (s *cseg) deriv(t float64) v2 {
	if !s.cubic {
		return s.p[1].sub(s.p[0])
	}
	d0, d1, d2 := s.p[1].sub(s.p[0]), s.p[2].sub(s.p[1]), s.p[3].sub(s.p[2])
	a, b := lerp2(d0, d1, t), lerp2(d1, d2, t)
	return lerp2(a, b, t).mul(3)
}

// direction returns the unit direction of the segment at t, falling back to
// the end tangents where the derivative vanishes.
func (s *cseg) direction(t float64) v2 {
	if d, ok := s.deriv(t).unit(); ok {
		return d
	}
	t0, t1 := s.tangents()
	if t < 0.5 {
		return t0
	}
	return t1
}

// subseg returns the part of s between t0 and t1 (kurbo's subsegment). The
// end points are exactly eval(t0) and eval(t1), so that consecutive parts
// share their end points bit for bit.
func (s *cseg) subseg(t0, t1 float64) cseg {
	if t0 == 0 && t1 == 1 {
		return *s
	}
	p0, p3 := s.eval(t0), s.eval(t1)
	if !s.cubic {
		return lineSeg(p0, p3)
	}
	k := (t1 - t0) * (1.0 / 3.0)
	return cubicSeg(p0, p0.add(s.deriv(t0).mul(k)), p3.sub(s.deriv(t1).mul(k)), p3)
}

func (s *cseg) array() [4][2]float64 {
	return [4][2]float64{{s.p[0].x, s.p[0].y}, {s.p[1].x, s.p[1].y}, {s.p[2].x, s.p[2].y}, {s.p[3].x, s.p[3].y}}
}

// subpath is one subpath of the input (or one dash) in float64.
type subpath struct {
	segs   []cseg
	start  v2
	closed bool
	// draws is true when the subpath has a drawing verb; a subpath with
	// draws but zero length is a dot.
	draws bool
	// dotDir is the direction of a dot's square cap; the zero vector means
	// the x axis.
	dotDir v2
}

// pathSubpaths converts p to float64 subpaths. A Close adds the closing line
// when the subpath does not end at its start.
func pathSubpaths(p Path) []subpath {
	var subs []subpath
	cur := -1
	ensure := func(at Point) {
		if cur < 0 {
			subs = append(subs, subpath{start: v2of(at)})
			cur = len(subs) - 1
		}
	}
	for s := range p.Segments() {
		switch s.Verb {
		case MoveTo:
			subs = append(subs, subpath{start: v2of(s.P[0])})
			cur = len(subs) - 1
			continue
		case LineTo:
			ensure(s.P[0])
			subs[cur].segs = append(subs[cur].segs, lineSeg(v2of(s.P[0]), v2of(s.P[1])))
		case QuadTo:
			ensure(s.P[0])
			subs[cur].segs = append(subs[cur].segs, quadSeg(v2of(s.P[0]), v2of(s.P[1]), v2of(s.P[2])))
		case CubicTo:
			ensure(s.P[0])
			subs[cur].segs = append(subs[cur].segs, cubicSeg(v2of(s.P[0]), v2of(s.P[1]), v2of(s.P[2]), v2of(s.P[3])))
		case Close:
			ensure(s.P[0])
			if s.P[0] != s.P[1] {
				subs[cur].segs = append(subs[cur].segs, lineSeg(v2of(s.P[0]), v2of(s.P[1])))
			}
			subs[cur].closed = true
			subs[cur].draws = true
			cur = -1 // a drawing verb after Close comes after an implicit MoveTo
			continue
		}
		subs[cur].draws = true
	}
	return subs
}

// piece is one flattened segment of a polyline being stroked: the
// parallelogram with corners a-m, b-m, b+m, a+m. For an ordinary chord t == u
// and the piece is a rectangle; the first or last chord of a curve at an
// exposed end uses the curve's true tangent as t, so that its end edge lies
// on the true normal.
type piece struct {
	a, b v2
	u    v2 // unit direction of b - a
	t    v2 // unit direction used for joins and caps; dot(u, t) > 0
	m    v2 // offset vector: t rotated by +90 degrees, times Width/2
}

// coords returns q in the piece's affine frame: q = a + s*(b-a) + r*m.
func (pc *piece) coords(q v2) (s, r float64) {
	ab := pc.b.sub(pc.a)
	d := cross2(ab, pc.m)
	aq := q.sub(pc.a)
	return cross2(aq, pc.m) / d, cross2(ab, aq) / d
}

// stroker holds the parameters and scratch buffers of one Stroke call.
type stroker struct {
	out *Path
	h   float64 // half width
	// tol is the overall tolerance; the others are derived from it.
	tol             float64
	flatTol, flatSq float64 // flattenCubic parameters (safety factor applied)
	arcTol          float64
	cosEnd          float64 // minimum cos between an exposed end chord and its true tangent
	cosSmooth       float64 // minimum cos(turn/2) of a turn drawn as a round join
	join            LineJoin
	cap             LineCap
	miter2          float64 // MiterLimit squared

	segs  []cseg
	tans  [][2]v2
	user  []bool
	verts []v2
	tmp   []v2
	pcs   []piece
	joins []bool // joins[i]: the join after pcs[i] uses the style's join (else round)
	fwd   []v2
	bwd   []v2
	qbuf  []quadSub
}

// pathMaxAbs returns the largest absolute coordinate of p.
func pathMaxAbs(p Path) float64 {
	m := 0.0
	for _, q := range p.pts {
		m = max(m, math.Abs(f64(q.X)), math.Abs(f64(q.Y)))
	}
	return m
}

// newStroker prepares a Stroke call. maxAbs is the largest absolute
// coordinate of the path: the tolerance is raised to about two float32 ulps
// of the largest output coordinate, below which it is meaningless and would
// only inflate the output.
func newStroker(style StrokeStyle, tolerance float32, maxAbs float64, out *Path) *stroker {
	h := f64(style.Width) / 2
	tol := f64(tolerance)
	if !(tol >= minFlattenTolerance) { // also catches NaN
		tol = minFlattenTolerance
	}
	tol = max(tol, (maxAbs+h)*0x1p-22)
	st := &stroker{
		out:    out,
		h:      h,
		tol:    tol,
		arcTol: strokeArcShare * tol,
		join:   style.Join,
		cap:    style.Cap,
		miter2: f64(style.MiterLimit) * f64(style.MiterLimit),
	}
	ft := max(strokeFlatShare*tol, minFlattenTolerance) * flattenSafety
	st.flatTol, st.flatSq = ft, math.Sqrt(ft)
	endTol := strokeEndShare * tol
	// Skewing the end piece by an angle d thins it by h*(1-cos d).
	st.cosEnd = max(1-endTol/h, 0.5)
	// A turn of angle a drawn round instead of mitered or beveled moves the
	// outline by at most h*(1/cos(a/2) - 1).
	st.cosSmooth = 1 / (1 + endTol/h)
	return st
}

// isCorner reports whether the turn from direction a to direction b (unit
// vectors) gets the style's join. Small turns are drawn as round joins: the
// difference is within the tolerance and they need no special handling of
// the curve ends.
func (st *stroker) isCorner(a, b v2) bool {
	if st.join == JoinRound {
		return false
	}
	d := dot2(a, b)
	if d > 0 && math.Sqrt((1+d)/2) >= st.cosSmooth {
		return false
	}
	return true
}

func (st *stroker) strokeSubpath(sp *subpath) {
	st.segs = st.segs[:0]
	for i := range sp.segs {
		if !sp.segs[i].degenerate() {
			st.segs = append(st.segs, sp.segs[i])
		}
	}
	n := len(st.segs)
	if n == 0 {
		if sp.draws {
			st.dot(sp.start, sp.dotDir)
		}
		return
	}
	closed := sp.closed
	st.tans = st.tans[:0]
	for i := range st.segs {
		t0, t1 := st.segs[i].tangents()
		st.tans = append(st.tans, [2]v2{t0, t1})
	}
	// user[k]: the join at the start of segment k uses the style's join.
	st.user = st.user[:0]
	for k := range n {
		var u bool
		switch {
		case k > 0:
			u = st.isCorner(st.tans[k-1][1], st.tans[k][0])
		case closed:
			u = st.isCorner(st.tans[n-1][1], st.tans[0][0])
		}
		st.user = append(st.user, u)
	}
	exposedCap := st.cap != CapRound
	st.pcs, st.joins = st.pcs[:0], st.joins[:0]
	for k := range n {
		expStart := st.user[k] || (!closed && k == 0 && exposedCap)
		var expEnd bool
		switch {
		case k+1 < n:
			expEnd = st.user[k+1]
		case closed:
			expEnd = st.user[0]
		default:
			expEnd = exposedCap
		}
		st.addSegment(&st.segs[k], st.tans[k], expStart, expEnd, st.user[k])
	}
	if len(st.pcs) == 0 {
		// Every chord vanished (sub-precision segments): draw a dot.
		if sp.draws {
			st.dot(sp.start, st.tans[0][0])
		}
		return
	}
	if closed && len(st.pcs) < 2 {
		closed = false // sub-precision closing segment
	}
	if closed {
		st.joins = append(st.joins, st.user[0])
	}
	st.strokePolyline(closed)
}

// addSegment flattens s and appends its pieces. expStart and expEnd tell
// whether the segment's ends are exposed (a butt or square cap, or a miter
// or bevel join): there the end pieces use the true tangents. userJoin is the
// kind of the join before the segment's first piece.
func (st *stroker) addSegment(s *cseg, tan [2]v2, expStart, expEnd, userJoin bool) {
	st.verts = st.segVerts(s, tan, expStart, expEnd, st.verts[:0])
	nc := len(st.verts) - 1
	first := true
	for i := range nc {
		a, b := st.verts[i], st.verts[i+1]
		u, ok := b.sub(a).unit()
		if !ok {
			continue
		}
		t := u
		switch {
		case first && expStart && dot2(u, tan[0]) >= st.cosEnd:
			t = tan[0]
		case i == nc-1 && expEnd && dot2(u, tan[1]) >= st.cosEnd:
			t = tan[1]
		}
		if len(st.pcs) > 0 {
			st.joins = append(st.joins, first && userJoin)
		}
		st.pcs = append(st.pcs, piece{a: a, b: b, u: u, t: t, m: t.perp().mul(st.h)})
		first = false
	}
}

// segVerts returns the polyline of s, start and end included, with no
// repeated points. At exposed ends of a curve the first or last chord is made
// to lie within acos(cosEnd) of the true tangent by splitting the curve near
// that end.
func (st *stroker) segVerts(s *cseg, tan [2]v2, expStart, expEnd bool, dst []v2) []v2 {
	if !s.cubic {
		return append(dst, s.p[0], s.p[1])
	}
	base := len(dst)
	dst = st.flattenRange(s, 0, 1, dst)
	full := dst[base:]
	if len(full) < 2 {
		return dst // below float precision: no chord
	}
	okS := !expStart || st.chordOK(full[0], full[1], tan[0])
	okE := !expEnd || st.chordOK(full[len(full)-2], full[len(full)-1], tan[1])
	// A single chord cannot follow two different end tangents.
	single := expStart && expEnd && len(full) == 2 && dot2(tan[0], tan[1]) < 1-1e-12
	if okS && okE && !single {
		return dst
	}
	dst = dst[:base]
	ts, te := 0.0, 1.0
	if expStart {
		ts = st.tighten(s, tan[0], true)
	}
	if expEnd {
		te = st.tighten(s, tan[1], false)
	}
	if ts > 0 {
		dst = st.flattenRange(s, 0, ts, dst)
	}
	if te > ts {
		dst = st.appendVerts(dst, st.flattenRange(s, ts, te, st.tmp[:0]))
	}
	if te < 1 {
		dst = st.appendVerts(dst, st.flattenRange(s, te, 1, st.tmp[:0]))
	}
	return dst
}

// appendVerts appends src to dst, skipping points equal to the last one.
func (st *stroker) appendVerts(dst, src []v2) []v2 {
	for _, q := range src {
		dst = appendVert(dst, q)
	}
	st.tmp = src[:0]
	return dst
}

// appendVert appends q unless it is (to within a relative 1e-9) the last
// point: chords that short carry no reliable direction.
func appendVert(dst []v2, q v2) []v2 {
	if n := len(dst); n > 0 {
		l := dst[n-1]
		eps := 1e-9 * (1 + max(math.Abs(l.x), math.Abs(l.y)))
		if math.Abs(q.x-l.x) <= eps && math.Abs(q.y-l.y) <= eps {
			return dst
		}
	}
	return append(dst, q)
}

// chordOK reports whether the chord a -> b is close enough to the tangent t
// for the end piece to be skewed onto t.
func (st *stroker) chordOK(a, b, t v2) bool {
	u, ok := b.sub(a).unit()
	return ok && dot2(u, t) >= st.cosEnd
}

// tighten returns the split parameter near the start (fromStart) or the end
// of s such that the end chord of the flattened end part is within
// acos(cosEnd) of the tangent t. It first halves the end part until the
// tangents sampled on it lie in that cone (any chord of a part whose tangents
// lie in the cone does too), then checks the chord of the actual flattening
// and keeps halving if needed.
func (st *stroker) tighten(s *cseg, t v2, fromStart bool) float64 {
	k := 0.5
	inCone := func(k float64) bool {
		for j := 1; j <= 8; j++ {
			u := k * float64(j) / 8
			if !fromStart {
				u = 1 - u
			}
			if dot2(s.direction(u), t) < st.cosEnd {
				return false
			}
		}
		return true
	}
	for i := 0; i < 60 && !inCone(k); i++ {
		k /= 2
	}
	for range 60 {
		var v []v2
		var a, b v2
		if fromStart {
			v = st.flattenRange(s, 0, k, st.tmp[:0])
			if len(v) >= 2 {
				a, b = v[0], v[1]
			}
		} else {
			v = st.flattenRange(s, 1-k, 1, st.tmp[:0])
			if n := len(v); n >= 2 {
				a, b = v[n-2], v[n-1]
			}
		}
		st.tmp = v[:0]
		if len(v) < 2 {
			// The end part is below float precision: keep the previous one.
			k = min(k*2, 0.5)
			break
		}
		if st.chordOK(a, b, t) {
			break
		}
		k /= 2
	}
	if fromStart {
		return k
	}
	return 1 - k
}

// flattenRange appends the polyline of s between t0 and t1 (with Path.Flatten's
// algorithm), end points included.
func (st *stroker) flattenRange(s *cseg, t0, t1 float64, dst []v2) []v2 {
	sub := s.subseg(t0, t1)
	dst = appendVert(dst, sub.p[0])
	st.qbuf = flattenCubic(sub.array(), st.flatTol, st.flatSq, st.qbuf[:0], func(_ Verb, pt Point) {
		dst = appendVert(dst, v2of(pt))
	})
	return appendVert(dst, sub.p[3])
}

// strokePolyline strokes st.pcs, joined by st.joins, as an open or closed
// polyline, following kurbo's StrokeCtx: a forward path along the -m side and
// a backward path along the +m side, then the end cap, the reversed backward
// path and the start cap (open), or two closed loops (closed).
func (st *stroker) strokePolyline(closed bool) {
	pcs := st.pcs
	n := len(pcs)
	fwd := append(st.fwd[:0], pcs[0].a.sub(pcs[0].m))
	bwd := append(st.bwd[:0], pcs[0].a.add(pcs[0].m))
	for i := range pcs {
		if i > 0 {
			fwd, bwd = st.doJoin(fwd, bwd, i-1, i, closed, false)
		}
		fwd = append(fwd, pcs[i].b.sub(pcs[i].m))
		bwd = append(bwd, pcs[i].b.add(pcs[i].m))
	}
	o := outliner{p: st.out}
	if closed {
		fwd, bwd = st.doJoin(fwd, bwd, n-1, 0, true, true)
		// The closing join ends both paths where they started.
		fwd, bwd = trimRepeat(fwd), trimRepeat(bwd)
		o.move(fwd[0])
		for _, q := range fwd[1:] {
			o.line(q)
		}
		o.close()
		o.move(bwd[len(bwd)-1])
		for i := len(bwd) - 2; i >= 0; i-- {
			o.line(bwd[i])
		}
		o.close()
	} else {
		o.move(fwd[0])
		for _, q := range fwd[1:] {
			o.line(q)
		}
		last := &pcs[n-1]
		st.cap1(&o, last.b, last.m.neg(), last.t)
		for i := len(bwd) - 1; i >= 0; i-- {
			o.line(bwd[i])
		}
		first := &pcs[0]
		st.cap1(&o, first.a, first.m, first.t.neg())
		o.close()
	}
	st.fwd, st.bwd = fwd[:0], bwd[:0]
}

// doJoin adds the join between pieces ia and ib (whose common point is
// pcs[ib].a) to the forward and backward paths, which end at that point
// offset by -m and +m of piece ia. The outer side gets the join shape (kurbo's
// do_join); the inner side goes through the inner miter point when kiteOK
// allows it, and through the center point otherwise. closing is set for the
// join that closes a closed polyline.
func (st *stroker) doJoin(fwd, bwd []v2, ia, ib int, closed, closing bool) ([]v2, []v2) {
	A, B := &st.pcs[ia], &st.pcs[ib]
	p := B.a
	cr, dt := cross2(A.t, B.t), dot2(A.t, B.t)
	if cr == 0 && dt > 0 {
		// Straight continuation.
		fwd = appendIfNew(fwd, p.sub(B.m))
		bwd = appendIfNew(bwd, p.add(B.m))
		return fwd, bwd
	}
	// The outer side is the one the path turns away from. A reversal
	// (cr == 0, dt < 0) puts it on the forward side, like kurbo.
	outerFwd := cr > 0 || (cr == 0 && dt < 0)
	oA, oB := A.m, B.m
	if outerFwd {
		oA, oB = oA.neg(), oB.neg()
	}
	sign := 1.0
	if cr < 0 {
		sign = -1
	}
	outer, inner := &bwd, &fwd
	if outerFwd {
		outer, inner = &fwd, &bwd
	}
	userJoin := st.joins[ia]
	switch {
	case !userJoin || st.join == JoinRound:
		*outer = st.arc(*outer, p, oA, oB, sign)
		*outer = append(*outer, p.add(oB))
	case st.join == JoinMiter && 2 <= (1+dt)*st.miter2:
		// SVG: miter length / width = 1/cos(turn/2) <= MiterLimit.
		fl, fth := p.add(oA), p.add(oB)
		h := cross2(A.t, fth.sub(fl)) / cr
		*outer = append(*outer, fth.sub(B.t.mul(h)), fth)
	default:
		*outer = append(*outer, p.add(oB))
	}
	iA, iB := oA.neg(), oB.neg()
	fracA, fracB := 0.5, 0.5
	if !closed && ia == 0 {
		fracA = 1
	}
	if !closed && ib == len(st.pcs)-1 {
		fracB = 1
	}
	if x, ok := kiteOK(p, A, B, iA, iB, fracA, fracB); ok {
		// Replace the end of A's inner edge by x; when closing, the start
		// of B's inner edge is the first point of the path.
		in := *inner
		in[len(in)-1] = x
		if closing {
			in[0] = x
		}
	} else {
		*inner = append(*inner, p, p.add(iB))
	}
	return fwd, bwd
}

// trimRepeat drops the last point of a closed loop when it repeats the first.
func trimRepeat(loop []v2) []v2 {
	if n := len(loop); n > 1 && loop[n-1] == loop[0] {
		return loop[:n-1]
	}
	return loop
}

func appendIfNew(dst []v2, q v2) []v2 {
	if n := len(dst); n > 0 && dst[n-1] == q {
		return dst
	}
	return append(dst, q)
}

// kiteOK decides whether the inner side of the join at p can be traced
// through the intersection x of the inner edges of A and B (offset vectors
// iA, iB from p) instead of through p. Tracing through x removes the "kite"
// p+iA, p, p+iB, x from the outline, which subtracts one from the winding
// number inside it. That is safe when the kite lies inside both pieces (where
// the winding is at least 2) and does not overlap another kite in the same
// piece: the kite must lie in the last fracA of A and the first fracB of B
// (half of each, or all of it when the other end of the piece is a cap).
func kiteOK(p v2, A, B *piece, iA, iB v2, fracA, fracB float64) (v2, bool) {
	den := cross2(A.u, B.u)
	if den == 0 {
		return v2{}, false
	}
	d := iB.sub(iA)
	x := p.add(iA).add(A.u.mul(cross2(d, B.u) / den))
	const slack = 1e-9
	inA := func(q v2) bool {
		s, r := A.coords(q)
		return s >= 1-fracA-slack && s <= 1+slack && math.Abs(r) <= 1+slack
	}
	inB := func(q v2) bool {
		s, r := B.coords(q)
		return s >= -slack && s <= fracB+slack && math.Abs(r) <= 1+slack
	}
	return x, inA(x) && inA(p.add(iB)) && inB(x) && inB(p.add(iA))
}

// cap1 adds the cap at point c, going from c+from to c-from, where dir is the
// unit direction pointing out of the stroke. The end points themselves are
// not added.
func (st *stroker) cap1(o *outliner, c, from, dir v2) {
	switch st.cap {
	case CapSquare:
		e := dir.mul(st.h)
		o.line(c.add(from).add(e))
		o.line(c.sub(from).add(e))
	case CapRound:
		mid := dir.mul(st.h)
		buf := st.arc(st.tmp[:0], c, from, mid, 1)
		for _, q := range buf {
			o.line(q)
		}
		o.line(c.add(mid))
		buf = st.arc(buf[:0], c, mid, from.neg(), 1)
		for _, q := range buf {
			o.line(q)
		}
		st.tmp = buf[:0]
	}
}

// dot draws the stroke of a zero-length subpath at c: a circle (round caps)
// or a square aligned with dir (square caps; the x axis when dir is zero).
func (st *stroker) dot(c, dir v2) {
	if st.cap == CapButt {
		return
	}
	t, ok := dir.unit()
	if !ok {
		t = v2{1, 0}
	}
	m := t.perp().mul(st.h)
	o := outliner{p: st.out}
	o.move(c.sub(m))
	st.cap1(&o, c, m.neg(), t)
	o.line(c.add(m))
	st.cap1(&o, c, m, t.neg())
	o.close()
}

// arc appends the points strictly inside the circular arc around c from
// c+from to c+to, turning in the direction of sign (+1: from x towards y)
// by the smaller angle (exactly pi when to == -from). |from| and |to| must
// both be st.h. The arc is cut into 2^k equal chords, inscribed, with the
// smallest k whose chords stay within st.arcTol of the arc. It uses only
// square roots (repeated bisection), so it is exactly reproducible.
func (st *stroker) arc(dst []v2, c, from, to v2, sign float64) []v2 {
	r := st.h
	if !(r > st.arcTol) {
		return dst // the chord is within tolerance
	}
	cosA := max(-1, min(1, dot2(from, to)/(r*r)))
	depth := 0
	for depth < maxArcDepth {
		// Sagitta of chords spanning angle/2^depth: r*(1 - cos(angle/2^(depth+1))).
		cosHalf := math.Sqrt((1 + cosA) / 2)
		if r*(1-cosHalf) <= st.arcTol {
			break
		}
		cosA = cosHalf
		depth++
	}
	var rec func(a, b v2, d int)
	rec = func(a, b v2, d int) {
		if d == 0 {
			return
		}
		s := a.add(b)
		l := s.norm()
		if l <= r*1e-9 {
			// Half turn: the bisector is perpendicular to a.
			s = a.perp().mul(sign)
			l = s.norm()
		}
		mid := s.mul(r / l)
		rec(a, mid, d-1)
		dst = append(dst, c.add(mid))
		rec(mid, b, d-1)
	}
	rec(from, to, depth)
	return dst
}

// outliner writes the output path, converting to float32. It emits a
// subpath's MoveTo only with its first LineTo (a loop that collapses to a
// point leaves nothing), skips repeated points, and merges a point that
// continues the last edge in the same direction into that edge, so that
// straight runs (an edge followed by a miter tip, consecutive collinear
// pieces) cost one edge.
type outliner struct {
	p         *Path
	prev, cur Point
	pending   bool // the MoveTo to cur is not written yet
	n         int  // LineTos in the current subpath
}

func (o *outliner) move(q v2) {
	o.cur = q.point()
	o.pending = true
	o.n = 0
}

func (o *outliner) line(q v2) {
	pt := q.point()
	if pt == o.cur {
		return
	}
	if o.pending {
		o.p.MoveTo(o.cur)
		o.pending = false
	}
	if o.n > 0 {
		ax, ay := f64(o.cur.X)-f64(o.prev.X), f64(o.cur.Y)-f64(o.prev.Y)
		bx, by := f64(pt.X)-f64(o.cur.X), f64(pt.Y)-f64(o.cur.Y)
		if float64(ax*by) == float64(ay*bx) && float64(ax*bx)+float64(ay*by) > 0 {
			o.p.pts[len(o.p.pts)-1] = pt
			o.cur = pt
			return
		}
	}
	o.p.LineTo(pt)
	o.prev, o.cur = o.cur, pt
	o.n++
}

func (o *outliner) close() {
	if o.n > 0 {
		o.p.Close()
	}
}
