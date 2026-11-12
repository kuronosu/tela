package raster

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestRegionClearTransparent renders, on a transparent canvas, the region
// of a list with nothing opaque under a glass pane over the pixels of the
// previous frame: the region grows by the blur's reach, and the pixels it
// reads beyond the region start from the canvas (Options.Clear), so the
// region equals a full render; started from the previous frame's pixels,
// as before Options.Clear, it does not.
func TestRegionClearTransparent(t *testing.T) {
	list := func(x float32) *gfx.DisplayList {
		return build(func(b *gfx.Builder) {
			for i := float32(0); i < 400; i += 10 {
				b.FillRect(gfx.RectXYWH(i, 0, 4, 300), sky.WithAlpha(0.4))
			}
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(x, 100, 32, 32), 4), rose.WithAlpha(0.6))
			pane := gfx.NewRRect(gfx.RectXYWH(60, 50, 280, 200), 16)
			b.BackdropFilter(pane, gfx.Blur{StdDev: 12})
			b.FillRRect(pane, gfx.White.WithAlpha(0.1))
		})
	}
	for _, scale := range []float32{1, 1.5} {
		for _, workers := range []int{1, 0} {
			t.Run(fmt.Sprintf("x%g_w%d", scale, workers), func(t *testing.T) {
				w, h := int(400*scale), int(300*scale)
				full := func(dl *gfx.DisplayList) *image.RGBA {
					img := image.NewRGBA(image.Rect(0, 0, w, h))
					NewRenderer().Render(img, dl, Options{Scale: scale, Workers: 1})
					return img
				}
				old, next := full(list(90)), list(101)
				want := full(next)
				rg := devRootRect(gfx.RectXYWH(89, 99, 46, 34), scale)
				if BackdropRegion(next, scale, rg, want.Rect) == rg {
					t.Fatal("the region does not grow")
				}
				var r Renderer
				for _, clear := range []bool{true, false} {
					fb := image.NewRGBA(old.Rect)
					copy(fb.Pix, old.Pix)
					opts := Options{Scale: scale, Workers: workers, Region: rg}
					if clear {
						opts.Clear = &gfx.Transparent
					} else {
						Clear(fb, rg, gfx.Transparent)
					}
					r.Render(fb, next, opts)
					exp := image.NewRGBA(old.Rect)
					copy(exp.Pix, old.Pix)
					copyRect(exp, want, rg)
					p, same := firstDiffRGBA(fb, exp)
					if same != clear {
						t.Errorf("Clear %v: the region equals a full render: %v (first difference at %v)", clear, same, p)
					}
				}
			})
		}
	}
}
