package raster

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Benchmarks of blend layers, mask layers and focal gradients over 512x512
// device pixels, on one goroutine. The backdrop and the content are
// gradients covering the whole frame; the cost of a layer is the content's
// raster plus the composite (the backdrop is part of every variant).

const benchSide = 512

func benchBackdrop(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(benchSide, benchSide),
		Stops: stops(indigo, rose, amber), Interp: gfx.InterpSRGB})
}

func benchContent(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(0, benchSide),
		Stops: stops(yellow, fuchsia.WithAlpha(0.6), sky), Interp: gfx.InterpSRGB})
}

func bench512(b *testing.B, dl *gfx.DisplayList) {
	img := image.NewRGBA(image.Rect(0, 0, benchSide, benchSide))
	r := NewRenderer()
	r.Render(img, dl, Options{Workers: 1})
	b.SetBytes(4 * benchSide * benchSide)
	b.ResetTimer()
	for range b.N {
		r.Render(img, dl, Options{Workers: 1})
	}
}

func benchBlend(b *testing.B, mode gfx.BlendMode) {
	bench512(b, build(func(bb *gfx.Builder) {
		benchBackdrop(bb)
		bb.PushLayer(1, mode)
		benchContent(bb)
		bb.PopLayer()
	}))
}

// BenchmarkLayer512NoLayer draws the content directly: the baseline.
func BenchmarkLayer512NoLayer(b *testing.B) {
	bench512(b, build(func(bb *gfx.Builder) {
		benchBackdrop(bb)
		benchContent(bb)
	}))
}

func BenchmarkLayer512SrcOver(b *testing.B)    { benchBlend(b, gfx.SrcOver) }
func BenchmarkLayer512Multiply(b *testing.B)   { benchBlend(b, gfx.Multiply) }
func BenchmarkLayer512Difference(b *testing.B) { benchBlend(b, gfx.Difference) }
func BenchmarkLayer512SoftLight(b *testing.B)  { benchBlend(b, gfx.SoftLight) }
func BenchmarkLayer512Hue(b *testing.B)        { benchBlend(b, gfx.Hue) }
func BenchmarkLayer512Plus(b *testing.B)       { benchBlend(b, gfx.PlusLighter) }

func benchMask(b *testing.B, mode gfx.MaskMode, blend gfx.BlendMode) {
	mask := build(func(mb *gfx.Builder) {
		mb.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, gfx.RadialGradient{Center: gfx.Pt(256, 256), Radius: gfx.Pt(256, 256),
			Stops: stops(gfx.White, gfx.Black.WithAlpha(0.5), gfx.Transparent), Interp: gfx.InterpSRGB})
	})
	bench512(b, build(func(bb *gfx.Builder) {
		benchBackdrop(bb)
		bb.PushMaskLayer(gfx.Mask{List: mask, Mode: mode}, 1, blend)
		benchContent(bb)
		bb.PopLayer()
	}))
}

func BenchmarkMask512Alpha(b *testing.B)         { benchMask(b, gfx.MaskAlpha, gfx.SrcOver) }
func BenchmarkMask512Luminance(b *testing.B)     { benchMask(b, gfx.MaskLuminance, gfx.SrcOver) }
func BenchmarkMask512AlphaMultiply(b *testing.B) { benchMask(b, gfx.MaskAlpha, gfx.Multiply) }

// BenchmarkMask512Text masks a gradient with 12 lines of glyphs
// (background-clip: text).
func BenchmarkMask512Text(b *testing.B) {
	mask := build(func(mb *gfx.Builder) {
		for i := range 12 {
			mb.DrawGlyphRun(testFontA.run("AoSil AoSil AoSil AoSil", 36, 4, float32(40+i*40), gfx.Black))
		}
	})
	bench512(b, build(func(bb *gfx.Builder) {
		benchBackdrop(bb)
		bb.PushMaskLayer(gfx.Mask{List: mask}, 1, gfx.SrcOver)
		benchContent(bb)
		bb.PopLayer()
	}))
}

func BenchmarkGradient512Radial(b *testing.B) {
	bench512(b, build(func(bb *gfx.Builder) {
		bb.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, gfx.RadialGradient{Center: gfx.Pt(256, 256), Radius: gfx.Pt(300, 300),
			Stops: stops(amber, rose, indigo), Interp: gfx.InterpSRGB})
	}))
}

func BenchmarkGradient512Focal(b *testing.B) {
	bench512(b, build(func(bb *gfx.Builder) {
		bb.FillRect(gfx.Rect{X1: benchSide, Y1: benchSide}, gfx.FocalGradient{Focal: gfx.Pt(180, 200), FocalRadius: 10,
			Center: gfx.Pt(256, 256), Radius: 300, Stops: stops(amber, rose, indigo), Interp: gfx.InterpSRGB})
	}))
}
