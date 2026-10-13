package core

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// scrollList is bigList (about 10,000 Elements, not virtualized) inside a
// scroll container that fills the window.
func scrollList(setters []func(string)) Node {
	return div(Key("list"), Class("h-screen overflow-y-auto"), bigList(benchRows, setters))
}

// BenchmarkScrollFrame10k is one scroll frame of a 10k-element list: the
// offset moves by a wheel notch (instant) and Frame runs. Until layers
// (T7b) this is a paint-only frame that repaints the whole tree; it is the
// baseline T7b must beat.
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

// BenchmarkScrollFrameRaster10k is BenchmarkScrollFrame10k plus the
// rasterization of the 800x600 window at scale 2 into a reused
// framebuffer, as a Host does (full damage until T7b): the frame time the
// phase-4 target (p95 at most 16 ms) is about.
func BenchmarkScrollFrameRaster10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, scrollList(setters), nil)
	tr.SetScale(2)
	tr.Frame()
	s := keyed(tr, "list").box.scr
	phys := platform.PhysicalSize(platform.Size{W: 800, H: 600}, 2)
	fb := image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
	var r raster.Renderer
	dir := float32(wheelNotch)
	b.ReportAllocs()
	for b.Loop() {
		if s.pos.Y+dir > s.max.Y || s.pos.Y+dir < 0 {
			dir = -dir
		}
		tr.setScroll(s, s.pos.Add(gfx.Point{Y: dir}))
		dl, _ := tr.Frame()
		raster.Clear(fb, image.Rectangle{}, tr.CanvasColor())
		r.Render(fb, dl, raster.Options{Scale: 2})
	}
}
