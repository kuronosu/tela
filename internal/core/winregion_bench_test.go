package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// regionBenchScene is an 800x600 window whose card has h.WindowResize and
// a drag area, with, when pop, an open popover of n items in a grid (no scroll
// container) away from the card's edges and a toast viewport of 4 toasts
// in portals, as an app keeps them mounted.
func regionBenchScene(pop bool, n int) Node {
	card := div(Class("absolute inset-4 rounded-2xl bg-white"), WindowResize(),
		div(Class("h-10"), WindowDrag()),
	)
	if !pop {
		return div(Class("relative w-full h-full"), card)
	}
	items := []Child{Class("fixed left-[100px] top-[60px] grid w-96 grid-cols-20 rounded-lg bg-white p-1")}
	for i := range n {
		items = append(items, div(Key(i), Class("flex h-4 items-center overflow-hidden text-xs"), span(Textf("Item %d", i))))
	}
	toasts := []Child{Class("fixed inset-0 pointer-events-none")}
	for i := range 4 {
		toasts = append(toasts, div(Key(i), Class("absolute right-[40px] w-64 h-12 rounded bg-white pointer-events-auto"),
			Style(style.Top(float32(400+i*14))), span(Textf("Toast %d", i))))
	}
	return div(Class("relative w-full h-full"), card, Portal(div(items...)), Portal(div(toasts...)))
}

func benchRegions(b *testing.B, pop bool, n int) {
	tr := newTestTree(b, 800, 600, regionBenchScene(pop, n), nil)
	tr.Frame()
	var buf []platform.HitRegion
	b.ReportAllocs()
	for b.Loop() {
		buf = tr.hitRegions(buf[:0])
	}
	if len(buf) != 13 {
		b.Fatalf("%d regions, want the card's 13 (nothing covers them): %v", len(buf), buf[13:])
	}
}

// BenchmarkHitRegions is the hit regions of a window with a resize card
// and two mounted portals (a 400-item popover and a toast viewport) that
// cover none of its regions, computed after every frame that ran the
// pipeline; BenchmarkHitRegionsBaseline is the card alone.
func BenchmarkHitRegions(b *testing.B)         { benchRegions(b, true, 400) }
func BenchmarkHitRegionsBaseline(b *testing.B) { benchRegions(b, false, 0) }
