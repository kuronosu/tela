//go:build darwin

package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The app's resize handles (h.WindowResize, ADR-257) and caption clicks
// (ADR-252); the pure rules are in resize.go.

var rsel = struct {
	pressedMouseButtons darwin.SEL
}{
	pressedMouseButtons: darwin.Sel("pressedMouseButtons"),
}

// resizeInput is a resize the backend runs itself from one of the app's
// resize handles (a HitResize… region, h.WindowResize; ADR-257). AppKit
// has no public call that starts the window server's resize from an event
// (performWindowDragWithEvent: only moves), and a transparent window is
// not resizable (its edges are the transparent margin), so the press
// starts a loop of the drags AppKit sends the window while the button is
// held: each one sets the frame from the press's frame and the cursor's
// motion since the press, in screen points (y up; resizedFrame).
type resizeInput struct {
	held  bool
	btn   platform.PointerButton
	edge  platform.Edge
	mouse [2]float64 // the cursor at the press
	frame [4]float64 // the window's frame at the press: x, y, w, h
	// hover is the edge of the handle under the pointer while no button
	// is held (its cursor is shown), 0 for none.
	hover platform.Edge
}

// captionPoll is how often a caption press handed to the window server is
// checked for its release (only while the primary button is held).
const captionPoll = 16 * time.Millisecond

// resizable reports whether the app's resize handles resize the window:
// not FixedSize, zoomed or in full screen.
func (w *Window) resizable() bool {
	return !w.opts.FixedSize && !w.ctl.fullscreen && !w.ctl.zoomed
}

// handleEdge returns the edge of the resize handle at pos (TelaView's
// points), 0 for none or when the window cannot be resized.
func (w *Window) handleEdge(pos platform.Point) platform.Edge {
	if len(w.ctl.regions) == 0 || !w.resizable() {
		return 0
	}
	return platform.HitEdge(w.ctl.regions, pos)
}

// screenMouse returns the cursor in screen points (y up).
func screenMouse() [2]float64 {
	p := darwin.SendPoint(darwin.ID(app.event), tpsel.mouseLocation)
	return [2]float64{p.X, p.Y}
}

// frameOf returns the window's frame as x, y, w, h.
func (w *Window) frameOf() [4]float64 {
	f := darwin.SendRect(w.ns, wsel.frame)
	return [4]float64{f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height}
}

// resizePress starts a resize for a primary press on a resize handle and
// reports whether the press was one (another button there does nothing
// and is not delivered either, as on the OS's own border).
func (w *Window) resizePress(btn platform.PointerButton, pos platform.Point) bool {
	r := &w.ctl.resize
	if r.held || w.ptr.held != 0 {
		return r.held
	}
	e := w.handleEdge(pos)
	if e == 0 {
		return false
	}
	*r = resizeInput{held: true, btn: btn, edge: e, mouse: screenMouse(), frame: w.frameOf()}
	if btn != platform.ButtonPrimary {
		r.edge = 0 // swallowed until its release
	}
	return true
}

// resizeDrag follows a drag of a resize press: the frame from the press's
// and the cursor's motion, within the size limits (AppKit's minSize and
// maxSize do not apply to setFrame:).
func (w *Window) resizeDrag() {
	r := &w.ctl.resize
	if r.edge == 0 {
		return
	}
	m := screenMouse()
	lo, hi := sizeLimits(w.opts.MinSize, w.opts.MaxSize)
	f := resizedFrame(r.frame, r.edge, m[0]-r.mouse[0], m[1]-r.mouse[1], lo, hi)
	darwin.Send(w.ns, wsel.setFrameDisplay, darwin.CGRect{
		Origin: darwin.CGPoint{X: f[0], Y: f[1]},
		Size:   darwin.CGSize{Width: f[2], Height: f[3]},
	}, true)
}

// resizeRelease ends a resize press at the release of btn and reports
// whether the release belonged to it.
func (w *Window) resizeRelease(btn platform.PointerButton) bool {
	r := &w.ctl.resize
	if !r.held || r.btn != btn {
		return r.held
	}
	r.held, r.edge = false, 0
	return true
}

// hoverHandles shows the resize cursor of the handle under pos while no
// button is held, and the app's cursor elsewhere.
func (w *Window) hoverHandles(pos platform.Point) {
	e := w.handleEdge(pos)
	if e == w.ctl.resize.hover {
		return
	}
	w.ctl.resize.hover = e
	darwin.Send(w.ns, sel.invalidateCursorRects, w.view)
	if w.ptr.inside && w.focused {
		darwin.Send(w.nsCursor(), isel.set)
	}
}

// shownCursor is the cursor TelaView shows: the resize cursor over a
// resize handle, else the app's (diagonal resize cursors are the arrow
// before macOS 15, see cursorSelector).
func (w *Window) shownCursor() platform.Cursor {
	if e := w.ctl.resize.hover; e != 0 {
		return e.Cursor()
	}
	return w.cursor
}

// --- caption clicks ---

// watchCaption starts following a caption press just handed to the window
// server (captionWatch).
func (w *Window) watchCaption() {
	f := w.frameOf()
	w.ctl.cwatch.start(screenMouse(), [2]float64{f[0], f[1]})
	w.armCaptionPoll()
}

// armCaptionPoll signals a pass after captionPoll, where pollCaption runs.
func (w *Window) armCaptionPoll() {
	if w.ctl.cwPoll == nil {
		w.ctl.cwPoll = time.AfterFunc(captionPoll, w.b.notify)
	} else {
		w.ctl.cwPoll.Reset(captionPoll)
	}
}

// pollCaption runs in every pass while a caption press is followed: once
// the primary button is up (+[NSEvent pressedMouseButtons], the hardware
// state, as the release may never reach the window) the press ends
// (endCaption); until then the cursor's distance is recorded.
func (w *Window) pollCaption() {
	c := &w.ctl.cwatch
	if !c.active || w.Closed() {
		return
	}
	if darwin.SendUint(darwin.ID(app.event), rsel.pressedMouseButtons)&1 == 0 {
		w.endCaption()
		return
	}
	c.moved(screenMouse())
	w.armCaptionPoll()
}

// endCaption ends a followed caption press at its release (the release
// event, the poll, or a move without buttons): a click is reported at the
// cursor as a CaptionClickEvent.
func (w *Window) endCaption() {
	c := &w.ctl.cwatch
	if !c.active {
		return
	}
	if w.ctl.cwPoll != nil {
		w.ctl.cwPoll.Stop()
	}
	f := w.frameOf()
	if c.end(screenMouse(), [2]float64{f[0], f[1]}) {
		p, _ := w.cursorInView()
		w.b.enqueue(w, platform.CaptionClickEvent{Pos: p, Time: w.b.Now()})
	}
}
