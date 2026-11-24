package canvas

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
)

// ---------------------------------------------------------------------
// The current path.

// BeginPath empties the path: the next Fill or Stroke draws only what is
// added after it.
func (c *Canvas) BeginPath() {
	if r := c.use("BeginPath"); r != nil {
		r.path.reset()
	}
}

// MoveTo starts a new subpath at x, y.
func (c *Canvas) MoveTo(x, y float64) {
	if r := c.use("MoveTo"); r != nil && finite(x, y) {
		r.path.moveTo(r.m.vec(x, y))
	}
}

// LineTo adds a straight line from the last point of the path to x, y, or
// starts a subpath there when the path is empty.
func (c *Canvas) LineTo(x, y float64) {
	if r := c.use("LineTo"); r != nil && finite(x, y) {
		r.path.lineTo(r.m.vec(x, y))
	}
}

// QuadraticCurveTo adds a quadratic Bezier curve from the last point of the
// path to x, y, with the control point cpx, cpy.
func (c *Canvas) QuadraticCurveTo(cpx, cpy, x, y float64) {
	if r := c.use("QuadraticCurveTo"); r != nil && finite(cpx, cpy, x, y) {
		r.path.quadTo(r.m.vec(cpx, cpy), r.m.vec(x, y))
	}
}

// BezierCurveTo adds a cubic Bezier curve from the last point of the path to
// x, y, with the control points cp1x, cp1y and cp2x, cp2y.
func (c *Canvas) BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) {
	if r := c.use("BezierCurveTo"); r != nil && finite(cp1x, cp1y, cp2x, cp2y, x, y) {
		r.path.cubicTo(r.m.vec(cp1x, cp1y), r.m.vec(cp2x, cp2y), r.m.vec(x, y))
	}
}

// ArcTo rounds the corner that the lines from the last point of the path to
// x1, y1 and from there to x2, y2 make: it adds the arc of radius that
// touches both, with a straight line to its start. A negative radius adds
// nothing, where the browser throws an error.
func (c *Canvas) ArcTo(x1, y1, x2, y2, radius float64) {
	if r := c.use("ArcTo"); r != nil && finite(x1, y1, x2, y2, radius) {
		r.path.arcTo(&r.m, x1, y1, x2, y2, radius)
	}
}

// Rect adds a rectangle as a closed subpath, from its corner x, y.
func (c *Canvas) Rect(x, y, width, height float64) {
	if r := c.use("Rect"); r != nil && finite(x, y, width, height) {
		r.path.rect(&r.m, x, y, width, height)
	}
}

// RoundRect adds a rectangle with rounded corners as a closed subpath. One
// radius is that of every corner; two are those of the top left and bottom
// right corners, and of the others; three, of the top left, of the top right
// and bottom left, and of the bottom right; four, of the corners clockwise
// from the top left. Radii that do not fit are scaled down together. A
// negative one adds nothing, where the browser throws an error.
func (c *Canvas) RoundRect(x, y, width, height float64, radii ...float64) {
	if r := c.use("RoundRect"); r != nil && finite(x, y, width, height) {
		r.path.roundRect(&r.m, x, y, width, height, radii)
	}
}

// Arc adds an arc of the circle of radius around x, y, from startAngle to
// endAngle in radians (0 points right, and angles grow clockwise on the
// screen), with a straight line to its start from the last point of the
// path, if there is one. It goes clockwise, or counterclockwise with true as
// the last argument, and at most once around. A negative radius adds
// nothing, where the browser throws an error.
func (c *Canvas) Arc(x, y, radius, startAngle, endAngle float64, counterclockwise ...bool) {
	if r := c.use("Arc"); r != nil && finite(x, y, radius, startAngle, endAngle) && radius >= 0 {
		r.path.ellipse(&r.m, x, y, radius, radius, 0, startAngle, arcSweep(startAngle, endAngle, isCCW(counterclockwise)))
	}
}

// Ellipse adds an arc of an ellipse, as Arc does of a circle: the ellipse
// around x, y with radii radiusX and radiusY, turned by rotation radians. The
// angles are those from its own axes.
func (c *Canvas) Ellipse(x, y, radiusX, radiusY, rotation, startAngle, endAngle float64, counterclockwise ...bool) {
	r := c.use("Ellipse")
	if r != nil && finite(x, y, radiusX, radiusY, rotation) && finite(startAngle, endAngle) && radiusX >= 0 && radiusY >= 0 {
		r.path.ellipse(&r.m, x, y, radiusX, radiusY, rotation, startAngle, arcSweep(startAngle, endAngle, isCCW(counterclockwise)))
	}
}

// ClosePath closes the last subpath with a straight line to its start, where
// the next one starts.
func (c *Canvas) ClosePath() {
	if r := c.use("ClosePath"); r != nil {
		r.path.close()
	}
}

func isCCW(v []bool) bool { return len(v) > 0 && v[0] }

// IsPointInPath reports whether the point x, y is inside the path, as the
// canvas's FillRule tells, or on it. The point is one of the canvas, in
// logical pixels, whatever the transform: the pointer's, in an event's
// coordinates of the element's content box. A handler of events asks
// [Path2D.Contains] instead: a Canvas is usable only while it draws.
func (c *Canvas) IsPointInPath(x, y float64) bool {
	r := c.use("IsPointInPath")
	return r != nil && r.path.contains(x, y, c.FillRule)
}

// IsPointInStroke reports whether the point x, y, one of the canvas as in
// IsPointInPath, is inside what Stroke would draw, or on its edge.
func (c *Canvas) IsPointInStroke(x, y float64) bool {
	r := c.use("IsPointInStroke")
	if r == nil || !finite(x, y) {
		return false
	}
	d := drawing{kind: drawStroke}
	if !r.stroke(c, &d, &r.path) {
		return false
	}
	// The outline, finer than what shows at any display scale.
	const tolerance = 0.01
	tol := float32(tolerance)
	if d.under {
		inv, _ := d.m.invert()
		x, y = inv.at(x, y)
		tol = float32(tolerance / max(d.m.extent(), 1e-6))
	}
	var outline path
	outline.setGfx(d.path.Stroke(d.line, tol))
	return outline.contains(x, y, gfx.NonZero)
}

// ---------------------------------------------------------------------
// Path2D.

// Path2D is a path that is kept: built once, in coordinates of its own, and
// drawn by any canvas, any number of times, with the transform current then
// ([Canvas.FillPath], [Canvas.StrokePath], [Canvas.ClipPath]). Its methods
// are those of the current path of a [Canvas]. The zero Path2D is an empty
// path, ready to use.
//
// It is not safe for use by several goroutines while one of them changes it;
// several may read it.
type Path2D struct{ p path }

// ParsePath returns the path that d, SVG path data (the d attribute of a path
// element, such as "M0 0 L10 0 L5 8 Z"), describes: the argument of the
// browser's new Path2D. An error tells what is wrong with the data, where the
// browser keeps the path up to there.
func ParsePath(d string) (*Path2D, error) {
	parsed, err := svg.ParsePath(d)
	if err != nil {
		return nil, fmt.Errorf("tela.ParsePath: %v", err)
	}
	p := &Path2D{}
	p.p.setGfx(parsed)
	return p, nil
}

// MustPath is ParsePath for data known to be valid: it panics on an error.
func MustPath(d string) *Path2D {
	p, err := ParsePath(d)
	if err != nil {
		panic(fmt.Sprintf("tela.MustPath(%q): %v", d, err))
	}
	return p
}

// MoveTo starts a new subpath at x, y.
func (p *Path2D) MoveTo(x, y float64) {
	if finite(x, y) {
		p.p.moveTo(vec{x, y})
	}
}

// LineTo adds a straight line from the last point to x, y, or starts a
// subpath there when the path is empty.
func (p *Path2D) LineTo(x, y float64) {
	if finite(x, y) {
		p.p.lineTo(vec{x, y})
	}
}

// QuadraticCurveTo adds a quadratic Bezier curve; see
// [Canvas.QuadraticCurveTo].
func (p *Path2D) QuadraticCurveTo(cpx, cpy, x, y float64) {
	if finite(cpx, cpy, x, y) {
		p.p.quadTo(vec{cpx, cpy}, vec{x, y})
	}
}

// BezierCurveTo adds a cubic Bezier curve; see [Canvas.BezierCurveTo].
func (p *Path2D) BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) {
	if finite(cp1x, cp1y, cp2x, cp2y, x, y) {
		p.p.cubicTo(vec{cp1x, cp1y}, vec{cp2x, cp2y}, vec{x, y})
	}
}

// ArcTo rounds a corner; see [Canvas.ArcTo].
func (p *Path2D) ArcTo(x1, y1, x2, y2, radius float64) {
	if finite(x1, y1, x2, y2, radius) {
		p.p.detail = pathDetail
		p.p.arcTo(&identity, x1, y1, x2, y2, radius)
	}
}

// Rect adds a rectangle as a closed subpath, from its corner x, y.
func (p *Path2D) Rect(x, y, width, height float64) {
	if finite(x, y, width, height) {
		p.p.rect(&identity, x, y, width, height)
	}
}

// RoundRect adds a rectangle with rounded corners as a closed subpath; see
// [Canvas.RoundRect].
func (p *Path2D) RoundRect(x, y, width, height float64, radii ...float64) {
	if finite(x, y, width, height) {
		p.p.detail = pathDetail
		p.p.roundRect(&identity, x, y, width, height, radii)
	}
}

// Arc adds an arc of a circle; see [Canvas.Arc].
func (p *Path2D) Arc(x, y, radius, startAngle, endAngle float64, counterclockwise ...bool) {
	if finite(x, y, radius, startAngle, endAngle) && radius >= 0 {
		p.p.detail = pathDetail
		p.p.ellipse(&identity, x, y, radius, radius, 0, startAngle, arcSweep(startAngle, endAngle, isCCW(counterclockwise)))
	}
}

// Ellipse adds an arc of an ellipse; see [Canvas.Ellipse].
func (p *Path2D) Ellipse(x, y, radiusX, radiusY, rotation, startAngle, endAngle float64, counterclockwise ...bool) {
	if finite(x, y, radiusX, radiusY, rotation) && finite(startAngle, endAngle) && radiusX >= 0 && radiusY >= 0 {
		p.p.detail = pathDetail
		p.p.ellipse(&identity, x, y, radiusX, radiusY, rotation, startAngle, arcSweep(startAngle, endAngle, isCCW(counterclockwise)))
	}
}

// ClosePath closes the last subpath with a straight line to its start, where
// the next one starts.
func (p *Path2D) ClosePath() { p.p.close() }

// AddPath adds the subpaths of q. What is added after them starts a subpath
// at their last point.
func (p *Path2D) AddPath(q *Path2D) {
	if q == nil {
		panic("tela: Path2D.AddPath(nil): pass a path (new(tela.Path2D) is an empty one)")
	}
	p.p.append(&q.p)
}

// Contains reports whether the point x, y, in the coordinates of the path, is
// inside it or on it, as the fill rule tells (NonZero without one). It needs
// no canvas and no draw: an OnClick, or any goroutine, asks it.
func (p *Path2D) Contains(x, y float64, rule ...FillRule) bool {
	r := gfx.NonZero
	if len(rule) > 0 {
		r = rule[0]
	}
	return p != nil && p.p.contains(x, y, r)
}

// usePath returns the path of p, an argument of the method, or nil when it is
// nil, which panics in dev mode.
func (c *Canvas) usePath(method string, p *Path2D) *path {
	if p != nil {
		return &p.p
	}
	if c.dev {
		panic(fmt.Sprintf("tela: Canvas.%s(nil): pass a path, built with the methods of tela.Path2D or by tela.ParsePath", method))
	}
	return nil
}
