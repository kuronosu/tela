package canvas

import (
	"math"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// matrix is the transform of a canvas, in the order of the browser's
// setTransform:
//
//	x' = a*x + c*y + e
//	y' = b*x + d*y + f
type matrix struct{ a, b, c, d, e, f float64 }

// identity leaves every point where it is.
var identity = matrix{a: 1, d: 1}

// mul returns the transform that applies n, then m: the current transform m
// after Transform(n).
func (m *matrix) mul(n matrix) matrix {
	return matrix{
		a: float64(m.a*n.a) + float64(m.c*n.b),
		b: float64(m.b*n.a) + float64(m.d*n.b),
		c: float64(m.a*n.c) + float64(m.c*n.d),
		d: float64(m.b*n.c) + float64(m.d*n.d),
		e: float64(m.a*n.e) + float64(m.c*n.f) + m.e,
		f: float64(m.b*n.e) + float64(m.d*n.f) + m.f,
	}
}

// at returns the point (x, y) transformed.
func (m *matrix) at(x, y float64) (float64, float64) {
	return float64(m.a*x) + float64(m.c*y) + m.e, float64(m.b*x) + float64(m.d*y) + m.f
}

// vec returns the point (x, y) transformed, as a path keeps it.
func (m *matrix) vec(x, y float64) vec {
	x, y = m.at(x, y)
	return vec{x, y}
}

// point returns the point (x, y) transformed, as a path holds it.
func (m *matrix) point(x, y float64) gfx.Point {
	x, y = m.at(x, y)
	return gfx.Point{X: float32(x), Y: float32(y)}
}

// rotation returns the rotation by angle radians, clockwise on the screen.
func rotation(angle float64) matrix {
	s, c := fmath.Sincos(angle)
	return matrix{a: c, b: s, c: -s, d: c}
}

// invert returns the inverse of m, and false for a transform that collapses
// the plane to a line or a point, or is not finite.
func (m *matrix) invert() (matrix, bool) {
	det := float64(m.a*m.d) - float64(m.b*m.c)
	k := 1 / det
	if !finite(det, k) {
		return matrix{}, false
	}
	return matrix{
		a: float64(m.d * k),
		b: float64(-m.b * k),
		c: float64(-m.c * k),
		d: float64(m.a * k),
		e: float64((float64(m.c*m.f) - float64(m.d*m.e)) * k),
		f: float64((float64(m.b*m.e) - float64(m.a*m.f)) * k),
	}, true
}

// axisAligned reports whether m maps upright rectangles to upright
// rectangles.
func (m *matrix) axisAligned() bool {
	return m.b == 0 && m.c == 0 || m.a == 0 && m.d == 0
}

// similar reports whether m keeps shapes (it moves, turns, mirrors and
// scales them evenly), and by how much it scales lengths.
func (m *matrix) similar() (scale float64, ok bool) {
	if m.b == 0 && m.c == 0 {
		return math.Abs(m.a), m.a == m.d || m.a == -m.d
	}
	// The products of rotations are rounded: what should be equal agrees to
	// a few units in the last place.
	eps := (math.Abs(m.a) + math.Abs(m.b) + math.Abs(m.c) + math.Abs(m.d)) * 0x1p-44
	near := func(x, y float64) bool { return math.Abs(x-y) <= eps }
	if near(m.a, m.d) && near(m.b, -m.c) || near(m.a, -m.d) && near(m.b, m.c) {
		return math.Sqrt(float64(m.a*m.a) + float64(m.b*m.b)), true
	}
	return 0, false
}

// extent bounds how much m stretches a length.
func (m *matrix) extent() float64 {
	return math.Sqrt(float64(m.a*m.a) + float64(m.b*m.b) + float64(m.c*m.c) + float64(m.d*m.d))
}

// affine returns m as gfx takes it.
func (m *matrix) affine() gfx.Affine {
	return gfx.Affine{float32(m.a), float32(m.b), float32(m.c), float32(m.d), float32(m.e), float32(m.f)}
}

// finite reports whether every number is finite: the methods of a canvas
// ignore calls with an infinite or NaN argument, as the browser does.
func finite(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

// length returns the length of the vector (x, y).
func length(x, y float64) float64 { return math.Sqrt(float64(x*x) + float64(y*y)) }
