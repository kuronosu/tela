//go:build linux

package wayland

import (
	"errors"
	"image"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// openWindow creates a window, starts the loop and answers the initial
// commit with a configure of width×height. It returns the window after its
// ConfigureEvent.
func openWindow(t *testing.T, h *harness, opts platform.WindowOptions, width, height int32, serial uint32, states ...protocols.XdgToplevelState) *Window {
	t.Helper()
	win, err := h.b.NewWindow(opts)
	if err != nil {
		t.Fatal(err)
	}
	h.start()
	setup := h.fc.until("wl_surface.commit")
	for _, want := range []string{"wl_compositor.create_surface", "xdg_wm_base.get_xdg_surface", "xdg_surface.get_toplevel",
		"xdg_toplevel.set_app_id", "xdg_toplevel.set_min_size", "xdg_toplevel.set_max_size",
		"zxdg_decoration_manager_v1.get_toplevel_decoration", "zxdg_toplevel_decoration_v1.set_mode"} {
		if _, i := find(setup, want); i < 0 {
			t.Errorf("window setup %v lacks %s", names(setup), want)
		}
	}
	if r, _ := find(setup, "zxdg_toplevel_decoration_v1.set_mode"); r.name != "" && r.u(0) != uint32(protocols.ZxdgToplevelDecorationV1ModeServerSide) {
		t.Errorf("decoration mode = %v, want server side", r)
	}
	if _, i := find(setup, "wl_surface.attach"); i >= 0 {
		t.Errorf("a buffer was attached before the first configure: %v", names(setup))
	}
	h.configure(width, height, serial, states...)
	return win.(*Window)
}

func TestConfigureAckSequence(t *testing.T) {
	h := newHarness(t, standardGlobals(2, protocols.WlShmFormatXrgb8888))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		if err := w.Present(solid(w, 0x11, 0x22, 0x33), nil); err != nil {
			t.Errorf("Present: %v", err)
		}
	}
	w := openWindow(t, h, platform.WindowOptions{Title: "t", Size: platform.Size{W: 300, H: 200}}, 400, 300, 10, protocols.XdgToplevelStateActivated)

	ce := expect[platform.ConfigureEvent](h)
	if want := (platform.Metrics{Size: platform.Size{W: 400, H: 300}, Scale: 2}); ce.Metrics != want {
		t.Errorf("first ConfigureEvent = %v, want %v (size from the compositor, scale guessed from the output)", ce.Metrics, want)
	}
	if fe := expect[platform.FocusEvent](h); !fe.Focused {
		t.Errorf("FocusEvent = %+v", fe)
	}
	expect[platform.FrameEvent](h)

	// The first frame: ack right before the commit that carries the
	// buffer, set_buffer_scale(2), an 800x600 buffer, full damage, a frame
	// callback.
	reqs := h.fc.until("wl_surface.commit")
	_, iAck := find(reqs, "xdg_surface.ack_configure")
	_, iAttach := find(reqs, "wl_surface.attach")
	if iAck < 0 || iAttach < 0 || iAck > iAttach || reqs[iAck].u(0) != 10 {
		t.Fatalf("first frame requests %v: want ack_configure(10) before attach", names(reqs))
	}
	if r, i := find(reqs, "wl_surface.set_buffer_scale"); i < 0 || r.i(0) != 2 {
		t.Errorf("set_buffer_scale = %v, want 2", r)
	}
	if r, _ := find(reqs, "wl_surface.damage_buffer"); r.name == "" || r.i(0) != 0 || r.i(1) != 0 || r.i(2) != 800 || r.i(3) != 600 {
		t.Errorf("damage_buffer = %v, want full 800x600", r)
	}
	if _, i := find(reqs, "wl_surface.frame"); i < 0 {
		t.Errorf("no frame callback requested: %v", names(reqs))
	}
	attach, _ := find(reqs, "wl_surface.attach")
	buf := h.fc.buffer(attach.u(0))
	if buf.w != 800 || buf.h != 600 || buf.stride != 3200 || buf.format != uint32(protocols.WlShmFormatXrgb8888) {
		t.Errorf("buffer = %+v, want 800x600 XRGB8888", buf)
	}
	px := h.fc.pixels(attach.u(0))
	if px[0] != 0x33 || px[1] != 0x22 || px[2] != 0x11 {
		t.Errorf("XRGB8888 pixel bytes = % x, want 33 22 11 (B, G, R)", px[:4])
	}

	// A resize: the ConfigureEvent precedes the frame, and the new serial
	// is acked with the first buffer of the new size.
	h.configure(500, 300, 11, protocols.XdgToplevelStateActivated)
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 500, H: 300}) {
		t.Errorf("resize ConfigureEvent = %v", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if r, i := find(reqs, "xdg_surface.ack_configure"); i < 0 || r.u(0) != 11 {
		t.Errorf("resize frame %v: want ack_configure(11)", names(reqs))
	}
	attach, _ = find(reqs, "wl_surface.attach")
	if b := h.fc.buffer(attach.u(0)); b.w != 1000 || b.h != 600 {
		t.Errorf("resized buffer %dx%d, want 1000x600", b.w, b.h)
	}

	// Same size (only the activated state changes): no event for the app,
	// ack and commit at once, without a buffer.
	h.configure(500, 300, 12)
	if fe := expect[platform.FocusEvent](h); fe.Focused {
		t.Errorf("FocusEvent = %+v, want unfocused", fe)
	}
	reqs = h.fc.until("wl_surface.commit")
	if !slices.Equal(names(reqs), []string{"xdg_surface.ack_configure", "wl_surface.commit"}) || reqs[0].u(0) != 12 {
		t.Errorf("no-op configure answered with %v, want ack_configure(12) + commit", reqs)
	}

	// Two configures before the app draws: one ConfigureEvent per change,
	// and only the newest serial is acked, with the frame.
	h.fc.sendRaw(append(h.configureMsg(600, 300, 13), h.configureMsg(600, 300, 14)...))
	if ce := expect[platform.ConfigureEvent](h); ce.Size != (platform.Size{W: 600, H: 300}) {
		t.Errorf("ConfigureEvent = %v", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	var acks []uint32
	for _, r := range reqs {
		if r.is("xdg_surface.ack_configure") {
			acks = append(acks, r.u(0))
		}
	}
	if !slices.Equal(acks, []uint32{14}) {
		t.Errorf("acks = %v, want [14]", acks)
	}

	// 0x0 while floating: the client picks, so the size is kept.
	h.configure(0, 0, 15)
	reqs = h.fc.until("wl_surface.commit")
	if reqs[0].u(0) != 15 {
		t.Errorf("0x0 configure: %v", reqs)
	}
	h.noEvent(50 * time.Millisecond)
	h.ui(func() {
		if m := w.Metrics(); m.Size != (platform.Size{W: 600, H: 300}) {
			t.Errorf("Metrics after 0x0 = %v", m)
		}
	})
}

func TestScaleChangeAndXBGR(t *testing.T) {
	h := newHarness(t, standardGlobals(1, protocols.WlShmFormatXrgb8888, protocols.WlShmFormatXbgr8888))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		w.Present(solid(w, 0x11, 0x22, 0x33), nil)
	}
	openWindow(t, h, platform.WindowOptions{}, 100, 50, 1)
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 1 {
		t.Errorf("scale = %v, want 1", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	if _, i := find(reqs, "wl_surface.set_buffer_scale"); i >= 0 {
		t.Errorf("set_buffer_scale sent for the default scale 1: %v", names(reqs))
	}
	attach, _ := find(reqs, "wl_surface.attach")
	b := h.fc.buffer(attach.u(0))
	if b.format != uint32(protocols.WlShmFormatXbgr8888) {
		t.Errorf("format %#x, want XBGR8888 when advertised", b.format)
	}
	if px := h.fc.pixels(attach.u(0)); px[0] != 0x11 || px[1] != 0x22 || px[2] != 0x33 {
		t.Errorf("XBGR8888 bytes = % x, want 11 22 33 (R, G, B)", px[:4])
	}

	// The compositor prefers scale 2: same logical size, new scale, no ack
	// needed, set_buffer_scale(2) with the 200x100 buffer.
	h.fc.send(h.fc.id("wl_surface"), 2, int32(2)) // preferred_buffer_scale
	ce := expect[platform.ConfigureEvent](h)
	if want := (platform.Metrics{Size: platform.Size{W: 100, H: 50}, Scale: 2}); ce.Metrics != want {
		t.Errorf("ConfigureEvent = %v, want %v", ce.Metrics, want)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if r, i := find(reqs, "wl_surface.set_buffer_scale"); i < 0 || r.i(0) != 2 {
		t.Errorf("requests %v: want set_buffer_scale(2)", names(reqs))
	}
	if _, i := find(reqs, "xdg_surface.ack_configure"); i >= 0 {
		t.Errorf("a scale change acked a configure: %v", names(reqs))
	}
	attach, _ = find(reqs, "wl_surface.attach")
	if b := h.fc.buffer(attach.u(0)); b.w != 200 || b.h != 100 {
		t.Errorf("buffer %dx%d, want 200x100", b.w, b.h)
	}
}

func TestOutputScaleFallback(t *testing.T) {
	// A compositor before wl_compositor 6 has no preferred_buffer_scale:
	// the scale comes from the outputs the surface entered.
	g := standardGlobals(1)
	g[0].version = 5
	h := newHarness(t, g)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(solid(w, 1, 2, 3), nil) }
	openWindow(t, h, platform.WindowOptions{}, 100, 50, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")

	surface, output := h.fc.id("wl_surface"), h.fc.id("wl_output")
	h.fc.send(surface, 0, output) // enter: scale 1, no change
	h.noEvent(50 * time.Millisecond)
	h.fc.send(output, 3, int32(3)) // scale 3 ...
	h.fc.send(output, 2)           // ... applied on done
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 3 {
		t.Errorf("scale = %v, want 3", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	attach, _ := find(reqs, "wl_surface.attach")
	if b := h.fc.buffer(attach.u(0)); b.w != 300 || b.h != 150 {
		t.Errorf("buffer %dx%d, want 300x150", b.w, b.h)
	}
}

func TestOldCompositorDamage(t *testing.T) {
	// wl_compositor 3 has set_buffer_scale but not damage_buffer: damage
	// goes in surface (logical) coordinates, rounded outward.
	g := standardGlobals(2)
	g[0].version = 3
	h := newHarness(t, g)
	w := openWindow(t, h, platform.WindowOptions{}, 10, 10, 1)
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 2 {
		t.Fatalf("scale %v, want 2", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	h.ui(func() { w.Present(solid(w, 1, 2, 3), nil) })
	reqs := h.fc.until("wl_surface.commit")
	if r, _ := find(reqs, "wl_surface.damage"); r.name == "" || r.i(2) != 10 || r.i(3) != 10 {
		t.Errorf("full damage = %v, want 10x10 logical", r)
	}
	h.ui(func() { w.Present(solid(w, 1, 2, 3), []image.Rectangle{image.Rect(3, 3, 6, 6)}) })
	reqs = h.fc.until("wl_surface.commit")
	if r, _ := find(reqs, "wl_surface.damage"); r.name == "" || r.i(0) != 1 || r.i(1) != 1 || r.i(2) != 2 || r.i(3) != 2 {
		t.Errorf("damage of physical (3,3)-(6,6) = %v, want logical (1,1) 2x2", r)
	}
	if _, i := find(reqs, "wl_surface.damage_buffer"); i >= 0 {
		t.Errorf("damage_buffer sent to a version 3 compositor")
	}
}

func TestFramePacing(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	var win platform.Window
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		win = w
		w.Present(solid(w, 9, 9, 9), nil)
		w.RequestFrame() // animate
	}
	openWindow(t, h, platform.WindowOptions{}, 64, 64, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	frame, _ := find(h.fc.until("wl_surface.commit"), "wl_surface.frame")

	// The animation waits for the frame callback.
	h.noEvent(100 * time.Millisecond)
	h.fc.send(frame.u(0), 0, uint32(1234)) // done
	expect[platform.FrameEvent](h)
	frame2, i := find(h.fc.until("wl_surface.commit"), "wl_surface.frame")
	if i < 0 || frame2.u(0) == frame.u(0) {
		t.Fatalf("second frame did not request a new callback")
	}

	// Stop animating: after the callback, nothing more arrives.
	h.ui(func() { h.onFrame = func(w platform.Window, _ platform.FrameEvent) {} })
	h.fc.send(frame2.u(0), 0, uint32(1240))
	expect[platform.FrameEvent](h)
	h.noEvent(100 * time.Millisecond)

	// Requests coalesce, and with no callback outstanding the frame is
	// delivered on the next iteration.
	h.ui(func() {
		for range 5 {
			win.RequestFrame()
		}
	})
	expect[platform.FrameEvent](h)
	h.noEvent(100 * time.Millisecond)
}

// loopIterations reads the backend's loop counter on the UI thread.
func (h *harness) loopIterations() int {
	h.t.Helper()
	n := 0
	h.ui(func() { n = h.b.iterations })
	return n
}

// stopRunaway wraps onFrame so that a backend delivering frames without
// pacing quits Run after 500 of them. Otherwise the flood would fill the
// events channel, block the handler, and hang the test instead of failing
// it.
func (h *harness) stopRunaway() {
	inner := h.onFrame
	n := 0
	h.onFrame = func(w platform.Window, e platform.FrameEvent) {
		if n++; n == 500 {
			h.t.Errorf("500 FrameEvents: frames are not paced; quitting the loop")
			h.b.Quit()
		}
		inner(w, e)
	}
}

// TestAnimationWithoutPresentWaitsForCallbacks is the regression test for a
// 100% CPU spin: a FrameEvent handler that requests the next frame without
// presenting (an animation tick with no pixel change) left no frame
// callback outstanding, so every request was due at once and poll ran with
// a zero timeout. The backend must request a callback with a commit that
// attaches no buffer, and deliver the frame when it fires.
func TestAnimationWithoutPresentWaitsForCallbacks(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	presented := false
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		if !presented { // map the window once, then only tick
			presented = true
			w.Present(solid(w, 1, 2, 3), nil)
		}
		w.RequestFrame()
	}
	h.stopRunaway()
	openWindow(t, h, platform.WindowOptions{}, 64, 64, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	cb, i := find(h.fc.until("wl_surface.commit"), "wl_surface.frame")
	if i < 0 {
		t.Fatal("the presented frame requested no callback")
	}

	for tick := range 3 {
		// No FrameEvent until the outstanding callback fires, and the loop
		// sleeps in poll meanwhile.
		before := h.loopIterations()
		h.noEvent(100 * time.Millisecond)
		if n := h.loopIterations() - before; n > 10 {
			t.Fatalf("tick %d: %d loop iterations in 100 ms while waiting for a frame callback: the loop spins", tick, n)
		}
		h.fc.send(cb.u(0), 0, uint32(1000+tick)) // done
		expect[platform.FrameEvent](h)

		// The handler did not present: the backend asks for the next
		// callback itself, with a commit that attaches nothing.
		reqs := h.fc.until("wl_surface.commit")
		if cb, i = find(reqs, "wl_surface.frame"); i < 0 {
			t.Fatalf("tick %d: requests %v lack a frame callback", tick, names(reqs))
		}
		for _, bad := range []string{"wl_surface.attach", "wl_surface.damage_buffer", "xdg_surface.ack_configure"} {
			if _, j := find(reqs, bad); j >= 0 {
				t.Errorf("tick %d: the pacing commit carries %s: %v", tick, bad, names(reqs))
			}
		}
	}

	// Stop animating: the last callback's FrameEvent arrives, then
	// nothing, and no more commits.
	h.ui(func() { h.onFrame = func(platform.Window, platform.FrameEvent) {} })
	h.fc.send(cb.u(0), 0, uint32(2000))
	expect[platform.FrameEvent](h)
	h.noEvent(100 * time.Millisecond)
	h.fc.quiet(50*time.Millisecond, "wl_surface.commit")
}

// TestAnimationWithoutBufferIsTimerPaced covers the same handler before the
// window has a buffer: an unmapped surface gets no frame callbacks, so the
// requests are paced by the 60 Hz timer instead of spinning.
func TestAnimationWithoutBufferIsTimerPaced(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.RequestFrame() }
	h.stopRunaway()
	openWindow(t, h, platform.WindowOptions{}, 64, 64, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)

	before := h.loopIterations()
	const window = 250 * time.Millisecond
	deadline := time.After(window)
	frames := 0
collect:
	for {
		select {
		case r := <-h.events:
			if _, ok := r.ev.(platform.FrameEvent); !ok {
				t.Fatalf("unexpected event %T", r.ev)
			}
			frames++
		case <-deadline:
			break collect
		}
	}
	iters := h.loopIterations() - before
	// 60 Hz gives 15 frames in 250 ms; a spinning loop gives thousands.
	if frames < 3 || frames > 25 {
		t.Errorf("%d FrameEvents in %v without a buffer, want about 15 (60 Hz)", frames, window)
	}
	if iters > 3*frames+10 {
		t.Errorf("%d loop iterations for %d frames: the loop does not sleep between frames", iters, frames)
	}
	// Nothing was committed: an unmapped surface cannot take a callback.
	h.fc.quiet(10*time.Millisecond, "wl_surface.commit")
}

func TestPresentDamageAndBufferReuse(t *testing.T) {
	h := newHarness(t, standardGlobals(1, protocols.WlShmFormatXbgr8888, protocols.WlShmFormatXrgb8888))
	w := platform.Window(openWindow(t, h, platform.WindowOptions{}, 8, 8, 1))
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)

	// want is the frame as the user should see it.
	want := image.NewRGBA(image.Rect(0, 0, 8, 8))
	present := func(damage image.Rectangle, r, g uint8) uint32 {
		t.Helper()
		fb := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for i := 0; i < len(fb.Pix); i += 4 { // garbage outside the damage
			fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = 0xff, 0x00, 0xff, 0xff
		}
		for y := damage.Min.Y; y < damage.Max.Y; y++ {
			for x := damage.Min.X; x < damage.Max.X; x++ {
				i := fb.PixOffset(x, y)
				fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2] = r, g, 0
				want.Pix[i], want.Pix[i+1], want.Pix[i+2], want.Pix[i+3] = r, g, 0, 0xff
			}
		}
		var d []image.Rectangle
		if damage != fb.Rect {
			d = []image.Rectangle{damage}
		}
		h.ui(func() {
			if err := w.Present(fb, d); err != nil {
				t.Errorf("Present: %v", err)
			}
		})
		reqs := h.fc.until("wl_surface.commit")
		a, _ := find(reqs, "wl_surface.attach")
		dmg, _ := find(reqs, "wl_surface.damage_buffer")
		if got := image.Rect(int(dmg.i(0)), int(dmg.i(1)), int(dmg.i(0)+dmg.i(2)), int(dmg.i(1)+dmg.i(3))); got != damage {
			t.Errorf("damage_buffer %v, want %v", got, damage)
		}
		// The buffer must hold the whole expected frame.
		px := h.fc.pixels(a.u(0))
		for y := range 8 {
			for x := range 8 {
				i := want.PixOffset(x, y)
				if px[i] != want.Pix[i] || px[i+1] != want.Pix[i+1] || px[i+2] != want.Pix[i+2] {
					t.Fatalf("buffer %d pixel (%d,%d) = % x, want % x", a.u(0), x, y, px[i:i+3], want.Pix[i:i+3])
				}
			}
		}
		return a.u(0)
	}
	release := func(id uint32) { h.fc.send(id, 0) }

	b1 := present(image.Rect(0, 0, 8, 8), 10, 10) // full
	b2 := present(image.Rect(0, 0, 2, 2), 20, 20) // b1 busy: new buffer
	if b2 == b1 {
		t.Fatalf("a busy buffer was reused")
	}
	b3 := present(image.Rect(6, 6, 8, 8), 30, 30) // both busy: third buffer
	if b3 == b1 || b3 == b2 {
		t.Fatalf("a busy buffer was reused")
	}
	release(b1)
	time.Sleep(20 * time.Millisecond)
	// b1 is two frames behind: it must get both missed regions from the
	// previous buffer, and the new damage from the framebuffer.
	if b := present(image.Rect(3, 0, 5, 8), 40, 40); b != b1 {
		t.Errorf("released buffer %d not reused (got %d)", b1, b)
	}
	release(b2)
	release(b3)
	time.Sleep(20 * time.Millisecond)
	present(image.Rect(0, 7, 8, 8), 50, 50)
	present(image.Rect(1, 1, 7, 7), 60, 60)

	// No more than three buffers were created.
	created := 0
	h.ui(func() { created = len(w.(*Window).chain.bufs) })
	if created != 3 {
		t.Errorf("swapchain has %d buffers, want 3", created)
	}
}

// TestCloseAfterResizeUnmapsEveryBuffer: the compositor still holds the
// buffer of the old size (no wl_buffer.release yet) when the window is
// resized and then closed. Every mapping must be gone after Close, and
// every wl_buffer destroyed.
func TestCloseAfterResizeUnmapsEveryBuffer(t *testing.T) {
	baseline := shmMappings.Load()
	h := newHarness(t, standardGlobals(1))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(solid(w, 1, 2, 3), nil) }
	w := openWindow(t, h, platform.WindowOptions{}, 40, 30, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	first, _ := find(h.fc.until("wl_surface.commit"), "wl_surface.attach")

	h.configure(50, 30, 2)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	h.ui(func() {
		if n := len(w.chain.retired); n != 1 {
			t.Errorf("retired buffers after the resize = %d, want 1 (the busy old buffer)", n)
		}
		if n := shmMappings.Load() - baseline; n != 2 {
			t.Errorf("mapped buffers = %d, want 2", n)
		}
		w.Close()
		if n := shmMappings.Load() - baseline; n != 0 {
			t.Errorf("%d shm buffers still mapped after Close", n)
		}
	})
	// Both wl_buffers are destroyed, the retired one included. (A late
	// release after Close is covered by TestSwapchainDestroyFreesRetired-
	// Buffers: this scripted fake answers destroy with delete_id at once,
	// after which a real compositor sends no more events for the buffer.)
	var destroyed []uint32
	for _, r := range h.fc.quiet(100*time.Millisecond, "") {
		if r.is("wl_buffer.destroy") {
			destroyed = append(destroyed, r.id)
		}
	}
	if len(destroyed) != 2 || !slices.Contains(destroyed, first.u(0)) {
		t.Errorf("wl_buffer.destroy for %v, want 2 buffers including the retired %d", destroyed, first.u(0))
	}
}

func TestPointerAndCursor(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	w := openWindow(t, h, platform.WindowOptions{}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	h.ui(func() { w.Present(solid(w, 0, 0, 0), nil) })
	h.fc.until("wl_surface.commit")

	ptr, surface := h.fc.id("wl_pointer"), h.fc.id("wl_surface")
	h.fc.send(ptr, 0, uint32(77), surface, 10.5, 20.25) // enter
	h.fc.send(ptr, 5)                                   // frame
	pe := expect[platform.PointerEvent](h)
	if pe.Action != platform.PointerEnter || pe.Pos != (platform.Point{X: 10.5, Y: 20.25}) {
		t.Errorf("enter = %+v", pe)
	}
	// The cursor is set on enter with the enter serial.
	if r := h.fc.until("wp_cursor_shape_device_v1.set_shape"); r[len(r)-1].u(0) != 77 || r[len(r)-1].u(1) != uint32(protocols.WpCursorShapeDeviceV1ShapeDefault) {
		t.Errorf("set_shape = %v", r[len(r)-1])
	}

	h.fc.send(ptr, 2, uint32(1000), 11.0, 21.0)                             // motion
	h.fc.send(ptr, 3, uint32(78), uint32(1001), uint32(btnLeft), uint32(1)) // press
	h.fc.send(ptr, 5)                                                       // frame
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerMove || pe.Pos.X != 11 {
		t.Errorf("motion = %+v", pe)
	}
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerDown || pe.Button != platform.ButtonPrimary || !pe.Buttons.Has(platform.ButtonPrimary) {
		t.Errorf("press = %+v", pe)
	}

	// A diagonal wheel notch in one frame is one ScrollEvent.
	h.fc.send(ptr, 6, uint32(protocols.WlPointerAxisSourceWheel)) // axis_source
	h.fc.send(ptr, 4, uint32(1002), uint32(0), 15.0)              // axis vertical
	h.fc.send(ptr, 9, uint32(0), int32(120))                      // value120 vertical
	h.fc.send(ptr, 4, uint32(1002), uint32(1), -7.5)              // axis horizontal
	h.fc.send(ptr, 9, uint32(1), int32(-60))                      // value120 horizontal
	h.fc.send(ptr, 5)                                             // frame
	se := expect[platform.ScrollEvent](h)
	if se.Delta != (platform.Point{X: -7.5, Y: 15}) || se.Steps != (platform.Point{X: -0.5, Y: 1}) || se.Pos != (platform.Point{X: 11, Y: 21}) {
		t.Errorf("scroll = %+v", se)
	}

	// SetCursor applies at once while the pointer is inside.
	h.ui(func() { w.SetCursor(platform.CursorText) })
	if r := h.fc.until("wp_cursor_shape_device_v1.set_shape"); r[len(r)-1].u(1) != uint32(protocols.WpCursorShapeDeviceV1ShapeText) {
		t.Errorf("set_shape = %v", r[len(r)-1])
	}
	h.ui(func() { w.SetCursor(platform.CursorNone) })
	if r := h.fc.until("wl_pointer.set_cursor"); r[len(r)-1].u(0) != 77 || r[len(r)-1].u(1) != 0 {
		t.Errorf("hide = %v, want set_cursor(77, NULL)", r[len(r)-1])
	}

	// Leaving with a button held: the press is cancelled.
	h.fc.send(ptr, 1, uint32(79), surface) // leave
	h.fc.send(ptr, 5)
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerCancel || pe.Buttons != 0 {
		t.Errorf("leave with a button held = %+v, want cancel", pe)
	}
	if pe := expect[platform.PointerEvent](h); pe.Action != platform.PointerLeave {
		t.Errorf("leave = %+v", pe)
	}
	h.ui(func() {
		if h.b.lastSerial != 79 {
			t.Errorf("lastSerial = %d, want 79", h.b.lastSerial)
		}
	})
}

func TestPingCloseAndClipboard(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	openWindow(t, h, platform.WindowOptions{}, 50, 50, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)

	h.fc.send(h.fc.id("xdg_wm_base"), 0, uint32(4242)) // ping
	if r := h.fc.until("xdg_wm_base.pong"); r[len(r)-1].u(0) != 4242 {
		t.Errorf("pong = %v", r[len(r)-1])
	}
	h.fc.send(h.fc.id("xdg_toplevel"), 1) // close
	r := h.event()
	if _, ok := r.ev.(platform.CloseRequestedEvent); !ok {
		t.Fatalf("got %T, want CloseRequestedEvent", r.ev)
	}

	var gotErr error
	inside := false
	called := make(chan struct{})
	h.ui(func() {
		inside = true
		h.b.Clipboard().ReadText(func(s string, err error) {
			if inside {
				t.Errorf("ReadText called back synchronously")
			}
			gotErr = err
			close(called)
		})
		inside = false
		if err := h.b.Clipboard().WriteText("x"); !errors.Is(err, errClipboard) {
			t.Errorf("WriteText = %v", err)
		}
	})
	h.b.Wake() // run another loop iteration
	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("ReadText never called back")
	}
	if !errors.Is(gotErr, errClipboard) {
		t.Errorf("ReadText error = %v", gotErr)
	}

	// Close destroys the objects in a valid order.
	h.ui(func() { r.w.Close() })
	reqs := h.fc.until("wl_surface.destroy")
	var order []string
	for _, n := range names(reqs) {
		switch n {
		case "zxdg_toplevel_decoration_v1.destroy", "xdg_toplevel.destroy", "xdg_surface.destroy", "wl_surface.destroy":
			order = append(order, n)
		}
	}
	if !slices.Equal(order, []string{"zxdg_toplevel_decoration_v1.destroy", "xdg_toplevel.destroy", "xdg_surface.destroy", "wl_surface.destroy"}) {
		t.Errorf("destroy order %v", order)
	}
}

func TestProtocolErrorEndsRun(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	openWindow(t, h, platform.WindowOptions{}, 50, 50, 1)
	expect[platform.ConfigureEvent](h)
	h.fc.send(wire.DisplayID, 0, h.fc.id("xdg_surface"), uint32(3), "bad things")
	select {
	case err := <-h.done:
		var pe *wire.ProtocolError
		if !errors.As(err, &pe) || pe.Code != 3 || pe.Interface != "xdg_surface" {
			t.Errorf("Run = %v, want the protocol error", err)
		}
		h.done <- err // for the cleanup
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after a protocol error")
	}
}

func TestDisconnectEndsRun(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	h.start()
	time.Sleep(20 * time.Millisecond)
	h.fc.wmu.Lock()
	err := unix.Shutdown(h.fc.fd, unix.SHUT_WR)
	h.fc.wmu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-h.done:
		if err == nil {
			t.Errorf("Run returned nil after the compositor hung up")
		}
		h.done <- err
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after a hang-up")
	}
}

// TestConformance runs the platform conformance suite against the fake
// compositor in automatic mode, so the contract is checked without a
// display. TestLiveConformance runs it against the real compositor.
func TestConformance(t *testing.T) {
	platformtest.Run(t, platformtest.Config{
		New: func(t *testing.T) platform.Backend {
			fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := wire.NewConn(fds[0])
			if err != nil {
				t.Fatal(err)
			}
			fc := startFake(t, fds[1], standardGlobals(2), newAutoState())
			b, err := newBackend(conn)
			if err != nil {
				t.Fatalf("newBackend: %v", err)
			}
			t.Cleanup(func() {
				// Run has returned and closed the client end.
				select {
				case <-fc.closed:
				case <-time.After(3 * time.Second):
					t.Errorf("fake compositor did not see the client hang up")
				}
				fc.closeFd()
			})
			return b
		},
		Skip: []string{"Clipboard"}, // phase 5
	})
}

func TestMissingGlobalIsUnavailable(t *testing.T) {
	g := standardGlobals(1)
	g = slices.DeleteFunc(g, func(x fakeGlobal) bool { return x.iface == "xdg_wm_base" })
	_, err := newBackendOnFake(t, g)
	if !errors.Is(err, platform.ErrUnavailable) {
		t.Errorf("newBackend without xdg_wm_base = %v, want ErrUnavailable", err)
	}
}

func TestNewWithoutDisplay(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("WAYLAND_SOCKET", "")
	if _, err := New(); !errors.Is(err, platform.ErrUnavailable) {
		t.Errorf("New() = %v, want ErrUnavailable", err)
	}
	t.Setenv("WAYLAND_DISPLAY", "tela-no-such-display")
	if _, err := New(); !errors.Is(err, platform.ErrUnavailable) {
		t.Errorf("New() with a bad display = %v, want ErrUnavailable", err)
	}
}
