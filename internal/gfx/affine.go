package gfx

import "math"

// Affine is a 2D affine transform stored as the six numbers [a b c d e f] of
// the matrix
//
//	| a c e |
//	| b d f |
//	| 0 0 1 |
//
// (kurbo's order). A point is transformed as column vector:
//
//	x' = a*x + c*y + e
//	y' = b*x + d*y + f
//
// The coordinate system is y-down, so a positive Rotate angle turns clockwise
// on screen.
type Affine [6]float32

// Identity is the transform that leaves every point unchanged.
var Identity = Affine{1, 0, 0, 1, 0, 0}

// Translate returns the transform that moves points by (dx, dy).
func Translate(dx, dy float32) Affine { return Affine{1, 0, 0, 1, dx, dy} }

// Scale returns the transform that scales by sx horizontally and sy
// vertically around the origin.
func Scale(sx, sy float32) Affine { return Affine{sx, 0, 0, sy, 0, 0} }

// ScaleAbout returns the scale transform whose fixed point is c.
func ScaleAbout(sx, sy float32, c Point) Affine {
	return Affine{sx, 0, 0, sy, c.X - float32(sx*c.X), c.Y - float32(sy*c.Y)}
}

// Rotate returns the rotation by angle radians around the origin. In the
// y-down coordinate system a positive angle turns clockwise on screen.
func Rotate(angle float32) Affine {
	s, c := math.Sincos(float64(angle))
	return Affine{float32(c), float32(s), float32(-s), float32(c), 0, 0}
}

// RotateAbout returns the rotation by angle radians around c.
func RotateAbout(angle float32, c Point) Affine {
	return Translate(c.X, c.Y).Mul(Rotate(angle)).Mul(Translate(-c.X, -c.Y))
}

// Skew returns the transform x' = x + tan(ax)*y, y' = y + tan(ay)*x, with the
// angles in radians (CSS skewX and skewY).
func Skew(ax, ay float32) Affine {
	return Affine{1, float32(math.Tan(float64(ay))), float32(math.Tan(float64(ax))), 1, 0, 0}
}

// Mul returns the matrix product a·b. Applying the result to a point applies
// b first and a second:
//
//	a.Mul(b).Apply(p) == a.Apply(b.Apply(p))
//
// This is kurbo's `a * b`. In a display list, PushTransform{T} inside a
// context whose current transform is C yields the new current transform
// C.Mul(T): T applies to the local coordinates first, then C maps them to the
// root.
//
// The product is computed in float64 and rounded once.
func (a Affine) Mul(b Affine) Affine {
	a0, a1, a2, a3, a4, a5 := f64(a[0]), f64(a[1]), f64(a[2]), f64(a[3]), f64(a[4]), f64(a[5])
	b0, b1, b2, b3, b4, b5 := f64(b[0]), f64(b[1]), f64(b[2]), f64(b[3]), f64(b[4]), f64(b[5])
	return Affine{
		float32(a0*b0 + a2*b1),
		float32(a1*b0 + a3*b1),
		float32(a0*b2 + a2*b3),
		float32(a1*b2 + a3*b3),
		float32(a0*b4 + a2*b5 + a4),
		float32(a1*b4 + a3*b5 + a5),
	}
}

// Then returns the transform that applies a first and b second. It is
// b.Mul(a), spelled in reading order:
//
//	Scale(2, 2).Then(Translate(10, 0)).Apply(Pt(1, 1)) == Pt(12, 2)
func (a Affine) Then(b Affine) Affine { return b.Mul(a) }

// Det returns the determinant of the linear part.
func (a Affine) Det() float32 { return float32(f64(a[0])*f64(a[3]) - f64(a[1])*f64(a[2])) }

// Invert returns the inverse transform. ok is false, and the result is
// Identity, when the matrix is singular or not finite.
func (a Affine) Invert() (inv Affine, ok bool) {
	a0, a1, a2, a3, a4, a5 := f64(a[0]), f64(a[1]), f64(a[2]), f64(a[3]), f64(a[4]), f64(a[5])
	det := a0*a3 - a1*a2
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) || !a.IsFinite() {
		return Identity, false
	}
	r := 1 / det
	inv = Affine{
		float32(a3 * r),
		float32(-a1 * r),
		float32(-a2 * r),
		float32(a0 * r),
		float32((a2*a5 - a3*a4) * r),
		float32((a1*a4 - a0*a5) * r),
	}
	if !inv.IsFinite() {
		return Identity, false
	}
	return inv, true
}

// Apply returns the transformed point. The explicit float32 conversions
// round each product, so the compiler cannot fuse it with the addition
// (ADR-027).
func (a Affine) Apply(p Point) Point {
	return Point{
		float32(a[0]*p.X) + float32(a[2]*p.Y) + a[4],
		float32(a[1]*p.X) + float32(a[3]*p.Y) + a[5],
	}
}

// ApplyVec transforms p as a vector: the translation part is ignored.
func (a Affine) ApplyVec(p Point) Point {
	return Point{float32(a[0]*p.X) + float32(a[2]*p.Y), float32(a[1]*p.X) + float32(a[3]*p.Y)}
}

// TransformRect returns the axis-aligned bounding rectangle of r transformed
// by a. For axis-aligned transforms (see IsAxisAligned) the result is exactly
// the transformed rectangle.
func (a Affine) TransformRect(r Rect) Rect {
	if a.IsScaleTranslate() {
		x0 := float32(a[0]*r.X0) + a[4]
		x1 := float32(a[0]*r.X1) + a[4]
		y0 := float32(a[3]*r.Y0) + a[5]
		y1 := float32(a[3]*r.Y1) + a[5]
		return Rect{min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)}
	}
	p0 := a.Apply(Point{r.X0, r.Y0})
	p1 := a.Apply(Point{r.X1, r.Y0})
	p2 := a.Apply(Point{r.X1, r.Y1})
	p3 := a.Apply(Point{r.X0, r.Y1})
	return Rect{
		min(p0.X, p1.X, p2.X, p3.X), min(p0.Y, p1.Y, p2.Y, p3.Y),
		max(p0.X, p1.X, p2.X, p3.X), max(p0.Y, p1.Y, p2.Y, p3.Y),
	}
}

// IsIdentity reports whether a is exactly Identity.
func (a Affine) IsIdentity() bool { return a == Identity }

// IsTranslate reports whether a is a pure translation (the linear part is
// exactly the identity). Pixel-aligned blits are possible only then.
func (a Affine) IsTranslate() bool { return a[0] == 1 && a[1] == 0 && a[2] == 0 && a[3] == 1 }

// IsScaleTranslate reports whether a maps axis-aligned rectangles to
// axis-aligned rectangles without swapping axes: b == c == 0. It includes
// IsTranslate and IsIdentity, and negative scales (flips).
func (a Affine) IsScaleTranslate() bool { return a[1] == 0 && a[2] == 0 }

// IsAxisAligned reports whether a maps axis-aligned rectangles to
// axis-aligned rectangles: IsScaleTranslate, or a rotation by a multiple of
// 90 degrees (a == d == 0), with any scale.
func (a Affine) IsAxisAligned() bool {
	return (a[1] == 0 && a[2] == 0) || (a[0] == 0 && a[3] == 0)
}

// IsFinite reports whether every element is finite.
func (a Affine) IsFinite() bool {
	for _, v := range a {
		if !finite(v) {
			return false
		}
	}
	return true
}

// MaxScale returns the largest singular value of the linear part: the
// greatest factor by which a stretches any vector. It is used to convert
// lengths from local to root space conservatively (blur radii, stroke
// outsets). It is 0 for a non-finite matrix.
func (a Affine) MaxScale() float32 {
	if !a.IsFinite() {
		return 0
	}
	a0, a1, a2, a3 := f64(a[0]), f64(a[1]), f64(a[2]), f64(a[3])
	p := a0*a0 + a1*a1 + a2*a2 + a3*a3
	det := a0*a3 - a1*a2
	disc := float64(p*p) - float64(4*det*det)
	if disc < 0 {
		disc = 0
	}
	return float32(math.Sqrt((p + math.Sqrt(disc)) / 2))
}

func f64(v float32) float64 { return float64(v) }
