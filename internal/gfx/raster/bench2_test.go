package raster

import (
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// benchShadowCards draws a 10x10 grid of white rounded-xl cards with
// Tailwind's shadow-xl (two blurred layers) on a 1920x1080 page.
func benchShadowCards(layers []cssShadow) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, twGray100)
	for i := 0; i < 100; i++ {
		x, y := float32(i%10)*190+25, float32(i/10)*106+12
		card := gfx.NewRRect(gfx.RectXYWH(x, y, 140, 70), 12)
		drawShadows(b, card, layers)
		b.FillRRect(card, gfx.White)
	}
	return b.Finish()
}

// BenchmarkShadowCards1080p renders 100 cards with shadow-xl.
func BenchmarkShadowCards1080p(b *testing.B) {
	benchRender(b, benchShadowCards(twShadows[5].layers), 1, image.Rectangle{})
}

// BenchmarkShadowCards1080pScale2 is the same at 3840x2160.
func BenchmarkShadowCards1080pScale2(b *testing.B) {
	benchRender(b, benchShadowCards(twShadows[5].layers), 2, image.Rectangle{})
}

// BenchmarkShadowCardsRotated1080p renders the cards under a small
// rotation (the per-pixel local evaluation).
func BenchmarkShadowCardsRotated1080p(b *testing.B) {
	benchRender(b, benchShadowCardsRotated(), 1, image.Rectangle{})
}

func benchShadowCardsRotated() *gfx.DisplayList {
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, twGray100)
	for i := 0; i < 100; i++ {
		x, y := float32(i%10)*190+25, float32(i/10)*106+12
		card := gfx.NewRRect(gfx.RectXYWH(x, y, 140, 70), 12)
		bl.PushTransform(gfx.RotateAbout(0.05, card.Rect.Center()))
		drawShadows(bl, card, twShadows[5].layers)
		bl.FillRRect(card, gfx.White)
		bl.PopTransform()
	}
	return bl.Finish()
}

// BenchmarkLayerBlur1080p blurs a full-screen layer with sigma 8.
func BenchmarkLayerBlur1080p(b *testing.B) {
	benchRender(b, benchLayerBlur(), 1, image.Rectangle{})
}

func benchLayerBlur() *gfx.DisplayList { return benchLayerBlurSigma(8) }

func benchLayerBlurSigma(sigma float32) *gfx.DisplayList {
	bl := gfx.NewBuilder()
	bl.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: sigma})
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.LinearGradient{End: gfx.Pt(1920, 1080), Stops: stops(indigo, fuchsia, rose)})
	for i := 0; i < 40; i++ {
		bl.FillRRect(gfx.NewRRect(gfx.RectXYWH(float32(i%8)*240+20, float32(i/8)*210+20, 200, 170), 24), gfx.White.WithAlpha(0.6))
	}
	bl.PopLayer()
	return bl.Finish()
}

// BenchmarkDropShadowPaths1080p renders 50 star paths, each in a layer with
// a drop shadow.
func BenchmarkDropShadowPaths1080p(b *testing.B) {
	rng := rand.New(rand.NewPCG(9, 9))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	bl := gfx.NewBuilder()
	bl.FillRect(gfx.Rect{X1: 1920, Y1: 1080}, gfx.White)
	for i := 0; i < 50; i++ {
		bl.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 4), StdDev: 3, Color: gfx.Black.WithAlpha(0.3)})
		bl.FillPath(starPath(f(60, 1860), f(60, 1020), f(20, 60)), gfx.NonZero, amber)
		bl.PopLayer()
	}
	benchRender(b, bl.Finish(), 1, image.Rectangle{})
}
