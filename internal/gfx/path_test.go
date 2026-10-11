package gfx

import (
	"math"
	"strings"
	"testing"
)

func TestPathBuilder(t *testing.T) {
	var p Path
	if !p.IsEmpty() || p.Len() != 0 {
		t.Fatal("zero Path should be empty")
	}
	if _, ok := p.CurrentPoint(); ok {
		t.Error("empty path has no current point")
	}
	p.MoveTo(Pt(1, 1))
	p.MoveTo(Pt(2, 2)) // collapses
	p.LineTo(Pt(10, 2))
	p.QuadTo(Pt(12, 4), Pt(10, 6))
	p.CubicTo(Pt(8, 8), Pt(6, 8), Pt(4, 6))
	if cur, _ := p.CurrentPoint(); cur != Pt(4, 6) {
		t.Errorf("CurrentPoint = %v", cur)
	}
	p.Close()
	p.Close() // second close is a no-op
	if cur, _ := p.CurrentPoint(); cur != Pt(2, 2) {
		t.Errorf("CurrentPoint after Close = %v, want subpath start", cur)
	}
	wantVerbs := []Verb{MoveTo, LineTo, QuadTo, CubicTo, Close}
	if got := p.Verbs(); len(got) != len(wantVerbs) {
		t.Fatalf("verbs = %v", got)
	} else {
		for i := range got {
			if got[i] != wantVerbs[i] {
				t.Fatalf("verbs = %v want %v", got, wantVerbs)
			}
		}
	}
	if len(p.Points()) != 1+1+2+3 {
		t.Errorf("points = %d", len(p.Points()))
	}
	// Drawing after Close inserts an implicit MoveTo at the subpath start.
	p.LineTo(Pt(20, 20))
	v := p.Verbs()
	if v[len(v)-2] != MoveTo || v[len(v)-1] != LineTo {
		t.Errorf("implicit MoveTo missing: %v", v)
	}
	pts := p.Points()
	if pts[len(pts)-2] != Pt(2, 2) {
		t.Errorf("implicit MoveTo point = %v", pts[len(pts)-2])
	}
}

func TestPathDrawBeforeMoveToPanics(t *testing.T) {
	for name, f := range map[string]func(p *Path){
		"LineTo":  func(p *Path) { p.LineTo(Pt(1, 1)) },
		"QuadTo":  func(p *Path) { p.QuadTo(Pt(1, 1), Pt(2, 2)) },
		"CubicTo": func(p *Path) { p.CubicTo(Pt(1, 1), Pt(2, 2), Pt(3, 3)) },
	} {
		func() {
			defer func() {
				r := recover()
				msg, _ := r.(string)
				if !strings.Contains(msg, name) || !strings.Contains(msg, "MoveTo") {
					t.Errorf("%s: panic = %v, want an actionable message naming %s and MoveTo", name, r, name)
				}
			}()
			var p Path
			f(&p)
		}()
	}
}

func TestPathElements(t *testing.T) {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.LineTo(Pt(1, 0))
	p.QuadTo(Pt(2, 0), Pt(2, 1))
	p.CubicTo(Pt(2, 2), Pt(1, 2), Pt(0, 2))
	p.Close()
	var got []string
	for el := range p.Elements() {
		got = append(got, el.Verb.String())
		if el.Verb == CubicTo && el.Pts[2] != Pt(0, 2) {
			t.Errorf("cubic end point = %v", el.Pts[2])
		}
		if el.Verb == QuadTo && (el.Pts[0] != Pt(2, 0) || el.Pts[1] != Pt(2, 1)) {
			t.Errorf("quad points = %v", el.Pts)
		}
	}
	if strings.Join(got, ",") != "MoveTo,LineTo,QuadTo,CubicTo,Close" {
		t.Errorf("elements = %v", got)
	}
	// Early break must be supported.
	n := 0
	for range p.Elements() {
		n++
		break
	}
	if n != 1 {
		t.Error("break did not stop iteration")
	}
}

func TestPathSegments(t *testing.T) {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.LineTo(Pt(10, 0))
	p.QuadTo(Pt(20, 0), Pt(20, 10))
	p.CubicTo(Pt(20, 20), Pt(10, 20), Pt(0, 10))
	p.Close()
	p.MoveTo(Pt(50, 50))
	p.LineTo(Pt(60, 50))
	want := []Segment{
		{MoveTo, [4]Point{{0, 0}}},
		{LineTo, [4]Point{{0, 0}, {10, 0}}},
		{QuadTo, [4]Point{{10, 0}, {20, 0}, {20, 10}}},
		{CubicTo, [4]Point{{20, 10}, {20, 20}, {10, 20}, {0, 10}}},
		{Close, [4]Point{{0, 10}, {0, 0}}},
		{MoveTo, [4]Point{{50, 50}}},
		{LineTo, [4]Point{{50, 50}, {60, 50}}},
	}
	i := 0
	for s := range p.Segments() {
		if i >= len(want) || s != want[i] {
			t.Errorf("segment %d = %+v want %+v", i, s, want[i])
		}
		i++
	}
	if i != len(want) {
		t.Errorf("%d segments, want %d", i, len(want))
	}
	if (Segment{Verb: Close, P: [4]Point{{1, 1}, {2, 2}}}).End() != Pt(2, 2) {
		t.Error("End of Close wrong")
	}
	// A Close followed by an implicit MoveTo starts at the subpath start.
	var q Path
	q.MoveTo(Pt(5, 5))
	q.LineTo(Pt(6, 6))
	q.Close()
	q.LineTo(Pt(7, 7))
	var starts []Point
	for s := range q.Segments() {
		if s.Verb == MoveTo {
			starts = append(starts, s.P[0])
		}
	}
	if len(starts) != 2 || starts[1] != Pt(5, 5) {
		t.Errorf("subpath starts = %v", starts)
	}
}

func TestPathCloneIsIndependent(t *testing.T) {
	p := RectPath(Rect{0, 0, 10, 10})
	q := p.Clone()
	q.MoveTo(Pt(50, 50))
	q.LineTo(Pt(60, 60))
	if p.Len() != 5 || q.Len() != 7 {
		t.Errorf("lens = %d, %d", p.Len(), q.Len())
	}
	q.Points()[0] = Pt(-1, -1)
	if p.Points()[0] != Pt(0, 0) {
		t.Error("Clone shares point storage")
	}
	var e Path
	if c := e.Clone(); !c.IsEmpty() {
		t.Error("clone of empty is not empty")
	}
}

func TestPathReset(t *testing.T) {
	p := RectPath(Rect{0, 0, 1, 1})
	p.Reset()
	if !p.IsEmpty() {
		t.Error("Reset should empty the path")
	}
	p.MoveTo(Pt(3, 3))
	p.LineTo(Pt(4, 4))
	if p.Len() != 2 {
		t.Errorf("len after reuse = %d", p.Len())
	}
}

func TestAddRect(t *testing.T) {
	p := RectPath(Rect{1, 2, 11, 22})
	want := []Point{{1, 2}, {11, 2}, {11, 22}, {1, 22}}
	got := p.Points()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rect point %d = %v want %v", i, got[i], want[i])
		}
	}
	if v := p.Verbs(); v[len(v)-1] != Close || len(v) != 5 {
		t.Errorf("rect verbs = %v", v)
	}
}

func TestEllipseAndCircle(t *testing.T) {
	c := CirclePath(Pt(50, 60), 40)
	b := c.Bounds()
	if !rectNear(b, Rect{10, 20, 90, 100}, 1e-3) {
		t.Errorf("circle bounds = %v", b)
	}
	cb := c.ControlBounds()
	if !rectNear(cb, Rect{10, 20, 90, 100}, 1e-3) {
		t.Errorf("circle control bounds = %v", cb)
	}
	e := EllipsePath(Pt(0, 0), 30, 10)
	if b := e.Bounds(); !rectNear(b, Rect{-30, -10, 30, 10}, 1e-3) {
		t.Errorf("ellipse bounds = %v", b)
	}
	// Every flattened point lies on the ellipse (tolerance 0.01 + float noise).
	e.Flatten(0.01, func(v Verb, pt Point) {
		if v == Close {
			return
		}
		d := math.Sqrt(math.Pow(float64(pt.X)/30, 2) + math.Pow(float64(pt.Y)/10, 2))
		if math.Abs(d-1) > 0.002 {
			t.Errorf("point %v is off the ellipse (normalized radius %v)", pt, d)
		}
	})
	// The path is closed exactly at its start.
	pts := e.Points()
	if pts[0] != pts[len(pts)-1] {
		t.Errorf("ellipse does not close exactly: %v vs %v", pts[0], pts[len(pts)-1])
	}
	// Clockwise on screen: the first curve goes from the rightmost point down.
	if pts[1].Y <= pts[0].Y && pts[1].X <= pts[0].X {
		t.Errorf("first control point %v not heading toward +Y", pts[1])
	}
}

func TestAddArc(t *testing.T) {
	// Quarter circle from angle 0 (right) clockwise to angle 90 deg (down).
	a := Arc{Center: Pt(10, 10), Radii: Pt(20, 20), StartAngle: 0, SweepAngle: math.Pi / 2}
	p := ArcPath(a, 0)
	pts := p.Points()
	if pts[0] != Pt(30, 10) {
		t.Errorf("arc start = %v", pts[0])
	}
	if end := pts[len(pts)-1]; !ptNear(end, Pt(10, 30), 1e-4) {
		t.Errorf("arc end = %v", end)
	}
	// Counter-clockwise sweep.
	ccw := ArcPath(Arc{Center: Pt(0, 0), Radii: Pt(10, 10), StartAngle: 0, SweepAngle: -math.Pi / 2}, 0)
	if end := ccw.Points()[len(ccw.Points())-1]; !ptNear(end, Pt(0, -10), 1e-4) {
		t.Errorf("ccw arc end = %v", end)
	}
	// Rotated ellipse: radii (20, 5) rotated by 90 degrees starts at (0, 20).
	rot := ArcPath(Arc{Center: Pt(0, 0), Radii: Pt(20, 5), SweepAngle: math.Pi / 2, XRotation: math.Pi / 2}, 0)
	if start := rot.Points()[0]; !ptNear(start, Pt(0, 20), 1e-4) {
		t.Errorf("rotated arc start = %v", start)
	}
	// Continuing from a current point adds a line to the arc start.
	var q Path
	q.MoveTo(Pt(0, 0))
	q.AddArc(a, 0.01)
	if v := q.Verbs(); v[1] != LineTo || v[2] != CubicTo {
		t.Errorf("verbs = %v, want MoveTo LineTo CubicTo", v)
	}
	// Zero sweep adds nothing but the start.
	z := ArcPath(Arc{Radii: Pt(5, 5)}, 0)
	if z.Len() != 1 {
		t.Errorf("zero sweep arc has %d verbs", z.Len())
	}
	// Segment count grows with radius for a fixed tolerance, and is
	// bounded for absurd radii.
	small := ArcPath(Arc{Radii: Pt(1, 1), SweepAngle: 2 * math.Pi}, 0.01)
	big := ArcPath(Arc{Radii: Pt(1e6, 1e6), SweepAngle: 2 * math.Pi}, 0.01)
	if !(big.Len() > small.Len()) || big.Len() > maxArcSegments+1 {
		t.Errorf("segments small=%d big=%d", small.Len(), big.Len())
	}
}

func TestArcCubicAccuracy(t *testing.T) {
	// The cubic approximation must stay within the requested tolerance of the
	// true circle.
	for _, tol := range []float32{0.5, 0.05, 0.005} {
		for _, r := range []float32{1, 17, 300, 5000} {
			p := ArcPath(Arc{Radii: Pt(r, r), SweepAngle: 2 * math.Pi}, tol)
			var maxErr float64
			var cur Point
			i := 0
			pts := p.Points()
			cur = pts[0]
			i = 1
			for _, v := range p.Verbs()[1:] {
				if v != CubicTo {
					continue
				}
				c1, c2, e := pts[i], pts[i+1], pts[i+2]
				i += 3
				for k := 0; k <= 50; k++ {
					q := evalCubic(cur, c1, c2, e, float64(k)/50)
					d := math.Abs(math.Hypot(float64(q.X), float64(q.Y)) - float64(r))
					maxErr = math.Max(maxErr, d)
				}
				cur = e
			}
			// Float32 coordinates of a big circle carry ~r*6e-8 of noise.
			slack := float64(r) * 2e-7
			if maxErr > float64(tol)+slack {
				t.Errorf("tol %v r %v: max deviation %v", tol, r, maxErr)
			}
		}
	}
}

func TestPathBounds(t *testing.T) {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.CubicTo(Pt(0, 100), Pt(100, 100), Pt(100, 0)) // arch: true max y = 75
	cb := p.ControlBounds()
	if cb != (Rect{0, 0, 100, 100}) {
		t.Errorf("ControlBounds = %v", cb)
	}
	b := p.Bounds()
	if !rectNear(b, Rect{0, 0, 100, 75}, 1e-4) {
		t.Errorf("Bounds = %v, want max y 75", b)
	}
	// Quadratic with extremum inside.
	var q Path
	q.MoveTo(Pt(0, 0))
	q.QuadTo(Pt(50, 100), Pt(100, 0)) // max y = 50
	if b := q.Bounds(); !rectNear(b, Rect{0, 0, 100, 50}, 1e-4) {
		t.Errorf("quad Bounds = %v", b)
	}
	// Cubic that overshoots in x (S-curve extrema on both axes).
	var s Path
	s.MoveTo(Pt(0, 0))
	s.CubicTo(Pt(100, 0), Pt(-100, 100), Pt(0, 100))
	sb := s.Bounds()
	if sb.X0 >= -1 || sb.X1 <= 1 || !near(sb.Y0, 0, 1e-4) || !near(sb.Y1, 100, 1e-4) {
		t.Errorf("S-curve Bounds = %v", sb)
	}
	// Bounds are within control bounds, and contain every flattened point.
	cbs := s.ControlBounds()
	if !cbs.ContainsRect(sb) {
		t.Errorf("Bounds %v not inside ControlBounds %v", sb, cbs)
	}
	s.Flatten(0.01, func(v Verb, pt Point) {
		if v != Close && !sb.Outset(0.001).Contains(pt) {
			t.Errorf("flattened point %v outside Bounds %v", pt, sb)
		}
	})
	if (Path{}).Bounds() != (Rect{}) || (Path{}).ControlBounds() != (Rect{}) {
		t.Error("empty path bounds should be the zero Rect")
	}
}

func TestPathTransform(t *testing.T) {
	p := RectPath(Rect{0, 0, 10, 10})
	q := p.Transform(Translate(5, 5).Mul(Scale(2, 2)))
	if b := q.Bounds(); b != (Rect{5, 5, 25, 25}) {
		t.Errorf("transformed bounds = %v", b)
	}
	if p.Bounds() != (Rect{0, 0, 10, 10}) {
		t.Error("Transform modified the receiver")
	}
	// Drawing on the transformed copy after Close keeps the transformed start.
	q.LineTo(Pt(0, 0))
	pts := q.Points()
	if pts[len(pts)-2] != Pt(5, 5) {
		t.Errorf("implicit MoveTo after Transform = %v, want (5,5)", pts[len(pts)-2])
	}
	if !q.IsFinite() {
		t.Error("IsFinite false")
	}
	bad := p.Transform(Affine{float32(math.NaN()), 0, 0, 1, 0, 0})
	if bad.IsFinite() {
		t.Error("NaN transform should give a non-finite path")
	}
}

func TestVerbAndRuleStrings(t *testing.T) {
	if MoveTo.NumPoints() != 1 || LineTo.NumPoints() != 1 || QuadTo.NumPoints() != 2 || CubicTo.NumPoints() != 3 || Close.NumPoints() != 0 {
		t.Error("NumPoints wrong")
	}
	if NonZero.String() != "nonzero" || EvenOdd.String() != "evenodd" || NonZero != 0 {
		t.Error("FillRule wrong")
	}
	if CubicTo.String() != "CubicTo" {
		t.Error("Verb.String wrong")
	}
}
