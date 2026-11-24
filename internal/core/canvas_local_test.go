package core

import (
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// The LocalX and LocalY of a canvas's handlers are in the coordinates it
// draws in: what is drawn there is under the pointer, whatever its border, its
// padding, its place and its transform. Other elements', and those of a
// handler that the event bubbles to, are from the border box.
func TestCanvasLocal(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		for _, class := range []string{
			"", "border-2 p-4", "border-2 pl-[7px] pt-1", "border p-[3.3px]", "p-4 rotate-30", "border-4 p-2 scale-150",
		} {
			var at, outer, inner gfx.Point
			draw := func(c *canvas.Canvas) { c.FillRect(at0.X-2, at0.Y-2, 4, 4) }
			root := div(Class("pt-[10.3px] pl-[20.7px]"),
				div(Class("w-60 h-40 p-8"),
					On("pointermove", func(e PointerEvent) { outer = gfx.Point{X: e.LocalX, Y: e.LocalY} }),
					canvasNode(draw, Class("block w-40 h-20 bg-white text-red-500 "+class),
						On("pointermove", func(e PointerEvent) { at = gfx.Point{X: e.LocalX, Y: e.LocalY} }))),
				div(Class("w-10 h-10 border-2 p-4"), On("pointermove", func(e PointerEvent) { inner = gfx.Point{X: e.LocalX, Y: e.LocalY} })))
			x := newWinHarness(t, Options{Root: root})
			x.windows()[0].SetScale(scale)
			x.drain()
			tr := x.h.Tree()
			b := findCanvasBox(tr)
			// The window point of the drawing's (at0): where it is painted.
			p := painter{t: tr, scale: scale}
			g := p.geom(b)
			r := p.contentRRect(b, &g).Rect
			w := gfx.Point{X: r.X0 + float32(at0.X), Y: r.Y0 + float32(at0.Y)}
			if !b.pcs.Transform.IsIdentity() {
				w = b.pcs.Transform.Matrix(b.u).Apply(w)
			}
			x.windows()[0].Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: w.X, Y: w.Y}})
			x.drain()
			if !nearPoint(at, gfx.Point{X: float32(at0.X), Y: float32(at0.Y)}) {
				t.Errorf("%q at %v: local %v, drawn at %v", class, scale, at, at0)
			}
			if want := w.Sub(gfx.Point{X: 20.7, Y: 10.3}); !nearPoint(outer, want) {
				t.Errorf("%q at %v: its parent's local %v, want %v", class, scale, outer, want)
			}
			x.windows()[0].Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 20.7 + 5, Y: 10.3 + 160 + 6}})
			x.drain()
			if !nearPoint(inner, gfx.Point{X: 5, Y: 6}) {
				t.Errorf("another element's local is %v, want its border box's", inner)
			}
		}
	}
}

// at0 is the point of the drawing.
var at0 = struct{ X, Y float64 }{30, 12}

func nearPoint(a, b gfx.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.01 && math.Abs(float64(a.Y-b.Y)) < 0.01
}

func findCanvasBox(tr *Tree) *Box {
	var find func(b *Box) *Box
	find = func(b *Box) *Box {
		if b.med != nil && b.med.cnv != nil {
			return b
		}
		for _, k := range b.kids {
			if f := find(k); f != nil {
				return f
			}
		}
		return nil
	}
	return find(tr.rootBox)
}
