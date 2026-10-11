package raster

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func imagePixel(img *gfx.Image, x, y int) [4]uint8 {
	o := y*img.Stride() + 4*x
	p := img.Pix()[o : o+4]
	return [4]uint8{p[0], p[1], p[2], p[3]}
}

// An image drawn 1:1 at a whole-pixel offset, with either sampling,
// reproduces its pixels exactly.
func TestImageIdentity(t *testing.T) {
	img := testImage
	for _, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
		for _, scale := range []float32{1, 2} {
			out := renderTransparent(20, 20, scale, func(b *gfx.Builder) {
				// At scale 2, draw at half size so that the device size is 1:1.
				k := 1 / scale
				b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(3*k, 4*k, 6*k, 6*k), s)
			})
			for y := 0; y < 6; y++ {
				for x := 0; x < 6; x++ {
					if got, want := rgba(out.RGBAAt(3+x, 4+y)), imagePixel(img, x, y); got != want {
						t.Fatalf("%v at scale %v: pixel (%d, %d) = %v, want %v", s, scale, x, y, got, want)
					}
				}
			}
			if out.RGBAAt(2, 4).A != 0 || out.RGBAAt(9, 4).A != 0 {
				t.Errorf("%v: drawn outside Dst", s)
			}
		}
	}
}

// Nearest upscaling by 4 gives 4x4 blocks of the source pixels.
func TestImageNearestUpscale(t *testing.T) {
	out := renderTransparent(24, 24, 1, func(b *gfx.Builder) {
		b.DrawImage(testImage, gfx.Rect{}, gfx.RectXYWH(0, 0, 24, 24), gfx.SampleNearest)
	})
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if got, want := rgba(out.RGBAAt(x, y)), imagePixel(testImage, x/4, y/4); got != want {
				t.Fatalf("pixel (%d, %d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// A Src sub-rectangle never samples the pixels around it, even with
// bilinear filtering and under rotation.
func TestImageNoBleeding(t *testing.T) {
	// A 4x4 image: an opaque red 2x2 center surrounded by opaque green.
	pix := make([]uint8, 4*16)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := []uint8{0, 255, 0, 255}
			if x >= 1 && x <= 2 && y >= 1 && y <= 2 {
				c = []uint8{255, 0, 0, 255}
			}
			copy(pix[4*(4*y+x):], c)
		}
	}
	img := gfx.NewImagePremul(4, 4, pix)
	for _, m := range []gfx.Affine{gfx.Identity, gfx.RotateAbout(0.7, gfx.Pt(20, 20))} {
		out := renderTransparent(40, 40, 1, func(b *gfx.Builder) {
			b.PushTransform(m)
			b.DrawImage(img, gfx.Rect{X0: 1, Y0: 1, X1: 3, Y1: 3}, gfx.RectXYWH(4, 4, 32, 32), gfx.SampleLinear)
			b.PopTransform()
		})
		for i := 3; i < len(out.Pix); i += 4 {
			if g := out.Pix[i-2]; g != 0 {
				x, y := (i/4)%40, (i/4)/40
				t.Fatalf("transform %v: pixel (%d, %d) has green %d from outside Src", m, x, y, g)
			}
		}
	}
}

// Parts of Src outside the image are clipped away with the matching part of
// Dst.
func TestImageSrcOutside(t *testing.T) {
	out := renderTransparent(16, 16, 1, func(b *gfx.Builder) {
		// Src covers x in [-6, 6): only its right half exists, drawn at
		// Dst x in [8, 16).
		b.DrawImage(testImage, gfx.Rect{X0: -6, Y0: 0, X1: 6, Y1: 6}, gfx.RectXYWH(0, 0, 16, 8), gfx.SampleNearest)
	})
	if out.RGBAAt(7, 3).A != 0 {
		t.Error("the part of Dst for Src outside the image is painted")
	}
	if got, want := rgba(out.RGBAAt(8, 0)), imagePixel(testImage, 0, 0); got != want {
		t.Errorf("pixel (8, 0) = %v, want %v", got, want)
	}
}
