package gfx

import "math"

// Corner indexes RRect.Radii. The order is clockwise from the top-left, the
// order of CSS border-radius shorthand values.
type Corner uint8

const (
	TopLeft Corner = iota
	TopRight
	BottomRight
	BottomLeft
)

// RRect is a rectangle with a separate elliptical radius per corner.
// Radii[c] holds the horizontal (X) and vertical (Y) radius of corner c.
//
// An RRect must be normalized (see Normalize) before it is used: radii
// non-negative and not overlapping. The constructors return normalized values
// and the DisplayList Builder normalizes every RRect it receives. Methods
// other than Normalize assume a normalized value.
type RRect struct {
	Rect  Rect
	Radii [4]Point
}

// RRectFromRect returns the RRect of r with square corners.
func RRectFromRect(r Rect) RRect { return RRect{Rect: r.Abs()} }

// NewRRect returns r with the same circular radius on every corner,
// normalized.
func NewRRect(r Rect, radius float32) RRect {
	p := Point{radius, radius}
	return RRect{r, [4]Point{p, p, p, p}}.Normalize()
}

// RRectXY returns r with the same elliptical radius (rx, ry) on every corner,
// normalized.
func RRectXY(r Rect, rx, ry float32) RRect {
	p := Point{rx, ry}
	return RRect{r, [4]Point{p, p, p, p}}.Normalize()
}

// RRectCorners returns r with the given per-corner radii, normalized.
func RRectCorners(r Rect, tl, tr, br, bl Point) RRect {
	return RRect{r, [4]Point{tl, tr, br, bl}}.Normalize()
}

// Normalize returns the RRect with valid radii, following CSS Backgrounds 3
// section 5.5 ("Overlapping curves"):
//
//   - the rectangle is made upright (X0 <= X1, Y0 <= Y1);
//   - a negative, NaN or infinite radius component becomes 0, and a corner
//     with either component 0 becomes square (both 0);
//   - for each side, the sum of the two radii along it must not exceed the
//     side length: let f be the smallest ratio side/sum over the four sides;
//     if f < 1, ALL radii are multiplied by f (uniformly, so that the shape's
//     proportions are kept);
//   - an empty rectangle gets square corners.
func (rr RRect) Normalize() RRect {
	out := RRect{Rect: rr.Rect.Abs()}
	w, h := out.Rect.Width(), out.Rect.Height()
	if !(w > 0 && h > 0) {
		return out
	}
	for i, r := range rr.Radii {
		x, y := r.X, r.Y
		if !(x > 0) || !(y > 0) || !finite(x) || !finite(y) {
			x, y = 0, 0
		}
		out.Radii[i] = Point{x, y}
	}
	rd := &out.Radii
	f := 1.0
	for _, s := range [4]struct{ side, sum float64 }{
		{f64(w), f64(rd[TopLeft].X) + f64(rd[TopRight].X)},
		{f64(h), f64(rd[TopRight].Y) + f64(rd[BottomRight].Y)},
		{f64(w), f64(rd[BottomLeft].X) + f64(rd[BottomRight].X)},
		{f64(h), f64(rd[TopLeft].Y) + f64(rd[BottomLeft].Y)},
	} {
		if s.sum > s.side {
			f = math.Min(f, s.side/s.sum)
		}
	}
	if f < 1 {
		for i := range rd {
			rd[i] = Point{float32(f64(rd[i].X) * f), float32(f64(rd[i].Y) * f)}
		}
	}
	return out
}

// IsRect reports whether all corners are square.
func (rr RRect) IsRect() bool {
	for _, r := range rr.Radii {
		if r.X != 0 || r.Y != 0 {
			return false
		}
	}
	return true
}

// IsUniform reports whether all four corners have the same radii (elliptical
// or circular). Rasterizers use it to take a single-radius fast path.
func (rr RRect) IsUniform() bool {
	return rr.Radii[1] == rr.Radii[0] && rr.Radii[2] == rr.Radii[0] && rr.Radii[3] == rr.Radii[0]
}

// IsCircular reports whether every corner is circular (RX == RY); different
// corners may still have different radii. IsUniform && IsCircular is the
// common "rounded-lg" case.
func (rr RRect) IsCircular() bool {
	for _, r := range rr.Radii {
		if r.X != r.Y {
			return false
		}
	}
	return true
}

// Bounds returns the bounding rectangle.
func (rr RRect) Bounds() Rect { return rr.Rect }

// IsFinite reports whether the rectangle and the radii are finite.
func (rr RRect) IsFinite() bool {
	if !rr.Rect.IsFinite() {
		return false
	}
	for _, r := range rr.Radii {
		if !r.IsFinite() {
			return false
		}
	}
	return true
}

// Inset returns the RRect shrunk by d on every side, with each radius
// component reduced by d and clamped at 0. This is the CSS rule for the inner
// edge of a border of width d (border box to padding box). It is not the CSS
// box-shadow spread rule, which the rasterizer implements; for a negative d
// the radii are simply increased by -d.
func (rr RRect) Inset(d float32) RRect {
	out := RRect{Rect: rr.Rect.Inset(d)}
	for i, r := range rr.Radii {
		out.Radii[i] = Point{max(r.X-d, 0), max(r.Y-d, 0)}
	}
	return out.Normalize()
}

// Contains reports whether p is inside the rounded rectangle.
func (rr RRect) Contains(p Point) bool {
	r := rr.Rect
	if !r.Contains(p) {
		return false
	}
	// Corner circle/ellipse centers and the sign of the corner quadrant.
	type corner struct {
		c      Point
		sx, sy float32
	}
	rd := rr.Radii
	for i, k := range [4]corner{
		{Point{r.X0 + rd[TopLeft].X, r.Y0 + rd[TopLeft].Y}, -1, -1},
		{Point{r.X1 - rd[TopRight].X, r.Y0 + rd[TopRight].Y}, 1, -1},
		{Point{r.X1 - rd[BottomRight].X, r.Y1 - rd[BottomRight].Y}, 1, 1},
		{Point{r.X0 + rd[BottomLeft].X, r.Y1 - rd[BottomLeft].Y}, -1, 1},
	} {
		rad := rd[i]
		if rad.X == 0 || rad.Y == 0 {
			continue
		}
		dx, dy := (p.X-k.c.X)*k.sx, (p.Y-k.c.Y)*k.sy
		if dx > 0 && dy > 0 { // in the corner quadrant
			nx, ny := dx/rad.X, dy/rad.Y
			if nx*nx+ny*ny > 1 {
				return false
			}
		}
	}
	return true
}

// AddRRect appends the rounded rectangle as a closed subpath, clockwise on
// screen starting at the end of the top-left corner on the top edge.
// Corners are built from cubic Beziers within 0.01 logical pixels of the
// exact ellipse; a rectangle with square corners becomes a plain rect.
func (p *Path) AddRRect(rr RRect) {
	if rr.IsRect() {
		p.AddRect(rr.Rect)
		return
	}
	r, rd := rr.Rect, rr.Radii
	tl, tr, br, bl := rd[TopLeft], rd[TopRight], rd[BottomRight], rd[BottomLeft]
	p.MoveTo(Point{r.X0 + tl.X, r.Y0})
	p.lineToIfMoved(Point{r.X1 - tr.X, r.Y0})
	p.addCorner(Point{r.X1 - tr.X, r.Y0 + tr.Y}, tr, -math.Pi/2, Point{r.X1, r.Y0 + tr.Y})
	p.lineToIfMoved(Point{r.X1, r.Y1 - br.Y})
	p.addCorner(Point{r.X1 - br.X, r.Y1 - br.Y}, br, 0, Point{r.X1 - br.X, r.Y1})
	p.lineToIfMoved(Point{r.X0 + bl.X, r.Y1})
	p.addCorner(Point{r.X0 + bl.X, r.Y1 - bl.Y}, bl, math.Pi/2, Point{r.X0, r.Y1 - bl.Y})
	p.lineToIfMoved(Point{r.X0, r.Y0 + tl.Y})
	p.addCorner(Point{r.X0 + tl.X, r.Y0 + tl.Y}, tl, math.Pi, Point{r.X0 + tl.X, r.Y0})
	p.Close()
}

// lineToIfMoved adds a LineTo unless pt is the current point.
func (p *Path) lineToIfMoved(pt Point) {
	if cur, _ := p.CurrentPoint(); cur != pt {
		p.LineTo(pt)
	}
}

// addCorner appends the quarter ellipse of a corner, starting at the current
// point, ending exactly at end.
func (p *Path) addCorner(center, radii Point, startAngle float64, end Point) {
	if radii.X == 0 || radii.Y == 0 {
		p.lineToIfMoved(end)
		return
	}
	n := len(p.verbs)
	p.addArcCubics(Arc{Center: center, Radii: radii, StartAngle: float32(startAngle), SweepAngle: math.Pi / 2}, defaultArcTolerance)
	if len(p.verbs) > n { // pin the last point to the exact value
		p.pts[len(p.pts)-1] = end
	}
}

// Path returns the outline as a closed path; see Path.AddRRect.
func (rr RRect) Path() Path {
	var p Path
	p.AddRRect(rr)
	return p
}
