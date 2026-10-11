package raster

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// colorDigest hashes the float64 results of the gradient color math and of
// the device-to-local mappings on random input. Pixels are 8-bit, which
// hides most last-bit differences; the intermediates reveal a product fused
// into a multiply-add on some architecture.
func colorDigest() uint64 {
	h := fnv.New64a()
	put := func(vs ...float64) {
		for _, v := range vs {
			b := math.Float64bits(v)
			var buf [8]byte
			for i := range buf {
				buf[i] = byte(b >> (8 * i))
			}
			h.Write(buf[:])
		}
	}
	rng := rand.New(rand.NewPCG(13, 13))
	for i := 0; i < 3000; i++ {
		r, g, b := rng.Float64(), rng.Float64(), rng.Float64()
		lr, lg, lb := srgbToLinear(r), srgbToLinear(g), srgbToLinear(b)
		put(lr, lg, lb)
		l, a, bb := linearToOklab(lr, lg, lb)
		put(l, a, bb)
		x, y, z := oklabToLinear(l, a, bb)
		put(x, y, z, linearToSRGB(x), linearToSRGB(y), linearToSRGB(z))
		c0 := newInterpColor(gfx.Color{R: float32(r), G: float32(g), B: float32(b), A: float32(rng.Float64())}, gfx.InterpOklab)
		put(c0.v[0], c0.v[1], c0.v[2], c0.a)
		m := gfx.RotateAbout(float32(rng.Float64()*6), gfx.Pt(float32(rng.Float64()*100), 3)).Mul(gfx.Scale(1.5, 0.75))
		inv, _ := invert(m)
		put(inv.compose(rng.Float64(), rng.Float64(), rng.Float64()))
	}
	// Whole gradient and image pixels.
	img := renderTransparent(64, 64, 1.5, func(b *gfx.Builder) {
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(20, 20)))
		b.FillRect(gfx.Rect{X1: 40, Y1: 20}, gfx.LinearGradient{End: gfx.Pt(37, 11), Stops: stops(indigo, fuchsia.WithAlpha(0.3), amber)})
		b.FillRect(gfx.Rect{Y0: 20, X1: 40, Y1: 30}, gfx.RadialGradient{Center: gfx.Pt(20, 25), Radius: gfx.Pt(13, 4), Stops: stops(sky, rose)})
		b.FillRect(gfx.Rect{Y0: 30, X1: 40, Y1: 40}, gfx.ConicGradient{Center: gfx.Pt(20, 35), StartAngle: 0.7, Stops: stops(sky, rose, sky)})
		b.DrawImage(testImage, gfx.Rect{}, gfx.Rect{X0: 1, Y0: 1, X1: 30.3, Y1: 17.7}, gfx.SampleLinear)
		b.PopTransform()
	})
	h.Write(img.Pix)
	return h.Sum64()
}

// wantColorDigest is colorDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx/raster
const wantColorDigest uint64 = 0xcf1c6364ed763e4f

func TestColorDeterministic(t *testing.T) {
	if got := colorDigest(); got != wantColorDigest {
		t.Errorf("colorDigest = %#x, want %#x (the same on every architecture)", got, wantColorDigest)
	}
}
