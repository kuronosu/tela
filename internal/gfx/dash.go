// Copyright 2023 the Kurbo Authors
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Dashing ported from kurbo's kurbo/src/stroke.rs (dash, DashIterator). The
// arc length computations replace kurbo's ParamCurveArclen with adaptive
// Gauss-Legendre quadrature. See THIRD_PARTY.md.

package gfx

import (
	"math"
	"sort"
)

// maxStrokeDashes bounds the number of dashes of one Stroke call. A pattern
// that would produce more (a tiny period on a long path) is ignored and the
// path is stroked solid, as Skia does past its own limit.
const maxStrokeDashes = 1 << 16

// Eight-point Gauss-Legendre rule on [-1, 1]: the positive nodes and their
// weights (the rule is symmetric). It integrates polynomials of degree 15
// exactly.
var (
	glNodes   = [4]float64{0.1834346424956498049394761, 0.5255324099163289858177390, 0.7966664774136267395915539, 0.9602898564975362316835609}
	glWeights = [4]float64{0.3626837833783619829651504, 0.3137066458778872873379622, 0.2223810344533744705443560, 0.1012285362903762591525314}
)

// glArcLen approximates the arc length of s between parameters a and b.
func (s *cseg) glArcLen(a, b float64) float64 {
	half := (b - a) / 2
	mid := a + half
	sum := 0.0
	for i := range 4 {
		dx := float64(half * glNodes[i])
		sum += float64(glWeights[i] * (s.deriv(mid-dx).norm() + s.deriv(mid+dx).norm()))
	}
	return float64(sum * half)
}

// segLength is the arc length of a segment, with the breakpoints and
// cumulative lengths of its adaptive quadrature for cubics.
type segLength struct {
	l       float64
	ts, cum []float64
}

// measure computes the arc length of s to within about acc.
func (s *cseg) measure(acc float64) segLength {
	if !s.cubic {
		return segLength{l: s.p[1].sub(s.p[0]).norm()}
	}
	poly := s.p[1].sub(s.p[0]).norm() + s.p[2].sub(s.p[1]).norm() + s.p[3].sub(s.p[2]).norm()
	acc = max(acc, 1e-12*poly)
	sl := segLength{ts: []float64{0}, cum: []float64{0}}
	var rec func(a, b, whole float64, depth int)
	rec = func(a, b, whole float64, depth int) {
		m := (a + b) / 2
		l1, l2 := s.glArcLen(a, m), s.glArcLen(m, b)
		if depth >= 12 || math.Abs(l1+l2-whole) <= acc*(b-a) {
			c := sl.cum[len(sl.cum)-1]
			sl.ts = append(sl.ts, m, b)
			sl.cum = append(sl.cum, c+l1, c+l1+l2)
			return
		}
		rec(a, m, l1, depth+1)
		rec(m, b, l2, depth+1)
	}
	rec(0, 1, s.glArcLen(0, 1), 0)
	sl.l = sl.cum[len(sl.cum)-1]
	return sl
}

// invArcLen returns the parameter at arc length d from the start of s.
func (s *cseg) invArcLen(sl *segLength, d, acc float64) float64 {
	if !(d > 0) || !(sl.l > 0) {
		return 0
	}
	if d >= sl.l {
		return 1
	}
	if !s.cubic {
		return d / sl.l
	}
	// The quadrature interval holding d, then Newton's method safeguarded by
	// bisection inside it.
	i := sort.SearchFloat64s(sl.cum, d) - 1
	i = max(0, min(i, len(sl.ts)-2))
	t0, lo, hi := sl.ts[i], sl.ts[i], sl.ts[i+1]
	target := d - sl.cum[i]
	span := sl.cum[i+1] - sl.cum[i]
	t := lo
	if span > 0 {
		t = lo + float64((hi-lo)*(target/span))
	}
	for range 40 {
		f := s.glArcLen(t0, t) - target
		if math.Abs(f) <= acc {
			break
		}
		if f > 0 {
			hi = t
		} else {
			lo = t
		}
		next := (lo + hi) / 2
		if v := s.deriv(t).norm(); v > 0 {
			if n := t - f/v; n > lo && n < hi {
				next = n
			}
		}
		if next == t {
			break
		}
		t = next
	}
	return t
}

// dashSubpaths applies the dash pattern (even length, positive total, as
// normalized by sanitizeStroke) to subs, with the semantics of kurbo's dash:
//
//   - the offset is reduced modulo the pattern period (a negative offset
//     wraps), and every subpath restarts the pattern;
//   - each dash is a subpath of its own (open, stroked with caps), except
//     that a closed subpath covered by a single dash stays closed, and on a
//     closed subpath a dash that runs through the end is joined with the
//     dash that starts at the subpath's start (kurbo's stash);
//   - a dash of zero length is a dot aligned with the path direction.
//
// acc is the arc length accuracy. ok is false when the pattern would produce
// more than maxStrokeDashes dashes; the caller then strokes solid.
func dashSubpaths(subs []subpath, pattern []float32, offset float32, tol float64) (out []subpath, ok bool) {
	n := len(pattern)
	dashes := make([]float64, n)
	period := 0.0
	for i, d := range pattern {
		dashes[i] = f64(d)
		period += dashes[i]
	}
	if n == 0 || n%2 != 0 || !(period > 0) || math.IsInf(period, 0) {
		return nil, false
	}
	acc := 1e-3 * tol
	lens := make([][]segLength, len(subs))
	total := 0.0
	for i := range subs {
		lens[i] = make([]segLength, len(subs[i].segs))
		for k := range subs[i].segs {
			lens[i][k] = subs[i].segs[k].measure(acc)
			total += lens[i][k].l
		}
	}
	// Each period holds n/2 dashes; one more may start in each subpath.
	if total/period*float64(n/2) > maxStrokeDashes {
		return nil, false
	}

	// Find the place in the pattern for the initial offset.
	off := math.Mod(f64(offset), period)
	if off < 0 {
		off += period
	}
	ix0, rem0, on0 := 0, dashes[0]-off, true
	for rem0 < 0 {
		ix0 = (ix0 + 1) % n
		rem0 += dashes[ix0]
		on0 = !on0
	}

	begin := func(at, dir v2) int {
		out = append(out, subpath{start: at, draws: true, dotDir: dir})
		return len(out) - 1
	}
	for si := range subs {
		sp := &subs[si]
		ix, rem, on := ix0, rem0, on0
		cur, first := -1, -1
		broken := false
		if on {
			cur = begin(sp.start, firstDirection(sp))
			first = cur
		}
		for k := range sp.segs {
			s, sl := &sp.segs[k], &lens[si][k]
			pos, tcur := 0.0, 0.0
			for {
				segRem := sl.l - pos
				if !(rem < segRem) {
					if on {
						out[cur].segs = append(out[cur].segs, s.subseg(tcur, 1))
					}
					rem -= segRem
					break
				}
				// The next dash transition falls inside this segment.
				t1 := s.invArcLen(sl, pos+rem, acc)
				t1 = max(t1, tcur)
				if on {
					out[cur].segs = append(out[cur].segs, s.subseg(tcur, t1))
					cur = -1
				} else {
					cur = begin(s.eval(t1), s.direction(t1))
				}
				if len(out) > maxStrokeDashes+len(subs) {
					return nil, false
				}
				broken = true
				pos += rem
				tcur = t1
				on = !on
				ix = (ix + 1) % n
				rem = dashes[ix]
			}
		}
		if !sp.closed {
			continue
		}
		switch {
		case !broken:
			// One dash covers the whole closed subpath (or none does).
			if on && cur >= 0 {
				out[cur].closed = true
			}
		case on && first >= 0 && cur >= 0 && cur != first:
			// The last dash runs through the start point: continue it with
			// the first dash.
			out[cur].segs = append(out[cur].segs, out[first].segs...)
			out[first] = subpath{}
		}
	}
	return out, true
}

// firstDirection returns the start direction of the first non-degenerate
// segment of sp, or the zero vector.
func firstDirection(sp *subpath) v2 {
	for k := range sp.segs {
		if !sp.segs[k].degenerate() {
			t, _ := sp.segs[k].tangents()
			return t
		}
	}
	return v2{}
}
