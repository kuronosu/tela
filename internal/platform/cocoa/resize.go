package cocoa

import (
	"math"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the app's resize handles and of caption clicks
// (resize_darwin.go), tested on every OS.

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

// resizeLimits returns the frame size limits of the backend's resize loop
// (resizedFrame's lo and hi) for the options' MinSize and MaxSize, which
// are content sizes (sizeLimits), deco, the frame's size beyond the
// content (the title bar of a titled window; zero for a frameless one,
// whose frame is its content), and floor, the content's
// platform.ResizeFloor at the press (the handles keep two 16 pt corners,
// the margin around them as it is, ADR-261; the fixed 32 pt of before let
// a card inset by a wide shadow margin lose its handles first): the
// content limits plus deco, floor on an axis without a minimum, the
// maximum raised to the minimum.
func resizeLimits(min, max platform.Size, deco [2]float64, floor platform.Size) (lo, hi [2]float64) {
	lo, hi = sizeLimits(min, max)
	fl := [2]float64{float64(floor.W), float64(floor.H)}
	for i := range 2 {
		if lo[i] <= 0 {
			lo[i] = fl[i]
		}
		lo[i] += deco[i]
		if hi[i] != noMaxSize {
			hi[i] = math.Max(hi[i]+deco[i], lo[i])
		}
	}
	return lo, hi
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
