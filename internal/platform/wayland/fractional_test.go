//go:build linux

package wayland

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// fractionalGlobals is standardGlobals plus wp_fractional_scale_v1 and
// wp_viewporter, like Hyprland, KWin and Mutter.
func fractionalGlobals(outputScale int32) []fakeGlobal {
	return append(standardGlobals(outputScale),
		fakeGlobal{iface: "wp_fractional_scale_manager_v1", version: 1},
		fakeGlobal{iface: "wp_viewporter", version: 1},
	)
}

// preferredScale sends wp_fractional_scale_v1.preferred_scale (n/120) for
// the latest window.
func (h *harness) preferredScale(n uint32) {
	h.fc.send(h.fc.id("wp_fractional_scale_v1"), 0, n)
}

// presented returns the buffer committed by the requests of one frame.
func (h *harness) presented(reqs []freq) fakeBuffer {
	h.t.Helper()
	attach, i := find(reqs, "wl_surface.attach")
	if i < 0 {
		h.t.Fatalf("no buffer attached: %v", names(reqs))
	}
	return h.fc.buffer(attach.u(0))
}

func TestFractionalScale(t *testing.T) {
	h := newHarness(t, fractionalGlobals(2))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) {
		if err := w.Present(solid(w, 1, 2, 3), nil); err != nil {
			t.Errorf("Present: %v", err)
		}
	}
	win, err := h.b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	h.start()

	// Both objects exist before the initial commit, so a scale sent at once
	// and the destination of the first buffer are never missed.
	setup := h.fc.until("wl_surface.commit")
	_, iView := find(setup, "wp_viewporter.get_viewport")
	_, iFrac := find(setup, "wp_fractional_scale_manager_v1.get_fractional_scale")
	if iView < 0 || iFrac < 0 {
		t.Fatalf("window setup %v lacks get_viewport or get_fractional_scale", names(setup))
	}

	// The scale arrives before the first configure (the output's integer
	// scale 2 was the guess): the first ConfigureEvent carries 1.5.
	h.preferredScale(180)
	h.configure(401, 301, 7, protocols.XdgToplevelStateActivated)
	ce := expect[platform.ConfigureEvent](h)
	if want := (platform.Metrics{Size: platform.Size{W: 401, H: 301}, Scale: 1.5}); ce.Metrics != want {
		t.Errorf("first ConfigureEvent = %v, want %v", ce.Metrics, want)
	}
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)

	// The first frame: a 602x452 buffer (601.5 and 451.5 round away from
	// zero), the viewport destination 401x301 in the same commit, no
	// set_buffer_scale, the ack, full damage in buffer pixels.
	reqs := h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 602 || b.h != 452 {
		t.Errorf("buffer %dx%d, want 602x452", b.w, b.h)
	}
	if r, i := find(reqs, "wp_viewport.set_destination"); i < 0 || r.i(0) != 401 || r.i(1) != 301 {
		t.Errorf("requests %v: want set_destination(401, 301)", names(reqs))
	}
	if _, i := find(reqs, "wl_surface.set_buffer_scale"); i >= 0 {
		t.Errorf("set_buffer_scale sent with a viewport: %v", names(reqs))
	}
	if r, i := find(reqs, "xdg_surface.ack_configure"); i < 0 || r.u(0) != 7 {
		t.Errorf("requests %v: want ack_configure(7)", names(reqs))
	}
	if r, _ := find(reqs, "wl_surface.damage_buffer"); r.name == "" || r.i(2) != 602 || r.i(3) != 452 {
		t.Errorf("damage_buffer = %v, want full 602x452", r)
	}

	// A new scale: same logical size, so no ack and no new destination;
	// only the buffer changes (501.25 -> 501, 376.25 -> 376).
	h.preferredScale(150)
	if ce := expect[platform.ConfigureEvent](h); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 401, H: 301}, Scale: 1.25}) {
		t.Errorf("ConfigureEvent = %v, want 401x301@1.25", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 501 || b.h != 376 {
		t.Errorf("buffer %dx%d, want 501x376", b.w, b.h)
	}
	for _, name := range []string{"wp_viewport.set_destination", "xdg_surface.ack_configure", "wl_surface.set_buffer_scale"} {
		if _, i := find(reqs, name); i >= 0 {
			t.Errorf("a scale change sent %s: %v", name, names(reqs))
		}
	}

	// Integer scales are ignored once a fractional one arrived: Hyprland
	// sends preferred_buffer_scale 2 and wl_output.scale 2 for 1.5.
	h.fc.send(h.fc.id("wl_surface"), 2, int32(2))             // preferred_buffer_scale
	h.fc.send(h.fc.id("wl_surface"), 0, h.fc.id("wl_output")) // enter
	h.noEvent(50 * time.Millisecond)

	// A resize: the new destination goes with the first buffer of the new
	// size and the ack.
	h.configure(500, 300, 8, protocols.XdgToplevelStateActivated)
	if ce := expect[platform.ConfigureEvent](h); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 500, H: 300}, Scale: 1.25}) {
		t.Errorf("ConfigureEvent = %v, want 500x300@1.25", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 625 || b.h != 375 {
		t.Errorf("buffer %dx%d, want 625x375", b.w, b.h)
	}
	if r, i := find(reqs, "wp_viewport.set_destination"); i < 0 || r.i(0) != 500 || r.i(1) != 300 {
		t.Errorf("requests %v: want set_destination(500, 300)", names(reqs))
	}
	if r, i := find(reqs, "xdg_surface.ack_configure"); i < 0 || r.u(0) != 8 {
		t.Errorf("requests %v: want ack_configure(8)", names(reqs))
	}

	// A resize and a scale change before the app draws: two
	// ConfigureEvents, one frame at the final size and scale that acks the
	// configure and sets the destination.
	h.fc.sendRaw(append(h.configureMsg(600, 400, 9, protocols.XdgToplevelStateActivated),
		event(h.fc.id("wp_fractional_scale_v1"), 0, uint32(210))...))
	if ce := expect[platform.ConfigureEvent](h); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 600, H: 400}, Scale: 1.25}) {
		t.Errorf("ConfigureEvent = %v, want 600x400@1.25", ce.Metrics)
	}
	if ce := expect[platform.ConfigureEvent](h); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 600, H: 400}, Scale: 1.75}) {
		t.Errorf("ConfigureEvent = %v, want 600x400@1.75", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 1050 || b.h != 700 {
		t.Errorf("buffer %dx%d, want 1050x700", b.w, b.h)
	}
	if r, i := find(reqs, "wp_viewport.set_destination"); i < 0 || r.i(0) != 600 || r.i(1) != 400 {
		t.Errorf("requests %v: want set_destination(600, 400)", names(reqs))
	}
	if r, i := find(reqs, "xdg_surface.ack_configure"); i < 0 || r.u(0) != 9 {
		t.Errorf("requests %v: want ack_configure(9)", names(reqs))
	}

	// Partial damage stays in buffer pixels.
	h.onFrame = nil
	h.ui(func() { w.Present(solid(w, 1, 2, 3), []image.Rectangle{image.Rect(3, 4, 10, 12)}) })
	reqs = h.fc.until("wl_surface.commit")
	if r, _ := find(reqs, "wl_surface.damage_buffer"); r.name == "" || r.i(0) != 3 || r.i(1) != 4 || r.i(2) != 7 || r.i(3) != 8 {
		t.Errorf("damage_buffer = %v, want (3,4) 7x8", r)
	}

	// Close: the scale objects go before their surface.
	h.ui(func() { w.Close() })
	reqs = h.fc.until("wl_surface.destroy")
	_, iFrac = find(reqs, "wp_fractional_scale_v1.destroy")
	_, iView = find(reqs, "wp_viewport.destroy")
	if iFrac < 0 || iView < 0 {
		t.Errorf("Close sent %v: want the fractional scale and viewport destroyed before the surface", names(reqs))
	}
}

func TestFractionalScaleAfterFirstFrame(t *testing.T) {
	// The compositor sends the scale only once the surface is mapped on an
	// output (after the first buffer): the window starts at the integer
	// guess, already through the viewport, then switches without an ack.
	h := newHarness(t, fractionalGlobals(2))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(solid(w, 1, 2, 3), nil) }
	openWindow(t, h, platform.WindowOptions{}, 100, 50, 1)
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 2 {
		t.Errorf("first scale = %v, want the output's 2", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 200 || b.h != 100 {
		t.Errorf("buffer %dx%d, want 200x100", b.w, b.h)
	}
	if r, i := find(reqs, "wp_viewport.set_destination"); i < 0 || r.i(0) != 100 || r.i(1) != 50 {
		t.Errorf("requests %v: want set_destination(100, 50) with the integer scale too", names(reqs))
	}
	if _, i := find(reqs, "wl_surface.set_buffer_scale"); i >= 0 {
		t.Errorf("set_buffer_scale sent with a viewport: %v", names(reqs))
	}

	h.fc.send(h.fc.id("wl_surface"), 0, h.fc.id("wl_output")) // enter: scale 2, no change
	h.preferredScale(180)
	if ce := expect[platform.ConfigureEvent](h); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 100, H: 50}, Scale: 1.5}) {
		t.Errorf("ConfigureEvent = %v, want 100x50@1.5", ce.Metrics)
	}
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if b := h.presented(reqs); b.w != 150 || b.h != 75 {
		t.Errorf("buffer %dx%d, want 150x75", b.w, b.h)
	}
	if _, i := find(reqs, "xdg_surface.ack_configure"); i >= 0 {
		t.Errorf("a scale change acked a configure: %v", names(reqs))
	}

	// A zero scale is invalid: ignored.
	h.preferredScale(0)
	h.noEvent(50 * time.Millisecond)
}

func TestFractionalScaleNeedsViewporter(t *testing.T) {
	// wp_fractional_scale_v1 without wp_viewporter cannot be applied: the
	// integer path (set_buffer_scale) is used and no fractional object is
	// created.
	g := append(standardGlobals(2), fakeGlobal{iface: "wp_fractional_scale_manager_v1", version: 1})
	h := newHarness(t, g)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(solid(w, 1, 2, 3), nil) }
	if _, err := h.b.NewWindow(platform.WindowOptions{}); err != nil {
		t.Fatal(err)
	}
	h.start()
	setup := h.fc.until("wl_surface.commit")
	if _, i := find(setup, "wp_fractional_scale_manager_v1.get_fractional_scale"); i >= 0 {
		t.Errorf("get_fractional_scale without a viewporter: %v", names(setup))
	}
	h.configure(100, 50, 1)
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 2 {
		t.Errorf("scale = %v, want 2", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	if r, i := find(reqs, "wl_surface.set_buffer_scale"); i < 0 || r.i(0) != 2 {
		t.Errorf("requests %v: want set_buffer_scale(2)", names(reqs))
	}
	if _, i := find(reqs, "wp_viewport.set_destination"); i >= 0 {
		t.Errorf("set_destination without a viewport: %v", names(reqs))
	}
}

func TestFractionalScaleOldCompositorDamage(t *testing.T) {
	// wl_compositor 3 has no damage_buffer: damage goes in surface
	// (logical) coordinates through the viewport's ratio, rounded outward.
	g := fractionalGlobals(1)
	g[0].version = 3
	h := newHarness(t, g)
	win, err := h.b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	h.start()
	h.fc.until("wl_surface.commit")
	h.preferredScale(180)
	h.configure(101, 101, 1)
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 1.5 {
		t.Fatalf("scale %v, want 1.5", ce.Scale)
	}
	expect[platform.FrameEvent](h)
	h.ui(func() { w.Present(solid(w, 1, 2, 3), nil) })
	reqs := h.fc.until("wl_surface.commit")
	if r, _ := find(reqs, "wl_surface.damage"); r.name == "" || r.i(0) != 0 || r.i(2) != 101 || r.i(3) != 101 {
		t.Errorf("full damage = %v, want 101x101 logical", r)
	}
	h.ui(func() { w.Present(solid(w, 1, 2, 3), []image.Rectangle{image.Rect(151, 0, 152, 3)}) })
	reqs = h.fc.until("wl_surface.commit")
	if r, _ := find(reqs, "wl_surface.damage"); r.name == "" || !slices.Equal(r.args, []any{int32(100), int32(0), int32(1), int32(2)}) {
		t.Errorf("damage of physical (151,0)-(152,3) = %v, want logical (100,0) 1x2", r)
	}
}
