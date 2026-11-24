package main

// planetShader is shaders/planet.glsl: a giant planet the color of sand, with
// rings of fine grain and two moons. Lengths are in radii of it.
type planetShader struct {
	axis, light      vec3
	meridian, east   vec3 // what turns with the planet
	meridian2, east2 vec3 // and with its finer clouds
	shade            float32
	ringSoft         float32 // half a pixel of the rings
	twinkle          float32
	pixel1, pixels   float32 // a pixel, and the pixels of a radius
	moons            [2]planetMoon
	planeX, planeY   float32 // the height of the plane of the rings over a point

	dust1, dust2   [3]noise2
	stars1, stars2 stars
	dust           grid
	rings, shadow  [3]line
	clouds         [2]noise3
	fine           [3]noise3
	bands          [3]line
	moonRock       [2][2]noise3
}

type planetMoon struct {
	at          vec3
	r           float32
	far         float32 // the radius, and a pixel
	bottom, top float32
	color       vec3
}

const (
	planetR = 0.36
	planetX = 0.10
	planetY = -0.02
	ringIn  = 1.30
	ringOut = 2.31
)

func newPlanet(u *uniforms) *planetShader {
	s := &planetShader{
		axis: xyz(u.V[0]), twinkle: u.V[0][3],
		meridian: xyz(u.V[1]), meridian2: xyz(u.V[2]), light: xyz(u.V[3]),
	}
	s.east = s.axis.cross(s.meridian)
	s.east2 = s.axis.cross(s.meridian2)
	s.shade = s.axis.dot(s.light)
	s.pixels = float32(u.Size[2] * planetR)
	s.pixel1 = 1 / s.pixels
	s.ringSoft = float32(0.8*s.pixel1) / s.axis.z
	s.dust = newGrid(u, s.dustAt)
	s.planeX, s.planeY = -s.axis.x/s.axis.z, -s.axis.y/s.axis.z
	for i, color := range [...]vec3{{0.62, 0.54, 0.45}, {0.52, 0.53, 0.54}} {
		at, r := xyz(u.V[4+i]), u.V[4+i][3]
		far := r + s.pixel1
		s.moons[i] = planetMoon{at: at, r: r, color: color, far: far, bottom: at.y - far, top: at.y + far}
	}
	return s
}

func (s *planetShader) pixel(col, row int, px, py float32) vec3 {
	x, y := float32((px-planetX)*(1/planetR)), float32((py-planetY)*(1/planetR))
	dd := dot2(x, y)
	r, cover := float32(2), float32(0) // of the planet
	if dd < 2 {
		r = sqrt(dd)
		cover = clamp(float32((1-r)*s.pixels) + 0.5)
	}
	// Where the ray meets the plane of the rings.
	q := vec3{x, y, float32(s.planeX*x) + float32(s.planeY*y)}
	qq := q.dot(q)
	ringed := qq > ringIn*ringIn && qq < ringOut*ringOut
	rr := float32(0)
	if ringed {
		rr = sqrt(qq)
	}
	var c vec3
	if cover < 1 {
		c = s.dust.seek(col, row)
		if !ringed { // no stars behind the rings
			c = s.stars1.at(c, px*26, py*26, 0, 0.13, 70, s.twinkle)
			c = s.stars2.at(c, px*55, py*55, 1, 0.15, 42, s.twinkle)
		}
		for i := range s.moons {
			if s.moons[i].at.z < 0 && s.moons[i].near(x, y) {
				c = s.moon(c, x, y, i)
			}
		}
		if ringed && q.z <= 0 {
			c = s.ring(c, q, rr)
		}
		c = s.glow(c, x, y, r)
	}
	if cover > 0 {
		c = mix(c, s.planet(x, y, dd), cover)
	}
	if ringed && q.z > 0 {
		c = s.ring(c, q, rr)
	}
	for i := range s.moons {
		if s.moons[i].at.z >= 0 && s.moons[i].near(x, y) {
			c = s.moon(c, x, y, i)
		}
	}
	return c
}

// dustAt is the color of the sky between the stars.
func (s *planetShader) dustAt(x, y float32) vec3 {
	c := vec3{0.0040, 0.0048, 0.0064}
	c = c.plus(vec3{0.030, 0.022, 0.016}, smooth(0.36, 0.72, fbm2(s.dust1[:], float32(1.5*x)+float32(0.4*y)+5, float32(1.9*y)-float32(0.5*x)+1)))
	return c.plus(vec3{0.010, 0.016, 0.024}, smooth(0.34, 0.78, fbm2(s.dust2[:], float32(1.1*x)+31, float32(1.3*y)+4)))
}

// glow is the air around the planet, on the side of the light.
func (s *planetShader) glow(c vec3, x, y, r float32) vec3 {
	if r > 1.25 {
		return c
	}
	side := smooth(0.10, 0.95, (float32(x*s.light.x)+float32(y*s.light.y))/r)
	return c.plus(vec3{0.42, 0.34, 0.26}, float32(float32(0.34*side)*decay(abs(r-1)*34)))
}

func (s *planetShader) planet(x, y, dd float32) vec3 {
	n := vec3{x, y, 0}
	if dd < 1 {
		n.z = sqrt(1 - dd)
	}
	ndl := n.dot(s.light)
	c := vec3{0.0036, 0.0030, 0.0024} // the night
	if ndl <= -0.15 {
		return c
	}
	lat := n.dot(s.axis)
	rim := 1 - n.z
	c = c.plus(vec3{0.85, 0.62, 0.42}, float32(float32(0.14*float32(rim*rim))*smooth(-0.05, 0.6, ndl)))
	c = c.plus(vec3{0.55, 0.26, 0.12}, float32(float32(0.11*smooth(-0.10, 0.04, ndl))*(1-smooth(0.04, 0.3, ndl))))
	diff := float32(1.2 * smooth(-0.06, 1, ndl))
	if diff <= 0 {
		return c
	}
	if hit := -lat / s.shade; hit > 0 { // the shadow of the rings
		q := n.plus(s.light, hit)
		if r := sqrt(q.dot(q)); r > ringIn && r < ringOut {
			diff = float32(diff * (1 - float32(0.72*ringDensity(&s.shadow, r, 0.03))))
		}
	}
	return c.plus(s.ground(n, lat), float32(diff*(0.55+float32(0.45*sqrt(n.z)))))
}

// ground is the color of the clouds: bands that the longitude bends.
func (s *planetShader) ground(n vec3, lat float32) vec3 {
	lx, lz := n.dot(s.meridian), n.dot(s.east)
	fx, fz := n.dot(s.meridian2), n.dot(s.east2)
	w1 := fbm3(s.clouds[:], float32(lx*2.2)+3, lat*5.5, lz*2.2) + 0.09
	w2 := fbm3(s.fine[:], float32(fx*3.4)+float32(9*w1), float32(lat*9)+float32(2*w1), float32(fz*3.4)+1) + 0.03
	v := float32(lat*3.3) + float32(0.42*(w1-0.47)) + float32(0.30*(w2-0.5))
	f := 0.10 + float32(0.56*s.bands[0].soft(float32(v*1.9)+10, 1)) +
		float32(0.27*s.bands[1].soft(float32(v*5.3)+40, 4)) +
		float32(0.14*s.bands[2].soft(float32(v*14)+70, 8)) + float32(0.28*(w2-0.5))
	c := mix(vec3{0.20, 0.105, 0.060}, vec3{0.52, 0.31, 0.17}, smooth(0.15, 0.45, f))
	c = mix(c, vec3{0.80, 0.60, 0.40}, smooth(0.40, 0.62, f))
	c = mix(c, vec3{0.93, 0.85, 0.72}, smooth(0.60, 0.85, f))
	return mix(c, vec3{0.30, 0.24, 0.20}, float32(0.65*smooth(0.62, 0.98, abs(lat))))
}

// ring lays the rings over c, at a point of their plane r from the center.
func (s *planetShader) ring(c, q vec3, r float32) vec3 {
	a := ringDensity(&s.rings, r, s.ringSoft)
	if a <= 0 {
		return c
	}
	k := 0.03 + float32(0.95*s.lit(q))
	return mix(c, mix(vec3{0.50, 0.40, 0.31}, vec3{0.88, 0.80, 0.69}, smooth(1.35, 2, r)).mul(k), a)
}

// lit is how much light the planet lets through to q.
func (s *planetShader) lit(q vec3) float32 {
	b := q.dot(s.light)
	if b >= 0 {
		return 1
	}
	d := q.dot(q) - float32(b*b)
	if d <= 0 {
		return 0
	}
	return smooth(0.90, 1.06, sqrt(d))
}

// ringDensity is how opaque the rings are at distance r of the center.
func ringDensity(n *[3]line, r, soft float32) float32 {
	e := float32(1)
	if r < 1.35+soft || r > 2.23 {
		if e = float32(smooth(1.32-soft, 1.35+soft, r) * (1 - smooth(2.23, 2.28+soft, r))); e <= 0 {
			return 0
		}
	}
	gap := float32(1)
	if g := abs(r - 1.985); g < 0.035+soft {
		gap = smooth(0.012, 0.035+soft, g)
	}
	if g := abs(r - 2.17); g < 0.02+soft {
		gap = float32(gap * (1 - float32(0.75*(1-smooth(0, 0.02+soft, g)))))
	}
	d := 0.35 + float32(0.65*n[0].at(r*9, 3)) + float32(0.35*(n[1].at(r*31, 7)-0.5))
	if grain := clamp(1.6 - float32(soft*100)); grain > 0 { // it is finer than a pixel of a small picture
		d += float32(float32(0.22*grain) * (n[2].at(r*97, 9) - 0.5))
	}
	return float32(float32(float32(e*gap)*clamp(d)) * lerp(0.55, 0.95, smooth(1.4, 1.75, r)))
}

// near reports whether a point may be of the moon.
func (m *planetMoon) near(x, y float32) bool {
	return y > m.bottom && y < m.top && x > m.at.x-m.far && x < m.at.x+m.far
}

func (s *planetShader) moon(c vec3, x, y float32, i int) vec3 {
	m := &s.moons[i]
	qx, qy := x-m.at.x, y-m.at.y
	dd := dot2(qx, qy)
	n := vec3{qx, qy, 0}
	if rr := float32(m.r * m.r); dd < rr {
		n.z = sqrt(rr - dd)
	}
	n = n.mul(1 / m.r)
	k := 0.45 + float32(1.1*fbm3(s.moonRock[i][:], float32(n.x*2.6)+9, float32(n.y*2.6)+float32(4*float32(i)), n.z*2.6))
	col := vec3{0.003, 0.003, 0.003}.plus(m.color, float32(float32(k*smooth(-0.05, 1, n.dot(s.light)))*s.lit(m.at)))
	return mix(c, col, clamp(float32((m.r-sqrt(dd))*s.pixels)+0.5))
}
