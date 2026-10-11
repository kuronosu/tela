// Copyright 2018 the Kurbo Authors
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Ported to Go from kurbo's kurbo/src/bezpath.rs (flatten), quadbez.rs
// (estimate_subdiv, determine_subdiv_t, the parabola integral
// approximations) and cubicbez.rs (to_quads). See THIRD_PARTY.md.

package gfx

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx/internal/fmath"
)

const (
	// minFlattenTolerance is the smallest tolerance honored by Flatten;
	// smaller values are raised to it to bound the segment count.
	minFlattenTolerance = 1e-4
	// maxFlattenSegments bounds the number of line segments emitted for one
	// curve segment.
	maxFlattenSegments = 1 << 14
	// toQuadTol is the fraction of the tolerance spent on approximating a
	// cubic with quadratics (kurbo's TO_QUAD_TOL).
	toQuadTol = 0.1
	// flattenSafety scales the requested tolerance; see Flatten.
	flattenSafety = 0.9
)

// Flatten converts the path to line segments such that every point of every
// curve is within tolerance of the emitted polyline (up to float32 rounding
// of the output coordinates). Tolerances below 1e-4 are raised to 1e-4.
//
// emit is called in order with MoveTo (1 point), LineTo (the segment's end
// point) and Close (the point is the zero Point and must be ignored). Curves
// are replaced by LineTo calls; straight segments pass through unchanged.
//
// The algorithm is kurbo's: quadratic Beziers are subdivided with Raph
// Levien's parabola-integral method, which yields close to the minimal
// number of segments for a given tolerance, and cubics are first
// approximated by quadratics (using a tenth of the tolerance) and then
// subdivided fractionally across the whole cubic.
//
// Differences from kurbo, to make the tolerance a hard bound: 90% of the
// tolerance is used internally; cubics are split at their inflections and
// sharp turns, which are emitted as vertices; and degenerate (collinear)
// curves that fold back on themselves emit their turning point.
//
// The output is the same, bit for bit, on every architecture: the float64
// arithmetic rounds every product explicitly (no fused multiply-adds), also
// the products that functions return, which the compiler could otherwise
// fuse with an addition in the caller after inlining, and it uses only
// correctly rounded or ported deterministic functions.
//
// Flatten allocates a scratch buffer for cubics on every call; a Flattener
// reuses it.
func (p Path) Flatten(tolerance float32, emit func(v Verb, pt Point)) {
	var f Flattener
	f.Flatten(p, tolerance, emit)
}

// Flattener flattens paths like Path.Flatten while keeping its scratch
// buffer between calls, so that a long-lived Flattener does not allocate.
// The zero value is ready to use. A Flattener must not be used by several
// goroutines at once.
type Flattener struct {
	quad []quadSub // cubic -> quads buffer
}

// Flatten is Path.Flatten with the receiver's reusable buffer.
func (f *Flattener) Flatten(p Path, tolerance float32, emit func(v Verb, pt Point)) {
	tol := f64(tolerance)
	if !(tol >= minFlattenTolerance) { // also catches NaN
		tol = minFlattenTolerance
	}
	// The parabola integral is an approximation and can overshoot the
	// requested tolerance by about 5% (measured on random curves); a 10%
	// margin makes the documented bound hold, at the cost of about 5% more
	// segments than kurbo emits.
	tol *= flattenSafety
	sqrtTol := math.Sqrt(tol)
	var (
		cur [2]float64
		i   int
	)
	for _, v := range p.verbs {
		switch v {
		case MoveTo:
			pt := p.pts[i]
			i++
			cur = [2]float64{f64(pt.X), f64(pt.Y)}
			emit(MoveTo, pt)
		case LineTo:
			pt := p.pts[i]
			i++
			cur = [2]float64{f64(pt.X), f64(pt.Y)}
			emit(LineTo, pt)
		case Close:
			emit(Close, Point{})
		case QuadTo:
			c, e := p.pts[i], p.pts[i+1]
			i += 2
			q := [3][2]float64{cur, {f64(c.X), f64(c.Y)}, {f64(e.X), f64(e.Y)}}
			flattenQuad(q, sqrtTol, emit)
			emit(LineTo, e)
			cur = q[2]
		case CubicTo:
			c1, c2, e := p.pts[i], p.pts[i+1], p.pts[i+2]
			i += 3
			c := [4][2]float64{cur, {f64(c1.X), f64(c1.Y)}, {f64(c2.X), f64(c2.Y)}, {f64(e.X), f64(e.Y)}}
			f.quad = flattenCubic(c, tol, sqrtTol, f.quad[:0], emit)
			emit(LineTo, e)
			cur = c[3]
		}
	}
}

// quadSub is a quadratic piece of a cubic together with its subdivision
// parameters.
type quadSub struct {
	q [3][2]float64
	flattenParams
}

// flattenParams holds the result of estimating the subdivision of a
// quadratic. val is the number of subdivisions times 2*sqrt(tolerance).
type flattenParams struct {
	a0, a2, u0, uscale, val float64
	// fold is the parameter of the extreme point of a degenerate (collinear)
	// quadratic that folds back on itself, or -1.
	fold float64
}

func flattenQuad(q [3][2]float64, sqrtTol float64, emit func(Verb, Point)) {
	params := estimateSubdiv(q, sqrtTol)
	if params.fold > 0 {
		emit(LineTo, evalQuad2(q, params.fold))
	}
	n := segmentCount(0.5 * params.val / sqrtTol)
	step := 1 / float64(n)
	for i := 1; i < n; i++ {
		t := determineSubdivT(&params, float64(float64(i)*step))
		emit(LineTo, evalQuad2(q, t))
	}
}

// flattenCubic flattens one cubic. Unlike kurbo, which hands the whole cubic
// to the quadratic subdivision, it first splits the cubic at its inflection
// points and at points where it turns sharply (near-cusps), and emits those
// points as vertices. kurbo's output can stray farther than the tolerance
// from the curve right across a strong inflection (a measured example: 0.31
// for a tolerance of 0.25) or cut off the tip of a cusp; with the split every
// piece has monotonic curvature and no tip to cut.
func flattenCubic(c [4][2]float64, tol, sqrtTol float64, buf []quadSub, emit func(Verb, Point)) []quadSub {
	ts, n := cubicSplitParams(c, tol)
	if n == 0 {
		return flattenCubicPiece(c, tol, sqrtTol, buf[:0], emit)
	}
	ts[n] = 1
	prev := 0.0
	for _, t := range ts[:n+1] {
		piece := cubicSubsegment(c, prev, t)
		buf = flattenCubicPiece(piece, tol, sqrtTol, buf[:0], emit)
		if t < 1 {
			emit(LineTo, pointOf(evalCubic2(c, t)))
		}
		prev = t
	}
	return buf
}

func pointOf(p [2]float64) Point { return Point{float32(p[0]), float32(p[1])} }

// cross2d and dot2d are the 2D cross and dot products with every product
// rounded explicitly (no fused multiply-add).
func cross2d(p, q [2]float64) float64 { return float64(p[0]*q[1]) - float64(p[1]*q[0]) }
func dot2d(p, q [2]float64) float64   { return float64(p[0]*q[0]) + float64(p[1]*q[1]) }

// cubicInflections returns, in increasing order, the parameters strictly
// inside (0, 1) where the curvature of the cubic changes sign, and how many
// there are. Parameters closer than 1e-3 to an end are ignored: splitting
// there gains nothing.
func cubicInflections(c [4][2]float64) (out [2]float64, n int) {
	v0 := [2]float64{c[1][0] - c[0][0], c[1][1] - c[0][1]}
	v1 := [2]float64{c[2][0] - c[1][0], c[2][1] - c[1][1]}
	v2 := [2]float64{c[3][0] - c[2][0], c[3][1] - c[2][1]}
	a := [2]float64{v1[0] - v0[0], v1[1] - v0[1]}
	cc := [2]float64{v2[0] - 2*v1[0] + v0[0], v2[1] - 2*v1[1] + v0[1]}
	// cross(B', B'') = cross(a, cc) t^2 + cross(v0, cc) t + cross(v0, a).
	qa, qb, qc := cross2d(a, cc), cross2d(v0, cc), cross2d(v0, a)
	var roots [2]float64
	nr := 0
	scale := math.Abs(qa) + math.Abs(qb) + math.Abs(qc)
	switch {
	case scale == 0 || math.IsNaN(scale) || math.IsInf(scale, 0):
		return out, 0
	case math.Abs(qa) <= 1e-12*scale:
		if qb != 0 {
			roots[0] = -qc / qb
			nr = 1
		}
	default:
		disc := float64(qb*qb) - float64(4*qa*qc)
		if disc < 0 {
			return out, 0
		}
		sq := math.Sqrt(disc)
		q := -0.5 * (qb + math.Copysign(sq, qb))
		roots[0] = q / qa
		nr = 1
		if q != 0 {
			roots[1] = qc / q
			nr = 2
		}
	}
	for _, t := range roots[:nr] {
		if t > 1e-3 && t < 1-1e-3 {
			out[n] = t
			n++
		}
	}
	if n == 2 {
		if out[0] > out[1] {
			out[0], out[1] = out[1], out[0]
		}
		if out[1]-out[0] < 1e-3 {
			n = 1
		}
	}
	return out, n
}

// cubicSplitParams returns the sorted parameters at which flattenCubic splits
// a cubic, and how many there are: its inflections, plus the point of
// minimum speed when the radius of curvature there is under 8 tolerances (a
// cusp, or a loop so tight that its tip would otherwise be cut off by a
// chord). The array has room for one more entry after the last.
func cubicSplitParams(c [4][2]float64, tol float64) (ts [4]float64, n int) {
	infl, ni := cubicInflections(c)
	copy(ts[:], infl[:ni])
	n = ni
	if t, ok := sharpTurn(c, 8*tol); ok {
		ts[n] = t
		n++
	}
	if n > 1 {
		// Insertion sort of at most 3 values, then drop near-duplicates.
		for i := 1; i < n; i++ {
			for j := i; j > 0 && ts[j-1] > ts[j]; j-- {
				ts[j-1], ts[j] = ts[j], ts[j-1]
			}
		}
		m := 1
		for _, t := range ts[1:n] {
			if t-ts[m-1] > 1e-3 {
				ts[m] = t
				m++
			}
		}
		n = m
	}
	return ts, n
}

// sharpTurn looks for the parameter in (0, 1) where the speed |B'(t)| is
// smallest and reports it if the radius of curvature there is below
// maxRadius.
func sharpTurn(c [4][2]float64, maxRadius float64) (float64, bool) {
	// B'(t)/3 = v0 + 2t*a + t^2*cc; B''(t)/6 = a + t*cc.
	v0 := [2]float64{c[1][0] - c[0][0], c[1][1] - c[0][1]}
	v1 := [2]float64{c[2][0] - c[1][0], c[2][1] - c[1][1]}
	v2 := [2]float64{c[3][0] - c[2][0], c[3][1] - c[2][1]}
	a := [2]float64{v1[0] - v0[0], v1[1] - v0[1]}
	cc := [2]float64{v2[0] - 2*v1[0] + v0[0], v2[1] - 2*v1[1] + v0[1]}
	// d/dt |B'/3|^2 / 2 = f(t) = k0 + k1 t + k2 t^2 + k3 t^3.
	k0, k1, k2, k3 := dot2d(v0, a), dot2d(v0, cc)+2*dot2d(a, a), float64(3*dot2d(a, cc)), dot2d(cc, cc)
	f := func(t float64) float64 { return k0 + float64(t*(k1+float64(t*(k2+float64(t*k3))))) }
	if sum := k0 + k1 + k2 + k3; math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0, false
	}
	// Split (0, 1) at the critical points of f (roots of f', a quadratic) so
	// that f is monotonic on each part, then bisect for sign changes.
	var bounds [4]float64 // bounds[0] = 0
	nb := 1
	if q2, q1, q0 := float64(3*k3), 2*k2, k1; q2 != 0 {
		if disc := float64(q1*q1) - float64(4*q2*q0); disc >= 0 {
			sq := math.Sqrt(disc)
			r1, r2 := (-q1-sq)/(2*q2), (-q1+sq)/(2*q2)
			if r1 > r2 {
				r1, r2 = r2, r1
			}
			for _, r := range [2]float64{r1, r2} {
				if r > 0 && r < 1 {
					bounds[nb] = r
					nb++
				}
			}
		}
	}
	bounds[nb] = 1
	nb++
	velocity := func(t float64) [2]float64 {
		tt := float64(t * t)
		return [2]float64{
			v0[0] + float64(2*t*a[0]) + float64(tt*cc[0]),
			v0[1] + float64(2*t*a[1]) + float64(tt*cc[1]),
		}
	}
	best, bestG := -1.0, math.Inf(1)
	for i := 1; i < nb; i++ {
		lo, hi := bounds[i-1], bounds[i]
		if !(f(lo) < 0 && f(hi) > 0) { // a minimum of the speed: f goes from - to +
			continue
		}
		for it := 0; it < 50; it++ {
			mid := (lo + hi) / 2
			if f(mid) < 0 {
				lo = mid
			} else {
				hi = mid
			}
		}
		t := (lo + hi) / 2
		p := velocity(t)
		if g := dot2d(p, p); g < bestG {
			best, bestG = t, g
		}
	}
	if best <= 1e-3 || best >= 1-1e-3 {
		return 0, false
	}
	p := velocity(best)
	q := [2]float64{a[0] + float64(best*cc[0]), a[1] + float64(best*cc[1])}
	crossPQ := math.Abs(cross2d(p, q))
	speed := math.Sqrt(dot2d(p, p))
	// radius = |B'|^3 / |B' x B''| = 27|p|^3 / (18 |p x q|) = 1.5 |p|^3/|p x q|.
	if crossPQ == 0 || 1.5*speed*speed*speed/crossPQ < maxRadius {
		return best, true
	}
	return 0, false
}

// cubicSubsegment returns the part of the cubic between t0 and t1 as a cubic
// (kurbo's CubicBez::subsegment).
func cubicSubsegment(c [4][2]float64, t0, t1 float64) [4][2]float64 {
	p0 := evalCubic2(c, t0)
	p3 := evalCubic2(c, t1)
	d0 := cubicDeriv(c, t0)
	d1 := cubicDeriv(c, t1)
	s := (t1 - t0) * (1.0 / 3.0)
	return [4][2]float64{
		p0,
		{p0[0] + float64(s*d0[0]), p0[1] + float64(s*d0[1])},
		{p3[0] - float64(s*d1[0]), p3[1] - float64(s*d1[1])},
		p3,
	}
}

func flattenCubicPiece(c [4][2]float64, tol, sqrtTol float64, buf []quadSub, emit func(Verb, Point)) []quadSub {
	// Subdivide into quadratics, and estimate the number of subdivisions
	// required for each, summing to arrive at an estimate for the number of
	// subdivisions for the cubic. The parameters are kept for later.
	n := cubicToQuadCount(c, tol*toQuadTol)
	sqrtRemain := sqrtTol * math.Sqrt(1-toQuadTol)
	sum := 0.0
	for k := 0; k < n; k++ {
		q := cubicSubQuad(c, float64(k)/float64(n), float64(k+1)/float64(n))
		params := estimateSubdiv(q, sqrtRemain)
		if params.val == params.val { // not NaN
			sum += params.val
		} else {
			params.val = 0
		}
		buf = append(buf, quadSub{q, params})
	}
	m := segmentCount(0.5 * sum / sqrtRemain)

	// Walk the quadratics, emitting the points of subdivision that fall
	// within each one.
	step := sum / float64(m)
	i := 1
	valSum := 0.0
	for k := range buf {
		s := &buf[k]
		if s.fold > 0 {
			emit(LineTo, evalQuad2(s.q, s.fold))
		}
		if !(s.val > 0) {
			continue
		}
		target := float64(float64(i) * step)
		recip := 1 / s.val
		for target < valSum+s.val {
			u := (target - valSum) * recip
			t := determineSubdivT(&s.flattenParams, u)
			emit(LineTo, evalQuad2(s.q, t))
			i++
			if i == m+1 {
				break
			}
			target = float64(float64(i) * step)
		}
		valSum += s.val
	}
	return buf
}

// segmentCount rounds a real segment estimate up to an int in
// [1, maxFlattenSegments]; NaN gives 1.
func segmentCount(v float64) int {
	nf := math.Ceil(v)
	if !(nf >= 1) {
		return 1
	}
	if nf > maxFlattenSegments {
		return maxFlattenSegments
	}
	return int(nf)
}

// cubicToQuadCount returns how many equal-parameter quadratics are needed to
// approximate the cubic within accuracy (kurbo's CubicBez::to_quads).
func cubicToQuadCount(c [4][2]float64, accuracy float64) int {
	// The maximum error is proportional to the third derivative, which is
	// constant across the segment, so it scales down as the third power of the
	// number of subdivisions. 432 is the square of 36/sqrt(3); see
	// http://caffeineowl.com/graphics/2d/vectorial/cubic2quad01.html
	maxHypot2 := 432 * accuracy * accuracy
	p1x2x, p1x2y := float64(3*c[1][0])-c[0][0], float64(3*c[1][1])-c[0][1]
	p2x2x, p2x2y := float64(3*c[2][0])-c[3][0], float64(3*c[2][1])-c[3][1]
	dx, dy := p2x2x-p1x2x, p2x2y-p1x2y
	err := float64(dx*dx) + float64(dy*dy)
	// The sixth root, as a cube root of a square root, both deterministic
	// (math.Pow is not the same on every architecture).
	nf := math.Ceil(fmath.Cbrt(math.Sqrt(err / maxHypot2)))
	if !(nf >= 1) { // also NaN
		return 1
	}
	if nf > 512 {
		return 512
	}
	return int(nf)
}

// cubicSubQuad returns the quadratic approximating the cubic's subsegment
// [t0, t1] (kurbo's ToQuads iterator).
func cubicSubQuad(c [4][2]float64, t0, t1 float64) [3][2]float64 {
	p0 := evalCubic2(c, t0)
	p3 := evalCubic2(c, t1)
	d0 := cubicDeriv(c, t0)
	d1 := cubicDeriv(c, t1)
	s := (t1 - t0) * (1.0 / 3.0)
	p1 := [2]float64{p0[0] + float64(s*d0[0]), p0[1] + float64(s*d0[1])}
	p2 := [2]float64{p3[0] - float64(s*d1[0]), p3[1] - float64(s*d1[1])}
	var q [3][2]float64
	q[0] = p0
	for k := 0; k < 2; k++ {
		p1x2 := float64(3*p1[k]) - p0[k]
		p2x2 := float64(3*p2[k]) - p3[k]
		q[1][k] = (p1x2 + p2x2) / 4
	}
	q[2] = p3
	return q
}

func cubicDeriv(c [4][2]float64, t float64) [2]float64 {
	mt := 1 - t
	a, b, e := float64(mt*mt), float64(2*mt*t), float64(t*t)
	var d [2]float64
	for k := 0; k < 2; k++ {
		d[k] = 3 * (float64(a*(c[1][k]-c[0][k])) + float64(b*(c[2][k]-c[1][k])) + float64(e*(c[3][k]-c[2][k])))
	}
	return d
}

func evalCubic2(c [4][2]float64, t float64) [2]float64 {
	mt := 1 - t
	a, b, cc, d := mt*mt*mt, 3*mt*mt*t, 3*mt*t*t, t*t*t
	return [2]float64{
		float64(a*c[0][0]) + float64(b*c[1][0]) + float64(cc*c[2][0]) + float64(d*c[3][0]),
		float64(a*c[0][1]) + float64(b*c[1][1]) + float64(cc*c[2][1]) + float64(d*c[3][1]),
	}
}

func evalQuad2(q [3][2]float64, t float64) Point {
	mt := 1 - t
	a, b, c := float64(mt*mt), float64(2*mt*t), float64(t*t)
	return Point{
		float32(float64(a*q[0][0]) + float64(b*q[1][0]) + float64(c*q[2][0])),
		float32(float64(a*q[0][1]) + float64(b*q[1][1]) + float64(c*q[2][1])),
	}
}

// estimateSubdiv estimates the number of subdivisions of a quadratic for
// flattening (kurbo's QuadBez::estimate_subdiv).
func estimateSubdiv(q [3][2]float64, sqrtTol float64) flattenParams {
	// Determine the transformation to the y = x^2 parabola.
	d01 := [2]float64{q[1][0] - q[0][0], q[1][1] - q[0][1]}
	d12 := [2]float64{q[2][0] - q[1][0], q[2][1] - q[1][1]}
	dd := [2]float64{d01[0] - d12[0], d01[1] - d12[1]}
	p02 := [2]float64{q[2][0] - q[0][0], q[2][1] - q[0][1]}
	cross := cross2d(p02, dd)
	x0 := dot2d(d01, dd) / cross
	x2 := dot2d(d12, dd) / cross
	// sqrt(dd·dd) rather than math.Hypot, whose result depends on the
	// architecture (assembly on amd64, fusable Go code elsewhere).
	scale := math.Abs(cross / (math.Sqrt(dot2d(dd, dd)) * (x2 - x0)))

	params := flattenParams{fold: -1}
	if math.IsInf(scale, 0) || math.IsNaN(scale) || cross == 0 {
		// Collinear control points: the curve is a line traced back and forth.
		// kurbo emits no intermediate points here; we also emit the turning
		// point so that a fold-back is not cut off.
		dd2 := dot2d(dd, dd)
		if dd2 > 0 {
			t := dot2d(d01, dd) / dd2
			if t > 0 && t < 1 {
				params.fold = t
			}
		}
		params.uscale = 1
		return params
	}

	// Compute the number of subdivisions needed.
	a0 := approxParabolaIntegral(x0)
	a2 := approxParabolaIntegral(x2)
	da := math.Abs(a2 - a0)
	sqrtScale := math.Sqrt(scale)
	var val float64
	if math.Signbit(x0) == math.Signbit(x2) {
		val = float64(da * sqrtScale)
	} else {
		// Handle the cusp case (the segment contains the curvature maximum).
		xmin := sqrtTol / sqrtScale
		val = sqrtTol * da / approxParabolaIntegral(xmin)
	}
	u0 := approxParabolaInvIntegral(a0)
	u2 := approxParabolaInvIntegral(a2)
	params.a0, params.a2, params.u0 = a0, a2, u0
	params.uscale = 1 / (u2 - u0)
	params.val = val
	return params
}

// determineSubdivT maps x in [0, 1] (uniform in the flattening measure) to
// the curve parameter t in [0, 1].
func determineSubdivT(p *flattenParams, x float64) float64 {
	a := p.a0 + float64((p.a2-p.a0)*x)
	u := approxParabolaInvIntegral(a)
	return float64((u - p.u0) * p.uscale)
}

// approxParabolaIntegral approximates the integral of (1 + 4x^2)^-0.25 dx.
func approxParabolaIntegral(x float64) float64 {
	const d = 0.67
	return x / (1 - d + math.Sqrt(math.Sqrt(d*d*d*d+float64(0.25*x*x))))
}

// approxParabolaInvIntegral approximates the inverse of approxParabolaIntegral.
func approxParabolaInvIntegral(x float64) float64 {
	const b = 0.39
	return float64(x * (1 - b + math.Sqrt(b*b+float64(0.25*x*x))))
}
