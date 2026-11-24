package canvas

import (
	"math"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// The current path. Its points are in the canvas's coordinates: each one is
// transformed by the transform current when it was added, so a transform
// changed afterwards does not move it.
//
// A segment of no length is not added: the browser removes them before it
// strokes a path ("trace a path"), where a stroke of gfx would draw a dot
// for a subpath made of nothing else. Filling them changes nothing.

// BeginPath empties the path: the next Fill or Stroke draws only what is
// added after it.
func (c *Canvas) BeginPath() {
	if r := c.use("BeginPath"); r != nil {
		r.path.Reset()
		r.line = false
	}
}

// MoveTo starts a new subpath at x, y.
func (c *Canvas) MoveTo(x, y float64) {
	if r := c.use("MoveTo"); r != nil && finite(x, y) {
		r.moveTo(r.m.point(x, y))
	}
}

// LineTo adds a straight line from the last point of the path to x, y, or
// starts a subpath there when the path is empty.
func (c *Canvas) LineTo(x, y float64) {
	if r := c.use("LineTo"); r != nil && finite(x, y) {
		r.lineTo(r.m.point(x, y))
	}
}

// ClosePath closes the last subpath with a straight line to its start, where
// the next one starts.
func (c *Canvas) ClosePath() {
	if r := c.use("ClosePath"); r != nil {
		r.closePath()
	}
}

// Rect adds a rectangle as a closed subpath, from its corner x, y.
func (c *Canvas) Rect(x, y, width, height float64) {
	if r := c.use("Rect"); r != nil && finite(x, y, width, height) {
		r.moveTo(r.m.point(x, y))
		r.lineTo(r.m.point(x+width, y))
		r.lineTo(r.m.point(x+width, y+height))
		r.lineTo(r.m.point(x, y+height))
		r.closePath()
	}
}

// Arc adds an arc of the circle of radius around x, y, from startAngle to
// endAngle in radians (0 points right, and angles grow clockwise on the
// screen), with a straight line to its start from the last point of the
// path, if there is one. It goes clockwise, or counterclockwise with true as
// the last argument, and at most once around. A negative radius adds
// nothing, where the browser throws an error.
func (c *Canvas) Arc(x, y, radius, startAngle, endAngle float64, counterclockwise ...bool) {
	r := c.use("Arc")
	if r == nil || !finite(x, y, radius, startAngle, endAngle) || radius < 0 {
		return
	}
	ccw := len(counterclockwise) > 0 && counterclockwise[0]
	r.arc(x, y, radius, startAngle, arcSweep(startAngle, endAngle, ccw))
}

func (r *Recorder) moveTo(p gfx.Point) {
	r.path.MoveTo(p)
	r.line = false
}

func (r *Recorder) lineTo(p gfx.Point) {
	last, ok := r.path.CurrentPoint()
	switch {
	case !ok:
		r.moveTo(p)
	case p != last:
		r.path.LineTo(p)
		r.line = true
	}
}

func (r *Recorder) closePath() {
	if r.line { // else its one point, where the next subpath starts too
		r.path.Close()
		r.line = false
	}
}

// arcSweep returns the angle an arc turns from start to end: at most once
// around, and in its direction, whichever angle is larger.
func arcSweep(start, end float64, ccw bool) float64 {
	const turn = 2 * math.Pi
	if ccw {
		return -arcSweep(-start, -end, false)
	}
	switch d := end - start; {
	case d >= turn:
		return turn
	case d < 0:
		return turn - math.Mod(-d, turn)
	default:
		return d
	}
}

// arcTolerance is how far, in logical pixels, the curves of an arc may be
// from it.
const arcTolerance = 0.005

// arcError is the distance between a quarter of the unit circle and its
// cubic Bezier; each halving of the angle divides it by 64.
const arcError = 2.73e-4

// arc adds the arc of the circle of radius around x, y that starts at angle
// start and turns by sweep.
func (r *Recorder) arc(x, y, radius, start, sweep float64) {
	at := func(angle float64) (sin, cos float64, p gfx.Point) {
		sin, cos = fmath.Sincos(angle)
		return sin, cos, r.m.point(x+float64(radius*cos), y+float64(radius*sin))
	}
	sin0, cos0, first := at(start)
	r.lineTo(first)
	if sweep == 0 || radius == 0 {
		return
	}
	// Quarters of the circle, halved until the curves are close enough.
	step := math.Pi / 2
	for e := float64(float64(radius*r.m.extent()) * arcError); e > arcTolerance && step > math.Pi/1024; e /= 64 {
		step /= 2
	}
	n := max(int(math.Ceil(math.Abs(sweep)/step-1e-9)), 1)
	step = sweep / float64(n)
	whole := math.Abs(sweep) == 2*math.Pi
	// The control points are on the tangents at the ends of each curve.
	arm := float64(radius * float64(4.0/3*fmath.Tan(step/4)))
	s0, c0 := sin0, cos0
	for i := 1; i <= n; i++ {
		s1, c1, p := at(start + float64(step*float64(i)))
		if i == n && whole {
			s1, c1, p = sin0, cos0, first // it ends exactly where it starts
		}
		x0, y0 := x+float64(radius*c0), y+float64(radius*s0)
		x1, y1 := x+float64(radius*c1), y+float64(radius*s1)
		r.path.CubicTo(
			r.m.point(x0-float64(arm*s0), y0+float64(arm*c0)),
			r.m.point(x1+float64(arm*s1), y1-float64(arm*c1)),
			p,
		)
		s0, c0 = s1, c1
	}
	r.line = true
}
