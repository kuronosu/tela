package raster

import (
	"image"
	"image/color"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func pixel(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

// blendExact computes round(s*c/255 + d*(1 - sa*c/255²)) with rationals.
func blendExact(s, sa, d, c int64) uint8 {
	v := new(big.Rat).SetFrac64(s*c*255+d*(65025-sa*c), 65025)
	f, _ := v.Float64()
	return uint8(math.Floor(f + 0.5))
}

func TestBlendPixelExact(t *testing.T) {
	for _, sa := range []int64{0, 1, 77, 128, 254, 255} {
		for s := int64(0); s <= sa; s += max(1, sa/7) {
			for _, c := range []int64{0, 1, 64, 127, 128, 200, 254, 255} {
				for d := int64(0); d <= 255; d += 15 {
					px := []uint8{uint8(d), uint8(d), uint8(d), 255}
					blendPixel(px, uint32(s), uint32(s), uint32(s), uint32(sa), uint32(c))
					want := blendExact(s, sa, d, c)
					wantA := blendExact(sa, sa, 255, c)
					if px[0] != want || px[3] != wantA {
						t.Fatalf("s=%d sa=%d d=%d c=%d: got %v, want %d (alpha %d)", s, sa, d, c, px, want, wantA)
					}
				}
			}
		}
	}
}

// run, mask and the generic shader path must give the same pixels.
type testShader struct{ c [4]uint8 }

func (s *testShader) shadeSpan(x, y int, dst []uint8) {
	for i := 0; i < len(dst); i += 4 {
		copy(dst[i:i+4], s.c[:])
	}
}
func (s *testShader) opaque() bool { return s.c[3] == 255 }

func TestBlitterPathsAgree(t *testing.T) {
	for _, c := range [][4]uint8{{200, 100, 50, 255}, {60, 20, 90, 128}, {0, 0, 0, 0}} {
		for _, cov := range []uint8{0, 1, 128, 255} {
			var out [3][4]uint8
			for k := 0; k < 3; k++ {
				img := image.NewRGBA(image.Rect(0, 0, 2, 1))
				img.Pix = []uint8{10, 20, 30, 40, 10, 20, 30, 40}
				b := blitter{pix: img.Pix, stride: img.Stride, clip: img.Rect}
				switch k {
				case 0:
					b.setShader(&solidShader{c})
					b.run(0, 1, 1, cov)
				case 1:
					b.setShader(&solidShader{c})
					b.mask(0, 1, []uint8{cov})
				case 2:
					b.setShader(&testShader{c})
					b.run(0, 1, 1, cov)
					img2 := []uint8{10, 20, 30, 40, 10, 20, 30, 40}
					b2 := blitter{pix: img2, stride: 8, clip: img.Rect}
					b2.setShader(&testShader{c})
					b2.mask(0, 1, []uint8{cov})
					if string(img2) != string(img.Pix) {
						t.Fatalf("generic run and mask differ: %v %v", img.Pix, img2)
					}
				}
				if string(img.Pix[:4]) != "\x0a\x14\x1e\x28" {
					t.Fatal("pixel outside the span was written")
				}
				copy(out[k][:], img.Pix[4:])
			}
			if out[0] != out[1] || out[0] != out[2] {
				t.Errorf("color %v coverage %d: run %v, mask %v, generic %v", c, cov, out[0], out[1], out[2])
			}
		}
	}
}

func TestCompositing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	Clear(img, image.Rectangle{}, gfx.White)
	Render(img, build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X0: 0, Y0: 0, X1: 4, Y1: 2}, gfx.Black.WithAlpha(0.5))
		b.FillRect(gfx.Rect{X0: 0, Y0: 2, X1: 2, Y1: 4}, gfx.RGB8(10, 200, 30))
		b.FillRect(gfx.Rect{X0: 2, Y0: 2, X1: 2.5, Y1: 4}, gfx.RGB8(255, 0, 0)) // half coverage
	}), Options{})
	if got := pixel(img, 0, 0); got != (color.RGBA{127, 127, 127, 255}) {
		// Premul8 gives alpha round(127.5) = 128: 255 - 128 = 127.
		t.Errorf("50%% black over white = %v", got)
	}
	if got := pixel(img, 1, 3); got != (color.RGBA{10, 200, 30, 255}) {
		t.Errorf("opaque fill = %v, want an exact copy", got)
	}
	if got := pixel(img, 2, 2); got != (color.RGBA{255, 127, 127, 255}) {
		t.Errorf("red at half coverage over white = %v", got)
	}
	if got := pixel(img, 3, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("uncovered pixel = %v", got)
	}

	// Onto transparent: the result is the premultiplied source.
	img = image.NewRGBA(image.Rect(0, 0, 1, 1))
	c := gfx.Color{R: 0.2, G: 0.6, B: 1, A: 0.4}
	Render(img, build(func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 1, Y1: 1}, c) }), Options{})
	if got := pixel(img, 0, 0); got != c.Premul8() {
		t.Errorf("onto transparent = %v, want %v", got, c.Premul8())
	}
}

// A pixel half covered by a vertical edge, by a horizontal edge, by a
// diagonal, and the same from a path, at scale 2.
func TestCoverageValues(t *testing.T) {
	img := coverageImage(8, 8, 2, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X0: 0.25, Y0: 0, X1: 1, Y1: 1}, gfx.Black) // device x 0.5..2
		var tri gfx.Path                                               // device (4,0) (8,0) (8,4): diagonal through pixels
		tri.MoveTo(gfx.Pt(2, 0))
		tri.LineTo(gfx.Pt(4, 0))
		tri.LineTo(gfx.Pt(4, 2))
		tri.Close()
		b.FillPath(tri, gfx.NonZero, gfx.Black)
	})
	a := func(x, y int) uint8 { return img.Pix[img.PixOffset(x, y)+3] }
	if a(0, 0) != 128 || a(1, 0) != 255 || a(2, 0) != 0 {
		t.Errorf("rect edge coverage = %d %d %d, want 128 255 0", a(0, 0), a(1, 0), a(2, 0))
	}
	// Diagonal from (4,0) to (8,4): pixels on it are half covered, those
	// above-right fully, those below-left not at all.
	for i := 0; i < 4; i++ {
		if got := a(4+i, i); got != 128 {
			t.Errorf("diagonal pixel (%d, %d) = %d, want 128", 4+i, i, got)
		}
		if i < 3 {
			if got := a(5+i, i); got != 255 {
				t.Errorf("pixel (%d, %d) above the diagonal = %d, want 255", 5+i, i, got)
			}
		}
		if got := a(3+i, i); got != 0 {
			t.Errorf("pixel (%d, %d) below the diagonal = %d, want 0", 3+i, i, got)
		}
	}
}

func TestFillRules(t *testing.T) {
	// Two overlapping squares wound the same way: winding 2 in the overlap.
	var p gfx.Path
	p.AddRect(gfx.Rect{X0: 0, Y0: 0, X1: 3, Y1: 2})
	p.AddRect(gfx.Rect{X0: 1, Y0: 0, X1: 4, Y1: 2})
	// A square wound the other way inside: winding 0 under NonZero.
	var q gfx.Path
	q.AddRect(gfx.Rect{X0: 0, Y0: 2, X1: 4, Y1: 4})
	q.MoveTo(gfx.Pt(1, 2))
	q.LineTo(gfx.Pt(1, 4))
	q.LineTo(gfx.Pt(3, 4))
	q.LineTo(gfx.Pt(3, 2))
	q.Close()
	for _, tc := range []struct {
		rule gfx.FillRule
		want [2][4]uint8
	}{
		{gfx.NonZero, [2][4]uint8{{255, 255, 255, 255}, {255, 0, 0, 255}}},
		{gfx.EvenOdd, [2][4]uint8{{255, 0, 0, 255}, {255, 0, 0, 255}}},
	} {
		img := coverageImage(4, 4, 1, func(b *gfx.Builder) {
			b.FillPath(p, tc.rule, gfx.Black)
			b.FillPath(q, tc.rule, gfx.Black)
		})
		for row, y := range []int{0, 3} {
			for x := 0; x < 4; x++ {
				if got := img.Pix[img.PixOffset(x, y)+3]; got != tc.want[row][x] {
					t.Errorf("%v: pixel (%d, %d) = %d, want %d", tc.rule, x, y, got, tc.want[row][x])
				}
			}
		}
	}
}

// Robustness: extreme but finite input renders without panicking, and the
// visible part is right.
func TestRobustness(t *testing.T) {
	huge := float32(1e30)
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	dl := build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X0: -huge, Y0: -huge, X1: huge, Y1: 16}, gfx.Black) // covers the top half
		var p gfx.Path                                                          // huge triangle, edges far away
		p.MoveTo(gfx.Pt(-huge, 16))
		p.LineTo(gfx.Pt(huge, 16))
		p.LineTo(gfx.Pt(0, huge))
		p.Close()
		b.FillPath(p, gfx.EvenOdd, gfx.RGB8(255, 0, 0))
		b.FillRRect(gfx.NewRRect(gfx.Rect{X0: -huge, Y0: -huge, X1: huge, Y1: huge}, 1e29), gfx.RGB8(0, 0, 255).WithAlpha(0.5))
		b.FillPath(gfx.CirclePath(gfx.Pt(16, -1e7+8), 1e7), gfx.NonZero, gfx.White) // edge crossing at y=8
		// Degenerate paths: a single point, a line, repeated points, a
		// zero-area polygon with area elsewhere.
		var d gfx.Path
		d.MoveTo(gfx.Pt(5, 5))
		d.LineTo(gfx.Pt(5, 5))
		d.MoveTo(gfx.Pt(1, 1))
		d.LineTo(gfx.Pt(9, 9))
		d.LineTo(gfx.Pt(1, 1))
		d.MoveTo(gfx.Pt(2, 30))
		d.LineTo(gfx.Pt(3, 31))
		d.LineTo(gfx.Pt(2, 30.5))
		b.FillPath(d, gfx.NonZero, gfx.Black)
		b.FillRect(gfx.Rect{X0: 10, Y0: 10, X1: 10.000001, Y1: 20}, gfx.Black)
		b.FillRect(gfx.Rect{X0: 1e-30, Y0: 1e-30, X1: 2e-30, Y1: 1}, gfx.Black)
		// Extreme transforms.
		b.PushTransform(gfx.Scale(1e18, 1e18))
		b.FillRect(gfx.Rect{X0: -1, Y0: -1, X1: 1, Y1: 1}, gfx.Black)
		b.FillPath(gfx.CirclePath(gfx.Pt(0, 0), 1), gfx.NonZero, gfx.Black)
		b.PopTransform()
		b.PushTransform(gfx.Scale(1e-18, 1e-18))
		b.FillRRect(gfx.NewRRect(gfx.Rect{X0: 0, Y0: 0, X1: 1e19, Y1: 1e19}, 1e18), gfx.Black)
		b.PopTransform()
		b.PushTransform(gfx.Affine{1e30, 1e30, -1e30, 1e30, 0, 0})
		b.FillPath(gfx.CirclePath(gfx.Pt(0, 0), 1), gfx.NonZero, gfx.Black)
		b.PopTransform()
	})
	for _, scale := range []float32{1, 1e-6, 3e30, 1e30} {
		Clear(img, image.Rectangle{}, gfx.Transparent)
		Render(img, dl, Options{Scale: scale})
	}
	// Region outside the image, a 0x0 image, a sub-image with negative
	// coordinates, a nil list.
	Render(img, dl, Options{Region: image.Rect(100, 100, 200, 200)})
	Render(image.NewRGBA(image.Rectangle{}), dl, Options{})
	neg := image.NewRGBA(image.Rect(-40, -40, 8, 8))
	Render(neg, dl, Options{})
	Render(img, nil, Options{})

	// The visible result at scale 1: black above y=16, red below (the
	// triangle), blended blue on top, white above the circle edge at y=8.
	img = image.NewRGBA(image.Rect(0, 0, 32, 32))
	dl = build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X0: -huge, Y0: -huge, X1: huge, Y1: 16}, gfx.Black)
		var p gfx.Path
		p.MoveTo(gfx.Pt(-huge, 16))
		p.LineTo(gfx.Pt(huge, 16))
		p.LineTo(gfx.Pt(0, huge))
		p.Close()
		b.FillPath(p, gfx.EvenOdd, gfx.RGB8(255, 0, 0))
		b.FillPath(gfx.CirclePath(gfx.Pt(16, -1e7+8), 1e7), gfx.NonZero, gfx.White)
	})
	Render(img, dl, Options{})
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, color.RGBA{255, 255, 255, 255}}, {31, 7, color.RGBA{255, 255, 255, 255}},
		{16, 8, color.RGBA{0, 0, 0, 255}}, {0, 15, color.RGBA{0, 0, 0, 255}},
		{16, 16, color.RGBA{255, 0, 0, 255}}, {31, 31, color.RGBA{255, 0, 0, 255}},
	} {
		if got := pixel(img, c.x, c.y); got != c.want {
			t.Errorf("pixel (%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestOptionsValidation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	dl := build(func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 1, Y1: 1}, gfx.Black) })
	for _, s := range []float32{-1, float32(math.NaN()), float32(math.Inf(1))} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, "Options.Scale") {
					t.Errorf("Scale %v: panic %q", s, r)
				}
			}()
			Render(img, dl, Options{Scale: s})
		}()
	}
	func() {
		defer func() {
			if r, _ := recover().(string); !strings.Contains(r, "nil destination") {
				t.Errorf("nil dst: panic %q", r)
			}
		}()
		Render(nil, dl, Options{})
	}()
}

func TestClear(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	Clear(img, image.Rect(1, 1, 5, 5), gfx.White.WithAlpha(0.5))
	if pixel(img, 0, 0) != (color.RGBA{}) || pixel(img, 2, 2) != (color.RGBA{128, 128, 128, 128}) {
		t.Errorf("Clear wrote %v / %v", pixel(img, 0, 0), pixel(img, 2, 2))
	}
}

// The renderer reuses its buffers: no allocations per frame once warm, for
// rects and rrects.
func TestNoAllocs(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	dl := build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(1.5, 1.5, 30, 30), gfx.Black)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 10.5, 40, 30), 8), gfx.Black.WithAlpha(0.5))
		b.FillPath(starPath(32, 32, 20), gfx.EvenOdd, gfx.Black)
		// Curves, gradients, images, clips and layers.
		b.FillPath(gfx.CirclePath(gfx.Pt(20, 40), 15), gfx.NonZero, gfx.LinearGradient{End: gfx.Pt(40, 10), Stops: stops(sky, rose, amber)})
		b.FillRect(gfx.RectXYWH(0, 0, 64, 20), gfx.RadialGradient{Center: gfx.Pt(32, 10), Radius: gfx.Pt(20, 8), Stops: stops(gfx.White, indigo)})
		b.FillRect(gfx.RectXYWH(0, 40, 64, 20), gfx.ConicGradient{Center: gfx.Pt(32, 50), Stops: stops(gfx.White, indigo, gfx.White)})
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(4.5, 4.5, 50, 50), 12))
		b.PushClipRect(gfx.RectXYWH(8, 8, 40, 40))
		b.PushLayer(0.5, gfx.SrcOver)
		b.DrawImage(testImage, gfx.Rect{}, gfx.RectXYWH(10, 10, 30, 30), gfx.SampleLinear)
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(32, 32)))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(32, 32), 18), gfx.NonZero)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 40, 30), 6), rose)
		b.PopClip()
		b.PopTransform()
		b.PopLayer()
		b.PopClip()
		b.PopClip()
	})
	r := NewRenderer()
	r.Render(img, dl, Options{})
	if n := testing.AllocsPerRun(20, func() { r.Render(img, dl, Options{}) }); n > 0 {
		t.Errorf("%v allocations per Render", n)
	}
}
