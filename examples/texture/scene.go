package main

import (
	"math"
	"time"
)

// uniforms is what the scene needs to know to shade any of its pixels: the
// push constants of shaders/scene.frag, and the input of shade.go. The
// vectors have four components, as GLSL lays them out.
type uniforms struct {
	// Size is the width and the height in pixels, and the pixels per unit
	// of the scene.
	Size [4]float32
	// Axis is the planet's axis, the normal of its ring; Meridian, across
	// it, is where longitude 0 is; Light is the direction to the light.
	Axis, Meridian, Light [4]float32
	// Moons are their positions and radii.
	Moons [4][4]float32
}

// sceneAt returns the scene at a time of its clock, for a picture of w by h
// pixels.
func sceneAt(at time.Duration, w, h int) uniforms {
	turn := func(period, phase float64) float64 { return 2*math.Pi*(at.Seconds()/period) + phase }

	u := uniforms{Size: [4]float32{float32(w), float32(h), float32(min(float64(h), float64(w)/1.6) / 2.75), 0}}
	// The axis leans toward the viewer and sways.
	axis := normal(-0.34+0.12*math.Sin(turn(23, 0)), 1, 0.52+0.16*math.Sin(turn(17, 1)))
	// Two directions across it: what turns around it is made of them.
	e1 := normal(axis[1], -axis[0], 0)
	e2 := cross(axis, e1)
	around := func(a, r float64) [3]float64 {
		c, s := math.Cos(a)*r, math.Sin(a)*r
		return [3]float64{e1[0]*c + e2[0]*s, e1[1]*c + e2[1]*s, e1[2]*c + e2[2]*s}
	}
	u.Axis = vec4(axis, 0)
	u.Meridian = vec4(around(turn(14, 0), 1), 0)
	u.Light = vec4(normal(0.85*math.Cos(turn(41, 2.6)), 0.42, 0.45+0.25*math.Sin(turn(41, 2.6))), 0)
	for i, m := range [...]struct{ period, phase, orbit, radius float64 }{
		{7, 0.4, 1.42, 0.055}, {11, 2.3, 1.62, 0.095}, {17, 4.0, 1.84, 0.070}, {29, 5.4, 2.06, 0.120},
	} {
		u.Moons[i] = vec4(around(turn(m.period, m.phase), m.orbit), m.radius)
	}
	return u
}

func normal(x, y, z float64) [3]float64 {
	n := math.Sqrt(x*x + y*y + z*z)
	return [3]float64{x / n, y / n, z / n}
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// vec4 rounds to 20 bits, so that the last bits of a sine, which are not the
// same on every processor, move no pixel of it.
func vec4(v [3]float64, w float64) [4]float32 {
	round := func(x float64) float32 { return float32(math.Round(x*(1<<20)) / (1 << 20)) }
	return [4]float32{round(v[0]), round(v[1]), round(v[2]), round(w)}
}
