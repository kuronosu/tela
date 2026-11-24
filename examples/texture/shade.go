package main

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
)

// The scene drawn by the CPU: shaders/scene.frag, function by function. The
// products are converted (float32(a*b)) so that no compiler fuses them with
// the sum around them, as Go's does on arm64: every architecture draws the
// same pixels.

const (
	planetR = 0.62
	ringIn  = 0.80
	ringOut = 1.22
)

var moonColors = [4]vec3{{0.85, 0.70, 0.55}, {0.45, 0.65, 0.85}, {0.75, 0.75, 0.78}, {0.80, 0.45, 0.35}}

// drawScene draws the scene into img, whose size the uniforms have, with
// every processor.
func drawScene(img *image.RGBA, u *uniforms) {
	s := newShader(u)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	const rows = 16
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), (h+rows-1)/rows) {
		wg.Go(func() {
			for {
				y0 := int(next.Add(rows)) - rows
				if y0 >= h {
					return
				}
				for y := y0; y < min(y0+rows, h); y++ {
					s.row(img.Pix[y*img.Stride:][:4*w], y)
				}
			}
		})
	}
	wg.Wait()
}

// shader is the uniforms, and what follows from them for every pixel.
type shader struct {
	w, h, unit            float32
	axis, meridian, light vec3
	across                vec3 // cross(axis, meridian)
	half                  vec3 // between the light and the viewer
	ringSoft              float32
	moons                 [4]moonShader
}

type moonShader struct {
	at    vec3
	r     float32
	color vec3
}

func newShader(u *uniforms) *shader {
	v := func(a [4]float32) vec3 { return vec3{a[0], a[1], a[2]} }
	s := &shader{w: u.Size[0], h: u.Size[1], unit: u.Size[2], axis: v(u.Axis), meridian: v(u.Meridian), light: v(u.Light)}
	a, m := s.axis, s.meridian
	s.across = vec3{
		float32(a.y*m.z) - float32(a.z*m.y),
		float32(a.z*m.x) - float32(a.x*m.z),
		float32(a.x*m.y) - float32(a.y*m.x),
	}
	s.half = vec3{s.light.x, s.light.y, s.light.z + 1}
	s.half = s.half.mul(1 / sqrt(s.half.dot(s.half)))
	s.ringSoft = 1 / float32(s.unit*s.axis.z)
	for i, m := range u.Moons {
		s.moons[i] = moonShader{at: v(m), r: m[3], color: moonColors[i]}
	}
	return s
}

// row draws row y into pix.
func (s *shader) row(pix []byte, y int) {
	fy := float32(y) + 0.5
	py := -(fy - float32(0.5*s.h)) / s.unit
	t := fy / s.h
	for x := 0; x < len(pix); x += 4 {
		px := (float32(x/4) + 0.5 - float32(0.5*s.w)) / s.unit
		c := s.pixel(px, py, t)
		pix[x], pix[x+1], pix[x+2], pix[x+3] = encode(c.x), encode(c.y), encode(c.z), 255
	}
}

func (s *shader) pixel(x, y, t float32) vec3 {
	c := sky(x, y, t)
	for i := range s.moons {
		if m := &s.moons[i]; m.at.z < 0 {
			c = s.moon(c, x, y, m)
		}
	}
	c = s.ring(c, x, y, false)
	c = s.planet(c, x, y)
	c = s.ring(c, x, y, true)
	for i := range s.moons {
		if m := &s.moons[i]; m.at.z >= 0 {
			c = s.moon(c, x, y, m)
		}
	}
	return c
}

func sky(x, y, t float32) vec3 {
	c := mix(vec3{0.010, 0.014, 0.045}, vec3{0.030, 0.012, 0.050}, t)
	c = c.add(vec3{0.085, 0.040, 0.160}.mul(1 / (1 + float32(2.2*dot2(x, y)))))
	return c.add(vec3{0.010, 0.075, 0.095}.mul(1 / (1 + float32(1.6*dot2(x-1.5, y+0.9)))))
}

func (s *shader) moon(c vec3, x, y float32, m *moonShader) vec3 {
	qx, qy := x-m.at.x, y-m.at.y
	dd, far := dot2(qx, qy), m.r+1/s.unit
	if dd > float32(far*far) {
		return c
	}
	n := vec3{qx, qy, sqrt(max(float32(m.r*m.r)-dd, 0))}.mul(1 / m.r)
	diff := max(n.dot(s.light), 0) * s.lit(m.at.add(n.mul(m.r)))
	cover := clamp(float32((m.r-sqrt(dd))*s.unit) + 0.5)
	return mix(c, m.color.mul(0.03+float32(0.97*diff)), cover)
}

func (s *shader) planet(c vec3, x, y float32) vec3 {
	dd, far := dot2(x, y), planetR+1/s.unit
	if dd > float32(far*far) {
		return c
	}
	n := vec3{x, y, sqrt(max(planetR*planetR-dd, 0))}.mul(1 / float32(planetR))
	lat, lx, lz := n.dot(s.axis), n.dot(s.meridian), n.dot(s.across)
	band := wave(float32(lat*2.3) + float32(0.07*wave(lx*1.4)) + float32(0.05*wave(float32(lz*1.9)+lat)))
	storm := wave(float32(lx*1.7)+float32(0.2*band)) * wave(float32(lz*1.3)-lat)
	body := mix(vec3{0.34, 0.13, 0.42}, vec3{0.95, 0.58, 0.30}, 0.5+float32(0.5*band))
	body = mix(body, vec3{0.98, 0.86, 0.70}, 0.35*max(storm, 0))
	diff := max(n.dot(s.light), 0)
	k := s.axis.dot(s.light)
	if abs(k) < 0.001 {
		k = 0.001
	}
	if hit := -planetR * lat / k; hit > 0 {
		q := n.mul(planetR).add(s.light.mul(hit))
		diff *= 1 - float32(0.75*ringDensity(sqrt(q.dot(q)), 0.02))
	}
	rim := 1 - n.z
	body = body.mul(0.025 + float32(0.95*diff))
	body = body.add(vec3{0.25, 0.30, 0.65}.mul(rim * rim * rim * (0.15 + float32(0.85*diff))))
	g := max(n.dot(s.half), 0)
	g *= g
	g *= g
	g *= g
	g *= g
	g *= g
	g = float32(0.16 * g * diff)
	body = body.add(vec3{g, g, g})
	return mix(c, body, clamp(float32((planetR-sqrt(dd))*s.unit)+0.5))
}

func (s *shader) ring(c vec3, x, y float32, front bool) vec3 {
	z := -(float32(s.axis.x*x) + float32(s.axis.y*y)) / s.axis.z
	if (z > 0) != front {
		return c
	}
	q := vec3{x, y, z}
	rr := q.dot(q)
	if rr < 0.5 || rr > 1.7 {
		return c
	}
	d := ringDensity(sqrt(rr), s.ringSoft)
	if d == 0 {
		return c
	}
	return mix(c, vec3{0.86, 0.74, 0.62}.mul(0.04+float32(0.80*s.lit(q))), d)
}

func (s *shader) lit(q vec3) float32 {
	b := q.dot(s.light)
	if b >= 0 {
		return 1
	}
	return ramp(planetR-0.05, planetR+0.02, sqrt(max(q.dot(q)-float32(b*b), 0)))
}

func ringDensity(r, soft float32) float32 {
	edge := ramp(ringIn-soft, ringIn+soft, r) * (1 - ramp(ringOut-soft, ringOut+soft, r))
	if edge == 0 {
		return 0
	}
	gap := ramp(0, 0.015+soft, abs(r-1.06))
	return edge * gap * (0.62 + float32(0.24*wave(r*11)) + float32(0.10*wave(r*37)))
}

// wave is nearly a sine of period 1 (a parabola per half).
func wave(t float32) float32 {
	t = float32(t) // a product of the caller's too
	t = float32(2*(t-float32(math.Floor(float64(t))))) - 1
	return 4 * t * (abs(t) - 1)
}

func ramp(lo, hi, x float32) float32 { return clamp((x - lo) / (hi - lo)) }

func clamp(x float32) float32 { return max(0, min(x, 1)) }

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrt(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// encode returns the byte of a component in linear light: textures take
// sRGB, and a square root is close enough to its curve.
func encode(x float32) byte { return byte(float32(sqrt(clamp(x))*255) + 0.5) }

// vec3 is a vector, or a color in linear light.
type vec3 struct{ x, y, z float32 }

func (a vec3) dot(b vec3) float32 { return float32(a.x*b.x) + float32(a.y*b.y) + float32(a.z*b.z) }

func (a vec3) add(b vec3) vec3 { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }

func (a vec3) mul(k float32) vec3 { return vec3{float32(a.x * k), float32(a.y * k), float32(a.z * k)} }

func dot2(x, y float32) float32 { return float32(x*x) + float32(y*y) }

func mix(a, b vec3, t float32) vec3 {
	return vec3{a.x + float32((b.x-a.x)*t), a.y + float32((b.y-a.y)*t), a.z + float32((b.z-a.z)*t)}
}
