package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/style"
)

// Benchmarks of layout animations through the whole host (a frame is a
// FrameEvent of the headless window: animate, pipeline, damage raster,
// present).
//
//	go test -run xxx -bench 'Layout(Reorder|Expand)' -benchmem ./internal/core/

// BenchmarkLayoutReorder100 reverses a list of 100 rows with AnimateLayout
// and runs the frames of the animation (until every row is at rest). It
// reports the time of a frame, the frame that lays out the new order
// included.
func BenchmarkLayoutReorder100(b *testing.B) {
	for _, scale := range []float64{1, 2} {
		b.Run(fmt.Sprintf("x%g", scale), func(b *testing.B) {
			var set Setter[[]int]
			root := Component(func() Node {
				order, s := UseState(func() []int {
					o := make([]int, 100)
					for i := range o {
						o[i] = i
					}
					return o
				}())
				set = s
				return div(Class("flex flex-col gap-1 p-4"), Map(order, func(i int) Node {
					return div(Key(i), AnimateLayout(), Class("flex h-5 items-center rounded bg-slate-100 px-2 text-xs text-slate-700 shadow-sm"),
						Text(fmt.Sprintf("Row %d", i)))
				}))
			})
			be, host, _ := benchHost(b, root, 800, 600, scale)
			order := make([]int, 100)
			for i := range order {
				order[i] = i
			}
			frames := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				slices.Reverse(order)
				set.Set(slices.Clone(order))
				be.Drain(host)
				for host.Tree().Animating() || host.Tree().Dirty() {
					be.Advance(be.FrameInterval())
					be.Drain(host)
					frames++
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed())/float64(max(frames, 1)), "ns/frame")
			b.ReportMetric(float64(frames)/float64(max(b.N, 1)), "frames/reorder")
		})
	}
}

// BenchmarkLayoutExpand compares the two ways of animating a size: a card
// with AnimateLayout drawn through a scale (layout once, then frames that
// only composite), and the same card whose height follows a tween of the
// same duration and easing through a layout-phase deferred prop
// (HeightFn: layout and recording every frame). Both cards hold text and
// have five rows below them.
func BenchmarkLayoutExpand(b *testing.B) {
	// la is AnimateLayout in the scale mode, and nothing in the relayout
	// mode (the rows below move by layout).
	card := func(la Child, extra ...Child) Node {
		kids := []Child{Class("w-[360px] overflow-hidden rounded-2xl bg-indigo-600 p-5 text-white shadow-lg")}
		kids = append(kids, extra...)
		kids = append(kids,
			div(la, Class("text-base font-semibold"), Text("Quarterly report")),
			p(la, Class("mt-2 text-sm"), Text("Revenue grew 18% over the last quarter, led by the new plans and fewer cancellations.")))
		return div(kids...)
	}
	rows := func(la Child, n int) []Child {
		var out []Child
		for i := range n {
			out = append(out, div(la, Class("h-8 rounded bg-slate-100 px-2 text-sm duration-300 ease-out"), Text(fmt.Sprintf("Row %d", i))))
		}
		return out
	}
	for _, n := range []int{5, 60} {
		for _, mode := range []string{"scale", "relayout"} {
			for _, scale := range []float64{1, 2} {
				b.Run(fmt.Sprintf("rows%d/%s/x%g", n, mode, scale), func(b *testing.B) {
					var set Setter[bool]
					root := Component(func() Node {
						open, s := UseState(false)
						set = s
						target := float32(100)
						if open {
							target = 260
						}
						var c Node
						var la Child = Prop{}
						if mode == "scale" {
							la = AnimateLayout()
							c = card(la, la, Class("duration-300 ease-out"), Style(style.Height(target)))
						} else {
							h := UseTween(float32(100), target, Duration(300*time.Millisecond), Ease("ease-out"))
							c = card(la, Style(style.HeightFn(h.Get)))
						}
						return div(append([]Child{Class("flex flex-col gap-1 p-6")}, append([]Child{c}, rows(la, n)...)...)...)
					})
					be, host, _ := benchHost(b, root, 800, 600, scale)
					frames := 0
					open := false
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						open = !open
						set.Set(open)
						be.Drain(host)
						for host.Tree().Animating() || host.Tree().Dirty() {
							be.Advance(be.FrameInterval())
							be.Drain(host)
							frames++
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(b.Elapsed())/float64(max(frames, 1)), "ns/frame")
					b.ReportMetric(float64(frames)/float64(max(b.N, 1)), "frames/toggle")
				})
			}
		}
	}
}
