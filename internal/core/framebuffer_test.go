package core

import (
	"bytes"
	"image"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// fbRoot is a page whose pixels differ across the window, with a color
// that a state flips (an incremental frame).
func fbRoot(set *Setter[bool]) Node {
	return Component(func() Node {
		on, s := UseState(false)
		*set = s
		dot := "bg-blue-500"
		if on {
			dot = "bg-red-500"
		}
		return div(Class("size-full p-4 bg-linear-to-br from-amber-200 to-sky-700"),
			div(Class("size-6 rounded-full "+dot)), p(Text("Resize me")))
	})
}

// checkFramebuffer fails unless the host's framebuffer holds what the
// window presented and what a full rasterization of the last frame gives.
func checkFramebuffer(t *testing.T, x *inputHarness) {
	t.Helper()
	fb := x.h.main.fb
	if got := x.w.Frame(); got == nil || !bytes.Equal(got.Pix, fb.Pix) {
		t.Fatal("the framebuffer differs from the presented frame")
	}
	tr := x.tree()
	full := image.NewRGBA(fb.Rect)
	raster.Clear(full, image.Rectangle{}, tr.clearColor())
	var r raster.Renderer
	r.Render(full, tr.LastFrame(), raster.Options{Scale: tr.lay.cur.scale})
	if !bytes.Equal(full.Pix, fb.Pix) {
		t.Fatal("the framebuffer differs from a full rasterization")
	}
}

// TestFramebufferResize: the first framebuffer has exactly the room its
// size needs; a drag-resize, a new size on every frame, reallocates it a
// few times (growing by a quarter more than it needs), not on every
// frame; once the size has not changed for fbSettle, the framebuffer is
// trimmed to its size with the same pixels, and the frames after it are
// incremental and exact.
func TestFramebufferResize(t *testing.T) {
	var set Setter[bool]
	x := newInput(t, fbRoot(&set), 200, 100)
	x.settle()
	v := x.h.main
	if n := 4 * 200 * 100; len(v.fb.Pix) != n || cap(v.fb.Pix) != n {
		t.Fatalf("first framebuffer: len %d, cap %d, want %d", len(v.fb.Pix), cap(v.fb.Pix), n)
	}
	if v.fbTrimAt != 0 {
		t.Error("a trim is scheduled for an exact framebuffer")
	}

	// The drag: grows to 280 x 140 (twice the area) and back.
	sizes := make([]platform.Size, 0, 80)
	for i := 1; i <= 40; i++ {
		sizes = append(sizes, platform.Size{W: float32(200 + 2*i), H: float32(100 + i)})
	}
	for i := 39; i >= 0; i-- {
		sizes = append(sizes, platform.Size{W: float32(200 + 2*i), H: float32(100 + i)})
	}
	allocs, last := 0, unsafe.SliceData(v.fb.Pix)
	for _, s := range sizes {
		x.w.Resize(s)
		x.frame()
		if got := v.fb.Rect.Size(); got != image.Pt(int(s.W), int(s.H)) {
			t.Fatalf("framebuffer %v after a resize to %v", got, s)
		}
		if p := unsafe.SliceData(v.fb.Pix); p != last {
			allocs++
			last = p
		}
	}
	checkFramebuffer(t, x)
	t.Logf("%d reallocations in %d frames of a drag", allocs, len(sizes))
	// Growing by a quarter at a time doubles the area in about three
	// steps; shrinking reuses the memory.
	if allocs > 4 {
		t.Errorf("the drag reallocated the framebuffer %d times in %d frames", allocs, len(sizes))
	}
	if cap(v.fb.Pix) == len(v.fb.Pix) || v.fbTrimAt == 0 {
		t.Fatalf("after the drag: len %d, cap %d, trim at %v", len(v.fb.Pix), cap(v.fb.Pix), v.fbTrimAt)
	}

	// Not settled yet: nothing changes.
	before := bytes.Clone(v.fb.Pix)
	x.b.Advance(fbSettle - x.b.FrameInterval() - time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if cap(v.fb.Pix) == len(v.fb.Pix) {
		t.Fatal("trimmed before the size settled")
	}

	// Settled: trimmed, same pixels, no timer left.
	x.b.Advance(fbSettle)
	x.b.Wake()
	x.b.Drain(x.h)
	if n := 4 * 200 * 100; len(v.fb.Pix) != n || cap(v.fb.Pix) != n {
		t.Fatalf("after the size settled: len %d, cap %d, want %d", len(v.fb.Pix), cap(v.fb.Pix), n)
	}
	if !bytes.Equal(v.fb.Pix, before) {
		t.Fatal("the trim changed the pixels")
	}
	if v.fbTrimAt != 0 {
		t.Error("a trim is still scheduled")
	}
	x.h.timerMu.Lock()
	timer := x.h.timer
	x.h.timerMu.Unlock()
	if timer != nil {
		t.Error("the timer is still armed after the trim")
	}

	// The next frames rasterize only their damage, over the trimmed
	// buffer.
	set.Set(true)
	x.settle()
	if x.tree().LastDamage().Full {
		t.Error("the frame after the trim was rasterized in full")
	}
	checkFramebuffer(t, x)
}

// TestHiddenWindowDropsFramebuffer: a window the app hides lets its
// framebuffer go (the backend shows its own copy of the frame when the
// window is shown again); the next frame is rasterized in full into a new
// one.
func TestHiddenWindowDropsFramebuffer(t *testing.T) {
	var set Setter[bool]
	x := newInput(t, fbRoot(&set), 200, 100)
	x.settle()
	v := x.h.main
	v.setHidden(true)
	x.settle()
	if v.fb != nil {
		t.Fatal("a hidden window keeps its framebuffer")
	}
	v.setHidden(false)
	x.settle()
	set.Set(true)
	x.settle()
	if v.fb == nil || cap(v.fb.Pix) != 4*200*100 {
		t.Fatal("no exact framebuffer after the window is shown and changes")
	}
	if !x.tree().LastDamage().Full {
		t.Error("the first frame into the new framebuffer was not rasterized in full")
	}
	checkFramebuffer(t, x)
}
