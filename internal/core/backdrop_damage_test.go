package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/style"
)

// A backdrop filter reads what is painted before it. A change painted
// after it, inside the filtered element (its content), leaves its input
// alone: the damage is the change, not the change grown by the filter's
// reach.

// glassBoxPage is a 600x400 page: stripes under a gradient, a panel with
// backdrop-blur-2xl (reach 120 px) and, inside the panel, a 40x40 box at
// the position *pos (set receives the setter).
func glassBoxPage(set *func(int)) Node {
	return Component(func() Node {
		n, setN := UseStateSet(0)
		*set = setN
		stripes := []Child{Class("absolute inset-0 flex")}
		for i := range 60 {
			c := "bg-slate-900"
			if i%2 == 1 {
				c = "bg-amber-300"
			}
			stripes = append(stripes, div(Key(i), Class("h-full w-[10px] shrink-0 "+c)))
		}
		return div(Class("relative h-screen overflow-hidden bg-linear-to-br from-indigo-500 to-rose-500"),
			div(stripes...),
			div(Class("absolute left-[100px] top-[50px] h-[300px] w-[400px] rounded-2xl border border-white/40 bg-white/20 backdrop-blur-2xl"),
				div(Class("absolute size-10 rounded-lg bg-sky-500"), Style(style.Translate(float32(100+7*n), float32(120+3*n))))),
		)
	})
}

// TestBackdropContentDamage moves a box inside a backdrop-blur-2xl panel:
// every frame's damage is the box's old and new bounds (with the pixel of
// margin of rounding, and anti-aliasing), not grown by the blur's reach,
// and the incremental frame equals a full rasterization.
func TestBackdropContentDamage(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var set func(int)
			tr := newTestTree(t, 600, 400, glassBoxPage(&set), nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(600*scale+0.5), int(400*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			ref := image.NewRGBA(fb.Rect)
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			// The box's device bounds at step n.
			box := func(n int) image.Rectangle {
				x0, y0 := float64(100+100+7*n)*float64(scale), float64(50+120+3*n)*float64(scale)
				return image.Rect(int(x0), int(y0), int(x0+40*float64(scale)+1), int(y0+40*float64(scale)+1))
			}
			const margin = 3 // device pixels: devRect's rounding pixel, anti-aliasing
			for n := 1; n <= 6; n++ {
				set(n)
				tr.Frame()
				d := tr.RasterizeFrame(fb, &r, false)
				if d.Full {
					t.Fatalf("step %d: the move rasterized the whole frame", n)
				}
				want := box(n - 1).Union(box(n)).Inset(-margin)
				var got image.Rectangle
				for _, rc := range d.Rects {
					got = got.Union(rc)
				}
				if !got.In(want) {
					t.Errorf("step %d: damage %v (bounds %v, %dx%d) is not inside the box's old and new bounds %v (%dx%d)",
						n, d.Rects, got, got.Dx(), got.Dy(), want, want.Dx(), want.Dy())
				}
				raster.Clear(ref, image.Rectangle{}, tr.CanvasColor())
				raster.Render(ref, tr.LastFrame(), raster.Options{Scale: scale})
				if p, ok := firstDiff(fb, ref); !ok {
					t.Fatalf("step %d: the damage %v gives other pixels than a full rasterization at %v: %v, want %v",
						n, d.Rects, p, fb.RGBAAt(p.X, p.Y), ref.RGBAAt(p.X, p.Y))
				}
			}
		})
	}
}
