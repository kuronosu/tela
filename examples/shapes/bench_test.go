package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// shapesWindow opens the page at 1416x846, scale 2, in a headless window.
func shapesWindow(tb testing.TB) (*headless.Backend, *core.Host, *headless.Window) {
	b := headless.New(headless.Options{Size: platform.Size{W: 1416, H: 846}, Scale: 2})
	host, err := core.NewHost(b, core.Options{Root: core.Component(Shapes)})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(host.Tree().Unmount)
	b.Drain(host)
	return b, host, host.Window().(*headless.Window)
}

// BenchmarkResize is a frame after a resize of the window at 1416x846,
// scale 2: a new layout and a full rasterization, where two blur-3xl glows
// dominate.
func BenchmarkResize(b *testing.B) {
	be, host, win := shapesWindow(b)
	ws := [2]float32{1416, 1400}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		i++
		win.Resize(platform.Size{W: ws[i&1], H: 846})
		be.Drain(host)
		be.Advance(be.FrameInterval())
		be.Drain(host)
	}
}

// BenchmarkPointerMove is a pointer move over the cards at 1416x846,
// scale 2: nothing in the page reacts to the pointer, so no frame runs.
func BenchmarkPointerMove(b *testing.B) {
	be, host, win := shapesWindow(b)
	presents := win.Presents()
	xs := [2]float32{300, 900}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		i++
		win.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: xs[i&1], Y: 400}})
		be.Drain(host)
		be.Advance(be.FrameInterval())
		be.Drain(host)
	}
	if win.Presents() != presents {
		b.Fatalf("%d frames presented for pointer moves", win.Presents()-presents)
	}
}
