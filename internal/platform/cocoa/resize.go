package cocoa

import (
	"math"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the app's resize handles and of caption clicks
// (resize_darwin.go), tested on every OS.

// resizeInput is a resize the backend runs itself from one of the app's
// resize handles (a HitResize… region, h.WindowResize; ADR-257). AppKit
// has no public call that starts the window server's resize from an event
// (performWindowDragWithEvent: only moves), and a transparent window is
// not resizable (its edges are the transparent margin), so the press
// starts a loop of the drags AppKit sends the window while the button is
// held: each one sets the frame from the press's frame and the cursor's
// motion since the press, in screen points (y up), as Chromium's and
// Electron's custom resize areas do on macOS.
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

// resizedFrame returns frame f (x, y, w, h in screen points, y up) resized
// from edge e by the cursor's motion (dx, dy; y up), its size kept within
// lo and hi: the opposite sides stay where they are.
func resizedFrame(f [4]float64, e platform.Edge, dx, dy float64, lo, hi [2]float64) [4]float64 {
	x, y, w, h := f[0], f[1], f[2], f[3]
	clamp := func(v float64, i int) float64 { return math.Max(lo[i], math.Min(v, hi[i])) }
	switch {
	case e&platform.EdgeRight != 0:
		w = clamp(f[2]+dx, 0)
	case e&platform.EdgeLeft != 0:
		w = clamp(f[2]-dx, 0)
		x = f[0] + f[2] - w
	}
	switch {
	case e&platform.EdgeTop != 0: // y up: the top is y + h
		h = clamp(f[3]+dy, 1)
	case e&platform.EdgeBottom != 0:
		h = clamp(f[3]-dy, 1)
		y = f[1] + f[3] - h
	}
	return [4]float64{x, y, w, h}
}

// captionWatch follows a caption press that performWindowDragWithEvent:
// handed to the window server, to tell a click from a drag (ADR-252): the
// call returns at once and the release may never reach the window, so
// the pass polls +[NSEvent pressedMouseButtons] and the cursor while the
// primary button is held. A click is a release whose cursor never went
// farther than the drag threshold from the press and whose window did not
// move.
type captionWatch struct {
	active bool
	mouse  [2]float64 // the cursor at the press (screen points)
	origin [2]float64 // the window's origin at the press
	far    bool       // the cursor left the threshold
}

// start begins watching a press at cursor mouse with the window at origin.
func (c *captionWatch) start(mouse, origin [2]float64) {
	*c = captionWatch{active: true, mouse: mouse, origin: origin}
}

// moved records the cursor at mouse.
func (c *captionWatch) moved(mouse [2]float64) {
	dx, dy := mouse[0]-c.mouse[0], mouse[1]-c.mouse[1]
	if dx*dx+dy*dy >= platform.CaptionDragThreshold*platform.CaptionDragThreshold {
		c.far = true
	}
}

// end stops watching at the release, with the cursor at mouse and the
// window at origin, and reports whether the press was a click.
func (c *captionWatch) end(mouse, origin [2]float64) bool {
	if !c.active {
		return false
	}
	c.moved(mouse)
	click := !c.far && origin == c.origin
	*c = captionWatch{}
	return click
}
