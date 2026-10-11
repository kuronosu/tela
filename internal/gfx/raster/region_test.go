package raster

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// randomScene returns a scene of random shapes: rects, rrects, polygons and
// curves with random fill rules, translucent colors, transforms with
// rotation and skew, and shapes with huge coordinates that cross the canvas.
func randomScene(seed uint64, w, h int) scene {
	return scene{fmt.Sprintf("random%d", seed), w, h, func(b *gfx.Builder) {
		rng := rand.New(rand.NewPCG(seed, 1))
		f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
		W, H := float64(w), float64(h)
		color := func() gfx.Color {
			a := float32(1)
			if rng.IntN(2) == 0 {
				a = f(0.2, 0.9)
			}
			return gfx.Color{R: f(0, 1), G: f(0, 1), B: f(0, 1), A: a}
		}
		rect := func() gfx.Rect {
			x, y := f(-20, W), f(-20, H)
			return gfx.RectXYWH(x, y, f(0.1, W/2), f(0.1, H/2))
		}
		rrect := func() gfx.RRect {
			r := rect()
			var rad [4]gfx.Point
			for i := range rad {
				rad[i] = gfx.Pt(f(0, 40), f(0, 40))
				if rng.IntN(3) == 0 {
					rad[i].Y = rad[i].X
				}
			}
			return gfx.RRectCorners(r, rad[0], rad[1], rad[2], rad[3])
		}
		path := func() gfx.Path {
			var p gfx.Path
			p.MoveTo(gfx.Pt(f(-10, W+10), f(-10, H+10)))
			for n := 3 + rng.IntN(8); n > 0; n-- {
				pt := func() gfx.Point { return gfx.Pt(f(-10, W+10), f(-10, H+10)) }
				switch rng.IntN(4) {
				case 0, 1:
					p.LineTo(pt())
				case 2:
					p.QuadTo(pt(), pt())
				case 3:
					p.CubicTo(pt(), pt(), pt())
				}
				if rng.IntN(8) == 0 {
					p.Close()
					p.MoveTo(pt())
				}
			}
			p.Close()
			return p
		}
		rule := func() gfx.FillRule { return gfx.FillRule(rng.IntN(2)) }
		draw := func() {
			switch rng.IntN(3) {
			case 0:
				b.FillRect(rect(), color())
			case 1:
				b.FillRRect(rrect(), color())
			case 2:
				b.FillPath(path(), rule(), color())
			}
		}
		for i := 0; i < 40; i++ {
			switch rng.IntN(8) {
			case 0, 1, 2, 3:
				draw()
			case 4, 5: // transformed
				c := gfx.Pt(f(0, W), f(0, H))
				var t gfx.Affine
				switch rng.IntN(4) {
				case 0:
					t = gfx.RotateAbout(f(-math.Pi, math.Pi), c)
				case 1:
					t = gfx.Translate(c.X, c.Y).Mul(gfx.Skew(f(-0.6, 0.6), f(-0.3, 0.3))).Mul(gfx.Translate(-c.X, -c.Y))
				case 2:
					t = gfx.ScaleAbout(f(-2, 2), f(-2, 2), c) // flips too
				case 3:
					t = gfx.Translate(f(-5, 5), f(-5, 5)).Mul(gfx.RotateAbout(math.Pi/2, c))
				}
				b.PushTransform(t)
				draw()
				b.PopTransform()
			case 6: // huge geometry crossing the canvas
				switch rng.IntN(4) {
				case 0:
					b.FillRect(gfx.Rect{X0: -1e9, Y0: f(0, H), X1: f(0, W), Y1: 1e12}, color())
				case 1:
					var p gfx.Path
					p.MoveTo(gfx.Pt(-1e30, f(0, H)))
					p.LineTo(gfx.Pt(f(0, W), f(0, H)))
					p.LineTo(gfx.Pt(f(0, W), 3e7))
					p.LineTo(gfx.Pt(5e6, -4e6))
					b.FillPath(p, rule(), color())
				case 2:
					r := f(1e5, 1e7)
					b.FillPath(gfx.CirclePath(gfx.Pt(float32(W)/2, -r+f(0, H)), r), gfx.NonZero, color())
				case 3:
					b.FillRRect(gfx.NewRRect(gfx.Rect{X0: f(-3e6, 0), Y0: f(0, H), X1: f(0, W), Y1: f(1e6, 1e7)}, f(0, 1e6)), color())
				}
			case 7: // long thin slivers
				if rng.IntN(2) == 0 {
					b.FillRect(gfx.RectXYWH(f(-W, 0), f(0, H), f(W, 3*W), f(0.05, 1.5)), color())
				} else {
					var p gfx.Path
					p.MoveTo(gfx.Pt(f(-W, 0), f(0, H)))
					p.LineTo(gfx.Pt(f(W, 2*W), f(0, H)))
					p.LineTo(gfx.Pt(f(W, 2*W), f(0, H)))
					b.FillPath(p, rule(), color())
				}
			}
		}
	}}
}

// background fills img with a deterministic translucent pattern, so that the
// test also checks the blending and that pixels outside the region are kept.
func background(img *image.RGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			i := img.PixOffset(x, y)
			a := uint8((x*13 + y*7) % 256)
			img.Pix[i] = uint8(int(a) * ((x * 3) % 5) / 4)
			img.Pix[i+1] = uint8(int(a) * ((y * 5) % 7) / 6)
			img.Pix[i+2] = a / 2
			img.Pix[i+3] = a
		}
	}
}

func cloneRGBA(img *image.RGBA) *image.RGBA {
	out := &image.RGBA{Pix: bytes.Clone(img.Pix), Stride: img.Stride, Rect: img.Rect}
	return out
}

// TestRegionMatchesFullRender is the foundation of tiling: rendering any
// region gives exactly the pixels of a full render, and nothing outside the
// region changes.
func TestRegionMatchesFullRender(t *testing.T) {
	testRegions(t)
}

// TestRegionMatchesFullRenderCulled repeats the test with curve culling
// (flattenCulled) forced for every path, so that curves outside small
// regions are replaced by their chords.
func TestRegionMatchesFullRenderCulled(t *testing.T) {
	old := nearMargin
	nearMargin = -1 << 20 // never "near": always cull
	defer func() { nearMargin = old }()
	testRegions(t)
}

func testRegions(t *testing.T) {
	all := append([]scene(nil), scenes...)
	for seed := uint64(1); seed <= 4; seed++ {
		all = append(all, randomScene(seed, 150, 110))
	}
	for _, s := range all {
		for _, scale := range []float32{1, 1.5, 2, 0.75} {
			t.Run(fmt.Sprintf("%s_x%g", s.name, scale), func(t *testing.T) {
				dl := build(s.draw)
				w := int(math.Ceil(float64(float32(s.w) * scale)))
				h := int(math.Ceil(float64(float32(s.h) * scale)))
				bg := image.NewRGBA(image.Rect(0, 0, w, h))
				background(bg)
				full := cloneRGBA(bg)
				Render(full, dl, Options{Scale: scale})

				rng := rand.New(rand.NewPCG(uint64(w*h), uint64(scale*4)))
				r := NewRenderer() // one renderer reused across regions
				for k := 0; k < 30; k++ {
					var region image.Rectangle
					switch k % 5 {
					case 0: // single pixel
						x, y := rng.IntN(w), rng.IntN(h)
						region = image.Rect(x, y, x+1, y+1)
					case 1: // one row
						y := rng.IntN(h)
						region = image.Rect(rng.IntN(w), y, w, y+1)
					case 2: // one column
						x := rng.IntN(w)
						region = image.Rect(x, 0, x+1, rng.IntN(h)+1)
					default: // any rectangle, possibly sticking out of the image
						x0, y0 := rng.IntN(w+20)-10, rng.IntN(h+20)-10
						region = image.Rect(x0, y0, x0+1+rng.IntN(w), y0+1+rng.IntN(h))
					}
					got := cloneRGBA(bg)
					r.Render(got, dl, Options{Scale: scale, Region: region})
					checkRegion(t, got, full, bg, region)

					// Rendering into a sub-image is the same as using Region.
					sub := cloneRGBA(bg)
					if si, ok := sub.SubImage(region).(*image.RGBA); ok && !si.Rect.Empty() {
						r.Render(si, dl, Options{Scale: scale})
						checkRegion(t, sub, full, bg, region)
					}
				}
				// Tiles covering the frame reproduce it exactly.
				tiled := cloneRGBA(bg)
				for ty := 0; ty < h; ty += 16 {
					for tx := 0; tx < w; tx += 16 {
						r.Render(tiled, dl, Options{Scale: scale, Region: image.Rect(tx, ty, tx+16, ty+16)})
					}
				}
				checkRegion(t, tiled, full, bg, tiled.Rect)
			})
		}
	}
}

func checkRegion(t *testing.T, got, full, bg *image.RGBA, region image.Rectangle) {
	t.Helper()
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		for x := got.Rect.Min.X; x < got.Rect.Max.X; x++ {
			i := got.PixOffset(x, y)
			want := bg.Pix[i : i+4]
			if image.Pt(x, y).In(region) {
				want = full.Pix[i : i+4]
			}
			if !bytes.Equal(got.Pix[i:i+4], want) {
				t.Fatalf("region %v: pixel (%d, %d) = %v, want %v (inside region: %v)",
					region, x, y, got.Pix[i:i+4], want, image.Pt(x, y).In(region))
			}
		}
	}
}
