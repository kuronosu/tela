//go:build darwin

package cocoa

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// TestTransparentWindow (ADR-249): a transparent window is non-opaque with
// a clear background, no shadow and no resizing, reports Transparent,
// shows the desktop where its frame is transparent, and lets the pointer
// through a nearly transparent pixel (ignoresMouseEvents) until the poll
// finds the cursor elsewhere.
func TestTransparentWindow(t *testing.T) {
	screenInfo(t)
	// A card at 40,30 to 160,110 (points) and a faint shadow below it.
	card := func(c *ctl, i int, w *Window) {
		s := w.Metrics().Scale
		px := func(v float64) int { return int(v * s) }
		fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
		for y := px(30); y < px(125); y++ {
			for x := px(40); x < px(160); x++ {
				c := color.RGBA{0xf0, 0xf0, 0xf0, 0xff}
				if y >= px(110) {
					c = color.RGBA{0, 0, 0, 0x20}
				}
				fb.SetRGBA(x, y, c)
			}
		}
		if err := w.Present(fb, nil); err != nil {
			t.Errorf("Present: %v", err)
		}
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela transparent", Size: platform.Size{W: 200, H: 150}, Frameless: true, Transparent: true, HasPosition: true, Position: image.Pt(120, 160)},
	}, card,
		ctlStep{name: "the transparent window",
			until: func(c *ctl) bool { return c.after(600 * time.Millisecond) },
			check: func(c *ctl) {
				w := c.w[0]
				if darwin.SendBool(w.ns, darwin.Sel("isOpaque")) || darwin.SendBool(w.view, darwin.Sel("isOpaque")) {
					t.Error("the window or TelaView is opaque")
				}
				if darwin.SendBool(w.ns, darwin.Sel("hasShadow")) {
					t.Error("the window has the system's shadow")
				}
				if darwin.SendUint(w.ns, wsel.styleMask)&darwin.NSWindowStyleMaskResizable != 0 {
					t.Error("the window is resizable")
				}
				if st := w.State(); !st.Transparent || !st.ClientSideDecorations {
					t.Errorf("state %v, want transparent with client-side decorations", st)
				}
				if !w.takesInput(platform.Point{X: 100, Y: 50}) || w.takesInput(platform.Point{X: 100, Y: 115}) || w.takesInput(platform.Point{X: 5, Y: 5}) {
					t.Error("the input mask does not follow the frame")
				}
				if pix := captureImage(w); pix != nil {
					// The capture is the window's own pixels: transparent
					// left of the card (x 20 of 200), opaque on it.
					if a := pix.RGBAAt(pix.Rect.Dx()/10, pix.Rect.Dy()/2).A; a > 8 {
						t.Errorf("the window left of the card reads back with alpha %d, want transparent", a)
					}
					if a := pix.RGBAAt(pix.Rect.Dx()/2, pix.Rect.Dy()/2).A; a < 250 {
						t.Errorf("the card reads back with alpha %d, want opaque", a)
					}
				} else {
					t.Logf("note: the window capture was refused; the pixels are not checked")
				}
			}},
		ctlStep{name: "the pointer over the shadow goes through",
			do: func(c *ctl) {
				w := c.w[0]
				if !w.passPointer(platform.Point{X: 100, Y: 115}) {
					t.Error("a move over the shadow did not pass")
				}
				if !darwin.SendBool(w.ns, darwin.Sel("ignoresMouseEvents")) {
					t.Error("ignoresMouseEvents is off while the pointer passes")
				}
			},
			// The cursor is wherever the runner left it: the poll takes the
			// pointer back unless it rests on the shadow.
			until: func(c *ctl) bool { return !c.w[0].trans.passing },
			check: func(c *ctl) {
				if darwin.SendBool(c.w[0].ns, darwin.Sel("ignoresMouseEvents")) {
					t.Error("ignoresMouseEvents stays on after the poll")
				}
			}},
	)
}
