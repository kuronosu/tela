package anim

import (
	"math"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Explicit animations of values (the public UseSpring, UseTween and
// Animate): a Motion moves a value of up to four components (a number, a
// point, a size, a rectangle, a color in its interpolation space) from
// where it is toward a target, with a spring or a tween, and reports its
// velocity so that the motion that interrupts it starts at the same
// position and speed (Motion's and Compose's retargeting).

// Vec is a value of N (1 to 4) components that a Motion animates.
type Vec struct {
	N int
	C [4]float64
}

// Finite reports whether every component is a finite number.
func (v Vec) Finite() bool {
	for i := range v.N {
		if math.IsNaN(v.C[i]) || math.IsInf(v.C[i], 0) {
			return false
		}
	}
	return true
}

// Same reports whether a and b have the same components, bit for bit (a
// NaN is the same as itself, unlike with ==).
func (v Vec) Same(w Vec) bool {
	if v.N != w.N {
		return false
	}
	for i := range v.N {
		if math.Float64bits(v.C[i]) != math.Float64bits(w.C[i]) {
			return false
		}
	}
	return true
}

// MotionSpec says how a Motion moves: a spring (Spring true) or a tween.
type MotionSpec struct {
	// Spring selects the spring Physics; otherwise the motion is a tween
	// of Duration with Curve.
	Spring bool
	// Physics is the spring's stiffness, damping and mass; its Velocity
	// is ignored (every component starts at its own velocity, Vel).
	Physics Spring
	// MinDuration is, for a spring found from a duration
	// (SpringForDuration), the duration it was found for: the motion does
	// not end before it (Motion ends such springs at their duration;
	// Tela also waits for them to be at rest, since a spring that
	// inherits a velocity may not have settled by then).
	MinDuration time.Duration
	// Duration and Curve are the tween's (a nil Curve is linear).
	Duration time.Duration
	Curve    Curve
}

// maxMotion bounds a spring motion: one that has not come to rest after
// 20 seconds (no damping) ends at its target, so that an app does not
// animate forever.
const maxMotion = maxSettle

// velocitySample is the interval of the finite difference that gives the
// velocity of a tween (Motion's velocitySampleDuration).
const velocitySample = 5 * time.Millisecond

// Motion is one run of a value toward a target: from From at time Start,
// with initial velocity Vel (units per second, per component; springs
// only), to To.
type Motion struct {
	Spec          MotionSpec
	From, To, Vel Vec
	Start         time.Duration
	rest          [4]Rest
}

// NewMotion returns the motion of spec from from (moving at vel) to to,
// starting at start. The rest thresholds of a spring follow Motion's for
// the distance of each component.
func NewMotion(spec MotionSpec, from, to, vel Vec, start time.Duration) Motion {
	m := Motion{Spec: spec, From: from, To: to, Vel: vel, Start: start}
	m.Vel.N = from.N
	for i := range from.N {
		m.rest[i] = DefaultRest(to.C[i] - from.C[i])
	}
	return m
}

// At returns the value and velocity of the motion at now, and whether it
// has ended: then x is exactly To and v is zero. Before Start it is at
// From with its initial velocity.
func (m *Motion) At(now time.Duration) (x, v Vec, done bool) {
	n := m.From.N
	x.N, v.N = n, n
	el := now - m.Start
	if el <= 0 {
		return m.From, m.Vel, false
	}
	if !m.Spec.Spring {
		d := m.Spec.Duration
		if el >= d {
			return m.To, Vec{N: n}, true
		}
		x = m.tweenAt(el)
		prev := m.tweenAt(max(el-velocitySample, 0))
		dt := float64(el-max(el-velocitySample, 0)) / float64(time.Second)
		for i := range n {
			v.C[i] = (x.C[i] - prev.C[i]) / dt
		}
		return x, v, false
	}
	t := el.Seconds()
	settled := true
	for i := range n {
		s := m.Spec.Physics
		s.Velocity = m.Vel.C[i]
		x.C[i], v.C[i] = s.At(t, m.From.C[i], m.To.C[i])
		r := m.rest[i]
		if math.Abs(v.C[i]) > r.Speed || math.Abs(m.To.C[i]-x.C[i]) > r.Delta {
			settled = false
		}
	}
	if (settled && el >= m.Spec.MinDuration) || el >= maxMotion {
		return m.To, Vec{N: n}, true
	}
	return x, v, false
}

// tweenAt returns the tween's value el after its start (el within its
// duration).
func (m *Motion) tweenAt(el time.Duration) Vec {
	x := Vec{N: m.From.N}
	d := m.Spec.Duration
	if d <= 0 {
		return m.To
	}
	c := m.Spec.Curve
	if c == nil {
		c = Linear{}
	}
	p := c.At(float64(el) / float64(d))
	for i := range x.N {
		switch p {
		case 0:
			x.C[i] = m.From.C[i]
		case 1:
			x.C[i] = m.To.C[i]
		default:
			x.C[i] = lerp64(m.From.C[i], m.To.C[i], p)
		}
	}
	return x
}

// ---------------------------------------------------------------------
// Colors.

// ColorsOklab reports whether a transition between a and b interpolates in
// Oklab (LerpColor's rule): unless both are legacy sRGB colors.
func ColorsOklab(a, b gfx.Color) bool { return !(isLegacy(a) && isLegacy(b)) }

// ColorVec returns c as the four components LerpColor interpolates:
// premultiplied by alpha, in sRGB (red, green, blue, alpha) or in Oklab
// (L, a, b, alpha). Interpolating these linearly and converting back with
// VecColor gives LerpColor's result, so a color can move like any value.
func ColorVec(c gfx.Color, oklab bool) Vec {
	if oklab {
		l, a, b := c.ToOklab()
		return Vec{N: 4, C: [4]float64{float64(l * c.A), float64(a * c.A), float64(b * c.A), float64(c.A)}}
	}
	return Vec{N: 4, C: [4]float64{float64(c.R * c.A), float64(c.G * c.A), float64(c.B * c.A), float64(c.A)}}
}

// VecColor converts components from ColorVec back to a color. The alpha is
// clamped to [0, 1] (a spring may overshoot it); a color with no alpha is
// transparent.
func VecColor(v Vec, oklab bool) gfx.Color {
	alpha := min(v.C[3], 1)
	if !(alpha > 0) {
		return gfx.Transparent
	}
	c0, c1, c2 := v.C[0]/alpha, v.C[1]/alpha, v.C[2]/alpha
	if oklab {
		return gfx.Oklab(float32(c0), float32(c1), float32(c2), float32(alpha))
	}
	return gfx.Color{R: float32(c0), G: float32(c1), B: float32(c2), A: float32(alpha)}
}
