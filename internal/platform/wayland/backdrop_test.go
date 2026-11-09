//go:build linux

package wayland

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// blurGlobals are the standard globals with ext_background_effect_manager_v1,
// whose capabilities are caps.
func blurGlobals(caps uint32) []fakeGlobal {
	return append(standardGlobals(1), fakeGlobal{iface: "ext_background_effect_manager_v1", version: 1, onBind: func(fc *fakeComp, id uint32) {
		fc.send(id, 0, caps) // capabilities
	}})
}

// TestTransparentBackdropBlur (ADR-261): a transparent window whose
// backdrop is a material blurs what is behind its input mask (not the
// margin, not the handles) where the compositor blurs: the blur region is
// set before the commit of the frame it follows, and SetBackdrop(None)
// removes it at once.
func TestTransparentBackdropBlur(t *testing.T) {
	h := newHarness(t, blurGlobals(uint32(protocols.ExtBackgroundEffectManagerV1CapabilityBlur)))
	rect := image.Rect(20, 10, 80, 40)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, rect), nil) }
	w, _ := openCtl(t, h, platform.WindowOptions{Frameless: true, Transparent: true, Backdrop: platform.BackdropAcrylic,
		Size: platform.Size{W: 100, H: 60}}, 100, 60, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true, Transparent: true})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	get, iGet := find(reqs, "ext_background_effect_manager_v1.get_background_effect")
	set, iSet := find(reqs, "ext_background_effect_surface_v1.set_blur_region")
	if iGet < 0 || iSet < iGet || get.u(1) != h.fc.id("wl_surface") {
		t.Fatalf("first frame %v: want get_background_effect for the surface, then set_blur_region", names(reqs))
	}
	if got := regionAdds(reqs, set.u(0)); !slices.Equal(got, []image.Rectangle{rect}) {
		t.Errorf("blur region %v, want the card %v", got, rect)
	}

	// A new card: the region follows it.
	h.ui(func() { rect = image.Rect(10, 10, 60, 30); w.RequestFrame() })
	frame, _ := last(reqs, "wl_surface.frame")
	h.fc.send(frame.u(0), 0, uint32(1))
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if set, i := find(reqs, "ext_background_effect_surface_v1.set_blur_region"); i < 0 || !slices.Equal(regionAdds(reqs, set.u(0)), []image.Rectangle{rect}) {
		t.Errorf("a new card's frame %v: want the blur region %v", names(reqs), rect)
	}

	// No backdrop: a null region, committed at once.
	h.ui(func() { w.SetBackdrop(platform.BackdropNone) })
	reqs = h.fc.until("wl_surface.commit")
	if set, i := find(reqs, "ext_background_effect_surface_v1.set_blur_region"); i < 0 || set.u(0) != 0 {
		t.Errorf("SetBackdrop(None) sent %v, want set_blur_region(null)", reqs)
	}
}

// A compositor without the blur capability, an opaque window and a
// transparent one without a material get no background effect.
func TestBackdropWithoutBlur(t *testing.T) {
	for _, c := range []struct {
		name string
		caps uint32
		opts platform.WindowOptions
	}{
		{"no capability", 0, platform.WindowOptions{Frameless: true, Transparent: true, Backdrop: platform.BackdropMica}},
		{"opaque", 1, platform.WindowOptions{Frameless: true, Backdrop: platform.BackdropMica}},
		{"no material", 1, platform.WindowOptions{Frameless: true, Transparent: true, Backdrop: platform.BackdropNone}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, blurGlobals(c.caps))
			h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, image.Rect(0, 0, 10, 10)), nil) }
			c.opts.Size = platform.Size{W: 20, H: 20}
			openCtl(t, h, c.opts, 20, 20, 1)
			reqs := h.fc.until("wl_surface.attach")
			reqs = append(reqs, h.fc.until("wl_surface.commit")...)
			if _, i := find(reqs, "ext_background_effect_manager_v1.get_background_effect"); i >= 0 {
				t.Errorf("first frame %v: a background effect", names(reqs))
			}
		})
	}
}
