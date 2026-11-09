package headless

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/platform"
)

// Transparent windows (ADR-249). A WindowOptions.Transparent window
// reports WindowState.Transparent unless Options.NoCompositing, keeps the
// input mask of the frames it is given (platform.InputMask), and lets the
// pointer events over the pixels outside the mask pass to "what is below":
// they are not delivered. As on the real backends, the pointer leaving
// the mask is a PointerLeave and coming back a PointerEnter, and a
// pointer sequence that started in the mask (a button held) stays the
// window's until its release, as the OS's implicit grab keeps it. While
// the window can be resized, its resize handles (HitResize… regions)
// take input too, over transparent pixels as well (contract v12,
// ADR-261: the outside of a rounded card's corner).

// passPointer reports whether e falls through the window, queueing the
// leave or enter the crossing of the mask's edge brings.
func (w *Window) passPointer(e platform.PointerEvent) bool {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if !w.opts.Transparent || !w.ctl.osState.Transparent || w.WindowState.Closed() {
		return false
	}
	switch e.Action {
	case platform.PointerEnter, platform.PointerLeave:
		return w.passing
	case platform.PointerCancel:
		w.passHeld = 0
		return w.passing
	}
	if w.passHeld != 0 {
		// The pointer sequence is the window's.
		switch e.Action {
		case platform.PointerDown:
			w.passHeld |= e.Button.Mask()
		case platform.PointerUp:
			w.passHeld &^= e.Button.Mask()
		}
		return false
	}
	in := w.takesInputLocked(e.Pos)
	if in && e.Action == platform.PointerDown {
		w.passHeld = e.Button.Mask()
	}
	switch {
	case in && w.passing:
		w.passing = false
		enter := e
		enter.Action, enter.Button = platform.PointerEnter, 0
		b.loop.Enqueue(w, enter)
		return false
	case !in && !w.passing:
		w.passing = true
		leave := e
		leave.Action, leave.Button = platform.PointerLeave, 0
		b.loop.Enqueue(w, leave)
		b.signal()
		return true
	}
	return !in
}

// TakesInput reports whether a pointer event at p (logical window
// coordinates) reaches the window: always, except for a Transparent
// window the fake desktop composites, where only the pixels of the last
// frames whose alpha is at least platform.InputAlpha take input, and the
// resize handles while the window can be resized.
func (w *Window) TakesInput(p platform.Point) bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.opts.Transparent || !w.ctl.osState.Transparent {
		return true
	}
	return w.takesInputLocked(p)
}

func (w *Window) takesInputLocked(p platform.Point) bool {
	s := w.WindowState.Metrics().Scale
	if s == 0 {
		s = 1
	}
	px := image.Pt(int(math.Floor(float64(p.X)*s)), int(math.Floor(float64(p.Y)*s)))
	return w.mask.Contains(px) || w.resizable() && platform.HitEdge(w.ctl.regions, p) != 0
}

// InputRects returns the input region of a Transparent window: the mask
// of its presented frames as rectangles in physical pixels (exact, as
// platform.InputMask.Rects with no limit), nil for other windows.
func (w *Window) InputRects() []image.Rectangle {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.opts.Transparent {
		return nil
	}
	return w.mask.Rects(math.MaxInt32)
}

// SetCompositing simulates a compositing manager starting (on) or
// stopping under a Transparent window: its WindowState.Transparent
// follows, with a WindowStateEvent when it changes. It does nothing for
// other windows. Safe from any goroutine.
func (w *Window) SetCompositing(on bool) {
	if !w.opts.Transparent {
		return
	}
	w.changeState(func(s *platform.WindowState) { s.Transparent = on })
}
