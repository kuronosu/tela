package raster

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// textScenes draw synthetic text (font_test.go): small and large sizes,
// fractional origins, solid and gradient paints, color glyphs, a clip with
// an opacity layer, a rotation and a non-uniform scale (path fallback), and
// glyphs too large to be cached.
var textScenes = []scene{
	{"text", 320, 240, func(b *gfx.Builder) {
		f := testFontA
		run := func(s string, size, x, y float32, p gfx.Paint) { b.DrawGlyphRun(f.run(s, size, x, y, p)) }
		run("olAS iolS loo Ail", 14, 8.3, 20, slate)
		run("SolA il ? Soil", 9, 8, 34.4, slate)
		run("AoS lio", 20, 190.6, 24.2, indigo)
		run("SAlo*oil", 26, 8, 66, gfx.LinearGradient{Start: gfx.Pt(8, 46), End: gfx.Pt(140, 66), Stops: stops(indigo, fuchsia)})
		run("o*A*S", 24, 170, 66, rose)

		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(8, 76, 150, 40), 10))
		b.FillRect(gfx.RectXYWH(8, 76, 150, 40), twGray100)
		b.PushLayer(0.6, gfx.SrcOver)
		run("SoAlSoAl", 30, 2.7, 108, emerald)
		b.PopLayer()
		b.PopClip()

		b.PushTransform(gfx.RotateAbout(-0.3, gfx.Pt(240, 110)))
		run("AoS*l", 18, 196, 115, amber)
		b.PopTransform()

		b.PushTransform(gfx.Translate(170, 140).Mul(gfx.Scale(1.6, 1)))
		run("lioS", 16, 0, 0, sky)
		b.PopTransform()

		run("o", 110, 8, 232, slate.WithAlpha(0.8))
		run("*", 64, 76, 230, indigo)
		run("@", 40, 140, 230, rose)
		run("S", 150, 196, 236, gfx.RadialGradient{Center: gfx.Pt(240, 180), Radius: gfx.Pt(60, 60), Stops: stops(sky, indigo)})
	}},
}

func TestTextGoldens(t *testing.T) {
	for _, s := range textScenes {
		for _, scale := range []float32{1, 2} {
			name := fmt.Sprintf("%s_x%g", s.name, scale)
			t.Run(name, func(t *testing.T) {
				golden.Image(t, name, renderScene(s, scale))
			})
		}
	}
}

// renderWith renders dl onto a transparent w x h image with r.
func renderWith(r *Renderer, w, h int, dl *gfx.DisplayList, opts Options) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r.Render(img, dl, opts)
	return img
}

// sameImages fails if a and b differ.
func sameImages(t *testing.T, what string, a, b *image.RGBA) {
	t.Helper()
	if !bytes.Equal(a.Pix, b.Pix) {
		for i := 0; i < len(a.Pix); i += 4 {
			if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
				x, y := (i/4)%a.Rect.Dx(), (i/4)/a.Rect.Dx()
				t.Fatalf("%s: pixel (%d, %d) = %v, want %v", what, x, y, a.Pix[i:i+4], b.Pix[i:i+4])
			}
		}
	}
}

// anyInk reports whether img has a non-zero byte.
func anyInk(img *image.RGBA) bool {
	for _, v := range img.Pix {
		if v != 0 {
			return true
		}
	}
	return false
}

// pixDiff returns the largest channel difference between a and b and the
// number of differing pixels.
func pixDiff(a, b *image.RGBA) (int, int) {
	md, n := 0, 0
	for i := 0; i < len(a.Pix); i += 4 {
		d := 0
		for c := 0; c < 4; c++ {
			d = max(d, int(a.Pix[i+c])-int(b.Pix[i+c]), int(b.Pix[i+c])-int(a.Pix[i+c]))
		}
		if d > 0 {
			n++
		}
		md = max(md, d)
	}
	return md, n
}

// A glyph found in the cache gives exactly the pixels of a miss, and the
// cache saves the font calls.
func TestGlyphCacheHitMatchesMiss(t *testing.T) {
	s := textScenes[0]
	dl := build(s.draw)
	for _, scale := range []float32{1, 1.5, 2} {
		w, h := int(float32(s.w)*scale), int(float32(s.h)*scale)
		cache := NewGlyphCache(DefaultGlyphCacheBytes)
		r := NewRenderer()
		opts := Options{Scale: scale, Workers: 1, GlyphCache: cache}
		before := testFontA.outline.Load()
		miss := renderWith(r, w, h, dl, opts)
		n, calls := cache.Len(), testFontA.outline.Load()-before
		if n == 0 {
			t.Fatal("the cache is empty after a render")
		}
		before = testFontA.outline.Load()
		hit := renderWith(r, w, h, dl, opts)
		sameImages(t, fmt.Sprintf("x%g hit vs miss", scale), hit, miss)
		if cache.Len() != n {
			t.Errorf("x%g: %d entries after the second render, want %d", scale, cache.Len(), n)
		}
		// Only the glyphs drawn without the cache (rotated or too large)
		// call the font again.
		again := testFontA.outline.Load() - before
		if again >= calls {
			t.Errorf("x%g: second render made %d outline calls, not fewer than the first", scale, again)
		}
		// A render through a fresh Renderer and the default cache too.
		sameImages(t, fmt.Sprintf("x%g default cache", scale), renderWith(NewRenderer(), w, h, dl, Options{Scale: scale, Workers: 1}), miss)
	}
}

// Pen positions are rounded to a quarter device pixel in x and to a whole
// pixel in y: positions closer than 1/8 share a mask, positions a quarter
// apart do not.
func TestGlyphSubpixel(t *testing.T) {
	f := testFontA
	draw := func(x, y float32) (*image.RGBA, *GlyphCache) {
		cache := NewGlyphCache(1 << 20)
		// Whole-pixel advances keep both glyphs at the origin's offset.
		run := gfx.DrawGlyphRun{Font: f, Size: 14, Origin: gfx.Pt(x, y), Paint: slate,
			Glyphs: []gfx.Glyph{{ID: gO}, {ID: gS, X: 9}, {ID: gA, X: 17, Y: -1}}}
		dl := build(func(b *gfx.Builder) { b.DrawGlyphRun(run) })
		img := renderWith(NewRenderer(), 40, 30, dl, Options{Workers: 1, GlyphCache: cache})
		return img, cache
	}
	base, _ := draw(10, 20)
	for _, p := range [][2]float32{{10.1, 20}, {10.12, 20}, {9.876, 20}, {10, 20.4}, {10.1, 19.6}} {
		img, _ := draw(p[0], p[1])
		sameImages(t, fmt.Sprintf("pen %v vs (10, 20)", p), img, base)
	}
	for _, p := range [][2]float32{{10.25, 20}, {10.13, 20}, {9.87, 20}, {10, 20.6}} {
		img, _ := draw(p[0], p[1])
		if bytes.Equal(img.Pix, base.Pix) {
			t.Errorf("pen %v draws the same pixels as (10, 20)", p)
		}
	}
	// Positions a whole pixel apart share the masks: the shifted image.
	shifted, _ := draw(11, 21)
	for y := 0; y < 29; y++ {
		for x := 0; x < 39; x++ {
			if !bytes.Equal(shifted.Pix[shifted.PixOffset(x+1, y+1):][:4], base.Pix[base.PixOffset(x, y):][:4]) {
				t.Fatalf("pen (11, 21) is not pen (10, 20) moved by one pixel at (%d, %d)", x, y)
			}
		}
	}
	// One mask per glyph and quarter offset.
	cache := NewGlyphCache(1 << 20)
	dl := build(func(b *gfx.Builder) {
		for i, x := range []float32{3, 13.1, 23.24, 33.26, 43.5, 53.75, 63.9} {
			b.DrawGlyphRun(f.run("o", 14, x, float32(10+i)+0.3, slate))
		}
	})
	renderWith(NewRenderer(), 80, 30, dl, Options{Workers: 1, GlyphCache: cache})
	// Offsets 0, 0.1->0, 0.24->0.25, 0.26->0.25, 0.5, 0.75, 0.9->1=0.
	if n := cache.Len(); n != 4 {
		t.Errorf("%d cached masks, want 4 (quarter offsets 0, 1, 2, 3)", n)
	}
}

// snapPen returns the local pen position of the cached path at display
// scale s (a power of two, so that the division is exact): device x rounded
// to a quarter pixel, device y to a whole one.
func snapPen(x, y, s float32) (float32, float32) {
	X := math.Floor(float64(x)*float64(s)*4+0.5) / 4
	Y := math.Floor(float64(y)*float64(s) + 0.5)
	return float32(X / float64(s)), float32(Y / float64(s))
}

// gridRun returns run with every glyph moved to its snapped pen position
// (and the origin at 0), so that the cached path and the path fallback
// draw it at the same place.
func gridRun(run gfx.DrawGlyphRun, s float32) gfx.DrawGlyphRun {
	out := run
	out.Origin = gfx.Point{}
	out.Glyphs = make([]gfx.Glyph, len(run.Glyphs))
	for i, g := range run.Glyphs {
		x, y := snapPen(run.Origin.X+g.X, run.Origin.Y+g.Y, s)
		out.Glyphs[i] = gfx.Glyph{ID: g.ID, X: x, Y: y}
	}
	return out
}

// snapRun returns the glyph run drawn as FillPath commands with the pen
// positions of the cached path at display scale s.
func snapRun(b *gfx.Builder, run gfx.DrawGlyphRun, s float32) {
	for _, g := range run.Glyphs {
		p, ok := run.Font.GlyphOutline(g.ID)
		if !ok {
			continue
		}
		x, y := snapPen(run.Origin.X+g.X, run.Origin.Y+g.Y, s)
		b.PushTransform(gfx.Translate(x, y))
		b.PushTransform(gfx.Scale(run.Size, run.Size))
		b.FillPath(p, gfx.NonZero, run.Paint)
		b.PopTransform()
		b.PopTransform()
	}
}

// The cached path gives exactly the pixels of filling the outlines at the
// snapped pen positions, and of the path fallback when the positions are
// already snapped.
func TestGlyphCachedMatchesPaths(t *testing.T) {
	f := testFontA
	for _, s := range []float32{1, 2} {
		for _, onGrid := range []bool{true, false} {
			var runs []gfx.DrawGlyphRun
			for i, size := range []float32{7, 11, 14, 16, 23, 40, 64} {
				run := f.run("olAS iSo Al", size, 4.37, float32(12+i*28)+0.31, slate.WithAlpha(0.9))
				if onGrid {
					run = gridRun(run, s)
				}
				runs = append(runs, run)
			}
			dl := build(func(b *gfx.Builder) {
				for _, r := range runs {
					b.DrawGlyphRun(r)
				}
			})
			ref := build(func(b *gfx.Builder) {
				for _, r := range runs {
					snapRun(b, r, s)
				}
			})
			w, h := int(420*s), int(220*s)
			opts := Options{Scale: s, Workers: 1, GlyphCache: NewGlyphCache(8 << 20)}
			got := renderWith(NewRenderer(), w, h, dl, opts)
			sameImages(t, fmt.Sprintf("x%g on grid %v: cached vs snapped paths", s, onGrid), got, renderWith(NewRenderer(), w, h, ref, opts))
			if onGrid {
				pr := NewRenderer()
				pr.tune.glyphPaths = true
				sameImages(t, fmt.Sprintf("x%g: cached vs path fallback", s), got, renderWith(pr, w, h, dl, opts))
			}
		}
	}
}

// Under a rotation (and a skew and a flip) glyphs are filled as paths under
// the glyph's transform: exactly the FillPath commands nested in the same
// transforms.
func TestGlyphRotationFallback(t *testing.T) {
	f := testFontA
	for _, m := range []gfx.Affine{
		gfx.RotateAbout(0.4, gfx.Pt(60, 40)),
		gfx.Translate(10, 0).Mul(gfx.Skew(0.3, 0)),
		gfx.Translate(120, 0).Mul(gfx.Scale(-1, 1)),
	} {
		run := f.run("SoA li", 18, 8.3, 50.7, indigo)
		dl := build(func(b *gfx.Builder) {
			b.PushTransform(m)
			b.DrawGlyphRun(run)
			b.PopTransform()
		})
		ref := build(func(b *gfx.Builder) {
			b.PushTransform(m)
			for _, g := range run.Glyphs {
				if p, ok := f.GlyphOutline(g.ID); ok {
					b.PushTransform(gfx.Translate(run.Origin.X+g.X, run.Origin.Y+g.Y))
					b.PushTransform(gfx.Scale(run.Size, run.Size))
					b.FillPath(p, gfx.NonZero, run.Paint)
					b.PopTransform()
					b.PopTransform()
				}
			}
			b.PopTransform()
		})
		cache := NewGlyphCache(1 << 20)
		opts := Options{Scale: 1.5, Workers: 1, GlyphCache: cache}
		sameImages(t, fmt.Sprintf("transform %v", m), renderWith(NewRenderer(), 200, 120, dl, opts), renderWith(NewRenderer(), 200, 120, ref, opts))
		if cache.Len() != 0 {
			t.Errorf("transform %v: %d cached glyphs, want 0", m, cache.Len())
		}
	}
}

// Glyphs too large for the cache are filled as paths and not cached.
func TestGlyphHugeFallback(t *testing.T) {
	f := testFontA
	run := f.run("oS", 300, 10.3, 280.6, rose)
	dl := build(func(b *gfx.Builder) { b.DrawGlyphRun(run) })
	cache := NewGlyphCache(64 << 20)
	got := renderWith(NewRenderer(), 400, 300, dl, Options{Workers: 1, GlyphCache: cache})
	if cache.Len() != 0 {
		t.Errorf("%d cached glyphs at 300 px, want 0", cache.Len())
	}
	pr := NewRenderer()
	pr.tune.glyphPaths = true
	sameImages(t, "huge glyphs", got, renderWith(pr, 400, 300, dl, Options{Workers: 1}))
	if !anyInk(got) {
		t.Error("nothing drawn")
	}
}

// Text inside a rounded clip and an opacity layer composites like the
// equivalent paths.
func TestGlyphClipLayer(t *testing.T) {
	f := testFontA
	run := f.run("SoAl*iS", 22, 6.5, 40, emerald)
	scene := func(text func(b *gfx.Builder)) *gfx.DisplayList {
		return build(func(b *gfx.Builder) {
			b.FillRect(gfx.Rect{X1: 200, Y1: 60}, twGray100)
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(12.5, 14.5, 120, 34), 12))
			b.PushLayer(0.55, gfx.SrcOver)
			text(b)
			b.PopLayer()
			b.PopClip()
		})
	}
	// The reference has no color glyph: compare it with a run without it.
	noColor := run
	noColor.Glyphs = nil
	for _, g := range run.Glyphs {
		if g.ID != gBadge {
			noColor.Glyphs = append(noColor.Glyphs, g)
		}
	}
	for _, s := range []float32{1, 2} {
		w, h := int(200*s), int(60*s)
		opts := Options{Scale: s, Workers: 1, GlyphCache: NewGlyphCache(1 << 20)}
		got := renderWith(NewRenderer(), w, h, scene(func(b *gfx.Builder) { b.DrawGlyphRun(noColor) }), opts)
		want := renderWith(NewRenderer(), w, h, scene(func(b *gfx.Builder) { snapRun(b, noColor, s) }), opts)
		sameImages(t, fmt.Sprintf("x%g clip and layer", s), got, want)
		// With the color glyph, at snapped positions, the cache and the
		// fallback differ only by the rounding of the sprite.
		grid := scene(func(b *gfx.Builder) { b.DrawGlyphRun(gridRun(run, s)) })
		full := renderWith(NewRenderer(), w, h, grid, opts)
		pr := NewRenderer()
		pr.tune.glyphPaths = true
		if d, n := pixDiff(full, renderWith(pr, w, h, grid, opts)); d > 2 {
			t.Errorf("x%g: cached and fallback differ by up to %d (%d pixels)", s, d, n)
		}
	}
}

// A gradient paint is sampled at the device pixel centers: every pixel is
// the gradient's color times the glyph coverage.
func TestGlyphGradientPaint(t *testing.T) {
	f := testFontA
	grad := gfx.LinearGradient{Start: gfx.Pt(5, 10), End: gfx.Pt(170, 50), Stops: stops(indigo, fuchsia.WithAlpha(0.6), amber)}
	area := gfx.Rect{X1: 180, Y1: 60}
	for _, s := range []float32{1, 1.5} {
		for _, paths := range []bool{false, true} {
			w, h := int(180*s), int(60*s)
			r := func() *Renderer {
				r := NewRenderer()
				r.tune.glyphPaths = paths
				return r
			}
			opts := Options{Scale: s, Workers: 1, GlyphCache: NewGlyphCache(1 << 20)}
			text := func(p gfx.Paint) *gfx.DisplayList {
				return build(func(b *gfx.Builder) { b.DrawGlyphRun(f.run("SAlo iSoA", 30, 4.6, 40.3, p)) })
			}
			got := renderWith(r(), w, h, text(grad), opts)
			cov := renderWith(r(), w, h, text(gfx.White), opts)
			g := renderWith(r(), w, h, build(func(b *gfx.Builder) { b.FillRect(area, grad) }), opts)
			drawn := 0
			for i := 0; i < len(got.Pix); i += 4 {
				c := uint32(cov.Pix[i+3])
				drawn += int(c)
				for k := 0; k < 4; k++ {
					want := uint8(div65025(uint32(g.Pix[i+k]) * c * 255))
					if got.Pix[i+k] != want {
						t.Fatalf("x%g paths %v: pixel %d channel %d = %d, want %d (gradient %v, coverage %d)",
							s, paths, i/4, k, got.Pix[i+k], want, g.Pix[i:i+4], c)
					}
				}
			}
			if drawn == 0 {
				t.Fatal("no coverage")
			}
		}
	}
}

// Color glyphs paint their own colors with the run's color as foreground,
// as sprites in the cache and inline under other transforms.
func TestColorGlyph(t *testing.T) {
	f := testFontA
	at := func(fg gfx.Paint, paths bool, cache *GlyphCache, m gfx.Affine) *image.RGBA {
		dl := build(func(b *gfx.Builder) {
			b.PushTransform(m)
			// Whole-pixel advances: every glyph at a quarter offset.
			b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: 32, Origin: gfx.Pt(6.25, 40), Paint: fg,
				Glyphs: []gfx.Glyph{{ID: gBadge}, {ID: gO, X: 26}, {ID: gBadge, X: 46}}})
			b.PopTransform()
		})
		r := NewRenderer()
		r.tune.glyphPaths = paths
		return renderWith(r, 160, 60, dl, Options{Scale: 1, Workers: 1, GlyphCache: cache})
	}
	cache := NewGlyphCache(1 << 20)
	redImg := at(red, false, cache, gfx.Identity)
	// Two color glyphs at the same offset share a sprite; the outline
	// glyph has a mask: 1 mask + 1 color marker + 1 sprite.
	if n := cache.Len(); n != 3 {
		t.Errorf("%d cache entries, want 3", n)
	}
	blueImg := at(blue, false, cache, gfx.Identity)
	if n := cache.Len(); n != 4 {
		t.Errorf("%d cache entries after a second foreground, want 4", n)
	}
	if bytes.Equal(redImg.Pix, blueImg.Pix) {
		t.Error("the foreground color does not change the color glyph")
	}
	// The sprite and the inline drawing differ only by rounding.
	if d, n := pixDiff(redImg, at(red, true, nil, gfx.Identity)); d > 2 {
		t.Errorf("sprite and inline color glyph differ by up to %d (%d pixels)", d, n)
	}
	// A transparent paint hides the outline glyphs and the color ones.
	clearImg := at(gfx.Transparent, false, NewGlyphCache(1<<20), gfx.Identity)
	clearPaths := at(gfx.Transparent, true, nil, gfx.Identity)
	if d, _ := pixDiff(clearImg, clearPaths); d > 2 {
		t.Errorf("transparent paint: sprite and inline differ by %d", d)
	}
	var inO, inBadge int
	for y := 0; y < 60; y++ {
		for x := 0; x < 160; x++ {
			a := int(clearImg.Pix[clearImg.PixOffset(x, y)+3])
			if x >= 32 && x < 52 {
				inO += a // the 'o' lies between the badges
			} else {
				inBadge += a
			}
		}
	}
	if inO != 0 || inBadge != 0 {
		t.Errorf("transparent paint: alpha %d in the outline glyph, %d in the color glyphs (want 0: Chromium hides both)", inO, inBadge)
	}
	// A translucent paint fades the color glyphs (sprites) by its alpha.
	half := at(gfx.Color{R: 1, A: 0.5}, false, NewGlyphCache(1<<20), gfx.Identity)
	var full, faded int
	for y := 0; y < 60; y++ {
		for x := 0; x < 32; x++ {
			full += int(redImg.Pix[redImg.PixOffset(x, y)+3])
			faded += int(half.Pix[half.PixOffset(x, y)+3])
		}
	}
	if full == 0 || faded < full*45/100 || faded > full*55/100 {
		t.Errorf("translucent paint: color glyph alpha %d, opaque %d (want half)", faded, full)
	}
	// Rotated: inline, close to the reference of the same commands.
	rot := gfx.RotateAbout(0.5, gfx.Pt(80, 30))
	if d, n := pixDiff(at(red, false, nil, rot), at(red, true, nil, rot)); d != 0 {
		t.Errorf("rotated color glyph depends on the cache: %d (%d pixels)", d, n)
	}
}

// A color glyph too large for a sprite is drawn inline, and the cache
// remembers it: later frames call ColorGlyph once per glyph, for the inline
// drawing only.
func TestColorGlyphTooLargeForSprite(t *testing.T) {
	const gWide gfx.GlyphID = 100
	f := newTestFont("Wide")
	f.glyphs[gWide] = &testGlyph{bounds: gfx.Rect{X0: 0, Y0: -0.8, X1: 0.8, Y1: 0}, adv: 1,
		color: func(_ *testFont, fg gfx.Color) *gfx.DisplayList {
			b := gfx.NewBuilder()
			b.FillRect(gfx.Rect{X0: -2, Y0: -0.8, X1: 3, Y1: 0}, fg)
			return b.Finish()
		}}
	dl := build(func(b *gfx.Builder) {
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: 250, Origin: gfx.Pt(100.25, 220), Paint: red,
			Glyphs: []gfx.Glyph{{ID: gWide}}})
	})
	cache := NewGlyphCache(1 << 20)
	render := func(paths bool) *image.RGBA {
		r := NewRenderer()
		r.tune.glyphPaths = paths
		return renderWith(r, 300, 240, dl, Options{Scale: 1, Workers: 1, GlyphCache: cache})
	}
	first := render(false)
	before := f.colors.Load()
	second := render(false)
	if n := f.colors.Load() - before; n != 1 {
		t.Errorf("%d ColorGlyph calls in a cached frame, want 1", n)
	}
	sameImages(t, "second frame", second, first)
	sameImages(t, "inline reference", first, render(true))
	if !anyInk(first) {
		t.Error("nothing drawn")
	}
}

// A color glyph that draws itself stops at the nesting limit.
func TestColorGlyphNesting(t *testing.T) {
	f := testFontA
	dl := build(func(b *gfx.Builder) { b.DrawGlyphRun(f.run("@", 80, 4, 76, rose)) })
	for _, paths := range []bool{false, true} {
		r := NewRenderer()
		r.tune.glyphPaths = paths
		img := renderWith(r, 80, 80, dl, Options{Workers: 1, GlyphCache: NewGlyphCache(1 << 20)})
		if !anyInk(img) {
			t.Errorf("paths %v: nothing drawn", paths)
		}
	}
}

// A tiny budget evicts glyphs all the time without changing the pixels, and
// the cache stays within it.
func TestGlyphCacheEviction(t *testing.T) {
	s := textScenes[0]
	dl := build(s.draw)
	want := renderScene(s, 2)
	for _, budget := range []int64{0, 16 * 600, 64 << 10} {
		cache := NewGlyphCache(budget)
		for k := 0; k < 2; k++ {
			img := image.NewRGBA(want.Rect)
			Clear(img, image.Rectangle{}, gfx.White)
			NewRenderer().Render(img, dl, Options{Scale: 2, Workers: 1, GlyphCache: cache})
			sameImages(t, fmt.Sprintf("budget %d, pass %d", budget, k), img, want)
			if cache.Bytes() > budget {
				t.Errorf("budget %d: the cache holds %d bytes", budget, cache.Bytes())
			}
		}
		if budget == 64<<10 && cache.Len() == 0 {
			t.Errorf("budget %d: nothing cached", budget)
		}
	}
	c := NewGlyphCache(1 << 20)
	NewRenderer().Render(image.NewRGBA(want.Rect), dl, Options{Scale: 2, Workers: 1, GlyphCache: c})
	if c.Len() == 0 || c.Bytes() == 0 {
		t.Fatal("nothing cached")
	}
	c.Reset()
	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("after Reset: %d entries, %d bytes", c.Len(), c.Bytes())
	}
}

// Parallel renders that share a cache, starting cold, give the pixels of a
// sequential render.
func TestGlyphCacheConcurrent(t *testing.T) {
	s := textScenes[0]
	dl := build(s.draw)
	want := renderScene(s, 1.5)
	cache := NewGlyphCache(DefaultGlyphCacheBytes)
	var wg sync.WaitGroup
	imgs := make([]*image.RGBA, 6)
	for i := range imgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			img := image.NewRGBA(want.Rect)
			Clear(img, image.Rectangle{}, gfx.White)
			r := NewRenderer()
			r.tune.minPixels = 1
			r.tune.bandRows = 1 + i*3
			r.Render(img, dl, Options{Scale: 1.5, Workers: 2 + i, GlyphCache: cache})
			imgs[i] = img
		}()
	}
	wg.Wait()
	for i, img := range imgs {
		sameImages(t, fmt.Sprintf("goroutine %d", i), img, want)
	}
}
