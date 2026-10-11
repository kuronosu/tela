package gfx

import (
	"iter"
	"math"
)

// Verb is a path segment kind.
type Verb uint8

// Path verbs. The number of points each consumes is given by Verb.NumPoints.
const (
	MoveTo  Verb = iota // starts a new subpath at 1 point
	LineTo              // straight line to 1 point
	QuadTo              // quadratic Bezier: 1 control point and 1 end point
	CubicTo             // cubic Bezier: 2 control points and 1 end point
	Close               // closes the current subpath with a straight line; 0 points
)

// NumPoints returns how many points the verb consumes.
func (v Verb) NumPoints() int {
	switch v {
	case MoveTo, LineTo:
		return 1
	case QuadTo:
		return 2
	case CubicTo:
		return 3
	}
	return 0
}

// String returns the verb name.
func (v Verb) String() string {
	switch v {
	case MoveTo:
		return "MoveTo"
	case LineTo:
		return "LineTo"
	case QuadTo:
		return "QuadTo"
	case CubicTo:
		return "CubicTo"
	case Close:
		return "Close"
	}
	return "Verb(?)"
}

// FillRule selects which regions a self-overlapping path fills.
type FillRule uint8

const (
	// NonZero fills a point if the path winds around it a non-zero number of
	// times. It is the zero value, like SVG's and CSS's default.
	NonZero FillRule = iota
	// EvenOdd fills a point if a ray from it crosses the path an odd number
	// of times.
	EvenOdd
)

// String returns "nonzero" or "evenodd".
func (r FillRule) String() string {
	if r == EvenOdd {
		return "evenodd"
	}
	return "nonzero"
}

// Path is a sequence of subpaths made of lines, quadratic and cubic Beziers,
// stored compactly as a verb slice plus a point slice (the points of all
// verbs, in order; see Verb.NumPoints).
//
// The zero Path is empty and ready to use. Builder methods have pointer
// receivers and append in place. Path values copy the slice headers, not the
// data: after `q := p`, appending to both can corrupt one of them. Use Clone
// to get an independent copy. Every Path stored in a DisplayList is cloned
// by the Builder and must be treated as read-only afterwards.
//
// Invariants kept by the builders: the first verb is MoveTo, and every
// subpath starts with MoveTo. Drawing after Close without a new MoveTo starts
// the next subpath at the previous subpath's start point (an implicit MoveTo
// is inserted), as in SVG and Canvas. Consecutive MoveTos collapse to the last.
type Path struct {
	verbs    []Verb
	pts      []Point
	start    Point // start point of the current subpath
	needMove bool  // true after Close: the next drawing verb emits an implicit MoveTo(start)
}

// Clone returns a deep copy of p.
func (p Path) Clone() Path {
	q := Path{start: p.start, needMove: p.needMove}
	if len(p.verbs) > 0 {
		q.verbs = append([]Verb(nil), p.verbs...)
		q.pts = append([]Point(nil), p.pts...)
	}
	return q
}

// IsEmpty reports whether the path has no verbs.
func (p Path) IsEmpty() bool { return len(p.verbs) == 0 }

// Len returns the number of verbs.
func (p Path) Len() int { return len(p.verbs) }

// Verbs returns the verb slice. It is shared with the path: do not modify it.
func (p Path) Verbs() []Verb { return p.verbs }

// Points returns the point slice. It is shared with the path: do not modify it.
func (p Path) Points() []Point { return p.pts }

// Reset empties the path, keeping its allocated storage. Do not call it on a
// path that has been put into a DisplayList or shares storage with one.
func (p *Path) Reset() {
	p.verbs = p.verbs[:0]
	p.pts = p.pts[:0]
	p.start = Point{}
	p.needMove = false
}

// CurrentPoint returns the last point of the path and true, or the zero
// Point and false for an empty path.
func (p Path) CurrentPoint() (Point, bool) {
	if len(p.verbs) == 0 {
		return Point{}, false
	}
	if p.needMove {
		return p.start, true
	}
	return p.pts[len(p.pts)-1], true
}

// MoveTo starts a new subpath at pt.
func (p *Path) MoveTo(pt Point) {
	if n := len(p.verbs); n > 0 && p.verbs[n-1] == MoveTo {
		p.pts[len(p.pts)-1] = pt // collapse consecutive MoveTos
	} else {
		p.verbs = append(p.verbs, MoveTo)
		p.pts = append(p.pts, pt)
	}
	p.start = pt
	p.needMove = false
}

// begin prepares for a drawing verb. It panics if there is no subpath to
// continue, and inserts the implicit MoveTo after Close.
func (p *Path) begin(op string) {
	if len(p.verbs) == 0 {
		panic("gfx: Path." + op + " called on an empty path; start every subpath with MoveTo")
	}
	if p.needMove {
		p.verbs = append(p.verbs, MoveTo)
		p.pts = append(p.pts, p.start)
		p.needMove = false
	}
}

// LineTo adds a straight line to pt. It panics if the path is empty.
func (p *Path) LineTo(pt Point) {
	p.begin("LineTo")
	p.verbs = append(p.verbs, LineTo)
	p.pts = append(p.pts, pt)
}

// QuadTo adds a quadratic Bezier with control point c and end point pt. It
// panics if the path is empty.
func (p *Path) QuadTo(c, pt Point) {
	p.begin("QuadTo")
	p.verbs = append(p.verbs, QuadTo)
	p.pts = append(p.pts, c, pt)
}

// CubicTo adds a cubic Bezier with control points c1, c2 and end point pt. It
// panics if the path is empty.
func (p *Path) CubicTo(c1, c2, pt Point) {
	p.begin("CubicTo")
	p.verbs = append(p.verbs, CubicTo)
	p.pts = append(p.pts, c1, c2, pt)
}

// Close closes the current subpath with a straight line back to its start.
// It does nothing on an empty path or right after another Close.
func (p *Path) Close() {
	if len(p.verbs) == 0 || p.needMove {
		return
	}
	p.verbs = append(p.verbs, Close)
	p.needMove = true
}

// AddRect appends the rectangle as a closed subpath, clockwise on screen
// starting at the top-left corner.
func (p *Path) AddRect(r Rect) {
	p.MoveTo(Point{r.X0, r.Y0})
	p.LineTo(Point{r.X1, r.Y0})
	p.LineTo(Point{r.X1, r.Y1})
	p.LineTo(Point{r.X0, r.Y1})
	p.Close()
}

// AddEllipse appends an axis-aligned ellipse as a closed subpath, clockwise
// on screen starting at its rightmost point.
func (p *Path) AddEllipse(center Point, rx, ry float32) {
	a := Arc{Center: center, Radii: Point{rx, ry}, SweepAngle: math.Pi / 2}
	start := a.eval(0)
	p.MoveTo(start)
	// One arc per quadrant, so that the extreme points of the ellipse are
	// segment endpoints and the control polygon stays inside the bounding box.
	for q := 0; q < 4; q++ {
		a.StartAngle = float32(q) * math.Pi / 2
		p.addArcCubics(a, defaultArcTolerance)
	}
	// Make the closing point exactly the start point instead of a rounding of it.
	p.pts[len(p.pts)-1] = start
	p.Close()
}

// AddCircle appends a circle; see AddEllipse.
func (p *Path) AddCircle(center Point, r float32) { p.AddEllipse(center, r, r) }

// AddArc appends an elliptical arc as cubic Beziers within tolerance (the
// maximum distance between the cubics and the true arc, in the same units as
// the coordinates; 0 selects 0.01). Tolerances finer than the float32
// resolution of the arc's coordinates (about 2^-22 times the largest
// absolute coordinate, center plus radius) are raised to it, which bounds
// the number of cubics for huge radii. If the path has a current point, a
// straight line is drawn from it to the start of the arc (as in Canvas'
// arc()), otherwise the arc starts a new subpath.
func (p *Path) AddArc(a Arc, tolerance float32) {
	start := a.eval(0)
	if cur, ok := p.CurrentPoint(); ok {
		if cur != start {
			p.LineTo(start)
		}
	} else {
		p.MoveTo(start)
	}
	p.addArcCubics(a, tolerance)
}

// RectPath returns the path of a rectangle; see Path.AddRect.
func RectPath(r Rect) Path {
	var p Path
	p.AddRect(r)
	return p
}

// EllipsePath returns the path of an ellipse; see Path.AddEllipse.
func EllipsePath(center Point, rx, ry float32) Path {
	var p Path
	p.AddEllipse(center, rx, ry)
	return p
}

// CirclePath returns the path of a circle; see Path.AddEllipse.
func CirclePath(center Point, r float32) Path { return EllipsePath(center, r, r) }

// ArcPath returns a path with a single elliptical arc.
func ArcPath(a Arc, tolerance float32) Path {
	var p Path
	p.AddArc(a, tolerance)
	return p
}

// PathEl is one path element as returned by Elements: the verb and its
// points. Only the first Verb.NumPoints() entries of Pts are meaningful. A
// QuadTo has the control point in Pts[0] and the end point in Pts[1]; a
// CubicTo has Pts[0], Pts[1] as control points and Pts[2] as the end point.
type PathEl struct {
	Verb Verb
	Pts  [3]Point
}

// Elements iterates over the path elements in order without allocating.
func (p Path) Elements() iter.Seq[PathEl] {
	return func(yield func(PathEl) bool) {
		i := 0
		for _, v := range p.verbs {
			el := PathEl{Verb: v}
			n := v.NumPoints()
			copy(el.Pts[:], p.pts[i:i+n])
			i += n
			if !yield(el) {
				return
			}
		}
	}
}

// Segment is one drawing segment with its start point, as a stroker or a
// flattener wants to see it. P[0] is always the start point. By verb:
//
//	MoveTo   P[0] is the new subpath start (nothing is drawn)
//	LineTo   P[0] -> P[1]
//	QuadTo   P[0], control P[1], end P[2]
//	CubicTo  P[0], controls P[1], P[2], end P[3]
//	Close    the closing line P[0] -> P[1], where P[1] is the subpath start
type Segment struct {
	Verb Verb
	P    [4]Point
}

// End returns the last point of the segment.
func (s Segment) End() Point {
	switch s.Verb {
	case LineTo, Close:
		return s.P[1]
	case QuadTo:
		return s.P[2]
	case CubicTo:
		return s.P[3]
	}
	return s.P[0]
}

// Segments iterates over the path as segments with explicit start points,
// without allocating.
func (p Path) Segments() iter.Seq[Segment] {
	return func(yield func(Segment) bool) {
		var cur, start Point
		i := 0
		for _, v := range p.verbs {
			s := Segment{Verb: v}
			switch v {
			case MoveTo:
				cur, start = p.pts[i], p.pts[i]
				s.P[0] = cur
			case LineTo:
				s.P[0], s.P[1] = cur, p.pts[i]
			case QuadTo:
				s.P[0], s.P[1], s.P[2] = cur, p.pts[i], p.pts[i+1]
			case CubicTo:
				s.P[0], s.P[1], s.P[2], s.P[3] = cur, p.pts[i], p.pts[i+1], p.pts[i+2]
			case Close:
				s.P[0], s.P[1] = cur, start
			}
			i += v.NumPoints()
			if v != Close {
				cur = s.End()
			} else {
				cur = start
			}
			if !yield(s) {
				return
			}
		}
	}
}

// IsFinite reports whether every point of the path is finite.
func (p Path) IsFinite() bool {
	for _, pt := range p.pts {
		if !pt.IsFinite() {
			return false
		}
	}
	return true
}

// ControlBounds returns the bounding box of all points of the path, control
// points included. The curve lies inside this box (convex hull property), but
// the box can be larger than the curve's tight bounds. It is O(n) with no
// math beyond min/max, and is what DisplayList culling uses. The result of an
// empty path is the zero Rect. The box of a degenerate path (a horizontal
// line) has zero height, so Empty() is true even though the path is not.
func (p Path) ControlBounds() Rect {
	if len(p.pts) == 0 {
		return Rect{}
	}
	r := Rect{p.pts[0].X, p.pts[0].Y, p.pts[0].X, p.pts[0].Y}
	for _, pt := range p.pts[1:] {
		r.X0 = min(r.X0, pt.X)
		r.Y0 = min(r.Y0, pt.Y)
		r.X1 = max(r.X1, pt.X)
		r.Y1 = max(r.Y1, pt.Y)
	}
	return r
}

// Bounds returns the tight bounding box of the curves, computed from the
// extrema of each segment (up to float32 rounding). It is empty-zero for an
// empty path.
func (p Path) Bounds() Rect {
	if len(p.pts) == 0 {
		return Rect{}
	}
	var r Rect
	first := true
	add := func(pt Point) {
		if first {
			r = Rect{pt.X, pt.Y, pt.X, pt.Y}
			first = false
			return
		}
		r.X0 = min(r.X0, pt.X)
		r.Y0 = min(r.Y0, pt.Y)
		r.X1 = max(r.X1, pt.X)
		r.Y1 = max(r.Y1, pt.Y)
	}
	var cur Point
	i := 0
	for _, v := range p.verbs {
		switch v {
		case MoveTo:
			cur = p.pts[i]
			add(cur)
			i++
		case LineTo:
			cur = p.pts[i]
			add(cur)
			i++
		case QuadTo:
			c, e := p.pts[i], p.pts[i+1]
			add(e)
			for _, t := range quadExtrema(cur, c, e) {
				add(evalQuad(cur, c, e, t))
			}
			cur = e
			i += 2
		case CubicTo:
			c1, c2, e := p.pts[i], p.pts[i+1], p.pts[i+2]
			add(e)
			for _, t := range cubicExtrema(cur, c1, c2, e) {
				add(evalCubic(cur, c1, c2, e, t))
			}
			cur = e
			i += 3
		}
	}
	return r
}

// Transform returns a new path with every point transformed by a.
func (p Path) Transform(a Affine) Path {
	q := Path{
		verbs:    append([]Verb(nil), p.verbs...),
		pts:      make([]Point, len(p.pts)),
		start:    a.Apply(p.start),
		needMove: p.needMove,
	}
	for i, pt := range p.pts {
		q.pts[i] = a.Apply(pt)
	}
	return q
}

func evalQuad(p0, p1, p2 Point, t float64) Point {
	mt := 1 - t
	return Point{
		float32(mt*mt*f64(p0.X) + 2*mt*t*f64(p1.X) + t*t*f64(p2.X)),
		float32(mt*mt*f64(p0.Y) + 2*mt*t*f64(p1.Y) + t*t*f64(p2.Y)),
	}
}

func evalCubic(p0, p1, p2, p3 Point, t float64) Point {
	mt := 1 - t
	a, b, c, d := mt*mt*mt, 3*mt*mt*t, 3*mt*t*t, t*t*t
	return Point{
		float32(a*f64(p0.X) + b*f64(p1.X) + c*f64(p2.X) + d*f64(p3.X)),
		float32(a*f64(p0.Y) + b*f64(p1.Y) + c*f64(p2.Y) + d*f64(p3.Y)),
	}
}

// quadExtrema returns the parameters in (0, 1) where the quadratic has a
// horizontal or vertical tangent.
func quadExtrema(p0, p1, p2 Point) []float64 {
	var out []float64
	for _, ax := range [2][3]float32{{p0.X, p1.X, p2.X}, {p0.Y, p1.Y, p2.Y}} {
		d := f64(ax[0]) - 2*f64(ax[1]) + f64(ax[2])
		if d != 0 {
			if t := (f64(ax[0]) - f64(ax[1])) / d; t > 0 && t < 1 {
				out = append(out, t)
			}
		}
	}
	return out
}

// cubicExtrema returns the parameters in (0, 1) where the cubic has a
// horizontal or vertical tangent.
func cubicExtrema(p0, p1, p2, p3 Point) []float64 {
	var out []float64
	for _, ax := range [2][4]float32{{p0.X, p1.X, p2.X, p3.X}, {p0.Y, p1.Y, p2.Y, p3.Y}} {
		a0, a1, a2, a3 := f64(ax[0]), f64(ax[1]), f64(ax[2]), f64(ax[3])
		// Derivative / 3: a*t^2 + b*t + c.
		a := -a0 + 3*a1 - 3*a2 + a3
		b := 2 * (a0 - 2*a1 + a2)
		c := a1 - a0
		if math.Abs(a) < 1e-12 {
			if b != 0 {
				if t := -c / b; t > 0 && t < 1 {
					out = append(out, t)
				}
			}
			continue
		}
		disc := b*b - 4*a*c
		if disc < 0 {
			continue
		}
		sq := math.Sqrt(disc)
		// Numerically stable quadratic roots.
		q := -0.5 * (b + math.Copysign(sq, b))
		for _, t := range [2]float64{q / a, c / q} {
			if q != 0 && t > 0 && t < 1 {
				out = append(out, t)
			}
		}
	}
	return out
}
