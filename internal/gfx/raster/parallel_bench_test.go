package raster

import (
	"fmt"
	"image"
	"runtime"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// parallelBenchScenes are the 1920x1080 frames of the parallel benchmarks.
func parallelBenchScenes() []struct {
	name string
	dl   *gfx.DisplayList
} {
	return []struct {
		name string
		dl   *gfx.DisplayList
	}{
		{"scene", benchScene(true, true)},
		{"shadowcards", benchShadowCards(twShadows[5].layers)},
		{"shadowcards_rotated", benchShadowCardsRotated()},
		{"blur8", benchLayerBlur()},
		{"blur32", benchLayerBlurSigma(32)},
		{"gradient", benchLinearGradient()},
		{"blobs", benchBlobs()},
	}
}

// benchBlobs is the shadow cards frame over three decorative 320 px
// circles blurred with Tailwind's blur-3xl (a standard deviation of 64 px),
// a common landing-page background.
func benchBlobs() *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, twGray100)
	for i, c := range []gfx.Color{indigo, fuchsia, sky} {
		b.PushLayer(0.7, gfx.SrcOver, gfx.Blur{StdDev: 64})
		b.FillPath(gfx.CirclePath(gfx.Pt(300+float32(i)*600, 300+float32(i%2)*400), 160), gfx.NonZero, c)
		b.PopLayer()
	}
	for i := 0; i < 100; i++ {
		x, y := float32(i%10)*190+25, float32(i/10)*106+12
		card := gfx.NewRRect(gfx.RectXYWH(x, y, 140, 70), 12)
		drawShadows(b, card, twShadows[5].layers)
		b.FillRRect(card, gfx.White.WithAlpha(0.8))
	}
	return b.Finish()
}

// BenchmarkParallel renders the frames with 1, 4, 8 and GOMAXPROCS workers
// and the automatic band height.
func BenchmarkParallel(b *testing.B) {
	for _, s := range parallelBenchScenes() {
		for _, w := range []int{1, 4, 8, runtime.GOMAXPROCS(0)} {
			b.Run(fmt.Sprintf("%s/workers=%d", s.name, w), func(b *testing.B) {
				img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
				r := NewRenderer()
				b.ReportAllocs()
				for b.Loop() {
					r.Render(img, s.dl, Options{Workers: w})
				}
			})
		}
	}
}

// BenchmarkBands renders the frames with GOMAXPROCS workers and fixed band
// heights, to tune the automatic one.
func BenchmarkBands(b *testing.B) {
	for _, s := range parallelBenchScenes() {
		for _, rows := range []int{8, 16, 32, 48, 64, 96, 128, 192, 256, 384, 0} {
			b.Run(fmt.Sprintf("%s/rows=%d", s.name, rows), func(b *testing.B) {
				img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
				r := NewRenderer()
				r.tune.bandRows = rows
				b.ReportAllocs()
				for b.Loop() {
					r.Render(img, s.dl, Options{})
				}
			})
		}
	}
}

// BenchmarkBandOverhead renders the frames band after band on one
// goroutine: the difference with rows=1080 (one band, the full frame) is
// the per-band overhead that a parallel render pays in total.
func BenchmarkBandOverhead(b *testing.B) {
	for _, s := range parallelBenchScenes() {
		for _, rows := range []int{8, 16, 32, 64, 128, 1080} {
			b.Run(fmt.Sprintf("%s/rows=%d", s.name, rows), func(b *testing.B) {
				img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
				r := NewRenderer()
				b.ReportAllocs()
				for b.Loop() {
					for y := 0; y < 1080; y += rows {
						r.Render(img, s.dl, Options{Workers: 1, Region: image.Rect(0, y, 1920, y+rows)})
					}
				}
			})
		}
	}
}

// BenchmarkSmallRegion measures the cost of a parallel render of small
// regions, to choose the sequential threshold.
func BenchmarkSmallRegion(b *testing.B) {
	dl := benchScene(true, true)
	for _, n := range []int{64, 96, 128, 192, 256} {
		for _, w := range []int{1, runtime.GOMAXPROCS(0)} {
			b.Run(fmt.Sprintf("%dx%d/workers=%d", n, n, w), func(b *testing.B) {
				img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
				r := NewRenderer()
				r.tune.minPixels = 1
				r.tune.bandRows = minBandRows
				b.ReportAllocs()
				for b.Loop() {
					r.Render(img, dl, Options{Workers: w, Region: image.Rect(800, 400, 800+n, 400+n)})
				}
			})
		}
	}
}
