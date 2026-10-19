package raster

import (
	"bytes"
	"fmt"
	"image"
	"runtime"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// multiBackdrop draws backdrop filters that read each other's output (under
// clips, a scale and a rotation, with a drop shadow and a per-pixel filter
// among them) over stripes and a gradient.
func multiBackdrop(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: 300, Y1: 240}, gfx.LinearGradient{End: gfx.Pt(300, 240), Stops: stops(indigo, rose, amber)})
	for x := float32(0); x < 300; x += 24 {
		b.FillRect(gfx.RectXYWH(x, 0, 9, 240), gfx.White.WithAlpha(0.7))
	}
	b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 200, 150), 30))
	a := gfx.NewRRect(gfx.RectXYWH(20, 30, 150, 110), 14)
	b.BackdropFilter(a, gfx.Blur{StdDev: 6})
	b.FillRRect(a, gfx.White.WithAlpha(0.2))
	b.PopClip()
	b.PushTransform(gfx.Translate(60, 70).Mul(gfx.Scale(1.2, 0.9)))
	b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(0, 0, 120, 100), 10), gfx.Blur{StdDev: 3}, gfx.Brightness(1.2))
	b.PopTransform()
	b.FillRect(gfx.RectXYWH(0, 150, 300, 12), slate)
	b.PushTransform(gfx.RotateAbout(0.2, gfx.Pt(200, 160)))
	b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(140, 110, 140, 100), 20),
		gfx.DropShadow{Offset: gfx.Pt(5, -4), StdDev: 4, Color: gfx.Black.WithAlpha(0.6)}, gfx.Blur{StdDev: 22})
	b.PopTransform()
	b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(100, 0, 60, 240), 0), gfx.Grayscale(1))
	b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(0, 200, 300, 40), 0), gfx.Blur{StdDev: 2})
}

// fadedBackdrop is multiBackdrop with backdrop filters painted at an
// opacity below 1 (ADR-084), and one at opacity 0, which is dropped.
func fadedBackdrop(b *gfx.Builder) {
	multiBackdrop(b)
	b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(30, 40, 180, 120), 16), 0.5, gfx.Blur{StdDev: 8})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, 40, 180, 120), 16), gfx.White.WithAlpha(0.1))
	b.PushTransform(gfx.RotateAbout(-0.15, gfx.Pt(150, 120)))
	b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(120, 90, 150, 90), 10), 0.25, gfx.Blur{StdDev: 3}, gfx.Saturate(1.5))
	b.PopTransform()
	b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(0, 0, 300, 240), 0), 0, gfx.Blur{StdDev: 30})
	b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(200, 0, 60, 240), 0), 0.7, gfx.Invert(1))
}

// TestPhasedMatchesSequential checks that a phased render (bands that wait
// at backdrop filters) gives the pixels of a sequential render, for several
// worker counts, the full frame and regions, and that lists with a
// spreading backdrop filter inside a layer are not phased.
func TestPhasedMatchesSequential(t *testing.T) {
	cases := []struct {
		name  string
		w, h  int
		dl    *gfx.DisplayList
		scale float32
	}{
		{"backdrop", 200, 100, build(backdropScene), 1},
		{"multi", 300, 240, build(multiBackdrop), 1},
		{"multi_x2", 600, 480, build(multiBackdrop), 2},
		{"multi_x1.5", 450, 360, build(multiBackdrop), 1.5},
		{"faded", 300, 240, build(fadedBackdrop), 1},
		{"faded_x1.5", 450, 360, build(fadedBackdrop), 1.5},
		{"blur_presets", 1400, 600, build(drawBlurScene), 1},
		{"glass", 1920, 1080, glassScene(1), 1},
	}
	for _, c := range cases {
		bfs, ok := phaseBarriers(c.dl)
		if !ok || len(bfs) == 0 {
			t.Fatalf("%s: not phased (%v, %v)", c.name, bfs, ok)
		}
		bounds := image.Rect(0, 0, c.w, c.h)
		bg := image.NewRGBA(bounds)
		background(bg)
		want := cloneRGBA(bg)
		NewRenderer().Render(want, c.dl, Options{Scale: c.scale, Workers: 1})
		for _, region := range []image.Rectangle{{}, image.Rect(c.w/5, c.h/3, c.w*3/5, c.h*4/5), image.Rect(0, c.h/2, c.w, c.h/2+7)} {
			full := want
			if !region.Empty() {
				full = cloneRGBA(bg)
				NewRenderer().Render(full, c.dl, Options{Scale: c.scale, Workers: 1, Region: region})
			}
			for _, workers := range []int{2, 3, 5, 8} {
				r := NewRenderer()
				r.tune.minPixels = 1
				got := cloneRGBA(bg)
				r.Render(got, c.dl, Options{Scale: c.scale, Workers: workers, Region: region})
				if !bytes.Equal(got.Pix, full.Pix) {
					t.Fatalf("%s region %v, %d workers: the phased render differs from the sequential one", c.name, region, workers)
				}
				if r.par == nil || r.par.comp == nil {
					t.Fatalf("%s region %v, %d workers: not rendered in phases", c.name, region, workers)
				}
			}
		}
	}
	// A blur behind a layer: the list is not phased (and still exact).
	inLayer := build(func(b *gfx.Builder) {
		backdropScene(b)
		b.PushLayer(0.8, gfx.SrcOver)
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(10, 10, 60, 60), 8), gfx.Blur{StdDev: 3})
		b.PopLayer()
	})
	if _, ok := phaseBarriers(inLayer); ok {
		t.Errorf("a backdrop filter inside a layer: phased")
	}
}

// TestPhasedPanic checks that a panic in a band of a phased render, before,
// between or after barriers, reaches the caller with its value without
// leaving bands waiting, and that the Renderer keeps working.
func TestPhasedPanic(t *testing.T) {
	const msg = "raster test: color glyph panics"
	pf := newTestFont("Panicking")
	pf.glyphs[gBadge].color = func(*testFont, gfx.Color) *gfx.DisplayList { panic(msg) }
	at := []func(b *gfx.Builder){
		func(b *gfx.Builder) { b.DrawGlyphRun(pf.run("*", 16, 20, 230, gfx.Black)) },
		func(b *gfx.Builder) { b.DrawGlyphRun(pf.run("*", 16, 250, 20, gfx.Black)) },
	}
	good := build(multiBackdrop)
	want := image.NewRGBA(image.Rect(0, 0, 300, 240))
	NewRenderer().Render(want, good, Options{Workers: 1})
	before := runtime.NumGoroutine()
	r := NewRenderer()
	r.tune.minPixels = 1
	for k := 0; k < 24; k++ {
		where := k % 3 // the panic before the first barrier, between barriers, after the last
		bad := build(func(b *gfx.Builder) {
			if where == 0 {
				at[k%2](b)
			}
			multiBackdrop(b)
			if where == 1 {
				at[k%2](b)
			}
			b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(30, 30, 200, 150), 6), gfx.Blur{StdDev: 5})
			if where == 2 {
				at[k%2](b)
			}
		})
		t.Run(fmt.Sprint(k), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 300, 240))
			func() {
				defer func() {
					if v := recover(); v != msg {
						t.Fatalf("panic %#v, want %q", v, msg)
					}
				}()
				r.Render(img, bad, Options{Workers: 2 + k%5})
				t.Fatalf("no panic")
			}()
			got := image.NewRGBA(image.Rect(0, 0, 300, 240))
			r.Render(got, good, Options{Workers: 2 + k%5})
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("render after a panic differs")
			}
		})
	}
	waitGoroutines(t, before)
}
