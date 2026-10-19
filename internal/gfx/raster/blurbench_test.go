package raster

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Benchmarks of large blurs (ADR-078): Tailwind's blur-3xl on a glow,
// backdrop-blur-xl on a full-width glass panel and drop-shadow-lg on text.
// Each runs on one goroutine (workers=1) and on GOMAXPROCS goroutines
// (workers=0).

// glassScene is a 1920x1080 frame: a gradient page with 2,000 glyphs of
// white text, a 200x200 tile under the panel whose color alternates with
// tile (the damage benchmarks change it), and a full-width glass panel
// with backdrop-blur-xl (a standard deviation of 24 px) and bg-white/20.
func glassScene(tile int) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.LinearGradient{End: gfx.Pt(1920, 1080), Stops: stops(indigo, fuchsia, rose)})
	const line = "olAS iolS loo Ail SoAl iS lo AoS lil oSA Sol iAo lSi oAl S" // 59 glyphs
	for i := 0; i < 20; i++ {
		s := line + line
		b.DrawGlyphRun(testFontA.run(s[:100], 14, 40.37+float32(i)*3.1, 60+float32(i)*48.6, gfx.White))
	}
	tiles := [2]gfx.Color{sky, amber}
	b.FillRRect(gfx.NewRRect(glassTile, 16), tiles[tile&1])
	panel := gfx.NewRRect(gfx.Rect{X0: 0, Y0: 340, X1: 1920, Y1: 740}, 0)
	b.BackdropFilter(panel, gfx.Blur{StdDev: 24})
	b.FillRRect(panel, gfx.White.WithAlpha(0.2))
	b.FillRect(gfx.Rect{X0: 0, Y0: 340, X1: 1920, Y1: 341}, gfx.White.WithAlpha(0.4))
	b.FillRect(gfx.Rect{X0: 0, Y0: 739, X1: 1920, Y1: 740}, gfx.White.WithAlpha(0.4))
	return b.Finish()
}

// glassTile is the tile of glassScene that changes, under the panel.
var glassTile = gfx.RectXYWH(860, 440, 200, 200)

func benchGlass(b *testing.B, workers int, damage bool) {
	dls := [2]*gfx.DisplayList{glassScene(0), glassScene(1)}
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	r := NewRenderer()
	r.Render(img, dls[0], Options{Workers: workers})
	// The damage of a tile change, as core computes it: the tile, the
	// output of the backdrop filter that reads it, closed under the
	// backdrop filters' reads.
	var region image.Rectangle
	if damage {
		region = glassDamage(dls[1])
	}
	i := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		i++
		r.Render(img, dls[i&1], Options{Workers: workers, Region: region})
	}
	if damage {
		b.ReportMetric(float64(region.Dx()*region.Dy()), "px/op")
	}
}

// glassDamage returns the region core rasterizes when the tile of
// glassScene changes (see core's damager: the backdrop closure and
// BackdropRegion).
func glassDamage(dl *gfx.DisplayList) image.Rectangle {
	full := image.Rect(0, 0, 1920, 1080)
	d := pixelBounds(float64(glassTile.X0), float64(glassTile.Y0), float64(glassTile.X1), float64(glassTile.Y1))
	for k := range dl.NumBackdrops() {
		d = d.Union(BackdropOutputRegion(dl, 1, k, d))
	}
	return BackdropRegion(dl, 1, d, full)
}

// BenchmarkGlass1080p renders the glass frame in full.
func BenchmarkGlass1080p(b *testing.B) {
	for _, w := range []int{1, 0} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) { benchGlass(b, w, false) })
	}
}

// BenchmarkGlassDamage1080p renders the damage of a tile change under the
// glass panel.
func BenchmarkGlassDamage1080p(b *testing.B) {
	for _, w := range []int{1, 0} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) { benchGlass(b, w, true) })
	}
}

// glowScene is a 960x640 page with a 448 px disc (size-[28rem]) blurred by
// blur-3xl (a standard deviation of 64 px), like the glows of
// examples/shapes, half outside the page.
func glowScene() *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 960, Y1: 640}, twGray100)
	b.PushClipRect(gfx.Rect{X1: 960, Y1: 640})
	b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 64})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(608, -160, 448, 448), 224), indigo.WithAlpha(0.6))
	b.PopLayer()
	b.PopClip()
	return b.Finish()
}

// BenchmarkGlow448 renders glowScene at scales 1 and 2.
func BenchmarkGlow448(b *testing.B) {
	dl := glowScene()
	for _, scale := range []float32{1, 2} {
		for _, w := range []int{1, 0} {
			b.Run(fmt.Sprintf("x%g/workers=%d", scale, w), func(b *testing.B) {
				img := image.NewRGBA(image.Rect(0, 0, int(960*scale), int(640*scale)))
				r := NewRenderer()
				b.ReportAllocs()
				for b.Loop() {
					r.Render(img, dl, Options{Scale: scale, Workers: w})
				}
			})
		}
	}
}

// BenchmarkDropShadowText renders 2,000 glyphs of 14 px text in a layer
// with drop-shadow-lg (0 4px, blur radius 4 px: a standard deviation of
// 2 px, black at 15%) on a 1920x1080 page.
func BenchmarkDropShadowText(b *testing.B) {
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.White)
	bl.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 4), StdDev: 2, Color: gfx.Black.WithAlpha(0.15)})
	bl.AppendList(benchText(), nil)
	bl.PopLayer()
	dl := bl.Finish()
	for _, w := range []int{1, 0} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) {
			img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
			r := NewRenderer()
			r.Render(img, dl, Options{Workers: w})
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				r.Render(img, dl, Options{Workers: w})
			}
		})
	}
}

// BenchmarkBlurPlane blurs a 1024x1024 RGBA plane full of content at
// several standard deviations, at full resolution (full) and with the
// kernel newKernel chooses (auto: downsampled from about 21).
func BenchmarkBlurPlane(b *testing.B) {
	const w = 1024
	src := make([]uint8, w*w*4)
	for i := range src {
		src[i] = uint8(i*7 + i/4096)
	}
	pix := make([]uint8, len(src))
	for _, s := range []float64{4, 8, 12, 16, 20, 24, 32, 48, 64, 128} {
		full, _ := newFullKernel(s, 0, 3*s, nil)
		auto, _ := newKernel(s, 0, nil)
		for _, k := range []struct {
			name string
			k    gaussKernel
		}{{"full", full}, {"auto", auto}} {
			if k.name == "auto" && auto.down < 2 {
				continue
			}
			b.Run(fmt.Sprintf("s=%v/%s", s, k.name), func(b *testing.B) {
				var sc blurScratch
				for b.Loop() {
					copy(pix, src)
					sc.blurPlane(pix, 4*w, w, w, 4, 0, 0, &k.k, &k.k)
				}
			})
		}
	}
}
