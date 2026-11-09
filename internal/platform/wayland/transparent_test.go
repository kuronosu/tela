//go:build linux

package wayland

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// card returns a frame of the window's size, transparent but for an
// opaque card over the logical rectangle r and a faint shadow below it.
func card(w platform.Window, r image.Rectangle) *image.RGBA {
	s := int(w.Metrics().Scale)
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	for y := r.Min.Y * s; y < (r.Max.Y+4)*s; y++ {
		for x := r.Min.X * s; x < r.Max.X*s; x++ {
			c := color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}
			if y >= r.Max.Y*s {
				c = color.RGBA{A: platform.InputAlpha - 1} // the shadow: no input
			}
			fb.SetRGBA(x, y, c)
		}
	}
	return fb
}

// TestTransparentWindow: a transparent window has buffers with alpha,
// reports Transparent, has no resize edges, and its input region follows
// the opaque pixels of the frame just committed, in surface coordinates.
func TestTransparentWindow(t *testing.T) {
	h := newHarness(t, standardGlobals(2)) // ARGB8888 and XRGB8888, no ABGR
	rect := image.Rect(20, 10, 80, 40)
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, rect), nil) }
	w, _ := openCtl(t, h, platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 100, H: 60}}, 100, 60, 1, protocols.XdgToplevelStateActivated)
	expect[platform.ConfigureEvent](h)
	expectState(t, h, platform.WindowState{ClientSideDecorations: true, Transparent: true})
	expect[platform.FocusEvent](h)
	expect[platform.FrameEvent](h)
	reqs := h.fc.until("wl_surface.commit")
	frame, _ := find(reqs, "wl_surface.frame")
	attach, iAttach := find(reqs, "wl_surface.attach")
	set, iSet := find(reqs, "wl_surface.set_input_region")
	if iAttach < 0 || iSet < 0 || iSet > iAttach {
		t.Fatalf("first frame %v: want set_input_region before attach", names(reqs))
	}
	if buf := h.fc.buffer(attach.u(0)); buf.format != uint32(protocols.WlShmFormatArgb8888) {
		t.Errorf("buffer format %#x, want ARGB8888", buf.format)
	}
	px := h.fc.pixels(attach.u(0))
	if px[3] != 0 {
		t.Errorf("a transparent pixel has alpha %d in the buffer", px[3])
	}
	o := 4 * (20*2*200 + 40) // physical (40, 20): the card
	if !slices.Equal(px[o:o+4], []byte{0x30, 0x20, 0x10, 0xff}) {
		t.Errorf("card pixel = % x, want 30 20 10 ff (premultiplied B, G, R, A)", px[o:o+4])
	}
	var adds []image.Rectangle
	for _, r := range reqs {
		if r.is("wl_region.add") {
			adds = append(adds, image.Rect(int(r.i(0)), int(r.i(1)), int(r.i(0)+r.i(2)), int(r.i(1)+r.i(3))))
		}
	}
	if !slices.Equal(adds, []image.Rectangle{rect}) || set.u(0) == 0 {
		t.Errorf("input region %v (set %v), want %v", adds, set, rect)
	}
	h.ui(func() {
		if e := w.resizeEdgeAt(platform.Point{X: 1, Y: 1}); e != 0 {
			t.Errorf("a transparent window has resize edge %v", e)
		}
	})

	// A frame with the same mask sets no region; another one does.
	h.ui(func() { w.RequestFrame() })
	h.fc.send(frame.u(0), 0, uint32(1)) // done
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	frame, _ = find(reqs, "wl_surface.frame")
	if _, i := find(reqs, "wl_surface.set_input_region"); i >= 0 {
		t.Error("an unchanged mask set the input region again")
	}
	h.ui(func() { rect = image.Rect(0, 0, 50, 30); w.RequestFrame() })
	h.fc.send(frame.u(0), 0, uint32(2))
	expect[platform.FrameEvent](h)
	reqs = h.fc.until("wl_surface.commit")
	if _, i := find(reqs, "wl_surface.set_input_region"); i < 0 {
		t.Errorf("a new mask set no input region: %v", names(reqs))
	}
}

// TestTransparentABGR: with ABGR8888 the frame is copied as is.
func TestTransparentABGR(t *testing.T) {
	h := newHarness(t, standardGlobals(1, protocols.WlShmFormatXrgb8888, protocols.WlShmFormatArgb8888, protocols.WlShmFormatAbgr8888))
	h.onFrame = func(w platform.Window, _ platform.FrameEvent) { w.Present(card(w, image.Rect(0, 0, 10, 10)), nil) }
	openCtl(t, h, platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 20, H: 20}}, 20, 20, 1)
	reqs := h.fc.until("wl_surface.attach")
	attach := reqs[len(reqs)-1]
	if buf := h.fc.buffer(attach.u(0)); buf.format != uint32(protocols.WlShmFormatAbgr8888) {
		t.Errorf("buffer format %#x, want ABGR8888", buf.format)
	}
	if px := h.fc.pixels(attach.u(0)); !slices.Equal(px[:4], []byte{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("card pixel = % x, want 10 20 30 ff (R, G, B, A)", px[:4])
	}
}

func TestInputRegionToSurface(t *testing.T) {
	// Scale 1.5: 150x90 physical for 100x60 logical; the card's rows map
	// outward and duplicates go.
	got := inputRegion([]image.Rectangle{image.Rect(30, 15, 120, 16), image.Rect(30, 16, 120, 60)}, image.Pt(150, 90), image.Pt(100, 60))
	want := []image.Rectangle{image.Rect(20, 10, 80, 11), image.Rect(20, 10, 80, 40)}
	if !slices.Equal(got, want) {
		t.Errorf("inputRegion = %v, want %v", got, want)
	}
}
