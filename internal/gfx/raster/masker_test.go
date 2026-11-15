package raster

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// alphaOf renders dl in opaque black onto a transparent image and returns
// its alpha channel over rect: the coverage the Renderer drew with (over a
// transparent destination, source-over stores the coverage as is).
func alphaOf(dl *gfx.DisplayList, rect image.Rectangle) []uint8 {
	img := image.NewRGBA(rect)
	NewRenderer().Render(img, dl, Options{Workers: 1, GlyphCache: NewGlyphCache(1 << 20)})
	out := make([]uint8, rect.Dx()*rect.Dy())
	for i := range out {
		out[i] = img.Pix[4*i+3]
	}
	return out
}

func sameCoverage(t *testing.T, what string, got, want []uint8, rect image.Rectangle) {
	t.Helper()
	ink := false
	for i := range want {
		ink = ink || want[i] != 0
		if got[i] != want[i] {
			t.Fatalf("%s: pixel (%d, %d) is %d, the Renderer's coverage is %d", what,
				rect.Min.X+i%rect.Dx(), rect.Min.Y+i/rect.Dx(), got[i], want[i])
		}
	}
	if !ink {
		t.Fatalf("%s: the Renderer drew nothing", what)
	}
}

// TestMaskerShapes checks that a Masker's coverage is the Renderer's, bit
// for bit, for fills, strokes, rectangles and rounded rectangles under
// several transforms, over rectangles that cut the shapes.
func TestMaskerShapes(t *testing.T) {
	star := gfxtest.StarPath(60, 50, 44)
	curve := gfxtest.TightCurve(20, 30)
	dashed := gfx.NewStrokeStyle(5)
	dashed.Dashes, dashed.Cap, dashed.Join = []float32{9, 4}, gfx.CapRound, gfx.JoinBevel
	rr := gfx.RRectCorners(gfx.RectXYWH(10.3, 12.7, 80, 60), gfx.Pt(30, 12), gfx.Pt(4, 4), gfx.Pt(20, 28), gfx.Pt(0, 0))
	var k Masker
	for _, m := range []gfx.Affine{
		gfx.Identity, gfx.Scale(1.5, 1.5), gfx.Translate(3.37, -2.81).Mul(gfx.Scale(2, 0.7)),
		gfx.RotateAbout(0.4, gfx.Pt(60, 50)), gfx.Translate(40, 10).Mul(gfx.Skew(0.3, -0.2)),
		gfx.Translate(120, 0).Mul(gfx.Scale(-1, 1)), {0, 1.25, -0.75, 0, 110.3, 4.4},
	} {
		for _, rect := range []image.Rectangle{image.Rect(-20, -20, 220, 200), image.Rect(31, 17, 97, 64), image.Rect(60, 40, 61, 41)} {
			n := rect.Dx() * rect.Dy()
			check := func(what string, draw func(b *gfx.Builder), cover func(dst []uint8)) {
				t.Helper()
				dl := gfxtest.Build(func(b *gfx.Builder) {
					b.PushTransform(m)
					draw(b)
					b.PopTransform()
				})
				got := make([]uint8, n)
				cover(got)
				if rect.Dx() == 1 {
					return // a single pixel may be empty: only compare
				}
				sameCoverage(t, what, got, alphaOf(dl, rect), rect)
			}
			for _, rule := range []gfx.FillRule{gfx.NonZero, gfx.EvenOdd} {
				check("fill", func(b *gfx.Builder) { b.FillPath(star, rule, gfx.Black) },
					func(dst []uint8) { k.Fill(dst, rect, m, star, rule) })
			}
			check("stroke", func(b *gfx.Builder) { b.StrokePath(curve, dashed, gfx.Black) },
				func(dst []uint8) { k.Fill(dst, rect, m, k.StrokeOutline(curve, dashed, m), gfx.NonZero) })
			check("rect", func(b *gfx.Builder) { b.FillRect(rr.Rect, gfx.Black) },
				func(dst []uint8) { k.Rect(dst, rect, m, rr.Rect) })
			check("rrect", func(b *gfx.Builder) { b.FillRRect(rr, gfx.Black) },
				func(dst []uint8) { k.RRect(dst, rect, m, rr) })
		}
	}
	if k.r.strokes.size() == 0 {
		t.Error("the Masker did not cache the stroke outline")
	}
	k.Trim()
	if n := k.r.ScratchBytes(); n != 0 {
		t.Errorf("after Trim the Masker keeps %d bytes", n)
	}
}

// TestMaskerGlyphs checks that the glyphs a Masker resolves, composited the
// way the Renderer does, are the Renderer's picture of the run: masks and
// sprites under scales and translations, outlines and lists otherwise.
func TestMaskerGlyphs(t *testing.T) {
	f := gfxtest.FontA
	rect := image.Rect(0, 0, 400, 200)
	for _, m := range []gfx.Affine{
		gfx.Identity, gfx.Scale(1.5, 1.5), gfx.Translate(0.3, 0.6).Mul(gfx.Scale(2, 1.25)),
		gfx.RotateAbout(0.2, gfx.Pt(100, 60)), gfx.Translate(-60, -420).Mul(gfx.Scale(13, 13)),
	} {
		run := f.Run("SoAl*i%S", 22, 6.37, 40.2, gfx.Black)
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.PushTransform(m)
			b.DrawGlyphRun(run)
			b.PopTransform()
		})
		want := image.NewRGBA(rect)
		cache := NewGlyphCache(1 << 22)
		NewRenderer().Render(want, dl, Options{Workers: 1, GlyphCache: cache})

		got := image.NewRGBA(rect)
		var k Masker
		var r Renderer // composites what the Masker returns
		alpha, ok := k.BeginGlyphRun(&run, m, cache, 0)
		if !ok || alpha != 255 {
			t.Fatalf("BeginGlyphRun: alpha %d, ok %v", alpha, ok)
		}
		kinds := map[GlyphKind]int{}
		for _, g := range run.Glyphs {
			var gl Glyph
			k.Glyph(g, rect, &gl)
			kinds[gl.Kind]++
			// Coverage rasterized between the glyphs of a run, under
			// another transform, does not disturb the run.
			k.Fill(make([]uint8, 100), image.Rect(0, 0, 10, 10), gfx.Scale(3, 3), gfxtest.StarPath(2, 2, 2), gfx.NonZero)
			var one *gfx.DisplayList
			switch gl.Kind {
			case GlyphMask:
				for y := gl.Rect.Min.Y; y < gl.Rect.Max.Y; y++ {
					for x := gl.Rect.Min.X; x < gl.Rect.Max.X; x++ {
						c := uint32(gl.Pix[(y-gl.Rect.Min.Y)*gl.Rect.Dx()+x-gl.Rect.Min.X])
						if image.Pt(x, y).In(rect) && c != 0 {
							blendPixel(got.Pix[got.PixOffset(x, y):][:4:4], 0, 0, 0, 255, c)
						}
					}
				}
			case GlyphSprite:
				for y := gl.Rect.Min.Y; y < gl.Rect.Max.Y; y++ {
					for x := gl.Rect.Min.X; x < gl.Rect.Max.X; x++ {
						s := gl.Pix[4*((y-gl.Rect.Min.Y)*gl.Rect.Dx()+x-gl.Rect.Min.X):][:4]
						if image.Pt(x, y).In(rect) && s[3] != 0 {
							blendPixel(got.Pix[got.PixOffset(x, y):][:4:4], uint32(s[0]), uint32(s[1]), uint32(s[2]), uint32(s[3]), 255)
						}
					}
				}
			case GlyphOutline:
				one = gfxtest.Build(func(b *gfx.Builder) {
					b.PushTransform(gl.Transform)
					b.FillPath(gl.Outline, gfx.NonZero, gfx.Black)
					b.PopTransform()
				})
			case GlyphList:
				one = gfxtest.Build(func(b *gfx.Builder) {
					b.PushTransform(gl.Transform)
					b.AppendList(gl.List, nil)
					b.PopTransform()
				})
			}
			if one != nil {
				r.Render(got, one, Options{Workers: 1, GlyphCache: cache})
			}
		}
		fast := m[1] == 0 && m[2] == 0 && m[0] < 13
		cached, direct := kinds[GlyphMask] > 0 && kinds[GlyphSprite] > 0, kinds[GlyphOutline] > 0
		if fast != cached || fast == direct || (m[1] != 0 && kinds[GlyphList] == 0) {
			t.Errorf("transform %v: kinds %v", m, kinds)
		}
		sameImages(t, "the glyphs of a Masker", got, want)
	}
}
