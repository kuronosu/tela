package raster

import (
	"image"
	"math/rand"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The axis-aligned bilinear fast path (shadeAxis) gives the pixels of the
// general loop, bit for bit, for scales above and below 1, fractional
// offsets, clamped edges and spans that start anywhere, and keeps doing so
// when its table of columns has to grow.
func TestImageAxisMatchesGeneral(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := range 80 {
		w, h := 1+rng.Intn(40), 1+rng.Intn(40)
		pix := make([]uint8, 4*w*h)
		for i := 0; i < len(pix); i += 4 {
			a := uint8(rng.Intn(256))
			pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(rng.Intn(int(a)+1)), uint8(rng.Intn(int(a)+1)), uint8(rng.Intn(int(a)+1)), a
		}
		img := gfx.NewImagePremul(w, h, pix)
		ku, kv := 0.2+rng.Float64()*3, 0.2+rng.Float64()*3 // source pixels per device pixel
		ox, oy := (rng.Float64()-0.5)*20, (rng.Float64()-0.5)*20
		s := &imageShader{img: img, pix: img.Pix(), stride: img.Stride()}
		s.ua, s.uc = ku, -ku*ox
		s.vb, s.vc = kv, -kv*oy
		s.ulo, s.uhi = 0, float64(w-1)
		s.vlo, s.vhi = 0, float64(h-1)
		s.axis = true
		fast, general := make([]uint8, 4*70), make([]uint8, 4*70)
		check := func(x, y, n int) {
			t.Helper()
			s.shadeAxis(x, y, fast[:4*n])
			s.shadeGeneral(x, y, general[:4*n])
			if string(fast[:4*n]) != string(general[:4*n]) {
				t.Fatalf("trial %d: span x=%d y=%d n=%d (%dx%d image, k=%.3f,%.3f o=%.2f,%.2f): the fast path differs from the general one\nfast    %v\ngeneral %v",
					trial, x, y, n, w, h, ku, kv, ox, oy, fast[:4*n], general[:4*n])
			}
		}
		// Spans anywhere, in any order: the caches miss and grow.
		for range 40 {
			check(rng.Intn(80)-10, rng.Intn(60)-10, 1+rng.Intn(70))
		}
		// Consecutive rows, down and up, as a rectangle is drawn: the
		// blended source rows are reused.
		x, n := rng.Intn(40)-10, 1+rng.Intn(70)
		y0 := rng.Intn(20) - 10
		for y := y0; y < y0+50; y++ {
			check(x, y, n)
		}
		for y := y0 + 50; y >= y0; y-- {
			check(x, y, n)
		}
	}
}

// BenchmarkShadeSpan is one 1920-pixel row of a 2x bilinear upscale, by
// the fast path and by the general loop.
func BenchmarkShadeSpan(b *testing.B) {
	pix := make([]uint8, 4*960*600)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(i), uint8(i>>8), uint8(i>>16), 255
	}
	img := gfx.NewImagePremul(960, 600, pix)
	s := &imageShader{img: img, pix: img.Pix(), stride: img.Stride(), ua: 0.5, vb: 0.5, ulo: 0, uhi: 959, vlo: 0, vhi: 599, axis: true}
	dst := make([]uint8, 4*1920)
	b.Run("axis", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			s.shadeAxis(0, i%1080, dst)
		}
	})
	b.Run("general", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			s.shadeGeneral(0, i%1080, dst)
		}
	})
}

// BenchmarkImageUpscale fills 1920x1080 with a 960x600 opaque image, as a
// window background that covers does.
func BenchmarkImageUpscale(b *testing.B) {
	pix := make([]uint8, 4*960*600)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(i), uint8(i>>8), uint8(i>>16), 255
	}
	img := gfx.NewImagePremul(960, 600, pix)
	var r Renderer
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bld := gfx.NewBuilder()
	bld.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(0, 0, 1920, 1080), gfx.SampleLinear)
	dl := bld.Finish()
	b.ReportAllocs()
	for b.Loop() {
		r.Render(dst, dl, Options{Scale: 1, Workers: 1})
	}
}
