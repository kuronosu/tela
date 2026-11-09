//go:build darwin

package cocoa

import (
	"image"
	"math"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Transparent windows (WindowOptions.Transparent, ADR-249), as Electron
// makes its transparent frameless windows:
//
//   - The frameless window (titled, full-size content view, no title bar
//     drawn; windowctl_darwin.go) is made non-opaque with a clear
//     background (setOpaque), without the system's shadow (hasShadow NO:
//     the app draws its own shadow-*, and AppKit's would follow the alpha
//     of the first frame only), and not resizable (its edges are usually
//     the transparent margin). TelaView's layer keeps the frame's
//     premultiplied alpha (kCGImageAlphaPremultipliedLast), so the
//     window server composites it over what is below.
//   - Pointer input: the window server already passes the pointer through
//     a non-opaque window's pixels whose alpha is zero. For the pixels
//     under platform.InputAlpha that are not zero (the faint part of a
//     shadow), a move or press there sets ignoresMouseEvents, which makes
//     the whole window transparent to the pointer, and the backend polls
//     +[NSEvent mouseLocation] every 16 ms until the cursor is over the
//     input mask again or off the window, when it is cleared (returning
//     nil from hitTest: would only drop the event: AppKit does not pass
//     it to the window below). The mask is updated in Present from the
//     damage (platform.InputMask).

// passPoll is the period of the cursor poll while a transparent window
// lets the pointer through.
const passPoll = 16 * time.Millisecond

var tpsel = struct {
	setHasShadow, setIgnoresMouseEvents, mouseLocation darwin.SEL
}{
	setHasShadow:          darwin.Sel("setHasShadow:"),
	setIgnoresMouseEvents: darwin.Sel("setIgnoresMouseEvents:"),
	mouseLocation:         darwin.Sel("mouseLocation"),
}

// transparentState is a transparent window's input mask and whether it
// lets the pointer through (ignoresMouseEvents and the poll).
type transparentState struct {
	mask    platform.InputMask
	passing bool
	poll    *time.Timer
}

// transparentStyle returns the style mask of a transparent window: not
// resizable.
func transparentStyle(style uint) uint { return style &^ darwin.NSWindowStyleMaskResizable }

// initTransparent makes a new transparent window non-opaque, clear and
// without the system's shadow.
func (w *Window) initTransparent() {
	w.setOpaque(false)
	darwin.Send(w.ns, tpsel.setHasShadow, false)
	darwin.Send(w.layer, wsel.setOpaque, false)
}

// updateInputMask updates the input mask of a transparent window with the
// rectangles of fb just presented.
func (w *Window) updateInputMask(fb *image.RGBA, rects []image.Rectangle) {
	if !w.opts.Transparent {
		return
	}
	if w.trans.mask.Update(fb, rects) && w.trans.passing {
		w.pollPassing() // the pixels under a resting pointer changed
	}
}

// takesInput reports whether the pointer at p (TelaView's points) is over
// the input mask.
func (w *Window) takesInput(p platform.Point) bool {
	s := w.Metrics().Scale
	if s == 0 {
		s = 1
	}
	return w.trans.mask.Contains(image.Pt(int(math.Floor(float64(p.X)*s)), int(math.Floor(float64(p.Y)*s))))
}

// passPointer reports whether a move or press at p goes to what is below
// a transparent window, which then lets the pointer through until it is
// over the mask again. A sequence with a button held stays the window's.
func (w *Window) passPointer(p platform.Point) bool {
	if !w.opts.Transparent || w.ptr.held != 0 || w.takesInput(p) {
		return false
	}
	w.setPassing(true)
	return true
}

// setPassing lets the pointer through the whole window (on) or takes it
// back (off).
func (w *Window) setPassing(on bool) {
	t := &w.trans
	if t.passing == on {
		return
	}
	t.passing = on
	darwin.Send(w.ns, tpsel.setIgnoresMouseEvents, on)
	if on {
		if w.ptr.inside {
			// The pointer left the app's input.
			w.ptr.exited(w.ptr.pos, func(pe platform.PointerEvent) {
				pe.Kind, pe.Time = platform.PointerMouse, w.b.Now()
				w.b.enqueue(w, pe)
			})
		}
		w.armPoll()
	} else if t.poll != nil {
		t.poll.Stop()
	}
}

// armPoll signals a pass after passPoll, where pollPassing runs.
func (w *Window) armPoll() {
	if w.trans.poll == nil {
		w.trans.poll = time.AfterFunc(passPoll, w.b.notify)
	} else {
		w.trans.poll.Reset(passPoll)
	}
}

// pollPassing takes the pointer back when the cursor is over the input
// mask again, or off the window (where the window server decides again),
// and polls on otherwise. It runs in every pass while the window lets the
// pointer through.
func (w *Window) pollPassing() {
	if !w.trans.passing || w.Closed() {
		return
	}
	screen := darwin.SendPoint(darwin.ID(app.event), tpsel.mouseLocation)
	loc := darwin.SendPoint(w.ns, isel.convertFrom, screen)
	v := darwin.SendPoint(w.view, isel.convertPointFromView, loc, darwin.ID(0))
	p := platform.Point{X: float32(v.X), Y: float32(v.Y)}
	b := darwin.SendRect(w.view, sel.bounds)
	if !inBounds(p, platform.Size{W: float32(b.Size.Width), H: float32(b.Size.Height)}) || w.takesInput(p) {
		w.setPassing(false)
		return
	}
	w.armPoll()
}

// releaseTransparent stops the poll of a closed window.
func (w *Window) releaseTransparent() {
	if w.trans.poll != nil {
		w.trans.poll.Stop()
	}
}
