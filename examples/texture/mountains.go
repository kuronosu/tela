package main

// mountainsShader is shaders/mountains.glsl: ranges in the mist at dusk, over
// still water.
type mountainsShader struct {
	sunX, sunY, twinkle float32
	clouds, mist        float32 // where what the wind carries is
	waves, drift        float32
	pixel1              float32 // a pixel
	margin              float32 // two tiles
	sunEdge             float32 // half the pixels of a unit

	// The sky's color at a height, the last one's, how dark it is there and
	// whether clouds are.
	skyY, skyDark, skyBand float32
	skyColor               vec3

	// What the water has in common at a height, the last one's: how deep
	// below the shore it is, the scale of the waves, how much of them shows,
	// and how much light.
	waterAt, depth, near, z    float32
	show1, show2, show3, part1 float32
	dim, shine, shore          float32

	top                 float32 // above all the ranges
	columns             []mountainColumn
	stars1, stars2      stars
	cloudBend           [2]noise2
	cloud               [3]noise2
	wisps               [2]noise2
	air                 grid // the clouds and the wisps of mist
	wave1, wave2, wave3 noise2
}

// mountainColumn is what does not change along a column of pixels: the
// heights of the ranges there, how thick the mist is before each, and the
// light of the sun on it.
type mountainColumn struct {
	height, mist [ranges]float32
	blurred      [ranges]float32 // the heights, without detail
	sun          float32
}

const (
	ranges = 6
	// waterY is the shore, at the end of the water.
	waterY = -0.205
)

// The ranges, the farthest first: where they stand, how tall they are, how
// many mountains there are, and how fast they go by.
var (
	rangeBase  = [ranges]float32{0.030, 0.012, -0.012, -0.050, -0.100, -0.172}
	rangeTall  = [ranges]float32{0.17, 0.20, 0.20, 0.19, 0.17, 0.075}
	rangeScale = [ranges]float32{1.5, 1.7, 1.9, 2.1, 2.3, 3.6}
	rangeSpeed = [ranges]float32{1, 1.6, 2.3, 3.1, 4, 5}
	rangeColor = [ranges]vec3{{0.40, 0.235, 0.140}, {0.235, 0.145, 0.095}, {0.120, 0.082, 0.062}, {0.052, 0.042, 0.037}, {0.019, 0.0175, 0.0175}, {0.0052, 0.0054, 0.0060}}
	rangeMist  = [ranges]vec3{{0.74, 0.44, 0.25}, {0.60, 0.36, 0.215}, {0.43, 0.27, 0.175}, {0.27, 0.185, 0.135}, {0.155, 0.118, 0.097}, {0.075, 0.063, 0.057}}
)

func newMountains(u *uniforms) *mountainsShader {
	s := &mountainsShader{
		sunX: u.V[0][0], sunY: u.V[0][1], twinkle: u.V[0][2],
		clouds: u.V[1][0], mist: u.V[1][1], waves: u.V[1][2], drift: u.V[1][3],
		pixel1: 1 / u.Size[2],
	}
	s.top, s.skyY, s.waterAt, s.sunEdge = -9, -9, -9, float32(0.5*u.Size[2])
	s.margin = 2 * u.V[6][0] * s.pixel1
	s.columns = make([]mountainColumn, int(u.Size[0]))
	for x := range s.columns {
		s.columns[x].set(s, float32((float32(x)+0.5-float32(0.5*u.Size[0]))*u.V[6][1]), u.V[0][3])
	}
	return s
}

// clone is the shader for another goroutine: the columns are the same ones.
func (s *mountainsShader) clone(u *uniforms) *mountainsShader {
	c := *s
	c.air = newGrid(u, c.airAt)
	return &c
}

func (c *mountainColumn) set(s *mountainsShader, x, by float32) {
	for i := range c.height {
		k := float32(i)
		h, b := ridge(float32(x*rangeScale[i])+float32(by*rangeSpeed[i])+float32(7.3*k), int32(i))
		c.height[i], c.blurred[i] = rangeBase[i]+float32(rangeTall[i]*(h-0.42)), rangeBase[i]+float32(rangeTall[i]*(b-0.42))
		s.top = max(s.top, c.height[i], c.blurred[i])
		c.mist[i] = noise1(float32(x*2.1)+float32(s.mist*(1+float32(0.3*k)))+float32(9.1*k), int32(100+i))
	}
	c.sun = decay(abs(x-s.sunX) * 3.2)
}

// ridge is the outline of a range, a sum of sharp crests; and without the
// finer ones.
func ridge(x float32, seed int32) (s, blurred float32) {
	x = float32(x)
	var a float32 = 0.5
	for o := range int32(6) {
		if o == 2 {
			blurred = s + 0.06
		}
		v := 1 - abs(float32(2*noise1(x, seed+16*o))-1)
		s += float32(a * float32(v*v))
		a *= 0.47
		x = float32(x*2.07) + 3.7
	}
	return s, blurred
}

// airAt is how much cloud there is at a point, and how much mist.
func (s *mountainsShader) airAt(x, y float32) (out vec3) {
	if y > -0.02-s.margin {
		v := float32(0.5 * fbm2(s.cloudBend[:], float32(x*2.2), y*4))
		out.x = fbm2(s.cloud[:], float32(x*1.35)+s.clouds, float32(y*10)+3+v)
	}
	if y < 0.25+s.margin && y > waterY-s.margin {
		out.y = fbm2(s.wisps[:], float32(x*2.6)+float32(1.7*s.mist), float32(y*15)-s.mist) * 1.333333
	}
	return out
}

func (s *mountainsShader) pixel(col, row int, x, y float32) vec3 {
	if y >= waterY {
		return s.land(col, row, x, y, s.pixel1, false)
	}
	return s.water(col, row, x, y)
}

// land is the ranges before the sky, with outlines as soft as soft; far is
// what water shows of them, without detail.
func (s *mountainsShader) land(col, row int, x, y, soft float32, far bool) vec3 {
	if y > s.top+soft {
		return s.sky(col, row, x, y, far)
	}
	m := &s.columns[col]
	height := &m.height
	if far {
		height = &m.blurred
	}
	var c vec3
	sharp := 1 / soft
	thin := float32(1)
	if far {
		thin = 0.32
	}
	left := float32(1) // of what lies behind
	var wisp float32 = -1
	for i := ranges - 1; i >= 0; i-- {
		cover := clamp(float32((height[i]-y)*sharp) + 0.5)
		if cover <= 0 {
			continue
		}
		if wisp < 0 {
			wisp = 0.5
			if !far {
				wisp = s.air.seek(col, row).y
			}
		}
		up := y - rangeBase[i] + 0.085
		if up < 0 {
			up = 0
		}
		fog := clamp(float32(thin*decay(up*(17-float32(1.2*float32(i))))) * (0.25 + float32(0.85*m.mist[i]) + float32(0.75*wisp)))
		k := mix(rangeColor[i], rangeMist[i], fog)
		k = k.plus(vec3{0.55, 0.27, 0.11}, float32(m.sun*(0.42-float32(0.08*float32(i)))))
		c = c.plus(k, float32(left*cover))
		if left = float32(left * (1 - cover)); left <= 0 {
			return c
		}
	}
	return c.plus(s.sky(col, row, x, y, far), left)
}

func (s *mountainsShader) sky(col, row int, x, y float32, far bool) vec3 {
	if y != s.skyY {
		h := clamp((y + 0.06) * 1.8)
		c := mix(vec3{0.78, 0.36, 0.155}, vec3{0.225, 0.150, 0.115}, smooth(0, 0.40, h))
		s.skyColor = mix(c, vec3{0.011, 0.016, 0.0215}, smooth(0.18, 0.95, h))
		s.skyY, s.skyDark = y, smooth(0.16, 0.46, y)
		s.skyBand = float32(smooth(0.01, 0.14, y) * (1 - smooth(0.26, 0.48, y)))
	}
	c := s.skyColor
	d := sqrt(dot2(x-s.sunX, y-s.sunY))
	c = c.plus(vec3{1, 0.50, 0.20}, float32(0.62*decay(d*6.5))+float32(0.14*decay(d*2.6)))
	if far {
		return c
	}
	if dark := s.skyDark; dark > 0 {
		k := float32(0.75*dark) * (1 - decay(d*1.5))
		t := s.stars1.at(vec3{}, x*28, y*28, 0, 0.13, 70, s.twinkle)
		t = s.stars2.at(t, x*60, y*60, 1, 0.15, 42, s.twinkle)
		c = c.plus(t, k)
	}
	// The sun, behind a little of the haze near the horizon.
	c = mix(c, vec3{3.2, 2.0, 1.05}, clamp(float32((0.046-d)*s.sunEdge)+0.5))
	// Streaks of cloud, lit from below.
	if band := s.skyBand; band > 0 {
		k := float32(smooth(0.42, 0.72, s.air.seek(col, row).x) * band)
		c = mix(c, mix(vec3{0.105, 0.078, 0.068}, vec3{1.15, 0.52, 0.22}, decay(d*3.1)), float32(0.72*k))
	}
	return c
}

// startWater prepares what the water has in common at a height.
func (s *mountainsShader) startWater(y float32) {
	dp := waterY - y
	s.waterAt, s.depth, s.near = y, dp, clamp(dp*3.3)
	s.z = 1 / sqrt(dp+0.012)
	// How much of a wave a pixel covers: those smaller than it are not seen.
	foot := float32(float32(float32(0.5*float32(s.z*s.z))*s.z) * s.pixel1)
	fine := smooth(0.3, 0.9, float32(foot*21))
	s.show1, s.show2, s.show3 = 1-smooth(0.3, 0.9, float32(foot*9)), 1-fine, 1-smooth(0.3, 0.9, float32(foot*47))
	s.part1 = lerp(0.14, 0.50, fine)
	s.dim = lerp(0.78, 0.17, smooth(0, 0.9, s.near))
	s.shine = float32(smooth(0, 0.05, dp) * lerp(0.95, 0.42, s.near))
	s.shore = decay(dp * 60)
}

// water is the land upside down, in waves that the distance makes finer, and
// the path of the sun on them.
func (s *mountainsShader) water(col, row int, x, y float32) vec3 {
	if y != s.waterAt {
		s.startWater(y)
	}
	dp, near := s.depth, s.near
	u := float32(x * s.z)
	w1 := float32((s.wave1.at(float32(u*0.8)+s.drift, float32(s.z*9)+s.waves) - 0.5) * s.show1)
	w2 := float32((s.wave2.at(float32(u*1.9)-s.drift, float32(s.z*21)+float32(1.5*s.waves)+5) - 0.5) * s.show2)
	w3 := float32((s.wave3.at(float32(u*4.4)+float32(1.7*s.drift), float32(s.z*47)+float32(2.1*s.waves)+9) - 0.5) * s.show3)
	w := float32(s.part1*w1) + float32(0.34*w2) + float32(0.40*w3)

	up := float32(dp*0.95) + float32((float32(0.60*dp)+0.010)*w)
	c := s.land(col, row, x, waterY+up, s.pixel1+0.018+float32(0.36*dp), true)
	// The water darkens toward the viewer, who looks into it there, and the
	// back of the waves more.
	c = c.mul(float32(s.dim * (1 + float32(float32(1.1*w)*near))))
	c = c.add(vec3{0.0040, 0.0045, 0.0052})

	// The path of the sun.
	side := float32(abs(x-s.sunX) / (0.018 + float32(0.30*dp)))
	glint := smooth(0.10, 0.34, float32(0.65*w1)+float32(0.85*w2)+float32(0.95*w3)+0.24-float32(0.20*side))
	c = c.plus(vec3{1.35, 0.62, 0.24}, float32(glint*s.shine))
	if glow := 1 - float32(float32(side*side)*0.04); glow > 0 {
		c = c.plus(vec3{0.55, 0.26, 0.11}, float32(0.16*float32(glow*glow)))
	}
	// Mist at the shore.
	return c.plus(vec3{0.20, 0.135, 0.098}, float32(float32(0.55+float32(0.5*s.columns[col].mist[4]))*s.shore))
}
