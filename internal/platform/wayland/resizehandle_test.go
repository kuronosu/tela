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

// TestTransparentResizeHandles (ADR-257): the app's resize handles over a
// transparent window's card show the resize cursor and start
// xdg_toplevel.resize from their edge; the press is not delivered.
func TestTransparentResizeHandles(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	rect := image.Rect(20, 10, 80, 40)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, rect), nil) }
	w, _ := openCtl(t, h, platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 100, H: 60}}, 100, 60, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true, Transparent: true})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	h.ui(func() {
		w.SetHitRegions([]platform.HitRegion{
			{Kind: platform.HitResizeLeft, Rect: platform.Rect{X: 20, Y: 10, W: 5, H: 30}},
			{Kind: platform.HitResizeBottomRight, Rect: platform.Rect{X: 64, Y: 35, W: 16, H: 5}},
		})
		if e := w.resizeEdgeAt(platform.Point{X: 22, Y: 20}); e != platform.EdgeLeft {
			t.Errorf("edge on the left handle = %v", e)
		}
		if e := w.resizeEdgeAt(platform.Point{X: 50, Y: 20}); e != 0 {
			t.Errorf("edge in the card = %v", e)
		}
	})
	ptr, surface, seat := h.fc.id("wl_pointer"), h.fc.id("wl_surface"), h.fc.id("wl_seat")
	h.fc.send(ptr, 0, uint32(10), surface, 70.0, 37.0) // enter on the corner handle
	h.fc.send(ptr, 5)
	expect[platform.PointerEvent](h)
	r := h.fc.until("wp_cursor_shape_device_v1.set_shape")
	if got := r[len(r)-1].u(1); got != uint32(cursorShape(platform.CursorSEResize)) {
		t.Errorf("cursor shape %d, want se-resize", got)
	}
	h.fc.send(ptr, 3, uint32(11), uint32(0), uint32(btnLeft), uint32(1))
	h.fc.send(ptr, 5)
	if r := h.fc.until("xdg_toplevel.resize"); r[len(r)-1].u(0) != seat || r[len(r)-1].u(1) != 11 ||
		r[len(r)-1].u(2) != uint32(protocols.XdgToplevelResizeEdgeBottomRight) {
		t.Errorf("resize = %v", r[len(r)-1])
	}
	h.fc.send(ptr, 3, uint32(12), uint32(0), uint32(btnLeft), uint32(0))
	h.fc.send(ptr, 5)
	h.noEvent(50 * time.Millisecond)
}

// regionAdds returns the rectangles added to wl_region id in reqs.
func regionAdds(reqs []freq, id uint32) []image.Rectangle {
	var out []image.Rectangle
	for _, r := range reqs {
		if r.is("wl_region.add") && r.id == id {
			out = append(out, image.Rect(int(r.i(0)), int(r.i(1)), int(r.i(0)+r.i(2)), int(r.i(1)+r.i(3))))
		}
	}
	return out
}

// TestTransparentHandlesInputRegion (ADR-261): the resize handles join a
// transparent window's input region, beside the mask, over transparent
// pixels too (the outside of a rounded card's corner); SetHitRegions
// commits the new region at once, without a buffer, and equal handles
// send nothing.
func TestTransparentHandlesInputRegion(t *testing.T) {
	h := newHarness(t, standardGlobals(1))
	rect := image.Rect(20, 10, 80, 40)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, rect), nil) }
	w, _ := openCtl(t, h, platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 100, H: 60}}, 100, 60, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true, Transparent: true})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	regions := []platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{X: 20, Y: 10, W: 60, H: 20}},
		{Kind: platform.HitResizeTopLeft, Rect: platform.Rect{X: 18.5, Y: 8, W: 16, H: 5}}, // partly over the margin
		{Kind: platform.HitResizeLeft, Rect: platform.Rect{X: 20, Y: 10, W: 5, H: 30}},
	}
	h.ui(func() { w.SetHitRegions(regions) })
	reqs := h.fc.until("wl_surface.commit")
	set, iSet := find(reqs, "wl_surface.set_input_region")
	if iSet < 0 {
		t.Fatalf("SetHitRegions sent %v, want set_input_region and a commit", names(reqs))
	}
	if _, i := find(reqs, "wl_surface.attach"); i >= 0 {
		t.Errorf("the handles' commit attached a buffer: %v", names(reqs))
	}
	want := []image.Rectangle{rect, image.Rect(18, 8, 35, 13), image.Rect(20, 10, 25, 40)}
	if got := regionAdds(reqs, set.u(0)); !slices.Equal(got, want) {
		t.Errorf("input region %v, want the mask then the handles %v", got, want)
	}
	// The same handles (a caption changed): nothing is sent, as the
	// title request right after it shows.
	h.ui(func() {
		regions[0].Rect.W = 50
		w.SetHitRegions(regions)
		w.SetTitle("next")
	})
	if reqs := h.fc.until("xdg_toplevel.set_title"); len(reqs) != 1 {
		t.Errorf("equal handles sent %v", names(reqs))
	}
}
