package win32

import "github.com/kuronosu/tela/internal/platform"

// captureAction tells the window what to do with the OS mouse capture after
// a button event.
type captureAction uint8

const (
	captureKeep    captureAction = iota
	captureSet                   // SetCapture: the first button went down
	captureRelease               // ReleaseCapture: the last button went up
)

// pointerState turns the facts mouse messages carry into the pointer events
// of the platform contract. It is the pure half of the Win32 pointer code:
// the window feeds it message by message and sends what it emits to the
// loop, and carries out the capture actions it returns.
//
// The contract it implements is the one the other backends honor:
//   - Enter precedes the first move or press inside the window, and Leave
//     follows the pointer going out.
//   - While a button is held, Windows keeps sending the moves to the window
//     (the mouse is captured), also outside it. A leave during that stay is
//     deferred to the release that ends it, like Wayland and X11, and
//     forgotten if the pointer comes back first.
//   - A press whose release the window will never see (capture stolen by
//     another window, a modal dialog, Alt+Tab) ends with a PointerCancel.
//   - A release without a press the window saw (the press began on the title
//     bar) is dropped.
type pointerState struct {
	inside       bool // the app believes the pointer is in the window
	leavePending bool // the pointer left while buttons were held
	held         platform.Buttons
	pos          platform.Point // last known position, logical px
}

// move handles WM_MOUSEMOVE at pos. inClient says whether the position lies
// inside the client area. It reports whether the window must (re)arm the
// WM_MOUSELEAVE tracking, which is the case while the pointer is inside.
func (p *pointerState) move(pos platform.Point, inClient bool, emit func(platform.PointerEvent)) (track bool) {
	p.pos = pos
	switch {
	case inClient && p.leavePending:
		// Back before the release: the app never saw the pointer leave.
		p.leavePending = false
	case inClient && !p.inside:
		p.enter(emit)
	case !inClient && p.inside && p.held != 0:
		p.leavePending = true
	}
	emit(platform.PointerEvent{Action: platform.PointerMove, Pos: pos, Buttons: p.held})
	return inClient
}

// button handles a button press or release at pos.
func (p *pointerState) button(btn platform.PointerButton, down bool, pos platform.Point, emit func(platform.PointerEvent)) captureAction {
	p.pos = pos
	if down {
		if !p.inside {
			p.enter(emit)
		}
		first := p.held == 0
		p.held |= btn.Mask()
		emit(platform.PointerEvent{Action: platform.PointerDown, Pos: pos, Button: btn, Buttons: p.held})
		if first {
			return captureSet
		}
		return captureKeep
	}
	if !p.held.Has(btn) {
		return captureKeep
	}
	p.held &^= btn.Mask()
	emit(platform.PointerEvent{Action: platform.PointerUp, Pos: pos, Button: btn, Buttons: p.held})
	if p.held != 0 {
		return captureKeep
	}
	if p.leavePending {
		p.leavePending = false
		p.leave(emit)
	}
	return captureRelease
}

// leaveMessage handles WM_MOUSELEAVE: the pointer left the client area. With
// buttons held the leave waits for their release.
func (p *pointerState) leaveMessage(emit func(platform.PointerEvent)) {
	switch {
	case !p.inside:
	case p.held != 0:
		p.leavePending = true
	default:
		p.leave(emit)
	}
}

// captureLost handles losing the mouse capture without the window releasing
// it (WM_CAPTURECHANGED, WM_CANCELMODE): the press sequence is aborted, and
// a pending leave is reported.
func (p *pointerState) captureLost(emit func(platform.PointerEvent)) {
	if p.held == 0 {
		return
	}
	p.held = 0
	emit(platform.PointerEvent{Action: platform.PointerCancel, Pos: p.pos})
	if p.leavePending {
		p.leavePending = false
		p.leave(emit)
	}
}

func (p *pointerState) enter(emit func(platform.PointerEvent)) {
	p.inside = true
	emit(platform.PointerEvent{Action: platform.PointerEnter, Pos: p.pos, Buttons: p.held})
}

func (p *pointerState) leave(emit func(platform.PointerEvent)) {
	p.inside = false
	emit(platform.PointerEvent{Action: platform.PointerLeave, Pos: p.pos, Buttons: p.held})
}
