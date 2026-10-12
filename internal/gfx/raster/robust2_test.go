package raster

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Robustness of shadows and filters: extreme but finite input renders
// without panicking and in bounded time.
func TestRobustnessShadowsFilters(t *testing.T) {
	const big, tiny = float32(1e30), float32(1e-30)
	dl := build(func(b *gfx.Builder) {
		full := gfx.Rect{X0: -big, Y0: -big, X1: big, Y1: big}
		box := gfx.NewRRect(gfx.RectXYWH(4, 4, 20, 20), 6)
		for _, s := range []gfx.DrawShadow{
			{RRect: box, Blur: big, Color: gfx.Black},
			{RRect: box, Blur: tiny, Color: gfx.Black},
			{RRect: box, Blur: 1e-3, Spread: 3, Color: gfx.Black},
			{RRect: box, Blur: 4, Spread: big, Color: gfx.Black},
			{RRect: box, Blur: 4, Spread: -big, Color: gfx.Black},
			{RRect: box, Blur: 4, Spread: -big, Color: gfx.Black, Inset: true},
			{RRect: box, Blur: 4, Spread: big, Color: gfx.Black, Inset: true},
			{RRect: box, Offset: gfx.Pt(big, -big), Blur: 4, Color: gfx.Black},
			{RRect: box, Offset: gfx.Pt(big, -big), Blur: 4, Color: gfx.Black, Inset: true},
			{RRect: gfx.NewRRect(full, 1e29), Blur: 6, Color: gfx.Black},
			{RRect: gfx.NewRRect(full, 1e29), Blur: 6, Color: gfx.Black, Inset: true},
			{RRect: gfx.NewRRect(gfx.RectXYWH(-1e6, -1e6, 2e6+16, 2e6+16), 1e6), Blur: 0.05, Color: gfx.Black},
			{RRect: gfx.NewRRect(gfx.RectXYWH(-1e4, 3, 2e4, 2e4), 1e4), Blur: 0.02, Color: gfx.Black},
			{RRect: gfx.NewRRect(gfx.RectXYWH(10, 10, 1e-6, 1e-6), 1), Blur: 3, Color: gfx.Black},
			{RRect: gfx.RRectXY(gfx.RectXYWH(2, 2, 28, 3), 14, 1.5), Blur: 1, Spread: 5, Color: gfx.Black},
		} {
			b.DrawShadow(s)
		}
		// Filters with extreme parameters, nested, under transforms.
		for _, fs := range [][]gfx.Filter{
			{gfx.Blur{StdDev: big}},
			{gfx.Blur{StdDev: tiny}},
			{gfx.Blur{StdDev: 1e5}},
			{gfx.DropShadow{Offset: gfx.Pt(big, big), StdDev: 2, Color: gfx.Black}},
			{gfx.DropShadow{Offset: gfx.Pt(1e6, -3), StdDev: 1e4, Color: gfx.Black}},
			{gfx.ColorMatrix{M: [20]float32{1e30, -1e30, 0, 0, 1e30, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 1}}},
			{gfx.Blur{StdDev: 2}, gfx.DropShadow{Offset: gfx.Pt(3, 3), StdDev: 1, Color: gfx.Black}, gfx.Grayscale(1)},
		} {
			b.PushLayer(0.9, gfx.SrcOver, fs...)
			b.FillRect(gfx.RectXYWH(5, 5, 10, 10), indigo)
			b.FillRect(gfx.RectXYWH(-40, -40, 110, 110), rose.WithAlpha(0.2))
			b.PopLayer()
		}
		for i := 0; i < 12; i++ {
			b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(16, 16)).Mul(gfx.Skew(0.2, 0)))
			b.PushLayer(0.95, gfx.SrcOver, gfx.Blur{StdDev: 0.7}, gfx.DropShadow{Offset: gfx.Pt(1, 1), StdDev: 0.5, Color: gfx.Black})
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(-float32(i), -float32(i), 40+float32(2*i), 40+float32(2*i)), 5))
			b.DrawShadow(gfx.DrawShadow{RRect: box, Blur: 5, Spread: 1, Color: gfx.Black.WithAlpha(0.3)})
		}
		b.FillRect(full, sky)
		for i := 0; i < 12; i++ {
			b.PopClip()
			b.PopLayer()
			b.PopTransform()
		}
		for _, m := range []gfx.Affine{gfx.Scale(1e18, 1e18), gfx.Scale(1e-18, 1e-18), gfx.Scale(1e6, 1e-6), {1e30, 1e30, -1e30, 1e30, 0, 0}} {
			b.PushTransform(m)
			b.DrawShadow(gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(-1, -1, 2, 2), 0.5), Blur: 0.1, Color: gfx.Black})
			b.DrawShadow(gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(-1, -1, 2, 2), 0.5), Blur: 0.1, Color: gfx.Black, Inset: true})
			b.PushLayer(0.5, gfx.SrcOver, gfx.Blur{StdDev: 0.1}, gfx.DropShadow{Offset: gfx.Pt(0.1, 0.1), StdDev: 0.1, Color: gfx.Black})
			b.FillRect(gfx.RectXYWH(-1, -1, 2, 2), slate)
			b.PopLayer()
			b.PopTransform()
		}
	})
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	start := time.Now()
	scales := []float32{1, 1e-6, 3e30, 2.5}
	if testing.Short() {
		scales = []float32{1}
	}
	for _, scale := range scales {
		Clear(img, image.Rectangle{}, gfx.Transparent)
		Render(img, dl, Options{Scale: scale})
		Render(img, dl, Options{Scale: scale, Region: image.Rect(5, 7, 9, 30)})
	}
	// Filters with absurd parameters are capped (maxSigma, maxOffset): at
	// the caps a blur extends its surface by 768 pixels and a drop shadow by
	// 1280 on each side, so this costs a few large blurs per render (about
	// 8 s on an idle 2024 desktop). The limit catches unbounded work, not
	// slowness; -short is what the arm64 check runs under emulation, roughly
	// ten times slower, with one scale.
	limit := 60 * time.Second
	if testing.Short() {
		limit = 120 * time.Second
	}
	if d := time.Since(start); d > limit {
		t.Errorf("rendering took %v", d)
	}
}
