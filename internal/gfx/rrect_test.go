package gfx

import (
	"math"
	"testing"
)

func TestRRectNormalize(t *testing.T) {
	r := Rect{0, 0, 200, 100}
	tests := []struct {
		name string
		in   RRect
		want [4]Point
	}{
		{"fits", RRect{r, [4]Point{{10, 10}, {20, 20}, {30, 30}, {40, 40}}}, [4]Point{{10, 10}, {20, 20}, {30, 30}, {40, 40}}},
		{"negative radius", RRect{r, [4]Point{{-5, 10}, {20, 20}, {0, 0}, {5, 5}}}, [4]Point{{0, 0}, {20, 20}, {0, 0}, {5, 5}}},
		{"zero component squares the corner", RRect{r, [4]Point{{10, 0}, {0, 10}, {5, 5}, {5, 5}}}, [4]Point{{0, 0}, {0, 0}, {5, 5}, {5, 5}}},
		{"nan and inf", RRect{r, [4]Point{{float32(math.NaN()), 5}, {float32(math.Inf(1)), 5}, {1, 1}, {1, 1}}}, [4]Point{{0, 0}, {0, 0}, {1, 1}, {1, 1}}},
		// CSS Backgrounds 3 section 5.5: border-radius: 100px on a 200x100
		// box: the vertical sides (100) are the limit, f = 100/200 = 0.5.
		{"css pill", RRect{r, [4]Point{{100, 100}, {100, 100}, {100, 100}, {100, 100}}}, [4]Point{{50, 50}, {50, 50}, {50, 50}, {50, 50}}},
		// Right side: 100+100 = 200 > 100, so f = 0.5 and ALL radii scale,
		// including the small bottom-left one (proportions are kept).
		{"scale all radii by the minimum ratio", RRect{r, [4]Point{{100, 100}, {100, 100}, {100, 100}, {10, 10}}}, [4]Point{{50, 50}, {50, 50}, {50, 50}, {5, 5}}},
		// Only the top side overflows: 150+150 = 300 > 200 -> f = 2/3.
		{"horizontal overflow", RRect{r, [4]Point{{150, 10}, {150, 10}, {0, 0}, {0, 0}}}, [4]Point{{100, 6.6666665}, {100, 6.6666665}, {0, 0}, {0, 0}}},
		// Left side: 80+80 = 160 > 100, f = 0.625; the TR corner has a zero
		// vertical radius and is square.
		{"unequal axes", RRect{r, [4]Point{{20, 80}, {20, 0}, {0, 0}, {20, 80}}}, [4]Point{{12.5, 50}, {0, 0}, {0, 0}, {12.5, 50}}},
	}
	for _, tt := range tests {
		got := tt.in.Normalize()
		for i := range got.Radii {
			if !ptNear(got.Radii[i], tt.want[i], 1e-4) {
				t.Errorf("%s: radii = %v want %v", tt.name, got.Radii, tt.want)
				break
			}
		}
		// Normalize is idempotent.
		if again := got.Normalize(); again != got {
			t.Errorf("%s: not idempotent: %v then %v", tt.name, got, again)
		}
	}
	// Normalized radii never overlap.
	for _, in := range []RRect{
		{r, [4]Point{{300, 300}, {300, 300}, {300, 300}, {300, 300}}},
		{r, [4]Point{{150, 90}, {120, 20}, {80, 70}, {200, 5}}},
	} {
		n := in.Normalize()
		if n.Radii[0].X+n.Radii[1].X > 200.01 || n.Radii[3].X+n.Radii[2].X > 200.01 ||
			n.Radii[0].Y+n.Radii[3].Y > 100.01 || n.Radii[1].Y+n.Radii[2].Y > 100.01 {
			t.Errorf("overlapping radii after Normalize: %v", n.Radii)
		}
	}
}

func TestRRectNormalizeRect(t *testing.T) {
	got := RRect{Rect{10, 10, 0, 0}, [4]Point{{2, 2}, {2, 2}, {2, 2}, {2, 2}}}.Normalize()
	if got.Rect != (Rect{0, 0, 10, 10}) || got.Radii[0] != (Point{2, 2}) {
		t.Errorf("inverted rect: %+v", got)
	}
	empty := RRect{Rect{0, 0, 0, 10}, [4]Point{{2, 2}, {2, 2}, {2, 2}, {2, 2}}}.Normalize()
	if !empty.IsRect() {
		t.Errorf("empty rect should lose its radii: %+v", empty)
	}
}

func TestRRectConstructors(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	if got := NewRRect(r, 8); got.Radii != [4]Point{{8, 8}, {8, 8}, {8, 8}, {8, 8}} || got.Rect != r {
		t.Errorf("NewRRect = %+v", got)
	}
	if got := NewRRect(r, 1000); got.Radii[0] != (Point{25, 25}) {
		t.Errorf("NewRRect(1000) should clamp to a pill: %+v", got)
	}
	if got := RRectXY(r, 10, 5); got.Radii[2] != (Point{10, 5}) {
		t.Errorf("RRectXY = %+v", got)
	}
	got := RRectCorners(r, Pt(1, 1), Pt(2, 2), Pt(3, 3), Pt(4, 4))
	if got.Radii[TopLeft] != Pt(1, 1) || got.Radii[TopRight] != Pt(2, 2) || got.Radii[BottomRight] != Pt(3, 3) || got.Radii[BottomLeft] != Pt(4, 4) {
		t.Errorf("RRectCorners order wrong: %+v", got.Radii)
	}
	if !RRectFromRect(Rect{5, 5, 0, 0}).IsRect() {
		t.Error("RRectFromRect should be a rect")
	}
}

func TestRRectPredicates(t *testing.T) {
	r := Rect{0, 0, 100, 100}
	tests := []struct {
		name                      string
		rr                        RRect
		isRect, uniform, circular bool
	}{
		{"rect", RRectFromRect(r), true, true, true},
		{"uniform circular", NewRRect(r, 8), false, true, true},
		{"uniform elliptical", RRectXY(r, 8, 4), false, true, false},
		{"mixed circular", RRectCorners(r, Pt(8, 8), Pt(4, 4), Pt(0, 0), Pt(2, 2)), false, false, true},
		{"mixed elliptical", RRectCorners(r, Pt(8, 4), Pt(4, 4), Pt(0, 0), Pt(2, 2)), false, false, false},
	}
	for _, tt := range tests {
		if tt.rr.IsRect() != tt.isRect || tt.rr.IsUniform() != tt.uniform || tt.rr.IsCircular() != tt.circular {
			t.Errorf("%s: IsRect=%v IsUniform=%v IsCircular=%v", tt.name, tt.rr.IsRect(), tt.rr.IsUniform(), tt.rr.IsCircular())
		}
	}
}

func TestRRectContains(t *testing.T) {
	rr := NewRRect(Rect{0, 0, 100, 100}, 20)
	tests := []struct {
		p    Point
		want bool
	}{
		{Pt(50, 50), true},
		{Pt(0.5, 50), true},
		{Pt(1, 1), false},   // outside the TL corner arc
		{Pt(99, 1), false},  // TR
		{Pt(99, 99), false}, // BR
		{Pt(1, 99), false},  // BL
		{Pt(20, 20), true},  // corner circle center
		{Pt(5.86, 5.86), true},
		{Pt(5.8, 5.8), false}, // just outside the arc: 20-14.142 = 5.858
		{Pt(-1, 50), false},
		{Pt(100, 50), false},
	}
	for _, tt := range tests {
		if got := rr.Contains(tt.p); got != tt.want {
			t.Errorf("Contains(%v) = %v", tt.p, got)
		}
	}
	// Elliptical corner.
	el := RRectXY(Rect{0, 0, 100, 100}, 40, 10)
	if !el.Contains(Pt(40-40*0.7072, 10-10*0.7072+0.01)) || el.Contains(Pt(40-40*0.7068, 10-10*0.7068-0.2)) {
		t.Error("elliptical corner containment wrong")
	}
}

func TestRRectInset(t *testing.T) {
	rr := NewRRect(Rect{0, 0, 100, 60}, 12)
	in := rr.Inset(2)
	if in.Rect != (Rect{2, 2, 98, 58}) || in.Radii[0] != (Point{10, 10}) {
		t.Errorf("Inset(2) = %+v", in)
	}
	if in := rr.Inset(20); in.Rect != (Rect{20, 20, 80, 40}) || !in.IsRect() {
		t.Errorf("Inset(20) = %+v", in)
	}
	if out := rr.Inset(-3); out.Radii[0] != (Point{15, 15}) || out.Rect != (Rect{-3, -3, 103, 63}) {
		t.Errorf("Inset(-3) = %+v", out)
	}
	if rr.Bounds() != rr.Rect {
		t.Error("Bounds != Rect")
	}
}

func TestRRectPath(t *testing.T) {
	rr := RRectCorners(Rect{10, 20, 110, 80}, Pt(10, 10), Pt(20, 5), Pt(0, 0), Pt(15, 25))
	p := rr.Path()
	if b := p.Bounds(); !rectNear(b, rr.Rect, 2e-3) {
		t.Errorf("path bounds = %v want %v", b, rr.Rect)
	}
	if cb := p.ControlBounds(); !rectNear(cb, rr.Rect, 1e-4) {
		t.Errorf("control bounds = %v want %v", cb, rr.Rect)
	}
	v := p.Verbs()
	if v[0] != MoveTo || v[len(v)-1] != Close {
		t.Errorf("verbs = %v", v)
	}
	// The path starts at the end of the TL arc on the top edge, goes clockwise
	// (first edge heads toward +X) and passes through the exact edge points.
	pts := p.Points()
	if pts[0] != Pt(20, 20) || pts[1] != Pt(90, 20) {
		t.Errorf("first points = %v %v", pts[0], pts[1])
	}
	// Flattened points stay inside the shape (and on the rounded outline).
	p.Flatten(0.01, func(v Verb, pt Point) {
		if v == Close {
			return
		}
		inflated := RRect{rr.Rect.Outset(0.02), rr.Radii}
		for i := range inflated.Radii {
			inflated.Radii[i] = Point{inflated.Radii[i].X + 0.02, inflated.Radii[i].Y + 0.02}
		}
		if inflated.Radii[BottomRight] == (Point{0.02, 0.02}) {
			inflated.Radii[BottomRight] = Point{}
		}
		if !inflated.Contains(pt) {
			t.Errorf("flattened point %v lies outside the rounded rect", pt)
		}
	})
	// A rect with square corners is a plain rect path with 5 verbs.
	if sq := RRectFromRect(Rect{0, 0, 5, 5}).Path(); sq.Len() != 5 {
		t.Errorf("square rrect path has %d verbs", sq.Len())
	}
	// A pill (radius = half height) has no redundant straight segments on the sides.
	pill := NewRRect(Rect{0, 0, 100, 40}, 20).Path()
	for el := range pill.Elements() {
		if el.Verb == LineTo && (el.Pts[0].X == 100 || el.Pts[0].X == 0) && el.Pts[0].Y != 0 && el.Pts[0].Y != 40 {
			t.Errorf("zero-length side segment in pill path: %v", el.Pts[0])
		}
	}
	// Circle-shaped rrect equals a circle within tolerance.
	circ := NewRRect(Rect{0, 0, 100, 100}, 50).Path()
	circ.Flatten(0.01, func(v Verb, pt Point) {
		if v == Close {
			return
		}
		if d := math.Hypot(float64(pt.X)-50, float64(pt.Y)-50); math.Abs(d-50) > 0.02 {
			t.Errorf("point %v at distance %v from the center", pt, d)
		}
	})
}
