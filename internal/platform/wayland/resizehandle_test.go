//go:build linux

package wayland

import (
	"image"
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
