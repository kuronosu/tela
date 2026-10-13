package core

import (
	"fmt"
	"image"
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// TestMain prints the peak resident memory of the test binary with
// TELA_PEAK_RSS=1 (the benchmarks' memory).
func TestMain(m *testing.M) {
	code := m.Run()
	if os.Getenv("TELA_PEAK_RSS") == "1" {
		if b, err := os.ReadFile("/proc/self/status"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(l, "VmHWM") {
					fmt.Println("peak RSS:", strings.TrimSpace(strings.TrimPrefix(l, "VmHWM:")))
				}
			}
		}
	}
	os.Exit(code)
}

// scrollList is bigList (about 10,000 Elements, not virtualized) inside a
// scroll container that fills the window.
func scrollList(setters []func(string)) Node {
	return div(Key("list"), Class("h-screen overflow-y-auto"), bigList(benchRows, setters))
}

// BenchmarkScrollFrame10k is one scroll frame of a 10k-element list: the
// offset moves by a wheel notch (instant) and Frame runs. It is a frame
// that only repaints: the boxes are placed again, the scrollbars recorded
// and the layers composed (layer.go); the list's content layer is
// recorded again only when the visible part leaves its interest rect.
func BenchmarkScrollFrame10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, scrollList(setters), nil)
	tr.SetScale(2)
	tr.Frame()
	s := keyed(tr, "list").box.scr
	if s == nil || s.max.Y < 10*wheelNotch {
		b.Fatalf("no scroll range: %+v", s)
	}
	dir := float32(wheelNotch)
	b.ReportAllocs()
	for b.Loop() {
		if s.pos.Y+dir > s.max.Y || s.pos.Y+dir < 0 {
			dir = -dir
		}
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: dir}))
		if _, changed := tr.Frame(); !changed {
			b.Fatal("the frame did not repaint")
		}
	}
}

// rasterBench runs Frame after change on every iteration and brings a
// framebuffer to it with RasterizeFrame, as a Host does (full: a full
// rasterization every frame, the behavior before damage tracking). It
// reports the rasterized device pixels per frame.
func rasterBench(b *testing.B, tr *Tree, w, h float32, scale float64, full bool, change func()) {
	b.Helper()
	tr.SetScale(float32(scale))
	phys := platform.PhysicalSize(platform.Size{W: w, H: h}, scale)
	fb := image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
	var r raster.Renderer
	tr.Frame()
	tr.RasterizeFrame(fb, &r, true)
	pixels, shifted := 0, 0
	b.ReportAllocs()
	for b.Loop() {
		change()
		tr.Frame()
		d := tr.RasterizeFrame(fb, &r, full)
		if d.Full {
			pixels += fb.Rect.Dx() * fb.Rect.Dy()
		}
		for _, rc := range d.Rects {
			pixels += rc.Dx() * rc.Dy()
		}
		for _, s := range d.Shifts {
			shifted += s.Rect.Dx() * s.Rect.Dy()
		}
	}
	b.ReportMetric(float64(pixels)/float64(b.N), "rasterpx/op")
	b.ReportMetric(float64(shifted)/float64(b.N), "shiftpx/op")
}

// scrollChange returns a change that moves the list of tr by a wheel
// notch, back and forth.
func scrollChange(b *testing.B, tr *Tree) func() {
	tr.Frame()
	s := keyed(tr, "list").box.scr
	if s == nil || s.max.Y < 10*wheelNotch {
		b.Fatalf("no scroll range: %+v", s)
	}
	dir := float32(wheelNotch)
	return func() {
		if s.pos.Y+dir > s.max.Y || s.pos.Y+dir < 0 {
			dir = -dir
		}
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: dir}))
	}
}

// BenchmarkScrollFrameRaster10k is BenchmarkScrollFrame10k plus bringing
// the 800x600 window's framebuffer at scale 2 to the frame (a shift and
// the exposed strip): the frame time the phase-4 target (p95 at most 16
// ms) is about.
func BenchmarkScrollFrameRaster10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, scrollList(setters), nil)
	rasterBench(b, tr, 800, 600, 2, false, scrollChange(b, tr))
}

// BenchmarkScrollFrameRasterFull10k is BenchmarkScrollFrameRaster10k with
// a full rasterization of every frame.
func BenchmarkScrollFrameRasterFull10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, scrollList(setters), nil)
	rasterBench(b, tr, 800, 600, 2, true, scrollChange(b, tr))
}
