package main

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
)

// The scenes drawn by the CPU: shaders/scene.frag, function by function.
// shade.go is shaders/common.glsl, and planet.go, mountains.go and satin.go
// are the files of their names.
//
// Both languages compute the same: everything is made of sums, products,
// divisions, square roots and integer hashes, whose results the standards of
// both fix (but for the last bit), and of no sine, exponential or power, which
// every GPU rounds in its own way. Here the products are converted
// (float32(a*b)) so that no compiler fuses them with the sum around them, as
// Go's does on arm64: every architecture draws the same pixels.

// shader draws the pixels of a scene. Each goroutine has its own: it keeps
// what it computed for the last pixel.
type shader interface {
	// pixel returns the color of a pixel, which is at a point of the scene,
	// in linear light.
	pixel(col, row int, x, y float32) vec3
}

// newShader prepares the scene of the uniforms, and returns what makes a
// shader for each goroutine.
func newShader(u *uniforms) func() shader {
	switch Scene(u.Size[3]) {
	case Mountains:
		m := newMountains(u)
		return func() shader { return m.clone(u) }
	case Satin:
		return func() shader { return newSatin(u) }
	}
	return func() shader { return newPlanet(u) }
}

// drawScene draws the scene into img, whose size the uniforms have, with
// every processor.
func drawScene(img *image.RGBA, u *uniforms) {
	mk := newShader(u)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	const rows = 8
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), (h+rows-1)/rows) {
		wg.Go(func() {
			s := mk()
			for {
				y0 := int(next.Add(rows)) - rows
				if y0 >= h {
					return
				}
				for y := y0; y < min(y0+rows, h); y++ {
					drawRow(s, img.Pix[y*img.Stride:][:4*w], u, y)
				}
			}
		})
	}
	wg.Wait()
}

// drawRow draws row y into pix.
func drawRow(s shader, pix []byte, u *uniforms, y int) {
	w, h, unit := u.Size[0], u.Size[1], u.V[6][1]
	py := float32((float32(0.5*h) - (float32(y) + 0.5)) * unit)
	for x := range len(pix) / 4 {
		px := float32((float32(x) + 0.5 - float32(0.5*w)) * unit)
		c := s.pixel(x, y, px, py)
		d := hash(int32(x), int32(y), 77)
		p := pix[4*x:][:4]
		p[0], p[1], p[2], p[3] = encode(c.x, d), encode(c.y, d), encode(c.z, d), 255
	}
}

// encode returns the byte of a component in linear light: a filmic curve,
// then sRGB, for which a square root is close enough, then a rounding up or
// down at random (d), the same on every frame, so that slow gradients show no
// steps.
func encode(x, d float32) byte {
	if x <= 0 {
		return 0
	}
	x = float32(x*(float32(2.51*x)+0.03)) / (float32(x*(float32(2.43*x)+0.59)) + 0.14)
	v := float32(sqrt(x)*255) + d
	if v >= 255 {
		return 255
	}
	return byte(v)
}

// grid is what changes slowly over the picture: up to three numbers, computed
// where tiles of some pixels meet and interpolated between, over each half of
// a tile, on the GPU too.
type grid struct {
	w, h, unit float32 // unit: of the scene, in it
	tile       int
	step       float32 // 1/tile
	at         func(x, y float32) vec3

	row, tileRow, from int // from is the tile's first column
	ty                 float32
	top, bottom        []vec3
	// The numbers along the row, in the half before the diagonal of the tile
	// and in the other: where it starts, and how they change.
	edge0, dx0, edge1, dx1 vec3
}

func newGrid(u *uniforms, at func(x, y float32) vec3) grid {
	g := grid{w: u.Size[0], h: u.Size[1], unit: u.V[6][1], tile: int(u.V[6][0]), row: -1, tileRow: -2, at: at}
	g.step = 1 / float32(g.tile)
	corners := int(g.w)/g.tile + 2
	g.top, g.bottom = make([]vec3, corners), make([]vec3, corners)
	return g
}

// seek returns the numbers of a pixel. Pixels come row by row.
func (g *grid) seek(col, row int) vec3 {
	if row != g.row {
		g.startRow(row)
	}
	col -= g.from
	if uint(col) >= uint(g.tile) {
		col = g.startTile(col + g.from)
	}
	t := float32((float32(col) + 0.5) * g.step)
	if t+g.ty >= 1 {
		return g.edge1.plus(g.dx1, 1-t)
	}
	return g.edge0.plus(g.dx0, t)
}

func (g *grid) startRow(row int) {
	j := row / g.tile
	if j == g.tileRow+1 {
		g.top, g.bottom = g.bottom, g.top
		g.corners(g.bottom, j+1)
	} else if j != g.tileRow {
		g.corners(g.top, j)
		g.corners(g.bottom, j+1)
	}
	g.row, g.tileRow, g.from = row, j, -g.tile
	g.ty = float32((float32(row-j*g.tile) + 0.5) * g.step)
}

func (g *grid) corners(out []vec3, j int) {
	y := float32((float32(0.5*g.h) - float32(j*g.tile)) * g.unit)
	for i := range out {
		out[i] = g.at((float32(i*g.tile)-float32(0.5*g.w))*g.unit, y)
	}
}

func (g *grid) startTile(col int) int {
	i := col / g.tile
	g.from = i * g.tile
	a, x, y, z := g.top[i], g.top[i+1], g.bottom[i], g.bottom[i+1]
	g.edge0, g.dx0 = mix(a, y, g.ty), x.sub(a)
	g.edge1, g.dx1 = mix(z, x, 1-g.ty), y.sub(z)
	return col - g.from
}

// hash is a number in [0, 1) that looks random.
func hash(x, y, z int32) float32 {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(z)*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	return float32(h>>8) * (1.0 / (1 << 24))
}

// noise1 is a value noise over a line, one for each seed.
func noise1(x float32, seed int32) float32 {
	x = float32(x)
	i := floor(x)
	return lerp(hash(i, seed, 0), hash(i+1, seed, 0), quint(x-float32(i)))
}

// line is noise1 of one seed. It keeps the ends of the cell it was
// in.
type line struct {
	i    int32
	ok   bool
	a, b float32
}

func (n *line) at(x float32, seed int32) float32 {
	x = float32(x)
	i := floor(x)
	if i != n.i || !n.ok {
		n.i, n.ok, n.a, n.b = i, true, hash(i, seed, 0), hash(i+1, seed, 0)
	}
	return lerp(n.a, n.b, quint(x-float32(i)))
}

// soft is the mean of at for a seed and the next one, whose values are less
// often extreme.
func (n *line) soft(x float32, seed int32) float32 {
	x = float32(x)
	i := floor(x)
	if i != n.i || !n.ok {
		n.i, n.ok = i, true
		n.a, n.b = 0.5*(hash(i, seed, 0)+hash(i, seed+1, 0)), 0.5*(hash(i+1, seed, 0)+hash(i+1, seed+1, 0))
	}
	return lerp(n.a, n.b, quint(x-float32(i)))
}

// noise2 is a value noise over a plane. It keeps the corners of the cell it
// was in: pixels that are neighbors share it.
type noise2 struct {
	i, j       int32
	ok         bool
	a, b, c, d float32
}

func (n *noise2) load(i, j int32) {
	n.i, n.j, n.ok = i, j, true
	n.a, n.b, n.c, n.d = hash(i, j, 0), hash(i+1, j, 0), hash(i, j+1, 0), hash(i+1, j+1, 0)
}

func (n *noise2) at(x, y float32) float32 {
	x, y = float32(x), float32(y)
	i, j := floor(x), floor(y)
	if i != n.i || j != n.j || !n.ok {
		n.load(i, j)
	}
	u := quint(x - float32(i))
	return lerp(lerp(n.a, n.b, u), lerp(n.c, n.d, u), quint(y-float32(j)))
}

// slope returns the noise and its derivatives.
func (n *noise2) slope(x, y float32) (v, dx, dy float32) {
	i, j := floor(x), floor(y)
	if i != n.i || j != n.j || !n.ok {
		n.load(i, j)
	}
	fx, fy := x-float32(i), y-float32(j)
	u, w := quint(fx), quint(fy)
	k1, k2, k3 := n.b-n.a, n.c-n.a, n.a-n.b-n.c+n.d
	v = n.a + float32(k1*u) + float32(k2*w) + float32(float32(k3*u)*w)
	dx = float32(dquint(fx) * (k1 + float32(k3*w)))
	dy = float32(dquint(fy) * (k2 + float32(k3*u)))
	return
}

// noise3 is a value noise over space.
type noise3 struct {
	i, j, k int32
	ok      bool
	v       [8]float32
}

func (n *noise3) at(x, y, z float32) float32 {
	x, y, z = float32(x), float32(y), float32(z)
	i, j, k := floor(x), floor(y), floor(z)
	if i != n.i || j != n.j || k != n.k || !n.ok {
		n.i, n.j, n.k, n.ok = i, j, k, true
		for c := range int32(8) {
			n.v[c] = hash(i+c&1, j+c>>1&1, k+c>>2)
		}
	}
	u, w := quint(x-float32(i)), quint(y-float32(j))
	return lerp(
		lerp(lerp(n.v[0], n.v[1], u), lerp(n.v[2], n.v[3], u), w),
		lerp(lerp(n.v[4], n.v[5], u), lerp(n.v[6], n.v[7], u), w), quint(z-float32(k)))
}

// fbm2 and fbm3 are sums of noises, each of twice the detail of the one
// before, and half as strong.
func fbm2(n []noise2, x, y float32) float32 {
	x, y = float32(x), float32(y)
	var s, a float32 = 0, 0.5
	for i := range n {
		s += float32(a * n[i].at(x, y))
		x, y = float32(1.6*x)+float32(1.2*y)+3.1, float32(1.6*y)-float32(1.2*x)+7.7
		a *= 0.5
	}
	return s
}

func fbm3(n []noise3, x, y, z float32) float32 {
	x, y, z = float32(x), float32(y), float32(z)
	var s, a float32 = 0, 0.5
	for i := range n {
		s += float32(a * n[i].at(x, y, z))
		x, y, z = float32(2.03*x)+1.7, float32(2.03*y)+9.2, float32(2.03*z)+4.3
		a *= 0.5
	}
	return s
}

// stars is a layer of stars, at most one for each cell of a grid.
type stars struct {
	i, j     int32
	ok, none bool
	sx, sy   float32
	phase    float32
	color    vec3
}

// at adds to c the stars of the layer at a point of its grid, whose cells
// have a star with a probability of rare. size is how small they are, and
// twinkle the phase of their twinkling.
func (s *stars) at(c vec3, x, y float32, layer int32, rare, size, twinkle float32) vec3 {
	x, y = float32(x), float32(y)
	i, j := floor(x), floor(y)
	if i != s.i || j != s.j || !s.ok {
		s.load(i, j, layer, rare)
	}
	if s.none {
		return c
	}
	dx, dy := x-float32(i)-s.sx, y-float32(j)-s.sy
	g := 1 - float32(dot2(dx, dy)*size)
	if g <= 0 {
		return c
	}
	g = float32(g * g)
	g = float32(g * g)
	return c.plus(s.color, float32(g*(0.8+float32(0.2*wave(twinkle+s.phase)))))
}

func (s *stars) load(i, j, layer int32, rare float32) {
	s.i, s.j, s.ok = i, j, true
	h := hash(i, j, layer+11)
	if s.none = h >= rare; s.none {
		return
	}
	s.sx, s.sy = 0.2+float32(0.6*hash(i, j, layer+21)), 0.2+float32(0.6*hash(i, j, layer+31))
	s.phase = hash(i, j, layer+41)
	m := 1 - h/rare
	s.color = mix(vec3{1, 0.82, 0.62}, vec3{0.78, 0.88, 1}, hash(i, j, layer+51)).mul(0.15 + float32(1.1*float32(float32(m*m)*m)))
}

// wave is nearly a sine of period 1 (a parabola per half).
func wave(t float32) float32 {
	t = float32(t) // a product of the caller's too
	t = float32(2*(t-float32(floor(t)))) - 1
	return float32(float32(4*t) * (abs(t) - 1))
}

// cosine is a cosine of period 1, and sine its sine.
func cosine(t float32) float32 {
	t = float32(t)
	t = 0.25 - abs(t-float32(floor(t+0.5)))
	q := float32(t * t)
	return float32(t * (6.283185 + float32(q*(-41.34148+float32(q*(81.59658+float32(q*-74.59)))))))
}

func sine(t float32) float32 { return cosine(float32(t) - 0.25) }

// decay is nearly exp(-x) where x is small, and 0 from 16 on.
func decay(x float32) float32 {
	x = 1 - float32(x*0.0625)
	if x <= 0 {
		return 0
	}
	x = float32(x * x)
	x = float32(x * x)
	x = float32(x * x)
	return float32(x * x)
}

func floor(x float32) int32 {
	i := int32(x)
	if float32(i) > x {
		i--
	}
	return i
}

// quint is the curve noises are interpolated with, dquint its derivative.
func quint(t float32) float32 {
	return float32(float32(float32(t*t)*t) * (float32(t*(float32(t*6)-15)) + 10))
}

func dquint(t float32) float32 {
	k := float32(t * (t - 1))
	return float32(30 * float32(k*k))
}

func lerp(a, b, t float32) float32 { return a + float32((b-a)*t) }

// smooth goes from 0 at a to 1 at b.
func smooth(a, b, x float32) float32 {
	t := clamp(float32((x - a) * (1 / (b - a))))
	return float32(float32(t*t) * (3 - float32(2*t)))
}

func clamp(x float32) float32 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	return x
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrt(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// vec3 is a vector, or a color in linear light.
type vec3 struct{ x, y, z float32 }

func xyz(a [4]float32) vec3 { return vec3{a[0], a[1], a[2]} }

func (a vec3) dot(b vec3) float32 { return float32(a.x*b.x) + float32(a.y*b.y) + float32(a.z*b.z) }

func (a vec3) add(b vec3) vec3 { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }

func (a vec3) sub(b vec3) vec3 { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }

func (a vec3) mul(k float32) vec3 { return vec3{float32(a.x * k), float32(a.y * k), float32(a.z * k)} }

// plus adds k times b.
func (a vec3) plus(b vec3, k float32) vec3 {
	return vec3{a.x + float32(b.x*k), a.y + float32(b.y*k), a.z + float32(b.z*k)}
}

func (a vec3) cross(b vec3) vec3 {
	return vec3{
		float32(a.y*b.z) - float32(a.z*b.y),
		float32(a.z*b.x) - float32(a.x*b.z),
		float32(a.x*b.y) - float32(a.y*b.x),
	}
}

func (a vec3) unit() vec3 { return a.mul(1 / sqrt(a.dot(a))) }

func dot2(x, y float32) float32 { return float32(x*x) + float32(y*y) }

func mix(a, b vec3, t float32) vec3 {
	return vec3{a.x + float32((b.x-a.x)*t), a.y + float32((b.y-a.y)*t), a.z + float32((b.z-a.z)*t)}
}
