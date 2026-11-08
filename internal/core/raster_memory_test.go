package core

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
)

// palettePage is a 600x400 page with a glass pane and, when open, what a
// command palette adds over it: a full-window backdrop blur and a faded
// panel with a shadow (set receives the setter of open).
func palettePage(set *func(bool)) Node {
	return Component(func() Node {
		open, setOpen := UseStateSet(false)
		*set = setOpen
		stripes := []Child{Class("absolute inset-0 flex")}
		for i := range 60 {
			c := "bg-slate-900"
			if i%2 == 1 {
				c = "bg-amber-300"
			}
			stripes = append(stripes, div(Key(i), Class("h-full w-[10px] shrink-0 "+c)))
		}
		kids := []Child{Class("relative h-screen overflow-hidden bg-linear-to-br from-indigo-500 to-rose-500"),
			div(stripes...),
			div(Class("absolute left-[40px] top-[40px] h-[200px] w-[260px] rounded-2xl bg-white/20 backdrop-blur-md")),
		}
		if open {
			kids = append(kids, div(Key("palette"), Class("absolute inset-0 bg-black/30 backdrop-blur-sm"),
				div(Class("absolute left-[150px] top-[80px] h-[220px] w-[300px] rounded-xl bg-white opacity-90 shadow-xl"))))
		}
		return div(kids...)
	})
}

// TestRasterMemoryAfterPalette opens and closes a palette-like overlay: the
// frame that closes it, after which nothing is pending, releases the
// rasterizer's scratch buffers and the overlay's filtered backdrop, and
// every frame equals a full rasterization.
func TestRasterMemoryAfterPalette(t *testing.T) {
	var set func(bool)
	tr := newTestTree(t, 600, 400, palettePage(&set), nil)
	tr.SetScale(2)
	tr.Frame()
	fb := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	ref := image.NewRGBA(fb.Rect)
	var r raster.Renderer
	frameBytes := 4 * fb.Rect.Dx() * fb.Rect.Dy()
	check := func(step string) {
		t.Helper()
		raster.Clear(ref, image.Rectangle{}, tr.CanvasColor())
		raster.Render(ref, tr.LastFrame(), raster.Options{Scale: 2})
		if p, ok := firstDiff(fb, ref); !ok {
			t.Fatalf("%s: the frame differs from a full rasterization at %v: %v, want %v", step, p, fb.RGBAAt(p.X, p.Y), ref.RGBAAt(p.X, p.Y))
		}
	}
	tr.RasterizeFrame(fb, &r, true)
	check("closed")
	closedCache := tr.lay.bdc.Bytes()
	for round := range 2 {
		set(true)
		tr.Frame()
		tr.RasterizeFrame(fb, &r, false)
		check("open")
		// The overlay's filtered window is a full frame's worth or more.
		if s := r.ScratchBytes(); s < frameBytes {
			t.Fatalf("round %d: the open palette left %d bytes of scratch, want at least a frame's %d (did the idle frame release what it used?)", round, s, frameBytes)
		}
		if c := tr.lay.bdc.Bytes(); c <= closedCache {
			t.Fatalf("round %d: the open palette's backdrop was not cached (%d bytes, %d closed)", round, c, closedCache)
		}
		set(false)
		tr.Frame()
		tr.RasterizeFrame(fb, &r, false)
		check("closed again")
		if s := r.ScratchBytes(); s >= frameBytes/4 {
			t.Errorf("round %d: after the palette closed, the renderer keeps %d bytes of scratch (a frame is %d)", round, s, frameBytes)
		}
		if c := tr.lay.bdc.Bytes(); c > closedCache {
			t.Errorf("round %d: after the palette closed, the backdrop cache holds %d bytes, %d before it opened", round, c, closedCache)
		}
	}
}
