package anim

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// The fills and strokes of two styles interpolate as their colors do, with
// the text color for currentColor, and other paints do not. The values are
// Chromium 155's (getComputedStyle at a quarter of a linear transition).
func TestBlendPaints(t *testing.T) {
	red, lime, blue := gfx.RGBA8(255, 0, 0, 255), gfx.RGBA8(0, 255, 0, 255), gfx.RGBA8(0, 0, 255, 255)
	col := func(c gfx.Color) style.SVGPaint { return style.SVGPaint{Kind: style.SVGPaintColor, Color: c} }
	cur := func(a float32) style.SVGPaint {
		return style.SVGPaint{Kind: style.SVGPaintCurrent, Color: gfx.Color{A: a}}
	}
	none := style.SVGPaint{Kind: style.SVGPaintNone}
	oklch1, oklch2 := gfx.OKLCH(0.623, 0.214, 259.815, 1), gfx.OKLCH(0.637, 0.237, 25.331, 1)
	for _, c := range []struct {
		name         string
		from, to     style.SVGPaint
		text0, text1 gfx.Color // the text color, which transitions too
		want         gfx.Color
	}{
		{"hex", col(red), col(blue), gfx.Black, gfx.Black, gfx.RGBA8(191, 0, 64, 255)},
		{"transparent", col(gfx.Color{R: 1}), col(blue), gfx.Black, gfx.Black, gfx.Color{B: 1, A: 0.25}},
		{"oklch", col(oklch1), col(oklch2), gfx.Black, gfx.Black, gfx.Oklab(0.6265, 0.0251725, -0.132621, 1)},
		{"current to color", cur(1), col(blue), red, red, gfx.RGBA8(191, 0, 64, 255)},
		{"current to color, the text color moves", cur(1), col(blue), red, lime, gfx.RGBA8(143, 48, 64, 255)},
		{"color to current, the text color moves", col(blue), cur(1), red, lime, gfx.RGBA8(48, 16, 191, 255)},
		// (Chromium mixes a color-mix() of currentColor as if it were transparent: 0.75.)
		{"current to half of it", cur(1), cur(0.5), red, blue, gfx.Color{R: 191. / 255, B: 64. / 255, A: 0.875}},
	} {
		for _, p := range []Prop{PropFill, PropStroke} {
			var a, b, dst style.ComputedStyle
			a.Text.Color, b.Text.Color = c.text0, c.text1
			*paintOf(p, &a), *paintOf(p, &b) = c.from, c.to
			if !CanInterpolate(p, &a, &b) || !CanInterpolate(p, &b, &a) || Equal(p, &a, &b) {
				t.Errorf("%s: %v does not interpolate", c.name, p)
			}
			Blend(PropColor, &dst, &a, &b, 0.25)
			Blend(p, &dst, &a, &b, 0.25)
			got := *paintOf(p, &dst)
			if g, w := got.Color, c.want; got.Kind != style.SVGPaintColor || !near(float64(g.R), float64(w.R), 0.003) ||
				!near(float64(g.G), float64(w.G), 0.003) || !near(float64(g.B), float64(w.B), 0.003) || !near(float64(g.A), float64(w.A), 1e-5) {
				t.Errorf("%s: %v is %+v at 1/4, want %v", c.name, p, got, w)
			}
			Blend(p, &dst, &a, &b, 0)
			if got := *paintOf(p, &dst); got != col(c.from.Resolve(dst.Text.Color)) {
				t.Errorf("%s: %v starts at %+v", c.name, p, got)
			}
			Copy(p, &dst, &b)
			if !Equal(p, &dst, &b) {
				t.Errorf("%s: Copy of %v", c.name, p)
			}
		}
	}
	for _, paint := range []style.SVGPaint{none, {}} {
		var a, b style.ComputedStyle
		a.Fill, b.Fill, a.Stroke, b.Stroke = col(red), paint, paint, cur(1)
		for _, p := range []Prop{PropFill, PropStroke} {
			if CanInterpolate(p, &a, &b) || CanInterpolate(p, &b, &a) {
				t.Errorf("%v interpolates between %+v and %+v", p, a.Fill, paint)
			}
		}
	}
	for p := PropFill; p <= PropStrokeWidth; p++ {
		if !p.Inherited() || !p.IsPaint() || p.Class() != ClassPaint {
			t.Errorf("%v: inherited %v, class %v", p, p.Inherited(), p.Class())
		}
	}
}

func TestBlendStrokeWidth(t *testing.T) {
	var a, b, unset, dst style.ComputedStyle
	a.StrokeWidth, a.StrokeWidthSet = 1, true
	b.StrokeWidth, b.StrokeWidthSet = 4, true
	Blend(PropStrokeWidth, &dst, &a, &b, 0.25)
	if dst.StrokeWidth != 1.75 || !dst.StrokeWidthSet {
		t.Errorf("a quarter from 1 to 4: %v", dst.StrokeWidth)
	}
	Blend(PropStrokeWidth, &dst, &b, &a, 1.5) // a curve that overshoots
	if dst.StrokeWidth != 0 {
		t.Errorf("below 0: %v", dst.StrokeWidth)
	}
	if !CanInterpolate(PropStrokeWidth, &a, &b) || CanInterpolate(PropStrokeWidth, &unset, &a) || CanInterpolate(PropStrokeWidth, &a, &unset) {
		t.Error("CanInterpolate")
	}
	Blend(PropStrokeWidth, &dst, &unset, &b, 0.6)
	if dst.StrokeWidth != 4 || !dst.StrokeWidthSet {
		t.Errorf("unset to 4: %v", dst.StrokeWidth)
	}
	// A width that is not set is not 0.
	zero := style.ComputedStyle{StrokeWidthSet: true}
	if Equal(PropStrokeWidth, &unset, &zero) {
		t.Error("unset equals 0")
	}
}
