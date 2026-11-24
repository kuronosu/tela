package raster

import (
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Porter-Duff operators (blend.go) and the layers that use them.

// scenes5 are the composite and pattern scenes; the first two are also
// written as HTML and compared with Chromium (chromium5_test.go).
var scenes5 = fromGfxtest(gfxtest.Scenes5)

// operators are the operators.
var operators = gfxtest.CompositeModes[2:]

// refFactors returns the Fa and Fb of W3C Compositing and Blending Level 1,
// section 9.1, for the alphas sa and da in [0, 1]: the result is s·Fa + d·Fb
// on premultiplied values. It is written from the specification's table, not
// from blend.go.
func refFactors(mode gfx.BlendMode, sa, da float64) (fa, fb float64) {
	switch mode {
	case gfx.Copy:
		return 1, 0
	case gfx.SrcOver:
		return 1, 1 - sa
	case gfx.DstOver:
		return 1 - da, 1
	case gfx.SrcIn:
		return da, 0
	case gfx.DstIn:
		return 0, sa
	case gfx.SrcOut:
		return 1 - da, 0
	case gfx.DstOut:
		return 0, 1 - sa
	case gfx.SrcAtop:
		return da, 1 - sa
	case gfx.DstAtop:
		return 1 - da, sa
	case gfx.Xor:
		return 1 - da, 1 - sa
	case gfx.PlusLighter: // Canvas's "lighter"
		return 1, 1
	}
	panic("no factors for " + mode.String())
}

// refComposite is one pixel of an operator at 8 bits, rounded once.
func refComposite(mode gfx.BlendMode, s, d [4]uint8) [4]uint8 {
	fa, fb := refFactors(mode, float64(s[3])/255, float64(d[3])/255)
	var o [4]uint8
	for c := range o {
		// A multiple of 1/255, never near a half: the epsilon of float64
		// does not decide the rounding.
		o[c] = uint8(math.Floor(min(float64(s[c])*fa+float64(d[c])*fb, 255) + 0.5))
	}
	return o
}

// TestPorterDuff checks every operator against the reference on every pair
// of alphas, with random colors: exact, and valid premultiplied pixels.
func TestPorterDuff(t *testing.T) {
	rng := rand.New(rand.NewPCG(319, 1))
	pixel := func(a int) [4]uint8 {
		var p [4]uint8
		for c := range 3 {
			switch rng.IntN(3) {
			case 0:
				p[c] = uint8(a)
			case 1:
				p[c] = uint8(rng.IntN(a + 1))
			}
		}
		p[3] = uint8(a)
		return p
	}
	for _, mode := range append([]gfx.BlendMode{gfx.PlusLighter}, operators...) {
		for sa := range 256 {
			for da := range 256 {
				s, d := pixel(sa), pixel(da)
				got, want := blendPx(mode, s, d), refComposite(mode, s, d)
				if got != want {
					t.Fatalf("%s: s %v d %v: %v, want %v", mode, s, d, got, want)
				}
				for c := range 3 {
					if got[c] > got[3] {
						t.Fatalf("%s: s %v d %v: %v is not premultiplied", mode, s, d, got)
					}
				}
			}
		}
		// Unbounded says which operators change the backdrop where the
		// source is transparent.
		d := [4]uint8{10, 20, 30, 200}
		if changes := blendPx(mode, [4]uint8{}, d) != d; changes != mode.Unbounded() {
			t.Errorf("%s: a transparent source changes the backdrop: %v, Unbounded() = %v", mode, changes, mode.Unbounded())
		}
	}
	for _, mode := range allBlendModes {
		if mode.Unbounded() {
			t.Errorf("%s is a mode of CSS and reports Unbounded", mode)
		}
	}
}

// lerpRef is the interpolation by a clip's coverage m.
func lerpRef(d, r [4]uint8, m uint8) [4]uint8 {
	var o [4]uint8
	for c := range o {
		o[c] = uint8(math.Floor((float64(r[c])*float64(m)+float64(d[c])*float64(255-m))/255 + 0.5))
	}
	return o
}

func pixelAt(img *image.RGBA, x, y int) [4]uint8 {
	return [4]uint8(img.Pix[img.PixOffset(x, y):][:4])
}

// layerCase is a layer composited with an operator: its content, the clip
// and the mask around it, its opacity.
type layerCase struct {
	name    string
	clip    func(b *gfx.Builder) // pushes a clip, or nil
	mask    func(b *gfx.Builder) // paints the mask, or nil
	content func(b *gfx.Builder)
	opacity float32
}

const layerCaseSize = 96

func layerCases() []layerCase {
	// Opaque, translucent and transparent pixels on both sides.
	content := func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(34, 20, 40, 62), gfx.LinearGradient{Start: gfx.Pt(0, 20), End: gfx.Pt(0, 82),
			Stops: stops(amber, emerald, emerald.WithAlpha(0.1)), Interp: gfx.InterpSRGB})
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(60, 60)))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(50, 30, 34, 50), 9), sky.WithAlpha(0.6))
		b.PopTransform()
	}
	rrect := func(b *gfx.Builder) { b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(10.3, 12.7, 70, 66), 25)) }
	return []layerCase{
		{name: "plain", content: content, opacity: 1},
		{name: "opacity", content: content, opacity: 0.55},
		{name: "opacity 0", content: content, opacity: 0},
		{name: "opacity below a level", content: content, opacity: 0.001},
		{name: "empty", content: func(*gfx.Builder) {}, opacity: 1},
		{name: "empty in a clip", clip: rrect, content: func(*gfx.Builder) {}, opacity: 1},
		{name: "content outside the surface", content: func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(-50, -50, 10, 10), rose) }, opacity: 1},
		{name: "whole-pixel clip", clip: func(b *gfx.Builder) { b.PushClipRect(gfx.RectXYWH(20, 10, 60, 50)) }, content: content, opacity: 1},
		{name: "rectangle clip", clip: func(b *gfx.Builder) { b.PushClipRect(gfx.RectXYWH(20.4, 10.6, 50.3, 60.5)) }, content: content, opacity: 1},
		{name: "rounded clip", clip: rrect, content: content, opacity: 0.8},
		{name: "path clip", clip: func(b *gfx.Builder) { b.PushClipPath(starPath(48, 48, 46), gfx.EvenOdd) }, content: content, opacity: 1},
		{name: "opacity 0 in a clip", clip: rrect, content: content, opacity: 0},
		{name: "mask", mask: func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 0, 60, 96), gfx.LinearGradient{Start: gfx.Pt(20, 0), End: gfx.Pt(80, 0), Stops: stops(gfx.Black, gfx.Transparent)})
		}, content: content, opacity: 0.9},
		{name: "mask in a clip", clip: rrect, mask: func(b *gfx.Builder) { b.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, 30, 30, 30), 15), gfx.Black) }, content: content, opacity: 1},
		{name: "empty mask", mask: func(*gfx.Builder) {}, content: content, opacity: 1},
		{name: "mask outside the clip", clip: rrect, mask: func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(85, 85, 8, 8), gfx.Black) }, content: content, opacity: 1},
	}
}

func compositeBackdrop(b *gfx.Builder) {
	b.FillRect(gfx.RectXYWH(6, 8, 56, 50), gfx.LinearGradient{Start: gfx.Pt(6, 0), End: gfx.Pt(62, 0), Stops: stops(red, sky), Interp: gfx.InterpSRGB})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20, 40, 50, 50), 25), indigo.WithAlpha(0.5))
}

// draw records the case with mode over the backdrop.
func (c *layerCase) draw(b *gfx.Builder, mode gfx.BlendMode) {
	if c.clip != nil {
		c.clip(b)
	}
	if c.mask != nil {
		b.PushMaskLayer(gfx.Mask{List: build(c.mask)}, c.opacity, mode)
	} else {
		b.PushLayer(c.opacity, mode)
	}
	c.content(b)
	b.PopLayer()
	if c.clip != nil {
		b.PopClip()
	}
}

// ref composites the case over back, pixel by pixel: the content, the mask
// and the clip are rendered apart.
func (c *layerCase) ref(back *image.RGBA, mode gfx.BlendMode) *image.RGBA {
	alone := func(draw func(b *gfx.Builder)) *image.RGBA {
		img := image.NewRGBA(back.Rect)
		Render(img, build(draw), Options{Workers: 1})
		return img
	}
	src := alone(c.content)
	var mask, clip *image.RGBA
	if c.mask != nil {
		mask = alone(c.mask)
	}
	if c.clip != nil {
		clip = alone(func(b *gfx.Builder) {
			c.clip(b)
			b.FillRect(gfx.RectXYWH(-10, -10, layerCaseSize+20, layerCaseSize+20), gfx.Black)
			b.PopClip()
		})
	}
	out := cloneRGBA(back)
	for y := range layerCaseSize {
		for x := range layerCaseSize {
			s, d := pixelAt(src, x, y), pixelAt(back, x, y)
			k := unit8(float64(c.opacity))
			if mask != nil {
				k = mul8(k, pixelAt(mask, x, y)[3])
			}
			for i := range s {
				s[i] = mul8(s[i], k)
			}
			r := refComposite(mode, s, d)
			if clip != nil {
				r = lerpRef(d, r, pixelAt(clip, x, y)[3])
			}
			copy(out.Pix[out.PixOffset(x, y):], r[:])
		}
	}
	return out
}

func sameImage(t *testing.T, what string, got, want *image.RGBA) {
	t.Helper()
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		for x := got.Rect.Min.X; x < got.Rect.Max.X; x++ {
			if g, w := pixelAt(got, x, y), pixelAt(want, x, y); g != w {
				t.Errorf("%s: pixel (%d, %d) = %v, want %v", what, x, y, g, w)
				return
			}
		}
	}
}

// TestCompositeLayers checks layers composited with each operator against
// the reference: at the root, inside a group (which an operator never looks
// out of), in regions and in parallel bands.
func TestCompositeLayers(t *testing.T) {
	rect := image.Rect(0, 0, layerCaseSize, layerCaseSize)
	back := image.NewRGBA(rect)
	Render(back, build(compositeBackdrop), Options{Workers: 1})
	page := image.NewRGBA(rect)
	background(page)
	for _, c := range layerCases() {
		for _, mode := range append([]gfx.BlendMode{gfx.PlusLighter}, operators...) {
			name := c.name + " " + mode.String()
			want := c.ref(back, mode)
			list := build(func(b *gfx.Builder) { compositeBackdrop(b); c.draw(b, mode) })
			got := image.NewRGBA(rect)
			Render(got, list, Options{Workers: 1})
			sameImage(t, name, got, want)

			// Bands, regions and tiles.
			r := NewRenderer()
			r.tune.bandRows, r.tune.minPixels = 5, 1
			par := image.NewRGBA(rect)
			r.Render(par, list, Options{Workers: 4})
			sameImage(t, name+" in bands", par, want)
			tiled := image.NewRGBA(rect)
			for y := 0; y < layerCaseSize; y += 17 {
				for x := 0; x < layerCaseSize; x += 23 {
					r.Render(tiled, list, Options{Region: image.Rect(x, y, x+23, y+17), Workers: 1})
				}
			}
			sameImage(t, name+" in tiles", tiled, want)

			// Appended into another list.
			app := gfx.NewBuilder()
			app.AppendList(list, nil)
			again := image.NewRGBA(rect)
			Render(again, app.Finish(), Options{Workers: 1})
			sameImage(t, name+" appended", again, want)

			// Over the pixels of the frame, at the root: the same formulas.
			over := cloneRGBA(page)
			Render(over, build(func(b *gfx.Builder) { c.draw(b, mode) }), Options{Workers: 1})
			sameImage(t, name+" over the frame", over, c.ref(page, mode))

			// In a group, which is composited over the page: the operator
			// sees the group's pixels only.
			grouped := cloneRGBA(page)
			Render(grouped, build(func(b *gfx.Builder) {
				b.PushLayer(1, gfx.SrcOver)
				compositeBackdrop(b)
				c.draw(b, mode)
				b.PopLayer()
			}), Options{Workers: 1})
			wantGroup := cloneRGBA(page)
			wb := blitter{pix: wantGroup.Pix, stride: wantGroup.Stride, clip: rect}
			for y := range layerCaseSize {
				wb.composite(y, 0, want.Pix[want.PixOffset(0, y):][:4*layerCaseSize], 255)
			}
			sameImage(t, name+" in a group", grouped, wantGroup)
		}
	}
}

// TestCompositeNested composites operators inside operators: the inner one
// changes the outer layer's pixels only, which the outer one composites.
func TestCompositeNested(t *testing.T) {
	rect := image.Rect(0, 0, layerCaseSize, layerCaseSize)
	back := image.NewRGBA(rect)
	Render(back, build(compositeBackdrop), Options{})
	cases := layerCases()
	inner := &cases[9] // a rounded clip and an opacity
	base := func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(16, 16, 60, 60), fuchsia.WithAlpha(0.7)) }
	for _, outer := range operators {
		for _, in := range operators {
			// The outer layer's pixels, by the reference.
			surf := image.NewRGBA(rect)
			Render(surf, build(base), Options{})
			layer := inner.ref(surf, in)
			want := cloneRGBA(back)
			for y := range layerCaseSize {
				for x := range layerCaseSize {
					r := refComposite(outer, pixelAt(layer, x, y), pixelAt(back, x, y))
					copy(want.Pix[want.PixOffset(x, y):], r[:])
				}
			}
			got := image.NewRGBA(rect)
			Render(got, build(func(b *gfx.Builder) {
				compositeBackdrop(b)
				b.PushLayer(1, outer)
				base(b)
				inner.draw(b, in)
				b.PopLayer()
			}), Options{})
			sameImage(t, fmt.Sprintf("%s in %s", in, outer), got, want)
		}
	}
}

// TestCompositeFilter checks that the pixels a filter spreads beyond an
// unbounded layer's content are composited, and the rest of the clip
// cleared.
func TestCompositeFilter(t *testing.T) {
	rect := image.Rect(0, 0, layerCaseSize, layerCaseSize)
	content := func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(30, 30, 24, 24), amber) }
	src := image.NewRGBA(rect)
	Render(src, build(func(b *gfx.Builder) {
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 4})
		content(b)
		b.PopLayer()
	}), Options{})
	back := image.NewRGBA(rect)
	Render(back, build(compositeBackdrop), Options{})
	for _, mode := range operators {
		want := image.NewRGBA(rect)
		for i := 0; i < len(want.Pix); i += 4 {
			r := refComposite(mode, [4]uint8(src.Pix[i:i+4]), [4]uint8(back.Pix[i:i+4]))
			copy(want.Pix[i:], r[:])
		}
		got := image.NewRGBA(rect)
		Render(got, build(func(b *gfx.Builder) {
			compositeBackdrop(b)
			b.PushLayer(1, mode, gfx.Blur{StdDev: 4})
			content(b)
			b.PopLayer()
		}), Options{})
		sameImage(t, mode.String(), got, want)
	}
}

// randomComposite is a random scene of layers composited with every blend
// mode, nested, clipped, masked and filtered, over shapes filled with
// colors and patterns.
func randomComposite(seed uint64, w, h int) scene {
	return scene{fmt.Sprintf("composite%d", seed), w, h, func(b *gfx.Builder) {
		rng := rand.New(rand.NewPCG(seed, 319))
		f := func(lo, hi float64) float32 { return float32(lo + float64(rng.Float64()*(hi-lo))) }
		W, H := float64(w), float64(h)
		rect := func() gfx.Rect { return gfx.RectXYWH(f(-20, W), f(-20, H), f(1, W/2), f(1, H/2)) }
		paint := func() gfx.Paint {
			if rng.IntN(3) == 0 {
				p := gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: gfx.PatternRepeat(rng.IntN(4)), Sampling: gfx.Sampling(rng.IntN(2)),
					Transform: gfx.Affine{f(-3, 3), f(-0.5, 0.5), f(-0.5, 0.5), f(-3, 3), f(0, W), f(0, H)}}
				switch rng.IntN(3) {
				case 0:
					p.Transform = gfx.Translate(float32(rng.IntN(w)), float32(rng.IntN(h)))
				case 1:
					p.Image, p.Transform = gfxtest.PatternBig, gfx.Scale(f(0.05, 0.6), f(0.05, 0.6))
				}
				return p
			}
			return gfx.Color{R: f(0, 1), G: f(0, 1), B: f(0, 1), A: f(0.2, 1)}
		}
		var block func(depth int)
		block = func(depth int) {
			for n := 2 + rng.IntN(4); n > 0; n-- {
				switch k := rng.IntN(8); {
				case k < 3 || depth == 0:
					switch rng.IntN(4) {
					case 0:
						b.FillRect(rect(), paint())
					case 1:
						b.FillRRect(gfx.NewRRect(rect(), f(0, 30)), paint())
					case 2:
						b.FillPath(starPath(f(0, W), f(0, H), f(5, 60)), gfx.FillRule(rng.IntN(2)), paint())
					default:
						b.StrokePath(tightCurve(f(0, W-70), f(0, H-40)), gfx.NewStrokeStyle(f(1, 12)), paint())
					}
				case k == 3:
					b.PushClipRRect(gfx.NewRRect(rect(), f(0, 30)))
					block(depth - 1)
					b.PopClip()
				case k == 4:
					b.PushTransform(gfx.RotateAbout(f(-1, 1), gfx.Pt(f(0, W), f(0, H))))
					block(depth - 1)
					b.PopTransform()
				default:
					mode := operators[rng.IntN(len(operators))]
					if rng.IntN(4) == 0 {
						mode = allBlendModes[rng.IntN(len(allBlendModes))]
					}
					op := f(0.1, 1)
					if rng.IntN(6) == 0 {
						op = 0
					}
					var fs []gfx.Filter
					if rng.IntN(5) == 0 {
						fs = append(fs, gfx.Blur{StdDev: f(0.5, 6)})
					}
					if rng.IntN(5) == 0 {
						fs = append(fs, gfx.DropShadow{Offset: gfx.Pt(f(-6, 6), f(-6, 6)), StdDev: f(0, 4), Color: gfx.Black.WithAlpha(0.5)})
					}
					if rng.IntN(4) == 0 {
						mr := rect()
						b.PushMaskLayer(gfx.Mask{List: build(func(mb *gfx.Builder) { mb.FillRRect(gfx.NewRRect(mr, 12), gfx.Black.WithAlpha(0.8)) })}, op, mode, fs...)
					} else {
						b.PushLayer(op, mode, fs...)
					}
					block(depth - 1)
					b.PopLayer()
				}
			}
		}
		b.FillRect(gfx.Rect{X1: float32(w), Y1: float32(h) * 0.7}, gfx.LinearGradient{End: gfx.Pt(float32(w), float32(h)), Stops: stops(sky, rose.WithAlpha(0.6))})
		block(4)
	}}
}
