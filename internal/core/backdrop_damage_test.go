package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
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

// stackedGlassPage is a 600x400 page with two glass panes: a large one
// (backdrop-blur-md) and, above it, a smaller one (backdrop-blur-sm)
// that reads its output, like the dashboard's command palette over its
// glass pane, and three boxes (page positions per step): one behind the
// large pane far from the small one, one in the large pane's content
// under the small one, and one in the small pane's content. Step 1 moves
// the second; steps 2 and 3 move the first and the third.
func stackedGlassPage(set *func(int)) Node {
	return Component(func() Node {
		n, setN := UseStateSet(0)
		*set = setN
		// Positions per step: behind, under, inside.
		pos := [][3]gfx.Point{
			{{X: 30, Y: 300}, {X: 330, Y: 110}, {X: 60, Y: 60}},
			{{X: 30, Y: 300}, {X: 334, Y: 112}, {X: 60, Y: 60}},
			{{X: 36, Y: 300}, {X: 334, Y: 112}, {X: 62, Y: 61}},
			{{X: 42, Y: 300}, {X: 334, Y: 112}, {X: 64, Y: 62}},
		}[n]
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
			div(Class("absolute size-10 rounded-full bg-lime-400"), Style(style.Translate(pos[0].X, pos[0].Y))),
			div(Class("absolute left-[10px] top-[10px] h-[380px] w-[580px] rounded-2xl bg-white/20 backdrop-blur-md"),
				div(Class("absolute size-10 rounded-lg bg-sky-500"), Style(style.Translate(pos[1].X-10, pos[1].Y-10)))),
			div(Class("absolute left-[300px] top-[60px] h-[200px] w-[260px] rounded-xl bg-black/20 backdrop-blur-sm"),
				div(Class("absolute size-8 rounded-md bg-fuchsia-500"), Style(style.Translate(pos[2].X, pos[2].Y)))),
		)
	})
}

// TestBackdropStackedDamage: a frame changes what the small pane reads
// (so its cached output covers only that damage), the next ones change
// what the large pane reads, far from the small one (it is not carried),
// and the small pane's content (it is carried and drawn from the cache):
// the large pane's reach grows the region into the small pane's shape,
// and every incremental frame equals a full rasterization.
func TestBackdropStackedDamage(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var set func(int)
			tr := newTestTree(t, 600, 400, stackedGlassPage(&set), nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(600*scale+0.5), int(400*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			ref := image.NewRGBA(fb.Rect)
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			for n := 1; n <= 3; n++ {
				set(n)
				tr.Frame()
				d := tr.RasterizeFrame(fb, &r, false)
				if d.Full {
					t.Fatalf("step %d: the frame was rasterized fully", n)
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
