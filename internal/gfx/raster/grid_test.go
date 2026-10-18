package raster

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestGridStep(t *testing.T) {
	cases := []struct {
		s, t float32
		want float32
		ok   bool
	}{
		{1.5, float32(80.0 / 1.5), 80, true}, // 53.333332: 79.999998 device px
		{1.25, float32(-37.0 / 1.25), -37, true},
		{1.75, float32(1001.0 / 1.75), 1001, true},
		{1.5, float32(200000.0 / 1.5), 200000, true}, // a long list: float32 error grows with the offset
		{2, 13.5, 27, true},                          // integer scales: exact
		{1.5, 0.3, 0, false},                         // a real sub-pixel translation
		{1.5, 10.0 / 3 * 1.001, 5, false},
		{1, 0.0005, 0, true}, // within 1/1024 px
		{1, 0.002, 0, false},
	}
	for _, c := range cases {
		got, ok := GridStep(c.s, c.t)
		if ok != c.ok || ok && got != c.want {
			t.Errorf("GridStep(%g, %g) = %g, %v; want %g, %v", c.s, c.t, got, ok, c.want, c.ok)
		}
	}
	if tol := GridTolerance(1e9); tol != 1.0/8 {
		t.Errorf("GridTolerance(1e9) = %g, want the 1/8 cap", tol)
	}
}

func TestGridMul(t *testing.T) {
	s := float32(1.5)
	root := gfx.Scale(s, s)
	tr := gfx.Translate(float32(80.0/1.5), float32(-40.0/1.5))
	if got := GridMul(root, tr); got[4] != 80 || got[5] != -40 {
		t.Errorf("GridMul(scale 1.5, whole-pixel translation) = %v, want a translation of (80, -40)", got)
	}
	// Nested: the current transform stays on the grid.
	inner := GridMul(GridMul(root, tr), gfx.Translate(float32(7.0/1.5), 0))
	if inner[4] != 87 {
		t.Errorf("nested translation x = %v, want 87", inner[4])
	}
	// A sub-pixel translation, or one under a transform off the grid or
	// rotated, is the plain product.
	for _, c := range []struct{ ctm, m gfx.Affine }{
		{root, gfx.Translate(0.3, 0.3)},
		{root.Mul(gfx.Translate(0.2, 0)), tr},
		{root.Mul(gfx.Rotate(0.5)), tr},
		{root, gfx.Scale(2, 2)},
	} {
		if got, want := GridMul(c.ctm, c.m), c.ctm.Mul(c.m); got != want {
			t.Errorf("GridMul(%v, %v) = %v, want the plain product %v", c.ctm, c.m, got, want)
		}
	}
}

// TestGridTranslationShifts checks the property core's damage relies on:
// content under a whole-pixel translation at a fractional scale renders as
// the same content under another one, shifted by the difference. Without
// GridMul some offsets compose to a hair off the grid, and a vertex near a
// step of the 1/256 px fixed-point grid lands on its other side.
func TestGridTranslationShifts(t *testing.T) {
	const w, h = 64, 48
	// half returns a logical y that lands exactly halfway between two
	// steps of the fixed-point grid at scale s (k + 1/512 device px, an
	// exact binary fraction): the vertices most sensitive to a translation
	// off the grid.
	half := func(s float32) float32 {
		switch s {
		case 1.25:
			return 205.0 / 128 // 2 + 1/512 device px
		case 1.5:
			return 684.0 / 1024 // 1 + 1/512
		}
		return 439.0 / 128 // 1.75: 6 + 1/512
	}
	var s float32
	scene := func(dy float32) *gfx.DisplayList {
		b := gfx.NewBuilder()
		b.PushTransform(gfx.Translate(0, dy))
		b.FillRRect(gfx.RRect{Rect: gfx.Rect{X0: 3.3, Y0: 4.7, X1: 40.1, Y1: 27.2},
			Radii: [4]gfx.Point{{X: 6, Y: 6}, {X: 6, Y: 6}, {X: 9, Y: 9}, {X: 3, Y: 3}}}, gfx.RGB8(200, 30, 90))
		var p gfx.Path
		p.MoveTo(gfx.Pt(5.17, 6.31))
		p.CubicTo(gfx.Pt(20.4, 1.9), gfx.Pt(31.7, 29.3), gfx.Pt(38.9, 12.6))
		p.LineTo(gfx.Pt(12.2, 26.95))
		p.Close()
		b.FillPath(p, gfx.NonZero, gfx.RGB8(10, 120, 200))
		var r gfx.Path
		y := half(s)
		r.MoveTo(gfx.Pt(2, y))
		r.LineTo(gfx.Pt(30, y))
		r.LineTo(gfx.Pt(30, y+20))
		r.LineTo(gfx.Pt(2, y+20))
		r.Close()
		b.FillPath(r, gfx.NonZero, gfx.RGB8(30, 30, 30))
		b.PopTransform()
		return b.Finish()
	}
	render := func(dl *gfx.DisplayList, s float32, dev int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, dev, w, dev+h)) // the content's rows
		Clear(img, image.Rectangle{}, gfx.White)
		NewRenderer().Render(img, dl, Options{Scale: s, Workers: 1})
		return img
	}
	for _, s = range []float32{1.25, 1.5, 1.75} {
		base := render(scene(0), s, 0)
		// Every offset: without GridMul, 140 of these compose off the
		// grid at 1.75 (from about ±1000 px); at 1.25 and 1.5 a single
		// translation in this range rounds back to whole pixels, but
		// nested ones (a scroller in a scroller) add their errors.
		for dev := -1500; dev <= 1500; dev++ {
			got := render(scene(float32(float64(dev)/float64(s))), s, dev)
			for y := range h {
				for x := range w {
					if a, b := base.RGBAAt(x, y), got.RGBAAt(x, y+dev); a != b {
						t.Fatalf("scale %g, offset %d device px: pixel (%d, %d) = %v, unshifted %v", s, dev, x, y, b, a)
					}
				}
			}
		}
	}
}
