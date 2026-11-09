//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestTransparentWindow (ADR-249): a transparent window is a layered popup
// that reports Transparent, has no resize band, keeps its frame's alpha
// in its layered bitmap, and answers WM_NCHITTEST over a nearly
// transparent pixel by letting the pointer through (WS_EX_TRANSPARENT and
// the cursor poll), which the poll takes back with the cursor elsewhere.
func TestTransparentWindow(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	done := false
	parkCursor(t)
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 200, H: 150}, Frameless: true, Transparent: true})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
		requireCursorOutside(t, w.hwnd)
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.WakeEvent:
			b.Quit()
			return
		case platform.FrameEvent:
			if done {
				return
			}
		default:
			return
		}
		done = true
		// A card at 40,30 to 160,110 (logical) and a faint shadow below.
		s := w.scale()
		px := func(v float64) int { return int(v * s) }
		fb := image.NewRGBA(image.Rectangle{Max: ew.Metrics().Physical()})
		for y := px(30); y < px(125); y++ {
			for x := px(40); x < px(160); x++ {
				c := color.RGBA{0xf0, 0xf0, 0xf0, 0xff}
				if y >= px(110) {
					c = color.RGBA{0, 0, 0, 0x20}
				}
				fb.SetRGBA(x, y, c)
			}
		}
		if err := ew.Present(fb, nil); err != nil {
			t.Fatalf("Present: %v", err)
		}
		if st := w.State(); !st.Transparent || !st.ClientSideDecorations {
			t.Errorf("state = %v, want transparent with client-side decorations", st)
		}
		if ex := getWindowLong(w.hwnd, gwlExStyle); ex&wsExLayered == 0 || ex&wsExTransparent != 0 {
			t.Errorf("extended style %#x, want WS_EX_LAYERED without WS_EX_TRANSPARENT", ex)
		}
		c, wr := getClientRect(w.hwnd), getWindowRect(w.hwnd)
		if c.width() != wr.width() || c.height() != wr.height() {
			t.Errorf("client %dx%d, window %dx%d: a transparent window is all client area", c.width(), c.height(), wr.width(), wr.height())
		}
		if got := w.lay.bits[3]; got != 0 {
			t.Errorf("the layered bitmap's corner has alpha %d, want 0", got)
		}
		o := 4 * (px(50)*w.lay.size.X + px(100))
		if got := w.lay.bits[o : o+4]; got[3] != 0xff || got[0] != 0xf0 {
			t.Errorf("the card's pixel in the layered bitmap is % x", got)
		}
		// The card takes the pointer; a resize edge is no edge.
		for _, tc := range []struct {
			x, y float64
			want uintptr
		}{{100, 50, htClient}, {40.5, 70, htClient}} {
			if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(screenAt(w, tc.x, tc.y))); got != tc.want {
				t.Errorf("WM_NCHITTEST at %v,%v = %d, want %d", tc.x, tc.y, int(got), int(tc.want))
			}
		}
		// The shadow lets it through.
		if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(screenAt(w, 100, 115))); got != htTransparent {
			t.Errorf("WM_NCHITTEST over the shadow = %d, want HTTRANSPARENT", int(got))
		}
		if ex := getWindowLong(w.hwnd, gwlExStyle); ex&wsExTransparent == 0 || !w.lay.passing {
			t.Errorf("extended style %#x after a hit test over the shadow, want WS_EX_TRANSPARENT", ex)
		}
		// The cursor is off the window: the poll takes the pointer back.
		w.pollPassing()
		if ex := getWindowLong(w.hwnd, gwlExStyle); ex&wsExTransparent != 0 || w.lay.passing {
			t.Errorf("extended style %#x after the poll with the cursor elsewhere, want no WS_EX_TRANSPARENT", ex)
		}
		b.Wake()
	})
	if !done {
		t.Fatal("no frame")
	}
}
