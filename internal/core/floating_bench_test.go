package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// floatBenchScene is a list of 2,000 rows in a scroll container that
// fills the 800x600 window, with an anchor near its top moved by a
// TranslateFn, and, with pop, a popover of 8 items in a Portal placed
// under the anchor (ADR-096).
func floatBenchScene(pop bool, x **Value[float32]) Node {
	comp := func() Node {
		anchor := UseElementRef()
		v := UseValue[float32](0)
		*x = v
		rows := []Child{Key("list"), Class("h-screen overflow-y-auto bg-white")}
		for i := range 2000 {
			if i == 3 {
				rows = append(rows, div(Key("anchor"), AttachRef(anchor), Class("relative mx-6 h-8 w-32 rounded bg-indigo-500 px-2 text-white"),
					Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })), Text("Anchor")))
			}
			rows = append(rows, div(Key(i), Class("flex h-8 items-center gap-2 border-b border-slate-100 px-3 text-sm"), Textf("Row %d", i)))
		}
		var p Node
		if pop {
			place := func(in FloatInput) FloatResult { return FloatResult{X: in.Anchor.X, Y: in.Anchor.Bottom() + 4} }
			items := []Child{Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: place}),
				Class("fixed w-48 rounded-lg bg-white p-1 shadow-lg ring-1 ring-black/10")}
			for i := range 8 {
				items = append(items, div(Key(i), Class("h-7 rounded px-2 text-sm"), Textf("Item %d", i)))
			}
			p = Portal(div(items...))
		}
		return div(div(rows...), p)
	}
	return Component(comp)
}

// benchFloatScroll scrolls the list by 1 px per frame, back and forth over
// 60 px (the anchor and the popover stay on screen), and rasterizes the
// damage at scale 2.
func benchFloatScroll(b *testing.B, pop bool) {
	var x *Value[float32]
	tr := newTestTree(b, 800, 600, floatBenchScene(pop, &x), nil)
	tr.Frame()
	s := keyed(tr, "list").box.scr
	dir := float32(1)
	rasterBench(b, tr, 800, 600, 2, false, func() {
		if s.pos.Y+dir > 60 || s.pos.Y+dir < 0 {
			dir = -dir
		}
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: dir}))
	})
}

// BenchmarkFloatingScroll is a scroll frame that moves the anchor and its
// popover (a frame that only repaints, with the float pass), with the
// damage rasterized at scale 2; BenchmarkFloatingScrollBaseline is the
// same scroll without the popover.
func BenchmarkFloatingScroll(b *testing.B)         { benchFloatScroll(b, true) }
func BenchmarkFloatingScrollBaseline(b *testing.B) { benchFloatScroll(b, false) }

// benchFloatMove moves the anchor by its TranslateFn (a drag), which
// moves the popover: only those two move.
func benchFloatMove(b *testing.B, pop bool) {
	var x *Value[float32]
	tr := newTestTree(b, 800, 600, floatBenchScene(pop, &x), nil)
	tr.Frame()
	pos := float32(0)
	rasterBench(b, tr, 800, 600, 2, false, func() {
		pos = float32(int(pos+7) % 300)
		x.storeUI(pos)
	})
}

// BenchmarkFloatingMove is a frame that only moves the anchor and the
// floating element (Value read by a TranslateFn); BenchmarkFloatingMove
// Baseline moves the anchor alone.
func BenchmarkFloatingMove(b *testing.B)         { benchFloatMove(b, true) }
func BenchmarkFloatingMoveBaseline(b *testing.B) { benchFloatMove(b, false) }

// BenchmarkFloatingMovePipeline is BenchmarkFloatingMove without the
// raster: the frame's pipeline (the float pass, the composition and the
// damage are not included; Frame only).
func BenchmarkFloatingMovePipeline(b *testing.B) {
	for _, pop := range []bool{true, false} {
		name := "popover"
		if !pop {
			name = "baseline"
		}
		b.Run(name, func(b *testing.B) {
			var x *Value[float32]
			tr := newTestTree(b, 800, 600, floatBenchScene(pop, &x), nil)
			tr.SetScale(2)
			tr.Frame()
			pos := float32(0)
			b.ReportAllocs()
			for b.Loop() {
				pos = float32(int(pos+7) % 300)
				x.storeUI(pos)
				tr.Frame()
			}
		})
	}
}
