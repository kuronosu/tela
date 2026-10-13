package gfx

import (
	"math"
	"math/rand"
	"testing"
)

// --- An independent oracle for the exact stroke region (SVG 2) ---

// oracleSeg is a drawing segment of the original path in float64.
type oracleSeg struct {
	pts []([2]float64) // 2 points (line) or 4 (cubic; quads are raised)
}

func (s oracleSeg) eval(t float64) [2]float64 {
	p := s.pts
	if len(p) == 2 {
		return [2]float64{p[0][0] + t*(p[1][0]-p[0][0]), p[0][1] + t*(p[1][1]-p[0][1])}
	}
	return evalCubic2([4][2]float64{p[0], p[1], p[2], p[3]}, t)
}

func (s oracleSeg) deriv(t float64) [2]float64 {
	p := s.pts
	if len(p) == 2 {
		return [2]float64{p[1][0] - p[0][0], p[1][1] - p[0][1]}
	}
	return cubicDeriv([4][2]float64{p[0], p[1], p[2], p[3]}, t)
}

// tangents follows the rule of SVG/kurbo: the first control point distinct
// from the end point gives the direction.
func (s oracleSeg) tangents() (t0, t1 [2]float64) {
	p := s.pts
	n := len(p)
	unit := func(a, b [2]float64) ([2]float64, bool) {
		d := [2]float64{b[0] - a[0], b[1] - a[1]}
		l := math.Hypot(d[0], d[1])
		if l == 0 {
			return d, false
		}
		return [2]float64{d[0] / l, d[1] / l}, true
	}
	for i := 1; i < n; i++ {
		if u, ok := unit(p[0], p[i]); ok {
			t0 = u
			break
		}
	}
	for i := n - 2; i >= 0; i-- {
		if u, ok := unit(p[i], p[n-1]); ok {
			t1 = u
			break
		}
	}
	return
}

func (s oracleSeg) zero() bool {
	for _, q := range s.pts[1:] {
		if q != s.pts[0] {
			return false
		}
	}
	return true
}

type oracleSub struct {
	segs   []oracleSeg
	start  [2]float64
	closed bool
	draws  bool
}

func oracleSubpaths(p Path) []oracleSub {
	var subs []oracleSub
	f := func(pt Point) [2]float64 { return [2]float64{float64(pt.X), float64(pt.Y)} }
	for s := range p.Segments() {
		if s.Verb == MoveTo {
			subs = append(subs, oracleSub{start: f(s.P[0])})
			continue
		}
		sp := &subs[len(subs)-1]
		sp.draws = true
		switch s.Verb {
		case LineTo:
			sp.segs = append(sp.segs, oracleSeg{[][2]float64{f(s.P[0]), f(s.P[1])}})
		case QuadTo:
			p0, p1, p2 := f(s.P[0]), f(s.P[1]), f(s.P[2])
			c1 := [2]float64{p0[0] + 2.0/3*(p1[0]-p0[0]), p0[1] + 2.0/3*(p1[1]-p0[1])}
			c2 := [2]float64{p2[0] + 2.0/3*(p1[0]-p2[0]), p2[1] + 2.0/3*(p1[1]-p2[1])}
			sp.segs = append(sp.segs, oracleSeg{[][2]float64{p0, c1, c2, p2}})
		case CubicTo:
			sp.segs = append(sp.segs, oracleSeg{[][2]float64{f(s.P[0]), f(s.P[1]), f(s.P[2]), f(s.P[3])}})
		case Close:
			if s.P[0] != s.P[1] {
				sp.segs = append(sp.segs, oracleSeg{[][2]float64{f(s.P[0]), f(s.P[1])}})
			}
			sp.closed = true
		}
	}
	return subs
}

// inSweep reports whether q is on a normal segment of half-length h of s.
func inSweep(s oracleSeg, q [2]float64, h float64) bool {
	// The sweep lies within the control box grown by h.
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range s.pts {
		x0, y0, x1, y1 = math.Min(x0, p[0]), math.Min(y0, p[1]), math.Max(x1, p[0]), math.Max(y1, p[1])
	}
	if q[0] < x0-h || q[0] > x1+h || q[1] < y0-h || q[1] > y1+h {
		return false
	}
	if len(s.pts) == 2 {
		a, b := s.pts[0], s.pts[1]
		d := [2]float64{b[0] - a[0], b[1] - a[1]}
		l2 := d[0]*d[0] + d[1]*d[1]
		t := ((q[0]-a[0])*d[0] + (q[1]-a[1])*d[1]) / l2
		if t < 0 || t > 1 {
			return false
		}
		return math.Abs((q[0]-a[0])*d[1]-(q[1]-a[1])*d[0])/math.Sqrt(l2) <= h
	}
	t0, t1 := s.tangents()
	f := func(t float64) float64 {
		c, d := s.eval(t), s.deriv(t)
		if l := math.Hypot(d[0], d[1]); l > 1e-9 {
			d = [2]float64{d[0] / l, d[1] / l}
		} else if t < 0.5 {
			d = t0 // the limit direction where the derivative vanishes
		} else {
			d = t1
		}
		return (q[0]-c[0])*d[0] + (q[1]-c[1])*d[1]
	}
	dist := func(t float64) float64 {
		c := s.eval(t)
		return math.Hypot(q[0]-c[0], q[1]-c[1])
	}
	const n = 256
	prevT, prevF := 0.0, f(0)
	if prevF == 0 && dist(0) <= h {
		return true
	}
	for i := 1; i <= n; i++ {
		t := float64(i) / n
		ft := f(t)
		if ft == 0 && dist(t) <= h {
			return true
		}
		if (prevF < 0) != (ft < 0) {
			lo, hi, flo := prevT, t, prevF
			for range 50 {
				mid := (lo + hi) / 2
				if fm := f(mid); (fm < 0) == (flo < 0) {
					lo, flo = mid, fm
				} else {
					hi = mid
				}
			}
			if dist((lo+hi)/2) <= h {
				return true
			}
		}
		prevT, prevF = t, ft
	}
	return false
}

func inConvexPolygon(q [2]float64, poly [][2]float64) bool {
	sign := 0.0
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		c := (b[0]-a[0])*(q[1]-a[1]) - (b[1]-a[1])*(q[0]-a[0])
		if c != 0 {
			if sign == 0 {
				sign = c
			} else if (c > 0) != (sign > 0) {
				return false
			}
		}
	}
	return true
}

// inJoin reports whether q is in the join shape at p between the unit
// tangents a (incoming) and b (outgoing).
func inJoin(q, p, a, b [2]float64, h float64, style StrokeStyle) bool {
	cr := a[0]*b[1] - a[1]*b[0]
	dt := a[0]*b[0] + a[1]*b[1]
	if style.Join == JoinRound {
		return math.Hypot(q[0]-p[0], q[1]-p[1]) <= h
	}
	if cr == 0 && dt > 0 {
		return false
	}
	// Outer offsets: the side the path turns away from.
	s := 1.0
	if cr > 0 {
		s = -1
	}
	na := [2]float64{-a[1] * h * s, a[0] * h * s}
	nb := [2]float64{-b[1] * h * s, b[0] * h * s}
	oa := [2]float64{p[0] + na[0], p[1] + na[1]}
	ob := [2]float64{p[0] + nb[0], p[1] + nb[1]}
	ml := float64(style.MiterLimit)
	if style.Join == JoinMiter && 2 <= (1+dt)*ml*ml {
		k := 1 / (1 + dt)
		tip := [2]float64{p[0] + (na[0]+nb[0])*k, p[1] + (na[1]+nb[1])*k}
		return inConvexPolygon(q, [][2]float64{p, oa, tip, ob})
	}
	return inConvexPolygon(q, [][2]float64{p, oa, ob})
}

func inCap(q, p, t [2]float64, h float64, c LineCap) bool {
	switch c {
	case CapRound:
		return math.Hypot(q[0]-p[0], q[1]-p[1]) <= h
	case CapSquare:
		d := [2]float64{q[0] - p[0], q[1] - p[1]}
		along := d[0]*t[0] + d[1]*t[1]
		across := d[0]*-t[1] + d[1]*t[0]
		return along >= 0 && along <= h && math.Abs(across) <= h
	}
	return false
}

// exactStrokeContains reports whether q lies in the exact stroke of p
// (no dashes; Width, Join, Cap and MiterLimit of a sanitized style).
func exactStrokeContains(subs []oracleSub, style StrokeStyle, q [2]float64) bool {
	h := float64(style.Width) / 2
	for _, sp := range subs {
		var segs []oracleSeg
		for _, s := range sp.segs {
			if !s.zero() {
				segs = append(segs, s)
			}
		}
		if len(segs) == 0 {
			if !sp.draws {
				continue
			}
			c := sp.start
			switch style.Cap {
			case CapRound:
				if math.Hypot(q[0]-c[0], q[1]-c[1]) <= h {
					return true
				}
			case CapSquare:
				if math.Abs(q[0]-c[0]) <= h && math.Abs(q[1]-c[1]) <= h {
					return true
				}
			}
			continue
		}
		for _, s := range segs {
			if inSweep(s, q, h) {
				return true
			}
		}
		for k := range segs {
			if k == 0 && !sp.closed {
				continue
			}
			prev := segs[(k+len(segs)-1)%len(segs)]
			_, a := prev.tangents()
			b, _ := segs[k].tangents()
			if inJoin(q, segs[k].pts[0], a, b, h, style) {
				return true
			}
		}
		if !sp.closed {
			t0, _ := segs[0].tangents()
			_, t1 := segs[len(segs)-1].tangents()
			last := segs[len(segs)-1].pts
			if inCap(q, segs[0].pts[0], [2]float64{-t0[0], -t0[1]}, h, style.Cap) ||
				inCap(q, last[len(last)-1], t1, h, style.Cap) {
				return true
			}
		}
	}
	return false
}

// checkAgainstOracle samples points around the stroke of p and checks that
// the NonZero fill of the outline agrees with the exact stroke wherever the
// exact stroke does not change within band of the point (tested on a ring
// of points at distance band).
func checkAgainstOracle(t *testing.T, name string, p Path, style StrokeStyle, tol float32, n int, rng *rand.Rand) {
	t.Helper()
	out := p.Stroke(style, tol)
	style, _ = sanitizeStroke(style)
	loops := outlineLoops(out)
	subs := oracleSubpaths(p)
	h := float64(style.Width) / 2
	k := max(1.5, float64(style.MiterLimit))
	bb := p.ControlBounds().Outset(float32(h * k))
	band := float64(tol) * 1.01
	bad := 0
	if testing.Short() {
		n /= 4
	}
	for range n {
		q := [2]float64{
			float64(bb.X0) + rng.Float64()*float64(bb.Width()),
			float64(bb.Y0) + rng.Float64()*float64(bb.Height()),
		}
		in := exactStrokeContains(subs, style, q)
		robust := true
		for j := range 8 {
			a := float64(j) * math.Pi / 4
			r := [2]float64{q[0] + band*math.Cos(a), q[1] + band*math.Sin(a)}
			if exactStrokeContains(subs, style, r) != in {
				robust = false
				break
			}
		}
		if !robust {
			continue
		}
		w := winding(loops, q[0], q[1])
		if w < 0 {
			t.Errorf("%s: negative winding %d at %v", name, w, q)
			bad++
		} else if (w != 0) != in {
			t.Errorf("%s: point %v: stroke fill %v, exact stroke %v", name, q, w != 0, in)
			bad++
		}
		if bad > 10 {
			t.Fatalf("%s: too many failures", name)
		}
	}
}

// --- Test paths ---

func cubicPath(p0, p1, p2, p3 Point) Path {
	var p Path
	p.MoveTo(p0)
	p.CubicTo(p1, p2, p3)
	return p
}

func polyPath(closed bool, pts ...Point) Path {
	var p Path
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	if closed {
		p.Close()
	}
	return p
}

type namedPath struct {
	name string
	p    Path
}

func oraclePaths() []namedPath {
	var quad Path
	quad.MoveTo(Pt(0, 0))
	quad.QuadTo(Pt(50, 100), Pt(100, 0))
	var tightQuad Path
	tightQuad.MoveTo(Pt(0, 0))
	tightQuad.QuadTo(Pt(40, 80), Pt(80, 0))
	var mixed Path
	mixed.MoveTo(Pt(0, 0))
	mixed.LineTo(Pt(40, 0))
	mixed.CubicTo(Pt(100, 0), Pt(100, 100), Pt(40, 100))
	mixed.LineTo(Pt(0, 100))
	mixed.Close()
	var curveCorners Path
	curveCorners.MoveTo(Pt(0, 50))
	curveCorners.QuadTo(Pt(50, -20), Pt(100, 50))
	curveCorners.CubicTo(Pt(60, 60), Pt(40, 90), Pt(50, 120))
	curveCorners.LineTo(Pt(0, 50))
	var twoSubs Path
	twoSubs.MoveTo(Pt(0, 0))
	twoSubs.LineTo(Pt(100, 40))
	twoSubs.MoveTo(Pt(0, 40))
	twoSubs.CubicTo(Pt(30, -20), Pt(70, 100), Pt(100, 0))
	var coincident Path
	coincident.MoveTo(Pt(0, 0))
	coincident.CubicTo(Pt(0, 0), Pt(100, 0), Pt(100, 60))
	coincident.CubicTo(Pt(100, 60), Pt(100, 60), Pt(0, 80))
	return []namedPath{
		{"line", polyPath(false, Pt(0, 0), Pt(100, 30))},
		{"zigzag", polyPath(false, Pt(0, 60), Pt(40, 0), Pt(80, 60), Pt(120, 20), Pt(125, 60))},
		{"spike", polyPath(false, Pt(0, 0), Pt(100, 10), Pt(0, 20))},
		{"reversal", polyPath(false, Pt(0, 0), Pt(100, 0), Pt(30, 0))},
		{"triangle", polyPath(true, Pt(0, 100), Pt(50, 0), Pt(100, 100))},
		{"thin triangle", polyPath(true, Pt(0, 0), Pt(100, 10), Pt(0, 20))},
		{"rect", RectPath(RectXYWH(0, 0, 100, 60))},
		{"quad", quad},
		{"tight quad", tightQuad},
		{"S", cubicPath(Pt(0, 0), Pt(100, 0), Pt(-20, 100), Pt(100, 100))},
		{"loop", cubicPath(Pt(0, 0), Pt(150, 100), Pt(-50, 100), Pt(100, 0))},
		{"cusp", cubicPath(Pt(0, 0), Pt(100, 100), Pt(0, 100), Pt(100, 0))},
		{"near cusp", cubicPath(Pt(0, -0.01), Pt(128, 128.001), Pt(128, -0.01), Pt(0, 128.001))},
		{"inflection", cubicPath(Pt(0, 0), Pt(60, -40), Pt(40, 140), Pt(100, 100))},
		{"flat 180", cubicPath(Pt(0, 0), Pt(0, -10), Pt(0, -10), Pt(0, 10))},
		{"circle", CirclePath(Pt(50, 50), 45)},
		{"small circle", CirclePath(Pt(10, 10), 4)},
		{"rounded rect", NewRRect(RectXYWH(0, 0, 120, 80), 20).Path()},
		{"D shape", mixed},
		{"curve corners", curveCorners},
		{"two subpaths", twoSubs},
		{"coincident controls", coincident},
	}
}

func TestStrokeOracleRound(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, np := range oraclePaths() {
		for _, w := range []float32{2, 13, 60} {
			st := StrokeStyle{Width: w, Join: JoinRound, Cap: CapRound}
			checkAgainstOracle(t, np.name, np.p, st, 0.1, 700, rng)
		}
	}
}

func TestStrokeOracleJoinsAndCaps(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	styles := []StrokeStyle{
		{Width: 10, Join: JoinMiter, Cap: CapButt, MiterLimit: 4},
		{Width: 24, Join: JoinMiter, Cap: CapSquare, MiterLimit: 10},
		{Width: 16, Join: JoinBevel, Cap: CapButt},
		{Width: 40, Join: JoinBevel, Cap: CapSquare},
		{Width: 7, Join: JoinMiter, Cap: CapRound, MiterLimit: 1.5},
		{Width: 18, Join: JoinRound, Cap: CapButt},
	}
	for _, np := range oraclePaths() {
		for _, st := range styles {
			checkAgainstOracle(t, np.name+"/"+st.Join.String()+"/"+st.Cap.String(), np.p, st, 0.1, 600, rng)
		}
	}
}

func TestStrokeOracleRandomCubics(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	pt := func() Point { return Pt(float32(rng.Float64()*200), float32(rng.Float64()*200)) }
	for i := range 40 {
		var p Path
		p.MoveTo(pt())
		for range 1 + rng.Intn(3) {
			if rng.Intn(3) == 0 {
				p.LineTo(pt())
			} else {
				p.CubicTo(pt(), pt(), pt())
			}
		}
		if rng.Intn(2) == 0 {
			p.Close()
		}
		st := StrokeStyle{
			Width:      float32(1 + rng.Float64()*50),
			Join:       LineJoin(rng.Intn(3)),
			Cap:        LineCap(rng.Intn(3)),
			MiterLimit: float32(1 + rng.Float64()*6),
		}
		tol := float32([]float64{0.05, 0.25, 1}[rng.Intn(3)])
		checkAgainstOracle(t, "random "+string(rune('A'+i)), p, st, tol, 400, rng)
	}
}

// --- Exact areas ---

func TestStrokeExactAreas(t *testing.T) {
	const tol = 0.01
	seg := polyPath(false, Pt(10, 10), Pt(110, 10))
	rect := RectPath(RectXYWH(0, 0, 100, 50))
	cases := []struct {
		name  string
		p     Path
		style StrokeStyle
		area  float64
		round bool // the shape has round parts, polygonized within tol
	}{
		{"segment butt", seg, StrokeStyle{Width: 10, Cap: CapButt}, 1000, false},
		{"segment square", seg, StrokeStyle{Width: 10, Cap: CapSquare}, 1100, false},
		{"segment round", seg, StrokeStyle{Width: 10, Cap: CapRound}, 1000 + 25*math.Pi, true},
		{"rect miter", rect, StrokeStyle{Width: 10, Join: JoinMiter, MiterLimit: 4}, 3000, false},
		{"rect bevel", rect, StrokeStyle{Width: 10, Join: JoinBevel}, 2950, false},
		{"rect round", rect, StrokeStyle{Width: 10, Join: JoinRound}, 2900 + 25*math.Pi, true},
		{"rect miter limit", rect, StrokeStyle{Width: 10, Join: JoinMiter, MiterLimit: 1.4}, 2950, false},
		{"reversed rect miter", polyPath(true, Pt(0, 0), Pt(0, 50), Pt(100, 50), Pt(100, 0)),
			StrokeStyle{Width: 10, Join: JoinMiter, MiterLimit: 4}, 3000, false},
	}
	for _, c := range cases {
		out := c.p.Stroke(c.style, tol)
		got := signedArea(outlineLoops(out))
		// Inscribed polygons lose at most tol times the perimeter of the round parts.
		lo, hi := c.area-1e-6*c.area, c.area+1e-6*c.area
		if c.round {
			lo = c.area - tol*2*math.Pi*5
		}
		if got < lo || got > hi {
			t.Errorf("%s: area %.6f, want %.6f (range [%.6f, %.6f])", c.name, got, c.area, lo, hi)
		}
	}
}

// --- Specific behaviors ---

func TestStrokeMiterLimit(t *testing.T) {
	// Two lines meeting at the origin with interior angle phi: the miter
	// length over the width is 1/sin(phi/2).
	const h = 5
	for _, tc := range []struct {
		phiDeg float64
		limit  float32
		miter  bool
	}{
		{61, 2, true},  // ratio 1.98
		{59, 2, false}, // ratio 2.03
		{90, 1.42, true},
		{90, 1.41, false},
		{170, 1, false}, // limit 1: never a miter (except straight lines)
	} {
		phi := tc.phiDeg * math.Pi / 180
		a := Pt(float32(100*math.Cos(phi/2)), float32(100*math.Sin(phi/2)))
		b := Pt(a.X, -a.Y)
		p := polyPath(false, a, Pt(0, 0), b)
		st := StrokeStyle{Width: 2 * h, Join: JoinMiter, MiterLimit: tc.limit}
		out := p.Stroke(st, 0.01)
		// The tip is the point of the outline farthest along -x.
		minX := math.Inf(1)
		for _, q := range out.Points() {
			minX = math.Min(minX, float64(q.X))
		}
		miterX := -h / math.Sin(phi/2)
		bevelX := -h * math.Sin(phi/2) // the bevel's farthest points are the offset corners
		want := bevelX
		if tc.miter {
			want = miterX
		}
		if math.Abs(minX-want) > 1e-3 {
			t.Errorf("phi %v limit %v: tip at x=%.4f, want %.4f (miter %v)", tc.phiDeg, tc.limit, minX, want, tc.miter)
		}
	}
}

func TestStrokeDots(t *testing.T) {
	var a, b, c, d, e Path
	a.MoveTo(Pt(10, 20))
	a.LineTo(Pt(10, 20))
	b.MoveTo(Pt(10, 20))
	b.Close()
	c.MoveTo(Pt(10, 20))
	c.CubicTo(Pt(10, 20), Pt(10, 20), Pt(10, 20))
	d.MoveTo(Pt(10, 20))
	d.QuadTo(Pt(10, 20), Pt(10, 20))
	d.LineTo(Pt(10, 20))
	d.Close()
	e.MoveTo(Pt(10, 20)) // a lone MoveTo draws nothing
	for i, p := range []Path{a, b, c, d} {
		round := p.Stroke(StrokeStyle{Width: 8, Cap: CapRound}, 0.01)
		area := signedArea(outlineLoops(round))
		if want := 16 * math.Pi; area > want || area < want-0.01*2*math.Pi*4 {
			t.Errorf("path %d: round dot area %v, want about %v", i, area, want)
		}
		if bb := round.Bounds(); !rectNear(bb, Rect{6, 16, 14, 24}, 1e-4) {
			t.Errorf("path %d: round dot bounds %v", i, bb)
		}
		square := p.Stroke(StrokeStyle{Width: 8, Cap: CapSquare, Join: JoinRound}, 0.01)
		if area := signedArea(outlineLoops(square)); math.Abs(area-64) > 1e-9 {
			t.Errorf("path %d: square dot area %v, want 64", i, area)
		}
		if bb := square.Bounds(); bb != (Rect{6, 16, 14, 24}) {
			t.Errorf("path %d: square dot bounds %v, want axis aligned", i, bb)
		}
		if butt := p.Stroke(StrokeStyle{Width: 8, Cap: CapButt}, 0.01); !butt.IsEmpty() {
			t.Errorf("path %d: butt dot is not empty: %v", i, butt.Verbs())
		}
	}
	for _, cp := range []LineCap{CapButt, CapRound, CapSquare} {
		if out := e.Stroke(StrokeStyle{Width: 8, Cap: cp}, 0.01); !out.IsEmpty() {
			t.Errorf("lone MoveTo with %v caps: got %v", cp, out.Verbs())
		}
	}
	// A dot next to a stroke: both drawn.
	var f Path
	f.MoveTo(Pt(0, 0))
	f.LineTo(Pt(10, 0))
	f.MoveTo(Pt(50, 0))
	f.LineTo(Pt(50, 0))
	out := f.Stroke(StrokeStyle{Width: 4, Cap: CapSquare}, 0.01)
	if area := signedArea(outlineLoops(out)); math.Abs(area-(14*4+16)) > 1e-9 {
		t.Errorf("segment plus dot: area %v, want %v", area, 14*4+16)
	}
}

func TestStrokeClosedJoinsAtStart(t *testing.T) {
	rect := RectPath(RectXYWH(0, 0, 100, 50))
	open := polyPath(false, Pt(0, 0), Pt(100, 0), Pt(100, 50), Pt(0, 50), Pt(0, 0))
	st := StrokeStyle{Width: 10, Join: JoinMiter, MiterLimit: 4, Cap: CapButt}
	box := Rect{-10, -10, 110, 60}
	closed := nonZeroArea(outlineLoops(rect.Stroke(st, 0.01)), box, 0.25)
	if math.Abs(closed-3000) > 1 {
		t.Errorf("closed rect: area %v, want 3000", closed)
	}
	// Open, the corner at the start gets two butt caps instead of a join.
	opened := nonZeroArea(outlineLoops(open.Stroke(st, 0.01)), box, 0.25)
	if math.Abs(opened-2975) > 1 {
		t.Errorf("open rect: area %v, want 2975", opened)
	}
	// The closed outline is two clean loops: the outer one and the hole.
	out := rect.Stroke(st, 0.01)
	if n := countVerb(out, MoveTo); n != 2 {
		t.Errorf("closed rect: %d subpaths, want 2", n)
	}
	// Outer: four miter tips and the start point; inner: the four inner
	// miter points (no detour through the center line).
	if len(out.Points()) != 9 {
		t.Errorf("closed rect: %d points, want 9: %v", len(out.Points()), out.Points())
	}
}

func countVerb(p Path, v Verb) int {
	n := 0
	for _, w := range p.Verbs() {
		if w == v {
			n++
		}
	}
	return n
}

func TestStrokeWithinBuilderBounds(t *testing.T) {
	// The Builder's conservative stroke bounds must contain the outline.
	rng := rand.New(rand.NewSource(4))
	pt := func() Point { return Pt(float32(rng.Float64()*100), float32(rng.Float64()*100)) }
	for i := range 300 {
		var p Path
		p.MoveTo(pt())
		for range 1 + rng.Intn(4) {
			switch rng.Intn(4) {
			case 0:
				p.LineTo(pt())
			case 1:
				p.QuadTo(pt(), pt())
			case 2:
				p.CubicTo(pt(), pt(), pt())
			default:
				p.Close()
			}
		}
		st := StrokeStyle{
			Width:      float32(0.5 + rng.Float64()*40),
			Join:       LineJoin(rng.Intn(3)),
			Cap:        LineCap(rng.Intn(3)),
			MiterLimit: float32(1 + rng.Float64()*8),
		}
		if rng.Intn(3) == 0 {
			st.Dashes = []float32{float32(rng.Float64() * 20), float32(rng.Float64() * 20)}
			st.DashOffset = float32(rng.Float64()*100 - 50)
		}
		out := p.Stroke(st, 0.1)
		st, _ = sanitizeStroke(st)
		k := float32(1)
		if st.Cap == CapSquare {
			k = 1.41421357
		}
		if st.Join == JoinMiter {
			k = max(k, st.MiterLimit)
		}
		bb := p.ControlBounds().Outset(st.Width/2*k + 1e-3)
		for _, q := range out.Points() {
			if !(q.X >= bb.X0 && q.X <= bb.X1 && q.Y >= bb.Y0 && q.Y <= bb.Y1) {
				t.Fatalf("case %d: outline point %v outside the stroke bounds %v", i, q, bb)
			}
		}
		// Never a negative winding.
		loops := outlineLoops(out)
		for range 50 {
			x := float64(bb.X0) + rng.Float64()*float64(bb.Width())
			y := float64(bb.Y0) + rng.Float64()*float64(bb.Height())
			if w := winding(loops, x, y); w < 0 {
				t.Fatalf("case %d: negative winding %d at (%v, %v)", i, w, x, y)
			}
		}
	}
}

func TestStrokeOutputIsLinesOnly(t *testing.T) {
	out := CirclePath(Pt(0, 0), 10).Stroke(StrokeStyle{Width: 3, Cap: CapRound, Join: JoinRound}, 0.05)
	for _, v := range out.Verbs() {
		if v != MoveTo && v != LineTo && v != Close {
			t.Fatalf("output has verb %v", v)
		}
	}
}

func TestStrokeToleranceScaling(t *testing.T) {
	// Smaller tolerances give more segments and stay within the oracle (the
	// oracle tests cover accuracy); here only check monotonic refinement.
	p := cubicPath(Pt(0, 0), Pt(100, 0), Pt(-20, 100), Pt(100, 100))
	st := StrokeStyle{Width: 20, Join: JoinRound, Cap: CapRound}
	prev := 0
	for _, tol := range []float32{1, 0.25, 0.05, 0.01} {
		n := len(p.Stroke(st, tol).Points())
		if n <= prev {
			t.Errorf("tolerance %v: %d points, not more than %d", tol, n, prev)
		}
		prev = n
	}
}

func TestStrokeDegenerateInputs(t *testing.T) {
	huge := float32(3e38)
	var paths []Path
	paths = append(paths, Path{})
	var p Path
	p.MoveTo(Pt(0, 0))
	paths = append(paths, p)
	paths = append(paths, polyPath(false, Pt(1e30, 1e30), Pt(-1e30, 1e30), Pt(1e30, -1e30)))
	paths = append(paths, polyPath(true, Pt(huge, huge), Pt(-huge, huge), Pt(huge, -huge)))
	paths = append(paths, cubicPath(Pt(huge, 0), Pt(-huge, huge), Pt(huge, -huge), Pt(0, huge)))
	paths = append(paths, cubicPath(Pt(1e6, 1e6), Pt(1e6+1e-1, 1e6), Pt(1e6, 1e6+0.1), Pt(1e6, 1e6)))
	paths = append(paths, polyPath(false, Pt(0, 0), Pt(1e-30, 0), Pt(0, 1e-30)))
	// kurbo's broken strokes.
	for _, c := range [][4]Point{
		{Pt(465.24423, 107.11105), Pt(475.50754, 107.11105), Pt(475.50754, 107.11105), Pt(475.50754, 107.11105)},
		{Pt(0, -0.01), Pt(128, 128.001), Pt(128, -0.01), Pt(0, 128.001)},
		{Pt(0, 0), Pt(0, -10), Pt(0, -10), Pt(0, 10)},
		{Pt(10, 0), Pt(0, 0), Pt(20, 0), Pt(10, 0)},
		{Pt(39, -39), Pt(40, -40), Pt(40, -40), Pt(0, 0)},
		{Pt(40, 40), Pt(0, 0), Pt(200, 200), Pt(0, 0)},
		{Pt(0, 0), Pt(1e-2, 0), Pt(-1e-2, 0), Pt(0, 0)},
		{Pt(400.75, 100.05), Pt(400.75, 100.05), Pt(100.05, 300.95), Pt(100.05, 300.95)},
		{Pt(0.5, 0), Pt(0, 0), Pt(20, 0), Pt(10, 0)},
		{Pt(10, 0), Pt(0, 0), Pt(10, 0), Pt(10, 0)},
		{Pt(602.469, 286.585), Pt(641.975, 286.585), Pt(562.963, 286.585), Pt(562.963, 286.585)},
	} {
		paths = append(paths, cubicPath(c[0], c[1], c[2], c[3]))
	}
	styles := []StrokeStyle{
		NewStrokeStyle(1),
		{Width: 30, Join: JoinMiter, Cap: CapButt, MiterLimit: 4},
		{Width: 30, Join: JoinRound, Cap: CapRound},
		{Width: 1e-30, Cap: CapRound},
		{Width: 1e30, Cap: CapSquare, Join: JoinBevel},
		{Width: huge, Cap: CapRound, Join: JoinRound},
		{Width: 2, Dashes: []float32{1e-3, 1e-3}},
		{Width: 2, Dashes: []float32{0, 0, 3}, DashOffset: -1e30},
		{Width: 2, Dashes: []float32{1e30, 1}, DashOffset: 1e30},
		{Width: 0},
		{Width: float32(math.NaN())},
		{Width: 2, Dashes: []float32{float32(math.Inf(1)), 1}},
		{Width: 2, MiterLimit: float32(math.Inf(1))},
	}
	nan := float32(math.NaN())
	for i, p := range paths {
		for j, st := range styles {
			for _, tol := range []float32{0.25, 0, -1, nan, 1e30, float32(math.Inf(1))} {
				out := p.Stroke(st, tol)
				if !out.IsFinite() {
					t.Errorf("path %d style %d tol %v: non-finite output", i, j, tol)
				}
				if v := out.Verbs(); len(v) > 0 && (v[0] != MoveTo || v[len(v)-1] == MoveTo) {
					t.Errorf("path %d style %d tol %v: malformed output verbs %v", i, j, tol, v)
				}
			}
		}
	}
	var nanPath Path
	nanPath.MoveTo(Pt(0, 0))
	nanPath.LineTo(Pt(nan, 1))
	if out := nanPath.Stroke(NewStrokeStyle(2), 0.25); !out.IsEmpty() {
		t.Errorf("non-finite path: got %v", out.Verbs())
	}
}

func TestStrokeDeterministic(t *testing.T) {
	p := CirclePath(Pt(3, 7), 31)
	st := StrokeStyle{Width: 5, Cap: CapRound, Join: JoinMiter, MiterLimit: 4, Dashes: []float32{7, 3}, DashOffset: 2}
	a, b := p.Stroke(st, 0.1), p.Stroke(st, 0.1)
	if len(a.Points()) != len(b.Points()) {
		t.Fatal("different lengths")
	}
	for i := range a.Points() {
		if a.Points()[i] != b.Points()[i] {
			t.Fatal("different points")
		}
	}
}

func BenchmarkStroke(b *testing.B) {
	var icon Path
	icon.MoveTo(Pt(4, 12))
	icon.CubicTo(Pt(4, 7), Pt(8, 4), Pt(12, 4))
	icon.LineTo(Pt(20, 4))
	icon.LineTo(Pt(14, 10))
	icon.QuadTo(Pt(20, 16), Pt(12, 20))
	cases := []struct {
		name  string
		p     Path
		style StrokeStyle
	}{
		{"circle", CirclePath(Pt(50, 50), 40), StrokeStyle{Width: 2, MiterLimit: 4}},
		{"rrect", NewRRect(RectXYWH(0.5, 0.5, 200, 40), 8).Path(), StrokeStyle{Width: 1, MiterLimit: 4}},
		{"icon", icon, StrokeStyle{Width: 2, Join: JoinRound, Cap: CapRound}},
		{"thick cubic", cubicPath(Pt(0, 0), Pt(100, 0), Pt(-20, 100), Pt(100, 100)), StrokeStyle{Width: 30, MiterLimit: 4}},
		{"dashed rect", RectPath(RectXYWH(0, 0, 300, 200)), StrokeStyle{Width: 1, MiterLimit: 4, Dashes: []float32{4, 4}}},
		{"dotted circle", CirclePath(Pt(50, 50), 40), StrokeStyle{Width: 3, Cap: CapRound, Dashes: []float32{0, 6}}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				c.p.Stroke(c.style, 0.25)
			}
		})
	}
}
