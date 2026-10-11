package raster

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

var huge = gfx.Rect{X0: -1e5, Y0: -1e5, X1: 1e5, Y1: 1e5}

// randomShape returns a random path, a polygon with curves, and a fill
// rule.
func randomShape(rng *rand.Rand) (gfx.Path, gfx.FillRule) {
	f := func() float32 { return float32(rng.Float64() * 64) }
	var p gfx.Path
	p.MoveTo(gfx.Pt(f(), f()))
	for n := 2 + rng.IntN(5); n > 0; n-- {
		switch rng.IntN(3) {
		case 0:
			p.LineTo(gfx.Pt(f(), f()))
		case 1:
			p.QuadTo(gfx.Pt(f(), f()), gfx.Pt(f(), f()))
		case 2:
			p.CubicTo(gfx.Pt(f(), f()), gfx.Pt(f(), f()), gfx.Pt(f(), f()))
		}
	}
	p.Close()
	return p, gfx.FillRule(rng.IntN(2))
}

type clipCase struct {
	name       string
	fill, clip func(b *gfx.Builder)
}

func randomClipCases(rng *rand.Rand) []clipCase {
	p, rule := randomShape(rng)
	rr := randomRRect(rng)
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	rect := gfx.RectXYWH(f(0, 40), f(0, 40), f(0.1, 30), f(0.1, 30))
	aligned := gfx.RectXYWH(float32(rng.IntN(40)), float32(rng.IntN(40)), float32(1+rng.IntN(30)), float32(1+rng.IntN(30)))
	return []clipCase{
		{"path", func(b *gfx.Builder) { b.FillPath(p, rule, gfx.Black) }, func(b *gfx.Builder) { b.PushClipPath(p, rule) }},
		{"rrect", func(b *gfx.Builder) { b.FillRRect(rr, gfx.Black) }, func(b *gfx.Builder) { b.PushClipRRect(rr) }},
		{"rect", func(b *gfx.Builder) { b.FillRect(rect, gfx.Black) }, func(b *gfx.Builder) { b.PushClipRect(rect) }},
		{"aligned", func(b *gfx.Builder) { b.FillRect(aligned, gfx.Black) }, func(b *gfx.Builder) { b.PushClipRect(aligned) }},
	}
}

var clipTransforms = []gfx.Affine{
	gfx.Identity, gfx.Translate(0.3, 0.7), gfx.Scale(1.3, 0.8),
	gfx.RotateAbout(0.5, gfx.Pt(32, 32)), gfx.Translate(64, 0).Mul(gfx.Rotate(math.Pi / 2)),
}

// Clipping an opaque fill that covers everything by a shape gives exactly
// the pixels of filling the shape (the mask is the shape's coverage), for
// every clip kind, transform and scale. The same holds when the fill is in
// an opaque layer inside the clip.
func TestClipEqualsFill(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	for i := 0; i < 100; i++ {
		m := clipTransforms[i%len(clipTransforms)]
		for _, c := range randomClipCases(rng) {
			for _, scale := range []float32{1, 1.5} {
				want := coverageImage(100, 100, scale, func(b *gfx.Builder) {
					b.PushTransform(m)
					c.fill(b)
					b.PopTransform()
				})
				for _, layer := range []bool{false, true} {
					got := coverageImage(100, 100, scale, func(b *gfx.Builder) {
						b.PushTransform(m)
						c.clip(b)
						if layer {
							b.PushLayer(1, gfx.SrcOver)
						}
						b.FillRect(huge, gfx.Black)
						if layer {
							b.PopLayer()
						}
						b.PopClip()
						b.PopTransform()
					})
					if d, _, at := alphaDiff(got, want); d != 0 {
						t.Fatalf("%s clip under %v at scale %v (layer %v): differs from the fill by %d at %v", c.name, m, scale, layer, d, at)
					}
				}
			}
		}
	}
}

// Nested clips intersect: the mask is the product of the coverages, each
// rounded to 8 bits.
func TestNestedClipsMultiply(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	for i := 0; i < 60; i++ {
		m := clipTransforms[i%len(clipTransforms)]
		a := randomClipCases(rng)[rng.IntN(4)]
		c := randomClipCases(rng)[rng.IntN(4)]
		covA := coverageImage(100, 100, 1, func(b *gfx.Builder) { b.PushTransform(m); a.fill(b); b.PopTransform() })
		covC := coverageImage(100, 100, 1, func(b *gfx.Builder) { c.fill(b) })
		// The Builder nests blocks strictly, so the inner clip undoes the
		// outer transform with a nested PushTransform of its inverse.
		inv, _ := m.Invert()
		got := coverageImage(100, 100, 1, func(b *gfx.Builder) {
			b.PushTransform(m)
			a.clip(b)
			b.PushTransform(inv)
			c.clip(b)
			b.FillRect(huge, gfx.Black)
			b.PopClip()
			b.PopTransform()
			b.PopClip()
			b.PopTransform()
		})
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				o := got.PixOffset(x, y) + 3
				if want := mul8(covA.Pix[o], covC.Pix[o]); got.Pix[o] != want {
					// The inverse transform is rounded to float32, so the
					// inner shape may move by a tiny amount: allow 1.
					if d := int(got.Pix[o]) - int(want); d > 1 || d < -1 {
						t.Fatalf("%s in %s under %v: pixel (%d, %d) = %d, want %d*%d/255 = %d", c.name, a.name, m, x, y, got.Pix[o], covA.Pix[o], covC.Pix[o], want)
					}
				}
			}
		}
	}
}

// A pixel-aligned rect clip needs no mask; other clips allocate one, which
// is returned to the Renderer's pool at PopClip.
func TestClipMaskUse(t *testing.T) {
	for _, tc := range []struct {
		clip  func(b *gfx.Builder)
		masks int
	}{
		{func(b *gfx.Builder) { b.PushClipRect(gfx.RectXYWH(2, 3, 10, 12)) }, 0},
		{func(b *gfx.Builder) { b.PushClipRect(gfx.RectXYWH(2.5, 3, 10, 12)) }, 1},
		{func(b *gfx.Builder) { b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(2, 3, 10, 12), 3)) }, 1},
	} {
		r := NewRenderer()
		img := image.NewRGBA(image.Rect(0, 0, 20, 20))
		r.Render(img, build(func(b *gfx.Builder) {
			tc.clip(b)
			b.FillRect(huge, gfx.Black)
			b.PopClip()
		}), Options{})
		if len(r.free) != tc.masks {
			t.Errorf("%d buffers used, want %d", len(r.free), tc.masks)
		}
	}
	// Pixel aligned at scale 2 too, and under an exact 90 degree rotation
	// (gfx.Rotate(math.Pi/2) is not exact: its cosine is 6e-17).
	r := NewRenderer()
	r.Render(image.NewRGBA(image.Rect(0, 0, 40, 40)), build(func(b *gfx.Builder) {
		b.PushTransform(gfx.Affine{0, 1, -1, 0, 20, 0})
		b.PushClipRect(gfx.RectXYWH(1.5, 2, 3, 4.5))
		b.FillRect(huge, gfx.Black)
		b.PopClip()
		b.PopTransform()
	}), Options{Scale: 2})
	if len(r.free) != 0 {
		t.Errorf("aligned clip under rotation and scale 2 used a mask")
	}
}

// Shapes in a translucent layer are composited as one group: where they
// overlap, the result is the top shape at the layer's opacity, not the two
// blended twice.
func TestLayerGroupOpacity(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 30, 10))
	Clear(img, image.Rectangle{}, gfx.White)
	Render(img, build(func(b *gfx.Builder) {
		b.PushLayer(0.5, gfx.SrcOver)
		b.FillRect(gfx.Rect{X0: 0, Y0: 0, X1: 20, Y1: 10}, rose)
		b.FillRect(gfx.Rect{X0: 10, Y0: 0, X1: 30, Y1: 10}, indigo)
		b.PopLayer()
	}), Options{})
	if a, b := img.RGBAAt(15, 5), img.RGBAAt(25, 5); a != b {
		t.Errorf("overlap %v differs from the top shape alone %v", a, b)
	}
	if a, b := img.RGBAAt(5, 5), img.RGBAAt(25, 5); a == b {
		t.Errorf("the two shapes have the same color %v", a)
	}
	// Opacity 50% of opaque indigo over white.
	want := image.NewRGBA(image.Rect(0, 0, 1, 1))
	Clear(want, image.Rectangle{}, gfx.White)
	Render(want, build(func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 1, Y1: 1}, indigo.WithAlpha(0.5)) }), Options{})
	if got := img.RGBAAt(25, 5); got != want.RGBAAt(0, 0) {
		t.Errorf("layer at 50%%: %v, direct translucent fill: %v", got, want.RGBAAt(0, 0))
	}
}

// An opaque layer onto a transparent surface gives the same pixels as
// drawing directly.
func TestLayerOpaqueEqualsDirect(t *testing.T) {
	draw := func(b *gfx.Builder) {
		b.FillPath(gfx.CirclePath(gfx.Pt(12.3, 10.6), 8), gfx.NonZero, rose.WithAlpha(0.7))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(5.5, 4.25, 16, 12), 4), gfx.LinearGradient{End: gfx.Pt(20, 0), Stops: stops(sky, amber)})
	}
	want := renderTransparent(30, 24, 1.5, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(1, 1, 5, 5), slate)
		draw(b)
	})
	got := renderTransparent(30, 24, 1.5, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(1, 1, 5, 5), slate)
		b.PushLayer(1, gfx.SrcOver)
		draw(b)
		b.PopLayer()
	})
	// Over the transparent background the layer is exact; over the slate
	// square, the blend of the group rounds differently from the blend of
	// each shape: allow 1.
	for i := range got.Pix {
		if d := int(got.Pix[i]) - int(want.Pix[i]); d > 1 || d < -1 {
			x, y := (i/4)%30, (i/4)/30
			t.Fatalf("pixel (%d, %d): layer %v, direct %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
		}
	}
}
