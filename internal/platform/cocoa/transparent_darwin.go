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
//     the whole window transparent to the pointer, until the cursor is
//     over the input mask again or off the window, when it is cleared
//     (returning nil from hitTest: would only drop the event: AppKit does
//     not pass it to the window below). The mask is updated in Present
//     from the damage (platform.InputMask).
//   - Taking the pointer back is event-driven (ADR-257): while a window
//     lets it through, a global monitor of mouse moves and drags
//     (+[NSEvent addGlobalMonitorForEventsMatchingMask:handler:], which
//     sees the events of other apps; mouse events need no accessibility
//     permission, key events would) signals a pass, and every pass reads
//     +[NSEvent mouseLocation] (pollPassing); the moves that reach the
//     app's own windows run a pass anyway. A slow poll (passSafety) stays
//     for the moves no app receives (over the Dock or the menu bar), and
//     the 16 ms poll of before where the monitor cannot be installed.
//   - The app's resize handles (h.WindowResize) take the pointer over
//     transparent pixels too while the window can be resized (contract
//     v12, ADR-261: the outside of a rounded card's corner): the window
//     server passes zero alpha through before the app sees anything, so
//     the retained frame gives those pixels platform.HandleAlpha
//     (platform.HandleCatch, invisible), and the handles count as input
//     beside the mask. New handles without new pixels set the layer's
//     contents again.
//   - A press arrives only after the window server picked the window, so
//     a press on a faint pixel that no move preceded (the content changed
//     under a resting cursor, the window appeared or moved under it) used
//     to be swallowed. Present, when the mask changed, and windowDidMove:
//     and windowDidResize: check the resting cursor (passIfFaint) and let
//     it through before any press.

// passPoll is the period of the cursor poll while a transparent window
// lets the pointer through and the global monitor could not be
// installed; passSafety the slow poll beside the monitor.
const (
	passPoll   = 16 * time.Millisecond
	passSafety = 250 * time.Millisecond
)

// passWatch is the global monitor of mouse moves installed while a
// transparent window lets the pointer through (n windows), or 0.
type passWatch struct {
	monitor darwin.ID
	failed  bool // the monitor could not be installed: poll at passPoll
	n       int
}

// eventMaskPassMoves are the events the monitor sees: moves and drags of
// every button.
const eventMaskPassMoves = uint64(1)<<darwin.NSEventTypeMouseMoved | uint64(1)<<darwin.NSEventTypeLeftMouseDragged |
	uint64(1)<<darwin.NSEventTypeRightMouseDragged | uint64(1)<<darwin.NSEventTypeOtherMouseDragged

// watchPasses counts a window that starts (on) or stops letting the
// pointer through, installing the monitor with the first and removing it
// with the last.
func (b *Backend) watchPasses(on bool) {
	p := &b.pass
	if on {
		p.n++
	} else if p.n > 0 {
		p.n--
	}
	switch {
	case p.n > 0 && p.monitor == 0 && !p.failed && !b.closed:
		handler := darwin.NewBlock(func(_ darwin.Block, _ darwin.ID) {
			if !b.closed {
				b.notify() // the pass reads the cursor (pollPassing)
			}
		})
		m := darwin.SendClass(app.event, dndSel.addGlobalMonitor, eventMaskPassMoves, handler)
		handler.Release() // the monitor copied it
		if m == 0 {
			p.failed = true
			return
		}
		p.monitor = darwin.Retain(m)
	case p.n == 0 && p.monitor != 0:
		b.stopPassWatch()
	}
}

// stopPassWatch removes the monitor (no window lets the pointer through,
// or Run returns).
func (b *Backend) stopPassWatch() {
	if m := b.pass.monitor; m != 0 {
		darwin.SendClass(app.event, dndSel.removeMonitor, m)
		darwin.Release(m)
		b.pass.monitor = 0
	}
}

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
	catch   platform.HandleCatch // the resize handles caught in Window.back
	passing bool
	poll    *time.Timer
}

// catchHandles catches the resize handles of a transparent window that
// can be resized in its retained frame, after the rectangles damage were
// copied into it (nil for none), and reports whether they changed.
func (w *Window) catchHandles(damage []image.Rectangle) bool {
	if !w.opts.Transparent || w.back == nil {
		return false
	}
	var handles []image.Rectangle
	if w.resizable() {
		handles = platform.HandleRects(w.ctl.regions, w.Metrics().Scale, w.back.Rect.Size())
	}
	return !w.trans.catch.Update(w.back.Pix, w.back.Stride, handles, damage).Empty()
}

// syncHandles catches new resize handles after SetHitRegions or a state
// change (zoom, full screen), showing the frame again when they changed.
func (w *Window) syncHandles() {
	if w.presented && w.catchHandles(nil) {
		w.showBack()
	}
}

// transparentStyle returns the style mask of a transparent window: not
// resizable.
func transparentStyle(style uint) uint { return style &^ darwin.NSWindowStyleMaskResizable }

// initTransparent makes a new transparent window non-opaque, clear and
// without the system's shadow. A backdrop (Mica, MicaAlt, Acrylic) is a
// vibrancy material behind the app's card only (maskEffect, ADR-257).
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
	if !w.trans.mask.Update(fb, rects) {
		return
	}
	w.maskEffect() // a backdrop behind the card follows its shape
	// The pixels under a resting pointer changed.
	if w.trans.passing {
		w.pollPassing()
	} else {
		w.passIfFaint()
	}
}

// passIfFaint lets the pointer through when the cursor rests over the
// window outside its input mask with no button held, so that a press there
// reaches what is below (see the top of this file).
func (w *Window) passIfFaint() {
	if !w.opts.Transparent || w.trans.passing || w.ptr.held != 0 || w.Closed() || !w.shown {
		return
	}
	p, in := w.cursorInView()
	if in && !w.takesInput(p) {
		w.setPassing(true)
	}
}

// cursorInView returns the cursor's position in TelaView's points
// (+[NSEvent mouseLocation]) and whether it is over the view.
func (w *Window) cursorInView() (platform.Point, bool) {
	screen := darwin.SendPoint(darwin.ID(app.event), tpsel.mouseLocation)
	loc := darwin.SendPoint(w.ns, isel.convertFrom, screen)
	v := darwin.SendPoint(w.view, isel.convertPointFromView, loc, darwin.ID(0))
	p := platform.Point{X: float32(v.X), Y: float32(v.Y)}
	b := darwin.SendRect(w.view, sel.bounds)
	return p, inBounds(p, platform.Size{W: float32(b.Size.Width), H: float32(b.Size.Height)})
}

// takesInput reports whether the pointer at p (TelaView's points) is over
// the input mask.
func (w *Window) takesInput(p platform.Point) bool {
	s := w.Metrics().Scale
	if s == 0 {
		s = 1
	}
	px := image.Pt(int(math.Floor(float64(p.X)*s)), int(math.Floor(float64(p.Y)*s)))
	return w.trans.mask.Contains(px) || w.trans.catch.Contains(px)
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
	w.b.watchPasses(on)
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

// armPoll signals a pass after passSafety (passPoll without the
// monitor), where pollPassing runs.
func (w *Window) armPoll() {
	d := passSafety
	if w.b.pass.monitor == 0 {
		d = passPoll
	}
	if w.trans.poll == nil {
		w.trans.poll = time.AfterFunc(d, w.b.notify)
	} else {
		w.trans.poll.Reset(d)
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
	if p, in := w.cursorInView(); !in || w.takesInput(p) {
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
	if w.trans.passing {
		w.trans.passing = false
		w.b.watchPasses(false)
	}
}
