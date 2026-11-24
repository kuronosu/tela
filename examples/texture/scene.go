package main

import (
	"math"
	"time"
)

// Scene is one of the scenes.
type Scene int

const (
	Planet Scene = iota
	Mountains
	Satin
)

// scenes are what the window says of them.
var scenes = [...]struct{ name, about string }{
	Planet:    {"Planet", "A ringed planet the color of sand, turning among stars"},
	Mountains: {"Mountains", "Mountain ranges in the mist at dusk, over still water"},
	Satin:     {"Satin", "Folds of graphite satin, flowing under a warm and a cool light"},
}

// uniforms is what a scene needs to know to shade any of its pixels: the
// push constants of shaders/scene.frag, and the input of shade.go. The
// vectors have four components, as GLSL lays them out; 128 bytes are what
// every GPU takes.
type uniforms struct {
	// Size is the width and the height in pixels, the pixels per unit of the
	// scene, and the Scene.
	Size [4]float32
	// V is the scene's own, but for the last: the pixels of a side of the
	// tiles of what changes slowly (grid), and 1 / pixels per unit.
	V [7][4]float32
}

// sceneAt returns a scene at a time of its clock, for a picture of w by h
// pixels.
func sceneAt(s Scene, at time.Duration, w, h int) uniforms {
	u := uniforms{Size: [4]float32{float32(w), float32(h), float32(min(float64(h), float64(w)/1.6)), float32(s)}}
	tiles := [...]float64{Planet: 54, Mountains: 144, Satin: 290}[s] // to a unit
	u.V[6] = [4]float32{float32(max(1, math.Round(float64(u.Size[2])/tiles))), 1 / u.Size[2]}
	// turn is how far what turns in period seconds is in its turn, from 0 to
	// 1, however long the clock runs.
	turn := func(period, phase float64) float64 {
		t := at.Seconds()/period + phase
		return t - math.Floor(t)
	}
	sin := func(period, phase float64) float64 { return math.Sin(2 * math.Pi * turn(period, phase)) }
	cos := func(period, phase float64) float64 { return math.Cos(2 * math.Pi * turn(period, phase)) }

	switch s {
	case Mountains:
		u.V[0] = vec4(
			0.27+0.004*sin(61, 0), // the sun
			0.026-0.018*cos(150, 0.03),
			turn(4, 0), // the twinkling of the stars
			at.Seconds()*0.004)
		u.V[1] = vec4(
			at.Seconds()*0.012, // the clouds
			at.Seconds()*0.020, // the mist
			at.Seconds()*0.45,  // the waves, toward the viewer
			at.Seconds()*0.06)  // and along the shore
	case Satin:
		u.V[0] = vec4(0.4+0.55*sin(71, 0.944), -0.2+0.40*sin(59, 0.432), -0.12+0.35*sin(83, 0.952), 0.30*sin(67, 0.940))
		u.V[1] = vec4(turn(28, 0), turn(63, 0), 0, 0)
		a := sin(47, 0)
		u.V[2] = vec3to4(normal(-0.55+0.10*a, 0.75, 0.55), 0)
		u.V[3] = vec3to4(normal(0.9, -0.35+0.10*a, 0.45), 0)
	default:
		// The axis leans toward the viewer and sways.
		axis := normal(-0.36+0.018*sin(37, 0), 1, 0.30+0.016*sin(53, 0.3))
		// Two directions across it: what turns around it is made of them.
		e1 := normal(axis[1], -axis[0], 0)
		e2 := cross(axis, e1)
		around := func(period, phase, r, up float64) [3]float64 {
			c, s := cos(period, phase)*r, sin(period, phase)*r
			return [3]float64{
				e1[0]*c + e2[0]*s + axis[0]*up,
				e1[1]*c + e2[1]*s + axis[1]*up,
				e1[2]*c + e2[2]*s + axis[2]*up,
			}
		}
		u.V[0] = vec3to4(axis, turn(4, 0)) // and the twinkling of the stars
		// The planet turns, and its finer clouds a little faster.
		u.V[1] = vec3to4(around(75, 0.02, 1, 0), 0)
		u.V[2] = vec3to4(around(66, 0.01, 1, 0), 0)
		u.V[3] = vec3to4(normal(-0.86, 0.34+0.05*sin(43, 0.1), 0.30+0.07*sin(71, 0)), 0)
		u.V[4] = vec3to4(around(90, 0.55, 2.62, 0.25), 0.060) // the moons
		u.V[5] = vec3to4(around(58, 0.905, 2.45, 0.05), 0.036)
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
func vec4(x, y, z, w float64) [4]float32 {
	round := func(x float64) float32 { return float32(math.Round(x*(1<<20)) / (1 << 20)) }
	return [4]float32{round(x), round(y), round(z), round(w)}
}

func vec3to4(v [3]float64, w float64) [4]float32 { return vec4(v[0], v[1], v[2], w) }
