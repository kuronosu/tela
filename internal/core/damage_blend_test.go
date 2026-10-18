package core

import (
	"bytes"
	"fmt"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// blendScene is the state of a synthetic frame with blend layers, mask
// layers and focal gradients (core does not paint them yet, so the frames
// are display lists built directly).
type blendScene struct {
	bg       [2]gfx.Rect
	bgColor  [2]gfx.Color
	mode     gfx.BlendMode
	src      gfx.Rect
	opacity  float32
	maskRect gfx.Rect
	maskMode gfx.MaskMode
	maskA    float32
	focal    gfx.Point
	nested   gfx.BlendMode
}

func (s *blendScene) list() *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 160, Y1: 120}, gfx.LinearGradient{End: gfx.Pt(160, 120),
		Stops: []gfx.GradientStop{{Offset: 0, Color: gfx.MustParseHex("#6366f1")}, {Offset: 1, Color: gfx.MustParseHex("#f59e0b")}}})
	for i := range s.bg {
		b.FillRect(s.bg[i], s.bgColor[i])
	}
	b.PushLayer(s.opacity, s.mode)
	b.FillRRect(gfx.NewRRect(s.src, 6), gfx.FocalGradient{Focal: s.focal, Center: s.src.Center(), Radius: s.src.Width() / 2,
		Stops: []gfx.GradientStop{{Offset: 0, Color: gfx.White}, {Offset: 1, Color: gfx.MustParseHex("#f43f5e")}}})
	b.PopLayer()
	mb := gfx.NewBuilder()
	mb.FillRect(s.maskRect, gfx.LinearGradient{Start: gfx.Pt(s.maskRect.X0, 0), End: gfx.Pt(s.maskRect.X1, 0),
		Stops: []gfx.GradientStop{{Offset: 0, Color: gfx.White.WithAlpha(s.maskA)}, {Offset: 1, Color: gfx.Black}}})
	b.PushLayer(1, gfx.SrcOver) // isolated group
	b.FillRect(gfx.RectXYWH(10, 60, 140, 50), gfx.MustParseHex("#10b981").WithAlpha(0.6))
	b.PushMaskLayer(gfx.Mask{List: mb.Finish(), Mode: s.maskMode}, 0.9, s.nested)
	b.FillRect(gfx.RectXYWH(20, 40, 120, 70), gfx.MustParseHex("#0ea5e9"))
	b.FillRect(gfx.RectXYWH(60, 50, 30, 60), gfx.MustParseHex("#c026d3").WithAlpha(0.7))
	b.PopLayer()
	b.PopLayer()
	return b.Finish()
}

func (s *blendScene) mutate(rng *rand.Rand) string {
	f := func(lo, hi float32) float32 { return lo + float32(rng.Float64())*(hi-lo) }
	switch rng.IntN(8) {
	case 0:
		i := rng.IntN(2)
		s.bg[i] = gfx.RectXYWH(f(0, 120), f(0, 90), f(10, 60), f(10, 40))
		return "move backdrop"
	case 1:
		s.bgColor[rng.IntN(2)] = gfx.Color{R: f(0, 1), G: f(0, 1), B: f(0, 1), A: f(0.3, 1)}
		return "recolor backdrop"
	case 2:
		s.mode = gfx.BlendMode(rng.IntN(int(gfx.PlusLighter) + 1))
		return "blend mode"
	case 3:
		s.src = gfx.RectXYWH(f(0, 110), f(0, 70), f(20, 60), f(20, 50))
		return "move source"
	case 4:
		s.opacity = f(0.2, 1)
		return "opacity"
	case 5:
		s.maskRect = gfx.RectXYWH(f(0, 80), f(20, 60), f(30, 80), f(20, 60))
		s.maskA = f(0, 1)
		return "mask"
	case 6:
		s.maskMode = gfx.MaskMode(rng.IntN(2))
		s.nested = gfx.BlendMode(rng.IntN(int(gfx.PlusLighter) + 1))
		return "mask mode"
	default:
		s.focal = gfx.Pt(f(0, 160), f(0, 120))
		return "focal"
	}
}

// TestDamageBlendAndMaskLayers checks that incremental frames stay
// bit-identical to full rasterizations when blend layers, mask layers and
// focal gradients change, or what lies under them does (ADR-061, ADR-074):
// the damage of the frame lists is rasterized as regions into the
// persistent framebuffer.
func TestDamageBlendAndMaskLayers(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		rng := rand.New(rand.NewPCG(74, uint64(scale*2)))
		s := &blendScene{
			bg:      [2]gfx.Rect{gfx.RectXYWH(10, 10, 50, 30), gfx.RectXYWH(90, 20, 40, 60)},
			bgColor: [2]gfx.Color{gfx.MustParseHex("#ef4444"), gfx.White.WithAlpha(0.5)},
			mode:    gfx.Multiply, src: gfx.RectXYWH(30, 15, 60, 50), opacity: 0.9,
			maskRect: gfx.RectXYWH(20, 40, 100, 60), maskA: 1, focal: gfx.Pt(50, 30), nested: gfx.Screen,
		}
		w, h := int(160*scale), int(120*scale)
		fb := image.NewRGBA(image.Rect(0, 0, w, h))
		prev := &frame{dl: s.list(), scale: scale}
		raster.Clear(fb, image.Rectangle{}, gfx.White)
		raster.Render(fb, prev.dl, raster.Options{Scale: scale})
		partial := 0
		for step := range 120 {
			what := s.mutate(rng)
			next := &frame{dl: s.list(), scale: scale}
			d := frameDamage(prev, next, fb.Rect)
			if d.Full {
				raster.Clear(fb, image.Rectangle{}, gfx.White)
				raster.Render(fb, next.dl, raster.Options{Scale: scale})
			} else {
				for _, sh := range d.Shifts {
					shiftPixels(fb, sh)
				}
				for _, rc := range d.Rects {
					raster.Clear(fb, rc, gfx.White)
					raster.Render(fb, next.dl, raster.Options{Scale: scale, Region: rc})
				}
				partial++
			}
			want := image.NewRGBA(fb.Rect)
			raster.Clear(want, image.Rectangle{}, gfx.White)
			raster.Render(want, next.dl, raster.Options{Scale: scale})
			if !bytes.Equal(fb.Pix, want.Pix) {
				p, _ := firstDiff(fb, want)
				t.Fatalf("x%g step %d (%s): incremental frame differs from a full rasterization at %v (damage %v)",
					scale, step, what, p, fmt.Sprint(d.Rects))
			}
			prev = next
		}
		t.Logf("x%g: %d of 120 frames rasterized as regions", scale, partial)
		if partial == 0 {
			t.Errorf("x%g: every frame was a full rasterization", scale)
		}
	}
}
