package gfx

import (
	"image"
	"math"
	"testing"
)

func near(a, b, eps float32) bool { return float32(math.Abs(float64(a-b))) <= eps }

func ptNear(a, b Point, eps float32) bool { return near(a.X, b.X, eps) && near(a.Y, b.Y, eps) }

func rectNear(a, b Rect, eps float32) bool {
	return near(a.X0, b.X0, eps) && near(a.Y0, b.Y0, eps) && near(a.X1, b.X1, eps) && near(a.Y1, b.Y1, eps)
}

func TestPoint(t *testing.T) {
	p, q := Pt(3, 4), Pt(1, 2)
	tests := []struct {
		name string
		got  Point
		want Point
	}{
		{"add", p.Add(q), Pt(4, 6)},
		{"sub", p.Sub(q), Pt(2, 2)},
		{"scale", p.Scale(2), Pt(6, 8)},
		{"neg", p.Neg(), Pt(-3, -4)},
		{"lerp", p.Lerp(q, 0.5), Pt(2, 3)},
		{"normalize", p.Normalize(), Pt(0.6, 0.8)},
		{"normalize zero", Point{}.Normalize(), Point{}},
	}
	for _, tt := range tests {
		if !ptNear(tt.got, tt.want, 1e-6) {
			t.Errorf("%s: got %v want %v", tt.name, tt.got, tt.want)
		}
	}
	if p.Len() != 5 || p.Len2() != 25 || p.Dot(q) != 11 || p.Cross(q) != 2 || p.Dist(q) != float32(math.Sqrt(8)) {
		t.Errorf("Len/Dot/Cross/Dist wrong: %v %v %v %v %v", p.Len(), p.Len2(), p.Dot(q), p.Cross(q), p.Dist(q))
	}
	if Pt(float32(math.NaN()), 0).IsFinite() || Pt(0, float32(math.Inf(1))).IsFinite() || !p.IsFinite() {
		t.Error("IsFinite wrong")
	}
}

func TestRectBasics(t *testing.T) {
	r := RectXYWH(10, 20, 30, 40)
	if r != (Rect{10, 20, 40, 60}) {
		t.Fatalf("RectXYWH = %v", r)
	}
	if r.Width() != 30 || r.Height() != 40 || r.Area() != 1200 || r.Center() != Pt(25, 40) {
		t.Errorf("dims wrong")
	}
	if r.Size() != (Size{30, 40}) || r.Origin() != Pt(10, 20) {
		t.Errorf("size/origin wrong")
	}
	if got := RectFromPoints(Pt(5, 9), Pt(1, 2)); got != (Rect{1, 2, 5, 9}) {
		t.Errorf("RectFromPoints = %v", got)
	}
	if got := RectFromOriginSize(Pt(1, 2), Size{3, 4}); got != (Rect{1, 2, 4, 6}) {
		t.Errorf("RectFromOriginSize = %v", got)
	}
	if got := RectFromImage(image.Rect(1, 2, 3, 4)); got != (Rect{1, 2, 3, 4}) {
		t.Errorf("RectFromImage = %v", got)
	}
	if got := (Rect{5, 5, 1, 1}).Abs(); got != (Rect{1, 1, 5, 5}) {
		t.Errorf("Abs = %v", got)
	}
}

func TestRectEmpty(t *testing.T) {
	nan := float32(math.NaN())
	tests := []struct {
		r    Rect
		want bool
	}{
		{Rect{}, true},
		{Rect{0, 0, 1, 1}, false},
		{Rect{0, 0, 0, 5}, true},
		{Rect{0, 0, 5, 0}, true},
		{Rect{5, 0, 1, 1}, true},
		{Rect{0, 0, nan, 1}, true},
	}
	for _, tt := range tests {
		if tt.r.Empty() != tt.want {
			t.Errorf("%v.Empty() = %v", tt.r, !tt.want)
		}
	}
	if (Rect{3, 3, 1, 1}).Area() != 0 {
		t.Error("Area of inverted rect should be 0")
	}
}

func TestRectSetOps(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	b := Rect{5, 5, 20, 8}
	c := Rect{20, 20, 30, 30}
	tests := []struct {
		name string
		got  Rect
		want Rect
	}{
		{"union", a.Union(b), Rect{0, 0, 20, 10}},
		{"union with empty receiver", Rect{}.Union(b), b},
		{"union with empty arg", a.Union(Rect{}), a},
		{"intersect", a.Intersect(b), Rect{5, 5, 10, 8}},
		{"intersect disjoint", a.Intersect(c), Rect{}},
		{"intersect touching", a.Intersect(Rect{10, 0, 20, 10}), Rect{}},
		{"inset", a.Inset(2), Rect{2, 2, 8, 8}},
		{"inset collapses", a.Inset(6), Rect{6, 6, 4, 4}},
		{"outset", a.Outset(2), Rect{-2, -2, 12, 12}},
		{"insetEdges", a.InsetEdges(1, 2, 3, 4), Rect{1, 2, 7, 6}},
		{"translate", a.Translate(3, -3), Rect{3, -3, 13, 7}},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v want %v", tt.name, tt.got, tt.want)
		}
	}
	if !a.Intersects(b) || a.Intersects(c) || a.Intersects(Rect{10, 0, 20, 10}) {
		t.Error("Intersects wrong")
	}
	if !a.ContainsRect(Rect{1, 1, 9, 9}) || a.ContainsRect(b) || !a.ContainsRect(Rect{}) {
		t.Error("ContainsRect wrong")
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{0, 0, 10, 10}
	tests := []struct {
		p    Point
		want bool
	}{
		{Pt(0, 0), true}, {Pt(5, 5), true}, {Pt(9.999, 9.999), true},
		{Pt(10, 5), false}, {Pt(5, 10), false}, {Pt(-0.001, 5), false},
	}
	for _, tt := range tests {
		if r.Contains(tt.p) != tt.want {
			t.Errorf("Contains(%v) = %v", tt.p, !tt.want)
		}
	}
}

func TestRectRoundOut(t *testing.T) {
	tests := []struct {
		r    Rect
		want image.Rectangle
	}{
		{Rect{0.2, 0.7, 9.1, 9.9}, image.Rect(0, 0, 10, 10)},
		{Rect{-0.5, -1.5, 2, 3}, image.Rect(-1, -2, 2, 3)},
		{Rect{1, 1, 2, 2}, image.Rect(1, 1, 2, 2)},
		{Rect{-1e30, -1e30, 1e30, 1e30}, image.Rect(math.MinInt32, math.MinInt32, math.MaxInt32, math.MaxInt32)},
	}
	for _, tt := range tests {
		if got := tt.r.RoundOut(); got != tt.want {
			t.Errorf("RoundOut(%v) = %v want %v", tt.r, got, tt.want)
		}
	}
}

func TestSizeEmpty(t *testing.T) {
	if !(Size{0, 5}).Empty() || (Size{1, 1}).Empty() || !(Size{-1, 5}).Empty() {
		t.Error("Size.Empty wrong")
	}
}

func TestAffineComposition(t *testing.T) {
	p := Pt(1, 1)
	s := Scale(2, 2)
	tr := Translate(10, 0)
	// Mul: right operand applies first.
	if got := tr.Mul(s).Apply(p); got != Pt(12, 2) {
		t.Errorf("Translate.Mul(Scale)(1,1) = %v, want (12,2): the scale applies first", got)
	}
	if got := s.Mul(tr).Apply(p); got != Pt(22, 2) {
		t.Errorf("Scale.Mul(Translate)(1,1) = %v, want (22,2): the translation applies first", got)
	}
	// Then: left operand applies first.
	if got := s.Then(tr).Apply(p); got != Pt(12, 2) {
		t.Errorf("Scale.Then(Translate)(1,1) = %v, want (12,2)", got)
	}
	// Associativity of Mul.
	a, b, c := Rotate(0.3), Translate(5, -2), Scale(1.5, 0.5)
	l := a.Mul(b).Mul(c).Apply(Pt(3, 4))
	r := a.Mul(b.Mul(c)).Apply(Pt(3, 4))
	if !ptNear(l, r, 1e-4) {
		t.Errorf("Mul is not associative: %v vs %v", l, r)
	}
	// Mul matches nested application.
	if got, want := a.Mul(b).Apply(Pt(3, 4)), a.Apply(b.Apply(Pt(3, 4))); !ptNear(got, want, 1e-5) {
		t.Errorf("a.Mul(b).Apply(p) = %v, a(b(p)) = %v", got, want)
	}
	if Identity.Mul(a) != a || a.Mul(Identity) != a {
		t.Error("Identity is not neutral for Mul")
	}
}

func TestAffineConstructors(t *testing.T) {
	tests := []struct {
		name string
		a    Affine
		in   Point
		want Point
	}{
		{"translate", Translate(3, 4), Pt(1, 1), Pt(4, 5)},
		{"scale", Scale(2, 3), Pt(1, 1), Pt(2, 3)},
		{"scale about", ScaleAbout(2, 2, Pt(10, 10)), Pt(11, 11), Pt(12, 12)},
		{"rotate 90 clockwise on screen", Rotate(math.Pi / 2), Pt(1, 0), Pt(0, 1)},
		{"rotate 90 of down", Rotate(math.Pi / 2), Pt(0, 1), Pt(-1, 0)},
		{"rotate about", RotateAbout(math.Pi/2, Pt(10, 10)), Pt(11, 10), Pt(10, 11)},
		{"skew x 45", Skew(math.Pi/4, 0), Pt(0, 1), Pt(1, 1)},
		{"skew y 45", Skew(0, math.Pi/4), Pt(1, 0), Pt(1, 1)},
	}
	for _, tt := range tests {
		if got := tt.a.Apply(tt.in); !ptNear(got, tt.want, 1e-5) {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
	if got := Translate(5, 6).ApplyVec(Pt(1, 2)); got != Pt(1, 2) {
		t.Errorf("ApplyVec must ignore translation, got %v", got)
	}
}

func TestAffineInvert(t *testing.T) {
	cases := []Affine{
		Translate(3, -7), Scale(2, 4), Rotate(0.7), Skew(0.3, 0.1),
		Translate(5, 6).Mul(Rotate(1.1)).Mul(Scale(3, 0.5)),
	}
	for _, a := range cases {
		inv, ok := a.Invert()
		if !ok {
			t.Fatalf("%v should be invertible", a)
		}
		for _, p := range []Point{{0, 0}, {1, 2}, {-30, 17.5}} {
			if got := inv.Apply(a.Apply(p)); !ptNear(got, p, 1e-3) {
				t.Errorf("inverse of %v: %v -> %v", a, p, got)
			}
		}
	}
	if inv, ok := Scale(0, 1).Invert(); ok || inv != Identity {
		t.Errorf("singular matrix: ok=%v inv=%v", ok, inv)
	}
	nan := float32(math.NaN())
	if _, ok := (Affine{nan, 0, 0, 1, 0, 0}).Invert(); ok {
		t.Error("NaN matrix must not invert")
	}
}

func TestAffineTransformRect(t *testing.T) {
	r := Rect{0, 0, 10, 20}
	tests := []struct {
		name string
		a    Affine
		want Rect
	}{
		{"identity", Identity, r},
		{"translate", Translate(5, -5), Rect{5, -5, 15, 15}},
		{"scale", Scale(2, 0.5), Rect{0, 0, 20, 10}},
		{"flip x", Scale(-1, 1), Rect{-10, 0, 0, 20}},
		{"rotate 90", Rotate(math.Pi / 2), Rect{-20, 0, 0, 10}},
	}
	for _, tt := range tests {
		if got := tt.a.TransformRect(r); !rectNear(got, tt.want, 1e-4) {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
	// 45 degrees: corners (0,0),(10,0),(10,20),(0,20) map to
	// (0,0),(7.07,7.07),(-7.07,21.2),(-14.1,14.1).
	got := Rotate(math.Pi / 4).TransformRect(r)
	want := Rect{-14.142136, 0, 7.0710678, 21.213203}
	if !rectNear(got, want, 1e-4) {
		t.Errorf("rotate 45: got %v want %v", got, want)
	}
}

func TestAffinePredicates(t *testing.T) {
	tests := []struct {
		name                                          string
		a                                             Affine
		identity, translate, scaleTranslate, axisAlig bool
	}{
		{"identity", Identity, true, true, true, true},
		{"translate", Translate(1, 2), false, true, true, true},
		{"scale", Scale(2, 3), false, false, true, true},
		{"flip", Scale(-1, 1), false, false, true, true},
		{"rotate 90", Affine{0, 1, -1, 0, 0, 0}, false, false, false, true},
		{"rotate 45", Rotate(math.Pi / 4), false, false, false, false},
		{"skew", Skew(0.2, 0), false, false, false, false},
	}
	for _, tt := range tests {
		if tt.a.IsIdentity() != tt.identity || tt.a.IsTranslate() != tt.translate ||
			tt.a.IsScaleTranslate() != tt.scaleTranslate || tt.a.IsAxisAligned() != tt.axisAlig {
			t.Errorf("%s: predicates = %v %v %v %v", tt.name,
				tt.a.IsIdentity(), tt.a.IsTranslate(), tt.a.IsScaleTranslate(), tt.a.IsAxisAligned())
		}
	}
}

func TestAffineMaxScale(t *testing.T) {
	tests := []struct {
		a    Affine
		want float32
	}{
		{Identity, 1}, {Scale(3, 2), 3}, {Rotate(0.9), 1}, {Scale(2, 5).Mul(Rotate(0.4)), 5},
		{Translate(100, 100), 1}, {Skew(math.Pi/4, 0), 1.618034},
	}
	for _, tt := range tests {
		if got := tt.a.MaxScale(); !near(got, tt.want, 1e-4) {
			t.Errorf("MaxScale(%v) = %v want %v", tt.a, got, tt.want)
		}
	}
	if (Affine{float32(math.NaN()), 0, 0, 1, 0, 0}).MaxScale() != 0 {
		t.Error("non-finite MaxScale should be 0")
	}
	if d := Scale(2, 3).Det(); d != 6 {
		t.Errorf("Det = %v", d)
	}
}
