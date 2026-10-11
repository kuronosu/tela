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

// BenchmarkLinearGradient1080p fills the whole 1920x1080 frame with a
// three-stop oklab gradient (Tailwind bg-linear-to-br).
func BenchmarkLinearGradient1080p(b *testing.B) {
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.LinearGradient{End: gfx.Pt(1920, 1080), Stops: stops(indigo, fuchsia, rose)})
	benchRender(b, bl.Finish(), 1, image.Rectangle{})
}

// BenchmarkRadialGradient1080p and BenchmarkConicGradient1080p fill the
// frame with a radial and a conic gradient.
func BenchmarkRadialGradient1080p(b *testing.B) {
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.RadialGradient{Center: gfx.Pt(960, 540), Radius: gfx.Pt(900, 500), Stops: stops(gfx.White, sky, indigo)})
	benchRender(b, bl.Finish(), 1, image.Rectangle{})
}

func BenchmarkConicGradient1080p(b *testing.B) {
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.ConicGradient{Center: gfx.Pt(960, 540), Stops: stops(rose, amber, emerald, sky, rose)})
	benchRender(b, bl.Finish(), 1, image.Rectangle{})
}

// benchCards draws a grid of 200 cards (rounded clip, gradient header,
// content rrects and a circle), each optionally wrapped in a layer.
func benchCards(layers bool) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.MustParseHex("#f8fafc"))
	for i := 0; i < 200; i++ {
		x, y := float32(i%20)*96+4.5, float32(i/20)*108+4.5
		card := gfx.RectXYWH(x, y, 88, 100)
		if layers {
			b.PushLayer(0.85, gfx.SrcOver)
		}
		b.PushClipRRect(gfx.NewRRect(card, 12))
		b.FillRect(card, gfx.White)
		b.FillRect(gfx.RectXYWH(x, y, 88, 36), gfx.LinearGradient{Start: card.Origin(), End: gfx.Pt(x+88, y+36), Stops: stops(indigo, fuchsia)})
		b.FillPath(gfx.CirclePath(gfx.Pt(x+20, y+36), 12), gfx.NonZero, amber)
		for k := 0; k < 3; k++ {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x+8, y+56+float32(k)*13, 72-float32(k)*14, 8), 4), slate.WithAlpha(0.2))
		}
		b.PopClip()
		if layers {
			b.PopLayer()
		}
	}
	return b.Finish()
}

// BenchmarkClippedCards1080p renders 200 cards, each in a rounded clip.
func BenchmarkClippedCards1080p(b *testing.B) {
	benchRender(b, benchCards(false), 1, image.Rectangle{})
}

// BenchmarkLayerCards1080p renders the same cards, each in an 85% layer.
func BenchmarkLayerCards1080p(b *testing.B) { benchRender(b, benchCards(true), 1, image.Rectangle{}) }

// BenchmarkStrokes1080p strokes 300 paths (polylines with joins, circles,
// dashed curves) across the frame. Stroking (gfx.Path.Stroke) runs on every
// frame and is included.
func BenchmarkStrokes1080p(b *testing.B) {
	rng := rand.New(rand.NewPCG(7, 7))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.White)
	for i := 0; i < 300; i++ {
		cx, cy, s := f(0, 1920), f(0, 1080), f(10, 80)
		st := gfx.NewStrokeStyle(f(1, 6))
		st.Join, st.Cap = gfx.LineJoin(i%3), gfx.LineCap(i%3)
		var p gfx.Path
		switch i % 3 {
		case 0:
			p.MoveTo(gfx.Pt(cx, cy))
			for k := 0; k < 6; k++ {
				d := float64(s)
				p.LineTo(gfx.Pt(cx+f(-d, d), cy+f(-d, d)))
			}
		case 1:
			p = gfx.CirclePath(gfx.Pt(cx, cy), s)
		case 2:
			p.MoveTo(gfx.Pt(cx, cy))
			p.CubicTo(gfx.Pt(cx+s, cy-s), gfx.Pt(cx+2*s, cy+s), gfx.Pt(cx+3*s, cy))
			st.Dashes = []float32{8, 4}
		}
		bl.StrokePath(p, st, gfx.Color{R: f(0, 1), G: f(0, 1), B: f(0, 1), A: 1})
	}
	benchRender(b, bl.Finish(), 1, image.Rectangle{})
}
