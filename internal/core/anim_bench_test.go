package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Benchmarks of animations through the whole host: a frame is a FrameEvent
// of the headless window (animate, pipeline, damage raster into the
// framebuffer, present), so the numbers are what a real frame costs
// without the OS.
//
//	go test -run xxx -bench 'Anim' -benchmem ./internal/core/

// buttonGrid is 100 buttons with Tailwind's bare "transition" (colors,
// opacity, shadow, transform, filters; 150 ms) that change color, shadow
// and position on hover.
func buttonGrid() Node {
	kids := []Child{Class("flex flex-wrap gap-2 p-4")}
	for i := range 100 {
		kids = append(kids, button(Key(fmt.Sprintf("b%d", i)),
			Class("h-9 w-[70px] rounded-lg bg-slate-100 text-sm text-slate-800 shadow-sm hover:-translate-y-0.5 hover:bg-indigo-600 hover:text-white hover:shadow-lg transition"),
			Text(fmt.Sprintf("Btn %d", i))))
	}
	return div(kids...)
}

func benchHost(b *testing.B, root Node, w, h float32, scale float64) (*headless.Backend, *Host, *headless.Window) {
	b.Helper()
	be := headless.New(headless.Options{Size: platform.Size{W: w, H: h}, Scale: scale})
	host, err := NewHost(be, Options{Config: Config{Fonts: testFonts, RootClass: "bg-white"}, Root: root})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(host.stopTimer)
	be.Drain(host)
	for range 3 {
		be.Advance(be.FrameInterval())
		be.Drain(host)
	}
	return be, host, host.Window().(*headless.Window)
}

// BenchmarkAnimHoverSequence hovers the 100 buttons of buttonGrid in
// sequence, running the frames of each transition (the pointer moves, the
// button starts 5 transitions, 10 frames later it is at rest). It reports
// the time of a frame, including the frames that handle the pointer.
func BenchmarkAnimHoverSequence(b *testing.B) {
	for _, scale := range []float64{1, 2} {
		b.Run(fmt.Sprintf("x%g", scale), func(b *testing.B) {
			be, host, win := benchHost(b, buttonGrid(), 800, 600, scale)
			in := host.Tree().Inspect()
			var centers [100]platform.Point
			for i := range centers {
				n := find(&in, fmt.Sprintf("b%d", i))
				if n == nil {
					b.Fatalf("no button %d", i)
				}
				c := n.Rect.Center()
				centers[i] = platform.Point{X: c.X, Y: c.Y}
			}
			frames := 0
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				win.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: centers[i%100]})
				be.Drain(host)
				for end := be.Now() + 220*time.Millisecond; be.Now() < end; frames++ {
					be.Advance(be.FrameInterval())
					be.Drain(host)
				}
			}
			b.StopTimer()
			if host.Tree().Animating() {
				b.Fatal("still animating")
			}
			b.ReportMetric(float64(b.Elapsed())/float64(max(frames, 1)), "ns/frame")
			b.ReportMetric(float64(frames)/float64(max(b.N, 1)), "frames/hover")
		})
	}
}

// BenchmarkAnimSpinFrame is one frame of a tree with 100 buttons and one
// animate-spin element: the frame only paints (the spinner is a layer),
// and only the spinner's area is rasterized.
func BenchmarkAnimSpinFrame(b *testing.B) {
	for _, scale := range []float64{1, 2} {
		b.Run(fmt.Sprintf("x%g", scale), func(b *testing.B) {
			root := div(
				div(Key("spin"), Class("absolute right-4 top-4 size-10 rounded-full border-4 border-slate-300 border-t-indigo-600 animate-spin")),
				buttonGrid())
			be, host, _ := benchHost(b, root, 800, 600, scale)
			before := host.Tree().Stats()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				be.Advance(be.FrameInterval())
				be.Drain(host)
			}
			b.StopTimer()
			after := host.Tree().Stats()
			if after.Builds != before.Builds || after.Layouts != before.Layouts || after.Styles != before.Styles {
				b.Fatalf("spinner frames did more than paint: %+v -> %+v", before, after)
			}
			if d := host.Tree().LastDamage(); d.Full || d.Empty() {
				b.Fatalf("damage of a spinner frame: %+v", d)
			}
		})
	}
}
