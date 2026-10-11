package gfx

import (
	"image"
	"math"
)

// Point is a position in logical pixels. It doubles as a 2D vector: Add, Sub,
// Scale, Dot, Cross and Len treat it as one.
type Point struct{ X, Y float32 }

// Pt returns Point{x, y}.
func Pt(x, y float32) Point { return Point{x, y} }

// Add returns p + q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p - q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Scale returns p multiplied by s.
func (p Point) Scale(s float32) Point { return Point{p.X * s, p.Y * s} }

// Neg returns -p.
func (p Point) Neg() Point { return Point{-p.X, -p.Y} }

// Dot returns the dot product of p and q.
func (p Point) Dot(q Point) float32 { return p.X*q.X + p.Y*q.Y }

// Cross returns the z component of the cross product of p and q
// (p.X*q.Y - p.Y*q.X).
func (p Point) Cross(q Point) float32 { return p.X*q.Y - p.Y*q.X }

// Len returns the Euclidean length of p.
func (p Point) Len() float32 { return float32(math.Hypot(float64(p.X), float64(p.Y))) }

// Len2 returns the squared length of p.
func (p Point) Len2() float32 { return p.X*p.X + p.Y*p.Y }

// Dist returns the distance between p and q.
func (p Point) Dist(q Point) float32 { return p.Sub(q).Len() }

// Normalize returns p scaled to unit length. The zero vector is returned
// unchanged.
func (p Point) Normalize() Point {
	l := p.Len()
	if l == 0 {
		return p
	}
	return Point{p.X / l, p.Y / l}
}

// Lerp returns the linear interpolation p + (q-p)*t.
func (p Point) Lerp(q Point, t float32) Point {
	return Point{p.X + (q.X-p.X)*t, p.Y + (q.Y-p.Y)*t}
}

// IsFinite reports whether both coordinates are neither NaN nor infinite.
func (p Point) IsFinite() bool { return finite(p.X) && finite(p.Y) }

// Size is a width and height in logical pixels.
type Size struct{ W, H float32 }

// Empty reports whether the size has no area (W <= 0 or H <= 0 or NaN).
func (s Size) Empty() bool { return !(s.W > 0 && s.H > 0) }

// Rect is an axis-aligned rectangle given by its corners, in the style of
// kurbo: (X0, Y0) is the top-left corner and (X1, Y1) the bottom-right one in
// the y-down coordinate system. A Rect is half-open: it contains the points
// with X0 <= x < X1 and Y0 <= y < Y1.
//
// A Rect with X1 <= X0 or Y1 <= Y0 is empty. The zero Rect is empty.
type Rect struct{ X0, Y0, X1, Y1 float32 }

// RectXYWH returns the rectangle with origin (x, y) and the given size.
func RectXYWH(x, y, w, h float32) Rect { return Rect{x, y, x + w, y + h} }

// RectFromPoints returns the smallest rectangle containing both points. The
// result is normalized even if p0 is below or to the right of p1.
func RectFromPoints(p0, p1 Point) Rect {
	return Rect{min(p0.X, p1.X), min(p0.Y, p1.Y), max(p0.X, p1.X), max(p0.Y, p1.Y)}
}

// RectFromOriginSize returns the rectangle with the given origin and size.
func RectFromOriginSize(o Point, s Size) Rect { return Rect{o.X, o.Y, o.X + s.W, o.Y + s.H} }

// RectFromImage converts an image.Rectangle to a Rect.
func RectFromImage(r image.Rectangle) Rect {
	return Rect{float32(r.Min.X), float32(r.Min.Y), float32(r.Max.X), float32(r.Max.Y)}
}

// infiniteRect is the clip rectangle used while no clip is active. It is
// large but finite so that arithmetic on it never produces NaN.
var infiniteRect = Rect{-1e30, -1e30, 1e30, 1e30}

// Width returns X1 - X0 (negative for an inverted rectangle).
func (r Rect) Width() float32 { return r.X1 - r.X0 }

// Height returns Y1 - Y0 (negative for an inverted rectangle).
func (r Rect) Height() float32 { return r.Y1 - r.Y0 }

// Size returns the width and height.
func (r Rect) Size() Size { return Size{r.Width(), r.Height()} }

// Origin returns the top-left corner.
func (r Rect) Origin() Point { return Point{r.X0, r.Y0} }

// Center returns the center point.
func (r Rect) Center() Point { return Point{(r.X0 + r.X1) / 2, (r.Y0 + r.Y1) / 2} }

// Area returns Width*Height, or 0 for an empty rectangle.
func (r Rect) Area() float32 {
	if r.Empty() {
		return 0
	}
	return r.Width() * r.Height()
}

// Empty reports whether the rectangle has no area: Width <= 0, Height <= 0 or
// a NaN coordinate.
func (r Rect) Empty() bool { return !(r.X1 > r.X0 && r.Y1 > r.Y0) }

// IsFinite reports whether every coordinate is finite.
func (r Rect) IsFinite() bool { return finite(r.X0) && finite(r.Y0) && finite(r.X1) && finite(r.Y1) }

// Abs returns the rectangle with its corners swapped, if needed, so that
// X0 <= X1 and Y0 <= Y1.
func (r Rect) Abs() Rect {
	return Rect{min(r.X0, r.X1), min(r.Y0, r.Y1), max(r.X0, r.X1), max(r.Y0, r.Y1)}
}

// Union returns the smallest rectangle containing both r and s. Empty
// rectangles are ignored, so that the zero Rect is the identity of Union.
func (r Rect) Union(s Rect) Rect {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	return Rect{min(r.X0, s.X0), min(r.Y0, s.Y0), max(r.X1, s.X1), max(r.Y1, s.Y1)}
}

// Intersect returns the overlap of r and s. If they do not overlap the result
// is the empty zero Rect.
func (r Rect) Intersect(s Rect) Rect {
	o := Rect{max(r.X0, s.X0), max(r.Y0, s.Y0), min(r.X1, s.X1), min(r.Y1, s.Y1)}
	if o.Empty() {
		return Rect{}
	}
	return o
}

// Intersects reports whether r and s overlap with non-zero area.
func (r Rect) Intersects(s Rect) bool {
	return max(r.X0, s.X0) < min(r.X1, s.X1) && max(r.Y0, s.Y0) < min(r.Y1, s.Y1)
}

// Inset returns r shrunk by d on every side. A negative d grows the
// rectangle. The result may be empty.
func (r Rect) Inset(d float32) Rect { return Rect{r.X0 + d, r.Y0 + d, r.X1 - d, r.Y1 - d} }

// InsetEdges returns r with the left, top, right and bottom edges moved
// inwards by the given amounts (note the order follows the Rect fields, not
// the CSS top-right-bottom-left order). Negative amounts move edges outwards.
func (r Rect) InsetEdges(left, top, right, bottom float32) Rect {
	return Rect{r.X0 + left, r.Y0 + top, r.X1 - right, r.Y1 - bottom}
}

// Outset returns r grown by d on every side. It is Inset(-d).
func (r Rect) Outset(d float32) Rect { return r.Inset(-d) }

// Translate returns r moved by (dx, dy).
func (r Rect) Translate(dx, dy float32) Rect {
	return Rect{r.X0 + dx, r.Y0 + dy, r.X1 + dx, r.Y1 + dy}
}

// Contains reports whether p lies inside r (half-open, see Rect).
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X0 && p.X < r.X1 && p.Y >= r.Y0 && p.Y < r.Y1
}

// ContainsRect reports whether s lies entirely inside r. An empty s is
// contained by any r.
func (r Rect) ContainsRect(s Rect) bool {
	if s.Empty() {
		return true
	}
	return s.X0 >= r.X0 && s.Y0 >= r.Y0 && s.X1 <= r.X1 && s.Y1 <= r.Y1
}

// RoundOut returns the smallest image.Rectangle of whole pixels containing r.
// Coordinates are clamped to the int32 range so that huge or non-finite
// values cannot overflow.
func (r Rect) RoundOut() image.Rectangle {
	return image.Rect(
		clampInt(math.Floor(float64(r.X0))), clampInt(math.Floor(float64(r.Y0))),
		clampInt(math.Ceil(float64(r.X1))), clampInt(math.Ceil(float64(r.Y1))),
	)
}

func clampInt(v float64) int {
	switch {
	case v != v:
		return 0
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	}
	return int(v)
}

func finite(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}
