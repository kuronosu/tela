package gfx

import (
	"math"
	"math/rand"
	"testing"
)

// flatSeg is one polyline subpath produced by Flatten.
type flatSeg struct {
	pts    []Point
	closed bool
}

func flattenAll(p Path, tol float32) []flatSeg {
	var out []flatSeg
	p.Flatten(tol, func(v Verb, pt Point) {
		switch v {
		case MoveTo:
			out = append(out, flatSeg{pts: []Point{pt}})
		case LineTo:
			out[len(out)-1].pts = append(out[len(out)-1].pts, pt)
		case Close:
			out[len(out)-1].closed = true
		}
	})
	return out
}

func distToSegment(p, a, b Point) float64 {
	px, py := float64(p.X), float64(p.Y)
	ax, ay, bx, by := float64(a.X), float64(a.Y), float64(b.X), float64(b.Y)
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func distToPolyline(p Point, poly []Point) float64 {
	d := math.Inf(1)
	for i := 1; i < len(poly); i++ {
		d = math.Min(d, distToSegment(p, poly[i-1], poly[i]))
	}
	if len(poly) == 1 {
		d = math.Hypot(float64(p.X-poly[0].X), float64(p.Y-poly[0].Y))
	}
	return d
}

// maxDeviation returns the largest distance from the exact curve (sampled
// densely) to the flattened polyline, and the largest distance from a polyline
// vertex to the curve.
func maxDeviation(curve func(t float64) Point, poly []Point) (curveToPoly float64) {
	const n = 4000
	for i := 0; i <= n; i++ {
		curveToPoly = math.Max(curveToPoly, distToPolyline(curve(float64(i)/n), poly))
	}
	return curveToPoly
}

func TestFlattenCubicsWithinTolerance(t *testing.T) {
	cubics := []struct {
		name           string
		p0, p1, p2, p3 Point
	}{
		{"arch", Pt(0, 0), Pt(0, 100), Pt(100, 100), Pt(100, 0)},
		{"S curve", Pt(0, 0), Pt(100, 0), Pt(-100, 100), Pt(0, 100)},
		{"loop", Pt(0, 0), Pt(150, 100), Pt(-50, 100), Pt(100, 0)},
		{"cusp", Pt(0, 0), Pt(100, 100), Pt(0, 100), Pt(100, 0)},
		{"tight loop tip", Pt(-28, 147), Pt(-185, -24), Pt(143, 159), Pt(-28, 75)},
		{"strong inflection", Pt(0, 0), Pt(100, 0), Pt(-100, 100), Pt(0, 100)},
		{"near line", Pt(0, 0), Pt(30, 0.5), Pt(60, -0.5), Pt(100, 0)},
		{"straight", Pt(0, 0), Pt(10, 10), Pt(20, 20), Pt(30, 30)},
		{"collinear overshoot", Pt(0, 0), Pt(150, 0), Pt(-50, 0), Pt(100, 0)},
		{"tiny", Pt(0, 0), Pt(0.2, 0.5), Pt(0.5, 0.5), Pt(0.7, 0)},
		{"huge", Pt(0, 0), Pt(0, 3000), Pt(3000, 3000), Pt(3000, 0)},
		{"circle quarter", Pt(100, 0), Pt(100, 55.2284749831), Pt(55.2284749831, 100), Pt(0, 100)},
		{"degenerate point", Pt(5, 5), Pt(5, 5), Pt(5, 5), Pt(5, 5)},
		{"coincident controls", Pt(0, 0), Pt(50, 50), Pt(50, 50), Pt(100, 0)},
	}
	for _, c := range cubics {
		for _, tol := range []float32{0.25, 0.1, 0.01} {
			var p Path
			p.MoveTo(c.p0)
			p.CubicTo(c.p1, c.p2, c.p3)
			segs := flattenAll(p, tol)
			if len(segs) != 1 {
				t.Fatalf("%s: %d subpaths", c.name, len(segs))
			}
			poly := segs[0].pts
			if poly[0] != c.p0 || poly[len(poly)-1] != c.p3 {
				t.Errorf("%s tol %v: endpoints %v..%v", c.name, tol, poly[0], poly[len(poly)-1])
			}
			curve := func(u float64) Point { return evalCubic(c.p0, c.p1, c.p2, c.p3, u) }
			dev := maxDeviation(curve, poly)
			// float32 output noise is relative to the coordinate magnitude.
			slack := 2e-7 * 3000 * math.Max(1, float64(c.p3.X)/3000)
			if dev > float64(tol)+slack {
				t.Errorf("%s tol %v: deviation %.5f exceeds tolerance (%d points)", c.name, tol, dev, len(poly))
			}
		}
	}
}

func TestFlattenQuadsWithinTolerance(t *testing.T) {
	quads := []struct {
		name       string
		p0, p1, p2 Point
	}{
		{"arch", Pt(0, 0), Pt(50, 100), Pt(100, 0)},
		{"skewed", Pt(0, 0), Pt(10, 80), Pt(100, 10)},
		{"sharp", Pt(0, 0), Pt(100, 1), Pt(0, 2)},
		{"straight", Pt(0, 0), Pt(50, 50), Pt(100, 100)},
		{"collinear fold", Pt(0, 0), Pt(100, 0), Pt(20, 0)},
		{"vertical fold", Pt(3, 0), Pt(3, -80), Pt(3, 10)},
		{"degenerate", Pt(7, 7), Pt(7, 7), Pt(7, 7)},
		{"huge", Pt(0, 0), Pt(5000, 9000), Pt(10000, 0)},
	}
	for _, c := range quads {
		for _, tol := range []float32{0.25, 0.1, 0.01} {
			var p Path
			p.MoveTo(c.p0)
			p.QuadTo(c.p1, c.p2)
			poly := flattenAll(p, tol)[0].pts
			curve := func(u float64) Point { return evalQuad(c.p0, c.p1, c.p2, u) }
			dev := maxDeviation(curve, poly)
			if dev > float64(tol)+1e-3 {
				t.Errorf("%s tol %v: deviation %.5f exceeds tolerance (%d points)", c.name, tol, dev, len(poly))
			}
		}
	}
}

func TestFlattenRandomCurvesWithinTolerance(t *testing.T) {
	// Deterministic pseudo-random curves, including loops, cusps and
	// inflections. The deviation is measured by dense sampling of the exact
	// curve; float32 coordinates up to 200 contribute about 2e-5.
	rng := rand.New(rand.NewSource(1))
	pt := func() Point { return Pt(float32(rng.Intn(401)-200), float32(rng.Intn(401)-200)) }
	for _, tol := range []float32{0.25, 0.05} {
		for i := 0; i < 250; i++ {
			c := [4]Point{pt(), pt(), pt(), pt()}
			var p Path
			p.MoveTo(c[0])
			p.CubicTo(c[1], c[2], c[3])
			poly := flattenAll(p, tol)[0].pts
			dev := maxDeviationN(func(u float64) Point { return evalCubic(c[0], c[1], c[2], c[3], u) }, poly, 500)
			if dev > float64(tol)+1e-4 {
				t.Errorf("cubic %v tol %v: deviation %.4f", c, tol, dev)
			}
			q := [3]Point{c[0], c[1], c[2]}
			var pq Path
			pq.MoveTo(q[0])
			pq.QuadTo(q[1], q[2])
			polyQ := flattenAll(pq, tol)[0].pts
			devQ := maxDeviationN(func(u float64) Point { return evalQuad(q[0], q[1], q[2], u) }, polyQ, 500)
			if devQ > float64(tol)+1e-4 {
				t.Errorf("quad %v tol %v: deviation %.4f", q, tol, devQ)
			}
		}
	}
}

func maxDeviationN(curve func(t float64) Point, poly []Point, n int) (d float64) {
	for i := 0; i <= n; i++ {
		d = math.Max(d, distToPolyline(curve(float64(i)/float64(n)), poly))
	}
	return d
}

func TestFlattenSegmentCounts(t *testing.T) {
	// A circle of radius 100 at tolerance 0.1 needs about
	// pi/acos(1-0.1/100) ~ 70 chords at best; kurbo's method is close to that.
	p := CirclePath(Pt(0, 0), 100)
	n := 0
	p.Flatten(0.1, func(v Verb, _ Point) {
		if v == LineTo {
			n++
		}
	})
	if n < 60 || n > 110 {
		t.Errorf("circle r=100 tol=0.1 flattened to %d segments, expected 60..110", n)
	}
	// Tighter tolerance means more segments.
	n2 := 0
	p.Flatten(0.01, func(v Verb, _ Point) {
		if v == LineTo {
			n2++
		}
	})
	if n2 <= n {
		t.Errorf("tol 0.01 gave %d segments, tol 0.1 gave %d", n2, n)
	}
}

func TestFlattenPassesLinesAndStructure(t *testing.T) {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.LineTo(Pt(10, 0))
	p.Close()
	p.MoveTo(Pt(20, 20))
	p.QuadTo(Pt(25, 30), Pt(30, 20))
	p.LineTo(Pt(40, 20))
	segs := flattenAll(p, 0.1)
	if len(segs) != 2 {
		t.Fatalf("subpaths = %d", len(segs))
	}
	if !segs[0].closed || len(segs[0].pts) != 2 || segs[1].closed {
		t.Errorf("structure wrong: %+v", segs)
	}
	last := segs[1].pts[len(segs[1].pts)-1]
	if last != Pt(40, 20) {
		t.Errorf("last point = %v", last)
	}
	// An implicit MoveTo after Close is emitted.
	var q Path
	q.MoveTo(Pt(1, 1))
	q.LineTo(Pt(2, 2))
	q.Close()
	q.LineTo(Pt(5, 5))
	var verbs []Verb
	q.Flatten(0.1, func(v Verb, _ Point) { verbs = append(verbs, v) })
	if len(verbs) != 5 || verbs[3] != MoveTo || verbs[4] != LineTo {
		t.Errorf("verbs = %v", verbs)
	}
}

func TestFlattenRobustness(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for name, build := range map[string]func(p *Path){
		"nan cubic": func(p *Path) { p.MoveTo(Pt(0, 0)); p.CubicTo(Pt(nan, 1), Pt(2, 2), Pt(3, 3)) },
		"inf cubic": func(p *Path) { p.MoveTo(Pt(0, 0)); p.CubicTo(Pt(inf, 1), Pt(2, 2), Pt(3, 3)) },
		"nan quad":  func(p *Path) { p.MoveTo(Pt(0, 0)); p.QuadTo(Pt(nan, nan), Pt(3, 3)) },
		"huge":      func(p *Path) { p.MoveTo(Pt(0, 0)); p.CubicTo(Pt(1e30, 1e30), Pt(-1e30, 1e30), Pt(1e30, -1e30)) },
	} {
		var p Path
		build(&p)
		count := 0
		p.Flatten(0.1, func(Verb, Point) { count++ })
		if count > 2*maxFlattenSegments+10 {
			t.Errorf("%s: %d segments emitted", name, count)
		}
	}
	// Non-positive and NaN tolerances fall back to the minimum instead of
	// looping forever.
	p := CirclePath(Pt(0, 0), 10)
	for _, tol := range []float32{0, -1, nan} {
		count := 0
		p.Flatten(tol, func(Verb, Point) { count++ })
		if count == 0 || count > 100000 {
			t.Errorf("tolerance %v produced %d segments", tol, count)
		}
	}
}
