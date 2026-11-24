package raster

import (
	"encoding/binary"
	"hash/fnv"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// refPattern samples p at the center of device pixel (x, y) under the
// transform m, in plain float64: the reference of patternShader. ok is false
// where the sample lies on a boundary between pixels of the image, which
// rounding decides.
func refPattern(p gfx.ImagePattern, m gfx.Affine, x, y int) (c [4]float64, ok bool) {
	total := m.Mul(p.Transform)
	a, b, cc, d, e, f := float64(total[0]), float64(total[1]), float64(total[2]), float64(total[3]), float64(total[4]), float64(total[5])
	det := a*d - b*cc
	dx, dy := float64(x)+0.5-e, float64(y)+0.5-f
	u, v := (d*dx-cc*dy)/det, (a*dy-b*dx)/det
	at := func(ix, iy float64) [4]float64 {
		w, h := float64(p.Image.Width()), float64(p.Image.Height())
		if p.Repeat.X() {
			ix -= w * math.Floor(ix/w)
		}
		if p.Repeat.Y() {
			iy -= h * math.Floor(iy/h)
		}
		if ix < 0 || iy < 0 || ix >= w || iy >= h {
			return [4]float64{}
		}
		o := int(iy)*p.Image.Stride() + 4*int(ix)
		var c [4]float64
		for i := range c {
			c[i] = float64(p.Image.Pix()[o+i])
		}
		return c
	}
	near := func(t float64) bool { return math.Abs(t-math.Round(t)) < 1e-3 }
	if p.Sampling == gfx.SampleNearest {
		return at(math.Floor(u), math.Floor(v)), !near(u) && !near(v)
	}
	u, v = u-0.5, v-0.5
	iu, iv := math.Floor(u), math.Floor(v)
	fu, fv := u-iu, v-iv
	p00, p10, p01, p11 := at(iu, iv), at(iu+1, iv), at(iu, iv+1), at(iu+1, iv+1)
	for i := range c {
		c[i] = (p00[i]*(1-fu)+p10[i]*fu)*(1-fv) + (p01[i]*(1-fu)+p11[i]*fu)*fv
	}
	return c, true
}

// TestPattern checks patterns that are not minified against the reference:
// every repeat mode and sampling, under scales, rotations, reflections and
// translations, far from the origin too.
func TestPattern(t *testing.T) {
	rng := rand.New(rand.NewPCG(319, 2))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	const size = 64
	for k := range 400 {
		rot := func() gfx.Affine { return gfx.Rotate(f(-4, 4)).Mul(gfx.Scale(f(1, 5), f(1, 5))) }
		var m, pm gfx.Affine
		switch k % 4 {
		case 0: // a translation by whole pixels
			m, pm = gfx.Translate(float32(rng.IntN(200)-100), float32(rng.IntN(200)-100)), gfx.Translate(float32(rng.IntN(20)), 3)
		case 1: // scales and reflections
			m, pm = gfx.Scale(f(1, 4), -f(1, 4)), gfx.Translate(f(-50, 50), f(-50, 50))
		case 2:
			m, pm = gfx.Translate(f(0, 60), f(0, 60)).Mul(rot()), gfx.Scale(f(1, 2), f(1, 2))
		default: // thousands of tiles away
			m, pm = rot(), gfx.Translate(f(-20000, 20000), f(-20000, 20000))
		}
		p := gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: gfx.PatternRepeat(rng.IntN(4)), Sampling: gfx.Sampling(rng.IntN(2)), Transform: pm}
		img := renderTransparent(size, size, 1, func(b *gfx.Builder) {
			b.PushTransform(m)
			b.FillRect(gfx.Rect{X0: -1e4, Y0: -1e4, X1: 1e4, Y1: 1e4}, p)
			b.PopTransform()
		})
		tol := 1.0
		if p.Sampling == gfx.SampleLinear {
			// The weights have 8 bits: two axes, half a step each, of
			// differences of at most 255.
			tol = 2.5
		}
		if k%4 == 3 {
			tol *= 4 // the float32 transform the reference composes
		}
		for y := range size {
			for x := range size {
				want, ok := refPattern(p, m, x, y)
				if !ok {
					continue
				}
				got := pixelAt(img, x, y)
				for i := range got {
					if math.Abs(float64(got[i])-want[i]) > tol {
						if k%4 == 3 && p.Sampling == gfx.SampleNearest {
							continue // a pixel's boundary, as above
						}
						t.Fatalf("%d: %s %s, transform %v, pattern %v: pixel (%d, %d) = %v, want %.1f", k, p.Repeat, p.Sampling, m, pm, x, y, got, want)
					}
				}
			}
		}
	}
}

// TestPatternMinified checks that a pattern drawn smaller than its image is
// filtered: a fine checkerboard comes out its mean gray, at any scale, and
// the tile's opacity is kept.
func TestPatternMinified(t *testing.T) {
	const n = 64
	pix := make([]uint8, 0, 4*n*n)
	for y := range n {
		for x := range n {
			v := uint8(255 * ((x + y) % 2))
			pix = append(pix, v, v, v, 255)
		}
	}
	checker := gfx.NewImagePremul(n, n, pix)
	for _, s := range []float32{0.5, 0.45, 0.3, 0.25, 0.13, 0.06, 0.02} {
		for _, angle := range []float32{0, 0.4} {
			img := renderTransparent(80, 80, 1, func(b *gfx.Builder) {
				b.FillRect(gfx.Rect{X1: 80, Y1: 80}, gfx.ImagePattern{Image: checker, Transform: gfx.Rotate(angle).Mul(gfx.Scale(s, s))})
			})
			for i := 0; i < len(img.Pix); i += 4 {
				if c := [4]uint8(img.Pix[i : i+4]); !near(c, [4]uint8{128, 128, 128, 255}, 1) {
					t.Fatalf("scale %v, angle %v: pixel %d = %v, want the mean of the checkerboard", s, angle, i/4, c)
				}
			}
		}
	}
	// The level is the smallest whose size is not smaller than the drawn
	// size, along the axis drawn larger.
	for _, c := range []struct {
		m, p gfx.Affine
		want int
	}{
		{gfx.Identity, gfx.Identity, 0},
		{gfx.Identity, gfx.Scale(0.51, 0.51), 0},
		{gfx.Identity, gfx.Scale(0.5, 0.5), 1},
		{gfx.Identity, gfx.Scale(0.26, 0.1), 1},
		{gfx.Scale(0.5, 0.5), gfx.Scale(0.25, 0.25), 3},
		{gfx.Rotate(1).Mul(gfx.Scale(2, 2)), gfx.Scale(0.1, 0.12), 2},
		{gfx.Identity, gfx.Scale(1e-30, 1e-30), maxMipLevel},
	} {
		if got := patternLevel(&c.m, &c.p); got != c.want {
			t.Errorf("patternLevel(%v, %v) = %d, want %d", c.m, c.p, got, c.want)
		}
	}
}

// TestPatternUnit checks that a pattern drawn pixel for pixel copies the
// image, at fractional scales whose logical coordinates are not exact too.
func TestPatternUnit(t *testing.T) {
	tile := gfxtest.PatternTile
	for _, scale := range []float32{1, 1.25, 1.5, 1.75, 2} {
		// The pattern's pixels are the device's, at device (24, 36).
		p := gfx.ImagePattern{Image: tile, Transform: gfx.Affine{1 / scale, 0, 0, 1 / scale, 24 / scale, 36 / scale}}
		img := renderTransparent(120, 100, scale, func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 120, Y1: 100}, p) })
		for y := range 100 {
			for x := range 120 {
				o := ((y+60)%8)*tile.Stride() + 4*((x+12)%12)
				if got, want := pixelAt(img, x, y), [4]uint8(tile.Pix()[o:o+4]); got != want {
					t.Fatalf("scale %v: pixel (%d, %d) = %v, want %v", scale, x, y, got, want)
				}
			}
		}
	}
	// A scale that is not 1 is not taken for it, however small the tile.
	if n := countUnit(gfx.Scale(1.0002, 1.0002)); n != 0 {
		t.Errorf("a scale of 1.0002 is drawn as 1")
	}
}

func countUnit(m gfx.Affine) int {
	r := NewRenderer()
	r.ctm = gfx.Identity
	if s, ok := r.patternFor(gfx.ImagePattern{Image: gfxtest.PatternTile, Transform: m}).(*patternShader); ok && s.unit {
		return 1
	}
	return 0
}

// TestPatternExtremes draws patterns under transforms at the limits of
// float32: nothing panics, and what is outside an axis that does not repeat
// stays transparent.
func TestPatternExtremes(t *testing.T) {
	for _, m := range []gfx.Affine{
		gfx.Scale(1e-30, 1e-30), gfx.Scale(1e30, 1e30), gfx.Scale(1e-18, 1e18), gfx.Translate(1e30, -1e30),
		{1e-20, 1, 1, 1e-20, 1e10, 3}, gfx.Scale(1e-38, 1e-38), {3e38, 0, 0, 3e38, 0, 0}, gfx.Translate(0.5, 1<<30),
	} {
		for rep := gfx.RepeatBoth; rep <= gfx.RepeatNone; rep++ {
			for _, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
				renderTransparent(40, 40, 1, func(b *gfx.Builder) {
					b.FillRect(gfx.Rect{X1: 40, Y1: 40}, gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: rep, Transform: m, Sampling: s})
				})
			}
		}
	}
	img := renderTransparent(40, 40, 1, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 40, Y1: 40}, gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: gfx.RepeatNone, Transform: gfx.Translate(10, 20)})
	})
	if got := img.Pix[3]; got != 0 {
		t.Errorf("outside the image: alpha %d", got)
	}
	if got, want := img.RGBAAt(10, 20).A, gfxtest.PatternTile.Pix()[3]; got != want {
		t.Errorf("the image's first pixel: alpha %d, want %d", got, want)
	}
}

// TestPatternOpaque checks what the blitter is told: only an opaque image
// that covers the plane is opaque.
func TestPatternOpaque(t *testing.T) {
	r := NewRenderer()
	r.ctm = gfx.Identity
	for _, c := range []struct {
		img  *gfx.Image
		rep  gfx.PatternRepeat
		want bool
	}{
		{gfxtest.PatternBig, gfx.RepeatBoth, true},
		{gfxtest.PatternBig, gfx.RepeatX, false},
		{gfxtest.PatternBig, gfx.RepeatNone, false},
		{gfxtest.PatternTile, gfx.RepeatBoth, false},
	} {
		if got := r.patternFor(gfx.ImagePattern{Image: c.img, Repeat: c.rep, Transform: gfx.Scale(0.3, 0.3)}).opaque(); got != c.want {
			t.Errorf("%dx%d %s: opaque() = %v, want %v", c.img.Width(), c.img.Height(), c.rep, got, c.want)
		}
	}
}

// patternDigest hashes the coefficients of pattern shaders under random
// transforms, the pixels they shade and the pixels of the composite and
// pattern scenes: a product fused into an addition on some architecture
// shows in the coefficients before the rounding to 8 bits hides it.
func patternDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(vs ...float64) {
		for _, v := range vs {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			h.Write(buf[:])
		}
	}
	rng := rand.New(rand.NewPCG(319, 3))
	f := func(lo, hi float64) float32 { return float32(lo + float64(rng.Float64()*(hi-lo))) }
	r := NewRenderer()
	row := make([]uint8, 4*64)
	for range 500 {
		r.ctm = gfx.Affine{f(-2, 2), f(-2, 2), f(-2, 2), f(-2, 2), f(-99, 99), f(-99, 99)}
		p := gfx.ImagePattern{Image: gfxtest.PatternBig, Repeat: gfx.PatternRepeat(rng.IntN(4)), Sampling: gfx.Sampling(rng.IntN(2)),
			Transform: gfx.Affine{f(-2, 2), f(-2, 2), f(-2, 2), f(-2, 2), f(-99, 99), f(-99, 99)}}
		s, ok := r.patternFor(p).(*patternShader)
		if !ok {
			continue
		}
		put(s.ua, s.ub, s.uc, s.va, s.vb, s.vc, float64(s.w))
		s.shadeSpan(rng.IntN(1000), rng.IntN(1000), row)
		h.Write(row)
	}
	for _, s := range scenes5 {
		for _, scale := range []float32{1, 1.5} {
			h.Write(renderScene(s, scale).Pix)
		}
	}
	return h.Sum64()
}

// wantPatternDigest is patternDigest() recorded on amd64 (GOAMD64=v1); see
// wantCompositingDigest.
const wantPatternDigest uint64 = 0xf82205bdfa44e1d

func TestPatternDeterministic(t *testing.T) {
	if got := patternDigest(); got != wantPatternDigest {
		t.Errorf("patternDigest = %#x, want %#x (the same on every architecture)", got, wantPatternDigest)
	}
}

// Benchmarks of the Porter-Duff layers and of patterns over 512x512 device
// pixels, as those of bench4_test.go.

func BenchmarkLayer512SrcIn(b *testing.B)   { benchBlend(b, gfx.SrcIn) }
func BenchmarkLayer512DstOver(b *testing.B) { benchBlend(b, gfx.DstOver) }
func BenchmarkLayer512Xor(b *testing.B)     { benchBlend(b, gfx.Xor) }

func BenchmarkPattern512(b *testing.B) {
	for _, c := range []struct {
		name string
		p    gfx.ImagePattern
	}{
		{"unit", gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Identity}},
		{"scaled", gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Scale(1.7, 1.7)}},
		{"rotated", gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Rotate(0.4)}},
		{"nearest", gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Scale(1.7, 1.7), Sampling: gfx.SampleNearest}},
		{"minified", gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Scale(0.2, 0.2)}},
	} {
		b.Run(c.name, func(b *testing.B) {
			bench512(b, build(func(bb *gfx.Builder) { bb.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, c.p) }))
		})
	}
}

// TestPatternAllocs checks that a warm Renderer draws patterns and
// operators without allocating.
func TestPatternAllocs(t *testing.T) {
	for _, s := range scenes5[:2] {
		dl := build(s.draw)
		img := image.NewRGBA(image.Rect(0, 0, s.w, s.h))
		r := NewRenderer()
		r.Render(img, dl, Options{Workers: 1})
		if n := testing.AllocsPerRun(5, func() { r.Render(img, dl, Options{Workers: 1}) }); n > 0 {
			t.Errorf("%s: %v allocations per frame", s.name, n)
		}
	}
}
