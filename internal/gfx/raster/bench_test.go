package raster

import (
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// benchScene is a 1920x1080 frame with 1000 rounded rectangles (cards,
// buttons and pills, a third translucent) and 200 paths (stars, circles and
// curved blobs).
func benchScene(rrects, paths bool) *gfx.DisplayList {
	rng := rand.New(rand.NewPCG(42, 42))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	color := func() gfx.Color {
		c := gfx.Color{R: f(0, 1), G: f(0, 1), B: f(0, 1), A: 1}
		if rng.IntN(3) == 0 {
			c.A = f(0.3, 0.9)
		}
		return c
	}
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.MustParseHex("#f8fafc"))
	if rrects {
		for i := 0; i < 1000; i++ {
			w, h := f(24, 360), f(16, 240)
			r := gfx.RectXYWH(f(-20, 1920), f(-20, 1080), w, h)
			var radius float32
			switch rng.IntN(3) {
			case 0:
				radius = 999 // pill
			default:
				radius = f(2, 24)
			}
			b.FillRRect(gfx.NewRRect(r, radius), color())
		}
	}
	if paths {
		for i := 0; i < 200; i++ {
			cx, cy, s := f(0, 1920), f(0, 1080), f(10, 120)
			switch i % 3 {
			case 0:
				b.FillPath(starPath(cx, cy, s), gfx.EvenOdd, color())
			case 1:
				b.FillPath(gfx.CirclePath(gfx.Pt(cx, cy), s), gfx.NonZero, color())
			case 2:
				var p gfx.Path
				p.MoveTo(gfx.Pt(cx, cy))
				p.CubicTo(gfx.Pt(cx+s, cy-s), gfx.Pt(cx+2*s, cy+s), gfx.Pt(cx+s, cy+2*s))
				p.QuadTo(gfx.Pt(cx-s, cy+2*s), gfx.Pt(cx, cy))
				p.Close()
				b.FillPath(p, gfx.NonZero, color())
			}
		}
	}
	return b.Finish()
}

func benchRender(b *testing.B, dl *gfx.DisplayList, scale float32, region image.Rectangle) {
	w, h := int(1920*scale), int(1080*scale)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := NewRenderer()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Render(img, dl, Options{Scale: scale, Region: region})
	}
}

// BenchmarkScene1080p renders 1000 rrects and 200 paths at 1920x1080.
func BenchmarkScene1080p(b *testing.B) {
	benchRender(b, benchScene(true, true), 1, image.Rectangle{})
}

func BenchmarkRRects1080p(b *testing.B) {
	benchRender(b, benchScene(true, false), 1, image.Rectangle{})
}
func BenchmarkPaths1080p(b *testing.B) { benchRender(b, benchScene(false, true), 1, image.Rectangle{}) }

// BenchmarkScene1080pScale2 is the same scene at 3840x2160.
func BenchmarkScene1080pScale2(b *testing.B) {
	benchRender(b, benchScene(true, true), 2, image.Rectangle{})
}

// BenchmarkSceneTile renders one 256x256 tile of the scene, the unit of
// work of the future tiled renderer.
func BenchmarkSceneTile(b *testing.B) {
	benchRender(b, benchScene(true, true), 1, image.Rect(768, 384, 1024, 640))
}
