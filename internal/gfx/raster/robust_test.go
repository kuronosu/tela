package raster

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Robustness of gradients, images, clips, layers and strokes: extreme but
// finite input renders without panicking and in bounded time.
func TestRobustnessPaintsClipsLayers(t *testing.T) {
	const big, tiny = float32(1e30), float32(1e-30)
	st := stops(rose, sky, amber)
	dl := build(func(b *gfx.Builder) {
		full := gfx.Rect{X0: -big, Y0: -big, X1: big, Y1: big}
		// Gradients with extreme geometry.
		b.FillRect(full, gfx.LinearGradient{Start: gfx.Pt(-big, 0), End: gfx.Pt(big, 3), Stops: st})
		b.FillRect(full, gfx.LinearGradient{Start: gfx.Pt(1, 1), End: gfx.Pt(1+tiny, 1), Stops: st})
		b.FillRect(full, gfx.LinearGradient{Start: gfx.Pt(1, 1), End: gfx.Pt(1, 1+1e-20), Stops: st})
		b.FillRect(full, gfx.RadialGradient{Center: gfx.Pt(5, 5), Radius: gfx.Pt(tiny, tiny), Stops: st})
		b.FillRect(full, gfx.RadialGradient{Center: gfx.Pt(-big, big), Radius: gfx.Pt(big, tiny), Stops: st})
		b.FillRect(full, gfx.ConicGradient{Center: gfx.Pt(big, -big), StartAngle: 1e30, Stops: st})
		b.FillRect(full, gfx.ConicGradient{Center: gfx.Pt(16, 16), StartAngle: -1e-30, Stops: []gfx.GradientStop{
			{Offset: 0, Color: rose}, {Offset: 0, Color: sky}, {Offset: 1, Color: amber}, {Offset: 1, Color: slate}}})
		// Images: tiny and huge destinations and sources.
		b.DrawImage(testImage, gfx.Rect{}, gfx.Rect{X0: 3, Y0: 3, X1: 3 + 1e-6, Y1: 3 + 1e-6}, gfx.SampleLinear)
		b.DrawImage(testImage, gfx.Rect{}, full, gfx.SampleLinear)
		b.DrawImage(testImage, gfx.Rect{X0: 1, Y0: 1, X1: 1 + 1e-6, Y1: 1 + 1e-6}, gfx.RectXYWH(0, 0, 30, 30), gfx.SampleLinear)
		b.DrawImage(testImage, gfx.Rect{X0: 5.9, Y0: -big, X1: big, Y1: 0.1}, gfx.RectXYWH(0, 0, 30, 30), gfx.SampleNearest)
		// Clips: huge, tiny, far away, deeply nested with layers.
		b.PushClipRect(full)
		b.PushClipRRect(gfx.NewRRect(full, 1e29))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(16, -1e7+8), 1e7), gfx.NonZero)
		for i := 0; i < 40; i++ {
			b.PushLayer(0.9, gfx.SrcOver)
			b.PushTransform(gfx.RotateAbout(0.1, gfx.Pt(16, 16)))
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(-float32(i), -float32(i), 40+float32(2*i), 40+float32(2*i)), 5))
		}
		b.FillRect(full, indigo)
		for i := 0; i < 40; i++ {
			b.PopClip()
			b.PopTransform()
			b.PopLayer()
		}
		b.PopClip()
		b.PopClip()
		b.PopClip()
		b.PushClipRect(gfx.Rect{X0: 3, Y0: 3, X1: 3 + 1e-6, Y1: 30})
		b.FillRect(full, indigo)
		b.PopClip()
		// Strokes: huge and tiny widths, huge coordinates, dashes.
		var p gfx.Path
		p.MoveTo(gfx.Pt(-big, 4))
		p.CubicTo(gfx.Pt(0, -big), gfx.Pt(5, big), gfx.Pt(big, 20))
		b.StrokePath(p, gfx.NewStrokeStyle(3), slate)
		b.StrokePath(gfx.CirclePath(gfx.Pt(16, 16), 10), gfx.NewStrokeStyle(1e6), slate)
		b.StrokePath(gfx.CirclePath(gfx.Pt(16, 16), 10), gfx.NewStrokeStyle(1e-6), slate)
		dash := gfx.NewStrokeStyle(2)
		dash.Dashes = []float32{1e-3, 1e-3}
		b.StrokePath(gfx.CirclePath(gfx.Pt(16, 16), 1e6), dash, slate)
		// Everything again under extreme transforms.
		for _, m := range []gfx.Affine{gfx.Scale(1e18, 1e18), gfx.Scale(1e-18, 1e-18), {1e30, 1e30, -1e30, 1e30, 0, 0}} {
			b.PushTransform(m)
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(-1, -1, 2, 2), 0.5))
			b.PushLayer(0.5, gfx.SrcOver)
			b.FillRect(gfx.RectXYWH(-1, -1, 2, 2), gfx.ConicGradient{Stops: st})
			b.DrawImage(testImage, gfx.Rect{}, gfx.RectXYWH(-1, -1, 2, 2), gfx.SampleLinear)
			b.StrokePath(gfx.CirclePath(gfx.Pt(0, 0), 1), gfx.NewStrokeStyle(0.1), slate)
			b.PopLayer()
			b.PopClip()
			b.PopTransform()
		}
	})
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	start := time.Now()
	for _, scale := range []float32{1, 1e-6, 3e30, 2.5} {
		Clear(img, image.Rectangle{}, gfx.Transparent)
		Render(img, dl, Options{Scale: scale})
		Render(img, dl, Options{Scale: scale, Region: image.Rect(5, 7, 9, 30)})
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("rendering took %v", d)
	}
}
