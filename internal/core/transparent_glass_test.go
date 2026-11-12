package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/style"
)

// transparentGlassPage is a 400x300 page for a transparent canvas
// (ADR-249): a root without a background, translucent stripes, a glass
// pane (backdrop-blur-md) and, under the pane and painted before its
// filter, a translucent box at a position that follows the state, so
// that each change damages the pane's output within the blur's reach and
// the region rendered grows by what the filter reads.
func transparentGlassPage(set *func(int)) Node {
	return Component(func() Node {
		n, setN := UseStateSet(0)
		*set = setN
		stripes := []Child{Class("absolute inset-0 flex gap-[6px]")}
		for i := range 40 {
			stripes = append(stripes, div(Key(i), Class("h-full w-[4px] shrink-0 bg-sky-500/40")))
		}
		return div(Class("relative h-screen"),
			div(stripes...),
			div(Class("absolute size-8 rounded bg-rose-500/60"), Style(style.Translate(float32(90+11*n), float32(100+5*n)))),
			div(Class("absolute left-[60px] top-[50px] h-[200px] w-[280px] rounded-2xl border border-white/30 bg-white/10 backdrop-blur-md")),
		)
	})
}

// TestTransparentCanvasGlassDamage moves a translucent box under a glass
// pane on a transparent canvas: the region a frame renders grows by the
// blur's reach beyond the damage, and every incremental frame equals a
// full rasterization (cleared to transparent).
func TestTransparentCanvasGlassDamage(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var set func(int)
			tr := newTestTree(t, 400, 300, transparentGlassPage(&set), nil)
			tr.SetTransparentCanvas(true)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(400*scale+0.5), int(300*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			grew := false
			for n := 1; n <= 8; n++ {
				set(n)
				tr.Frame()
				d := tr.RasterizeFrame(fb, &r, false)
				if d.Full {
					t.Fatalf("step %d: a full frame", n)
				}
				for _, rc := range d.Rects {
					if raster.BackdropRegion(tr.LastFrame(), scale, rc, fb.Rect) != rc {
						grew = true
					}
				}
				ref := image.NewRGBA(fb.Rect)
				raster.Clear(ref, image.Rectangle{}, gfx.Transparent)
				raster.Render(ref, tr.LastFrame(), raster.Options{Scale: scale})
				if p, ok := firstDiff(fb, ref); !ok {
					t.Fatalf("step %d: the damage %v gives other pixels than a full rasterization at %v: %v, want %v",
						n, d.Rects, p, fb.RGBAAt(p.X, p.Y), ref.RGBAAt(p.X, p.Y))
				}
			}
			if !grew {
				t.Error("no rendered region grew by the blur's reach: the test does not test it")
			}
		})
	}
}
