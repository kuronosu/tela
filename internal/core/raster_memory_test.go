package core

import (
	"image"
	"testing"
	"time"

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
// frame that closes it leaves nothing pending, so a trim is due, and the
// trim releases the rasterizer's scratch buffers and the overlay's
// filtered backdrop; every frame equals a full rasterization.
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
			t.Fatalf("round %d: the open palette left %d bytes of scratch, want at least a frame's %d", round, s, frameBytes)
		}
		if c := tr.lay.bdc.Bytes(); c <= closedCache {
			t.Fatalf("round %d: the open palette's backdrop was not cached (%d bytes, %d closed)", round, c, closedCache)
		}
		set(false)
		tr.Frame()
		tr.RasterizeFrame(fb, &r, false)
		check("closed again")
		if !tr.RasterTrimDue(&r) {
			t.Fatalf("round %d: no trim is due after the frame that closed the palette", round)
		}
		tr.TrimRaster(&r)
		if s := r.ScratchBytes(); s != 0 {
			t.Errorf("round %d: after the trim, the renderer keeps %d bytes of scratch", round, s)
		}
		if c := tr.lay.bdc.Bytes(); c > closedCache {
			t.Errorf("round %d: after the trim, the backdrop cache holds %d bytes, %d before the palette opened", round, c, closedCache)
		}
		if tr.RasterTrimDue(&r) {
			t.Errorf("round %d: a trim is still due after it ran", round)
		}
	}
}

// TestHostRasterTrim checks the host's deferred trim: a frame that leaves
// nothing pending schedules it RasterSettle later, a frame before then
// moves it, and when it runs the renderer keeps no scratch, the cache no
// spare, no timer is left, and the frames after it are exact.
func TestHostRasterTrim(t *testing.T) {
	var set func(bool)
	x := newInput(t, palettePage(&set), 600, 400)
	x.settle()
	v := x.h.main
	set(true)
	x.settle()
	if v.renderer.ScratchBytes() == 0 || v.rasterTrimAt == 0 {
		t.Fatalf("after the palette opened: %d bytes of scratch, trim at %v", v.renderer.ScratchBytes(), v.rasterTrimAt)
	}
	first := v.rasterTrimAt

	// Before the deadline nothing is trimmed; a frame moves the deadline.
	x.b.Advance(RasterSettle / 2)
	x.b.Wake()
	x.b.Drain(x.h)
	if v.renderer.ScratchBytes() == 0 {
		t.Fatal("trimmed before the window rested for RasterSettle")
	}
	set(false)
	x.settle()
	due := v.rasterTrimAt
	if due <= first {
		t.Fatalf("the frame that closed the palette did not move the trim (%v, first %v)", due, first)
	}
	x.b.Advance(max(due-x.b.Now(), 0) - time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if v.renderer.ScratchBytes() == 0 {
		t.Fatal("trimmed before the moved deadline")
	}

	// The deadline: trimmed, and nothing left armed.
	x.b.Advance(2 * time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if s, sp := v.renderer.ScratchBytes(), x.tree().lay.bdc.SpareBytes(); s != 0 || sp != 0 {
		t.Fatalf("after the trim: %d bytes of scratch, %d of spare backdrops", s, sp)
	}
	if v.rasterTrimAt != 0 {
		t.Error("a trim is still scheduled")
	}
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("the timer is still armed after the trim")
	}

	// The frames after the trim are exact.
	set(true)
	x.settle()
	checkFramebuffer(t, x)
	set(false)
	x.settle()
	checkFramebuffer(t, x)
}

// TestHostRasterTrimWithoutPixels: the frame that leaves the tree at rest
// may draw nothing new (an animation whose last step changes no pixel), so
// it returns before rasterizing; it must still schedule the trim, or the
// window would keep the scratch memory of its last heavy frame.
func TestHostRasterTrimWithoutPixels(t *testing.T) {
	var set func(bool)
	x := newInput(t, palettePage(&set), 600, 400)
	x.settle()
	v := x.h.main
	set(true)
	x.settle()
	if v.renderer.ScratchBytes() == 0 {
		t.Fatal("the palette left no scratch memory to trim")
	}
	v.rasterTrimAt = 0 // as after a last rasterized frame that was still animating
	x.h.frame(v, x.b.Now())
	if v.rasterTrimAt == 0 {
		t.Fatal("a frame that drew nothing new did not schedule the trim")
	}
	x.b.Advance(RasterSettle + time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if s := v.renderer.ScratchBytes(); s != 0 {
		t.Fatalf("after the trim: %d bytes of scratch", s)
	}
}
