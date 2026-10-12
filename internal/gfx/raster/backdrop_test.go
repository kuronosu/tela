package raster

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// backdropScene paints vertical red and blue halves split at x = 50 and a
// blurred backdrop over a box whose left edge is on the split.
func backdropScene(b *gfx.Builder) {
	b.FillRect(gfx.Rect{X1: 50, Y1: 100}, gfx.MustParseHex("#ff0000"))
	b.FillRect(gfx.Rect{X0: 50, X1: 200, Y1: 100}, gfx.MustParseHex("#0000ff"))
	b.FillRect(gfx.RectXYWH(120, 0, 10, 100), gfx.White)
	b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(50, 20, 100, 60), 12), gfx.Blur{StdDev: 4})
}

// TestBackdropMirror checks Chromium's edge rule: the backdrop is read
// inside the shape's bounds only, mirrored beyond them, so the red outside
// the box never bleeds in, while the white stripe inside is blurred.
func TestBackdropMirror(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	NewRenderer().Render(img, build(backdropScene), Options{Workers: 1})
	blue := color.RGBA{0, 0, 255, 255}
	for _, p := range []image.Point{{50, 50}, {51, 50}, {60, 30}, {49, 50}, {50, 10}} {
		want := blue
		if p.X < 50 {
			want = color.RGBA{255, 0, 0, 255}
		}
		if got := pixel(img, p.X, p.Y); got != want {
			t.Errorf("pixel %v = %v, want %v", p, got, want)
		}
	}
	// The white stripe (x 120-130) is blurred inside the box (it spreads
	// to x = 112) and untouched outside it (y < 20).
	if got := pixel(img, 112, 50); got.R == 0 || got.R == 255 || got.B != 255 {
		t.Errorf("blurred pixel (112, 50) = %v, want a light blue", got)
	}
	if got := pixel(img, 125, 50); got.R == 255 {
		t.Errorf("stripe center (125, 50) = %v, want it blurred", got)
	}
	if got := pixel(img, 112, 10); got != blue {
		t.Errorf("pixel (112, 10) = %v outside the box, want blue", got)
	}
	// The rounded corner keeps the original pixel outside the shape.
	if got := pixel(img, 50, 20); got != blue {
		t.Errorf("corner pixel (50, 20) = %v, want the unfiltered blue", got)
	}
}

// TestBackdropRegions checks that regions and worker counts give the
// pixels of a full render, at a fractional scale.
func TestBackdropRegions(t *testing.T) {
	dl := build(func(b *gfx.Builder) {
		backdropScene(b)
		b.PushTransform(gfx.Rotate(0.3))
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(90, 0, 50, 30), 6), gfx.Blur{StdDev: 2}, gfx.Saturate(2))
		b.PopTransform()
	})
	full := image.NewRGBA(image.Rect(0, 0, 300, 150))
	NewRenderer().Render(full, dl, Options{Scale: 1.5, Workers: 1})
	for _, opts := range []Options{
		{Scale: 1.5, Workers: 4},
		{Scale: 1.5, Region: image.Rect(70, 30, 160, 110)},
		{Scale: 1.5, Region: image.Rect(0, 0, 300, 61), Workers: 3},
	} {
		img := image.NewRGBA(full.Rect)
		r := NewRenderer()
		r.tune.minPixels = 1
		r.Render(img, dl, opts)
		reg := opts.Region
		if reg.Empty() {
			reg = full.Rect
		}
		for y := full.Rect.Min.Y; y < full.Rect.Max.Y; y++ {
			for x := full.Rect.Min.X; x < full.Rect.Max.X; x++ {
				want := color.RGBA{}
				if (image.Point{x, y}).In(reg) {
					want = full.RGBAAt(x, y)
				}
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("%+v: pixel (%d, %d) = %v, want %v", opts, x, y, got, want)
				}
			}
		}
	}
}

// TestBackdropInLayer checks that inside a layer the backdrop is the
// layer's content only (a layer is a backdrop root).
func TestBackdropInLayer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	NewRenderer().Render(img, build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 40, Y1: 40}, gfx.MustParseHex("#ff0000"))
		b.PushLayer(0.999, gfx.SrcOver)
		b.FillRect(gfx.Rect{X1: 20, Y1: 40}, gfx.MustParseHex("#0000ff"))
		b.BackdropFilter(gfx.RRectFromRect(gfx.Rect{X1: 40, Y1: 40}), gfx.Invert(1))
		b.PopLayer()
	}), Options{})
	// Left: the layer's blue inverted to yellow. Right: transparent in the
	// layer (invert keeps alpha 0), so the red below shows.
	if got := pixel(img, 5, 5); got.R < 250 || got.G < 250 || got.B > 5 {
		t.Errorf("left pixel %v, want yellow", got)
	}
	if got := pixel(img, 35, 5); got.R < 250 || got.G > 5 || got.B > 5 {
		t.Errorf("right pixel %v, want red", got)
	}
}
