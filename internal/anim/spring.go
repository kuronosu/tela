// The spring model, the closed-form solution and the duration/bounce
// parametrization (SpringForDuration) are ported from Motion
// (https://github.com/motiondivision/motion, packages/motion-dom/src/animation/generators/spring/),
// MIT License, Copyright (c) 2018 Framer B.V. Tela solves in seconds
// instead of milliseconds, returns the velocity too, and rounds explicitly
// so that results are the same on every architecture (ADR-041).

package anim

import (
	"math"
	"time"

	"github.com/kuronosu/tela/internal/fmath"
)

// Spring is a damped harmonic oscillator in Motion's model (motion.dev
// "spring"): a value pulled to its target by a spring of the given
// stiffness, slowed by damping, with a mass, starting at a velocity. It is
// solved in closed form, so a sample at any time is exact and independent
// of the frame rate.
//
//	stiffness 100, damping 10, mass 1 (Motion's defaults) bounces slightly
//	damping = 2*sqrt(stiffness*mass) is critically damped (no overshoot)
//
// The units are seconds for time and whatever the animated value uses for
// distance.
type Spring struct {
	// Stiffness is the spring constant k.
	Stiffness float64
	// Damping is the damping coefficient c.
	Damping float64
	// Mass is the mass m of the moving object; zero means 1.
	Mass float64
	// Velocity is the initial velocity of the value, in units per second
	// (positive moves toward larger values), for example the release
	// velocity of a drag.
	Velocity float64
}

// DefaultSpring returns Motion's default spring (stiffness 100, damping 10,
// mass 1).
func DefaultSpring() Spring { return Spring{Stiffness: 100, Damping: 10, Mass: 1} }

func (s Spring) mass() float64 {
	if s.Mass > 0 {
		return s.Mass
	}
	return 1
}

// DampingRatio returns the damping ratio: below 1 the spring overshoots and
// oscillates, 1 is critically damped, above 1 it creeps to the target.
func (s Spring) DampingRatio() float64 {
	return s.Damping / (2 * math.Sqrt(float64(s.Stiffness*s.mass())))
}

// Motion's limits for the duration/bounce parametrization.
const (
	springMinDamping  = 0.05
	springMaxDamping  = 1.0
	springMinDuration = 0.01
	springMaxDuration = 10.0
	springSafeMin     = 0.001
)

// SpringForDuration returns the spring that settles in about the given
// duration with the given bounce, Motion's duration/bounce parametrization
// (findSpring): a bounce of 0 is critically damped, 0.25 is a little
// bouncy, and values toward 1 oscillate longer. The duration is clamped to
// 0.01 to 10 seconds, the bounce so that the damping ratio is in [0.05, 1].
// The mass is 1.
func SpringForDuration(duration time.Duration, bounce float64) Spring {
	return SpringForDurationVelocity(duration, bounce, 0)
}

// SpringForDurationVelocity is SpringForDuration with an initial velocity
// (in units of the distance to travel per second, as Motion takes it).
func SpringForDurationVelocity(duration time.Duration, bounce, velocity float64) Spring {
	zeta := min(max(1-bounce, springMinDamping), springMaxDamping)
	dur := min(max(duration.Seconds(), springMinDuration), springMaxDuration)
	var envelope, derivative func(w float64) float64
	if zeta < 1 {
		envelope = func(w float64) float64 {
			decay := float64(w * zeta)
			delta := float64(decay * dur)
			a := decay - velocity
			b := w * math.Sqrt(1-float64(zeta*zeta))
			c := fmath.Exp(-delta)
			return springSafeMin - float64(float64(a/b)*c)
		}
		derivative = func(w float64) float64 {
			decay := float64(w * zeta)
			delta := float64(decay * dur)
			d := float64(delta*velocity) + velocity
			e := float64(float64(float64(zeta*zeta)*float64(w*w)) * dur)
			f := fmath.Exp(-delta)
			g := float64(w*w) * math.Sqrt(1-float64(zeta*zeta))
			factor := 1.0
			if -envelope(w)+springSafeMin > 0 {
				factor = -1
			}
			return float64(float64(factor*(d-e))*f) / g
		}
	} else {
		envelope = func(w float64) float64 {
			a := fmath.Exp(-float64(w * dur))
			b := float64((w-velocity)*dur) + 1
			return -springSafeMin + float64(a*b)
		}
		derivative = func(w float64) float64 {
			a := fmath.Exp(-float64(w * dur))
			b := float64((velocity - w) * float64(dur*dur))
			return float64(a * b)
		}
	}
	w := 5 / dur
	for i := 1; i < 12; i++ {
		w -= envelope(w) / derivative(w)
	}
	if math.IsNaN(w) || math.IsInf(w, 0) {
		return DefaultSpring()
	}
	stiffness := float64(w * w)
	return Spring{Stiffness: stiffness, Damping: float64(zeta*2) * math.Sqrt(stiffness), Mass: 1, Velocity: velocity}
}

// At returns the position and velocity (units per second) at t seconds of a
// value that starts at from and is pulled to to. Negative times give from.
func (s Spring) At(t float64, from, to float64) (x, v float64) {
	if t <= 0 {
		return from, s.Velocity
	}
	m := s.mass()
	w0 := math.Sqrt(float64(s.Stiffness / m))
	zeta := s.DampingRatio()
	d0 := to - from // displacement from the target, measured toward it
	dv0 := -s.Velocity
	sigma := float64(zeta * w0)
	var d, dv float64
	switch {
	case zeta < 1:
		wd := float64(w0 * math.Sqrt(1-float64(zeta*zeta)))
		sin, cos := fmath.Sincos(float64(wd * t))
		env := fmath.Exp(-float64(sigma * t))
		a := d0
		b := (dv0 + float64(sigma*d0)) / wd
		d = float64(env * (float64(a*cos) + float64(b*sin)))
		inner := float64(-sigma*(float64(a*cos)+float64(b*sin))) + float64(wd*(float64(b*cos)-float64(a*sin)))
		dv = float64(env * inner)
	case zeta == 1:
		env := fmath.Exp(-float64(w0 * t))
		c := dv0 + float64(w0*d0)
		d = float64(env * (d0 + float64(c*t)))
		dv = float64(env * (c - float64(w0*(d0+float64(c*t)))))
	default:
		wd := float64(w0 * math.Sqrt(float64(zeta*zeta)-1))
		arg := min(float64(wd*t), 300)
		ep, em := fmath.Exp(arg), fmath.Exp(-arg)
		sinh, cosh := (ep-em)/2, (ep+em)/2
		env := fmath.Exp(-float64(sigma * t))
		p := (dv0 + float64(sigma*d0)) / wd
		q := d0
		d = float64(env * (float64(p*sinh) + float64(q*cosh)))
		inner := float64(-sigma*(float64(p*sinh)+float64(q*cosh))) + float64(wd*(float64(p*cosh)+float64(q*sinh)))
		dv = float64(env * inner)
	}
	return to - d, -dv
}

// Rest are the thresholds under which a spring is considered at rest: it is
// when its speed and its distance from the target are both below them.
type Rest struct {
	Speed, Delta float64
}

// DefaultRest returns Motion's thresholds for an animation over the given
// distance: 2 units per second and 0.5 units, or 0.01 and 0.005 for
// distances of 5 or less (opacity, scale).
func DefaultRest(distance float64) Rest {
	if math.Abs(distance) <= 5 {
		return Rest{Speed: 0.01, Delta: 0.005}
	}
	return Rest{Speed: 2, Delta: 0.5}
}

// Settled reports whether the spring started at from toward to is at rest
// at t seconds.
func (s Spring) Settled(t float64, from, to float64, r Rest) bool {
	x, v := s.At(t, from, to)
	return math.Abs(v) <= r.Speed && math.Abs(to-x) <= r.Delta
}

// maxSettle bounds the search of Duration.
const maxSettle = 20 * time.Second

// Duration returns the time after which the spring stays at rest (see
// Settled), in steps of 1/240 s, at most 20 seconds. A value that never
// moves (from == to without velocity) settles at once.
func (s Spring) Duration(from, to float64, r Rest) time.Duration {
	const step = 1.0 / 240
	n := int(maxSettle.Seconds() / step)
	last := -1
	for i := 0; i <= n; i++ {
		if !s.Settled(float64(i)*step, from, to, r) {
			last = i
		}
	}
	if last == n {
		return maxSettle
	}
	return time.Duration(float64(last+1) * step * float64(time.Second))
}
