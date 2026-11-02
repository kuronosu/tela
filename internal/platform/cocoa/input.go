package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the Cocoa pointer and scroll input: the rules that turn
// the facts an NSEvent carries into the events of the platform contract.
// It has no build constraint, so it is tested on every OS; the darwin
// files read the NSEvents and feed it.

// NSEventPhase bits ([NSEvent phase] and momentumPhase, NSEvent.h),
// repeated here because internal/darwin exists only on darwin.
const (
	phaseBegan      = 1 << 0
	phaseStationary = 1 << 1
	phaseChanged    = 1 << 2
	phaseEnded      = 1 << 3
	phaseCancelled  = 1 << 4
	phaseMayBegin   = 1 << 5
)

// macButton returns the pointer button of [NSEvent buttonNumber]: 0 is the
// left (primary) button, 1 the right, 2 the middle and 3 and 4 the side
// buttons (back and forward, as winit and Chromium read them). Other
// buttons are not reported (ButtonNone).
//
// AppKit sends a Control-click as a left mouseDown: with the Control flag,
// not as a right click; it stays a primary press with ModCtrl, as in
// browsers (the layers above decide whether it opens a context menu).
func macButton(number int) platform.PointerButton {
	switch number {
	case 0:
		return platform.ButtonPrimary
	case 1:
		return platform.ButtonSecondary
	case 2:
		return platform.ButtonMiddle
	case 3:
		return platform.ButtonBack
	case 4:
		return platform.ButtonForward
	}
	return platform.ButtonNone
}

// macScroll is what macScrollEvent reads of an NSEvent of type
// scrollWheel.
type macScroll struct {
	// DX and DY are scrollingDeltaX and scrollingDeltaY: points with
	// precise deltas, lines (accelerated, possibly fractional) without.
	DX, DY  float64
	Precise bool   // hasPreciseScrollingDeltas
	Phase   uint64 // phase: the fingers on a touchpad
	// Momentum is momentumPhase: the inertia AppKit generates after the
	// fingers lift.
	Momentum uint64
}

// macScrollEvent translates a scrollWheel: event into the Delta, Steps and
// Phase of a ScrollEvent; ok is false when nothing is delivered.
//
//   - Signs: AppKit's deltas move the content (positive Y = the content
//     moves down, toward the top of the document) and already include the
//     user's natural scrolling setting, so the DOM's signs (positive Y
//     scrolls toward the end) are their negation, whatever the setting, as
//     in Chromium and Safari.
//   - Precise deltas (touchpads, the Magic Mouse) are Delta in points;
//     other wheels report lines, which are Steps (notches).
//   - Phases: the touchpad's gesture (phase began, changed, stationary or
//     mayBegin) is ScrollPhaseUpdate, its end (ended or cancelled: the
//     fingers lifted) ScrollPhaseEnd, as the contract wants for the layers
//     above to start their fling. An update without movement (mayBegin,
//     stationary) is dropped.
//   - Momentum: the events AppKit generates after the fingers lift
//     (momentumPhase set, phase none) are dropped: the layers above fling
//     themselves from the gesture's velocity after the ScrollPhaseEnd, with
//     the same deceleration on every platform (ADR-057), and delivering
//     both would scroll twice.
func macScrollEvent(s macScroll) (delta, steps platform.Point, phase platform.ScrollPhase, ok bool) {
	if s.Phase == 0 && s.Momentum != 0 {
		return delta, steps, phase, false
	}
	d := platform.Point{X: float32(-s.DX), Y: float32(-s.DY)}
	// -0 would compare equal anyway; keep zero deltas clean in logs.
	if s.DX == 0 {
		d.X = 0
	}
	if s.DY == 0 {
		d.Y = 0
	}
	switch {
	case s.Phase&(phaseEnded|phaseCancelled) != 0:
		phase = platform.ScrollPhaseEnd
	case s.Phase != 0:
		phase = platform.ScrollPhaseUpdate
		if d == (platform.Point{}) {
			return delta, steps, phase, false
		}
	default:
		if d == (platform.Point{}) {
			return delta, steps, phase, false
		}
	}
	if s.Precise || phase != platform.ScrollPhaseNone {
		return d, steps, phase, true
	}
	return delta, d, phase, true
}

// pointerState turns mouse events into the pointer events of the platform
// contract, like the Win32 backend's pointerState:
//
//   - Enter precedes the first move or press inside the view, and Leave
//     follows the pointer going out (the tracking area's mouseExited:, or
//     a move outside the bounds while no button is held).
//   - While a button is held AppKit keeps sending the drags to the view
//     that got the press, also outside the window (implicit capture). A
//     leave during that stay is deferred to the release that ends it, as
//     on the other backends, and forgotten if the pointer comes back
//     first.
//   - A release without a press the view saw (the press began on the title
//     bar, or in another window) is dropped.
type pointerState struct {
	inside       bool // the app believes the pointer is in the view
	leavePending bool // the pointer left while buttons were held
	held         platform.Buttons
	pos          platform.Point // last known position, logical px
}

// move handles mouseMoved: and the drags at pos; inside says whether pos
// lies in the view's bounds.
func (p *pointerState) move(pos platform.Point, inside bool, emit func(platform.PointerEvent)) {
	p.pos = pos
	switch {
	case inside && p.leavePending:
		p.leavePending = false
	case inside && !p.inside:
		p.enter(emit)
	case !inside && p.inside && p.held != 0:
		p.leavePending = true
	case !inside && p.inside:
		// A move outside without a button (the exit was missed): leave
		// before reporting it would put a move outside the view.
		p.leave(emit)
		return
	case !inside:
		return // not ours: no button held and not inside
	}
	emit(platform.PointerEvent{Action: platform.PointerMove, Pos: pos, Buttons: p.held})
}

// entered handles mouseEntered: at pos.
func (p *pointerState) entered(pos platform.Point, emit func(platform.PointerEvent)) {
	p.pos = pos
	switch {
	case p.leavePending:
		p.leavePending = false
	case !p.inside:
		p.enter(emit)
	}
}

// exited handles mouseExited: at pos: with buttons held the leave waits
// for their release.
func (p *pointerState) exited(pos platform.Point, emit func(platform.PointerEvent)) {
	p.pos = pos
	switch {
	case !p.inside:
	case p.held != 0:
		p.leavePending = true
	default:
		p.leave(emit)
	}
}

// button handles a press or a release of btn at pos.
func (p *pointerState) button(btn platform.PointerButton, down bool, pos platform.Point, emit func(platform.PointerEvent)) {
	if btn == platform.ButtonNone {
		return
	}
	p.pos = pos
	if down {
		if !p.inside {
			p.enter(emit)
		}
		p.held |= btn.Mask()
		emit(platform.PointerEvent{Action: platform.PointerDown, Pos: pos, Button: btn, Buttons: p.held})
		return
	}
	if !p.held.Has(btn) {
		return
	}
	p.held &^= btn.Mask()
	emit(platform.PointerEvent{Action: platform.PointerUp, Pos: pos, Button: btn, Buttons: p.held})
	if p.held == 0 && p.leavePending {
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

// inBounds reports whether pos lies in a view of size s (points).
func inBounds(pos platform.Point, s platform.Size) bool {
	return pos.X >= 0 && pos.Y >= 0 && pos.X < s.W && pos.Y < s.H
}

// eventClock maps NSEvent timestamps (seconds of system uptime, the base of
// -[NSProcessInfo systemUptime]) to the backend's clock.
type eventClock struct {
	// offset is the backend time at uptime 0.
	offset time.Duration
}

// maxClockSkew is how far an event time may be from the backend's clock
// before the mapping is recomputed: the two clocks are both monotonic
// uptimes, but a sleep could still separate them.
const maxClockSkew = 10 * time.Second

// time returns the backend time of an event stamped ts, given the current
// backend time now and system uptime; it re-bases the mapping when ts is
// implausibly far from now (and then returns now).
func (c *eventClock) time(ts float64, now time.Duration, uptime func() float64) time.Duration {
	t := c.offset + time.Duration(ts*float64(time.Second))
	if d := t - now; d > maxClockSkew || d < -maxClockSkew {
		c.offset = now - time.Duration(uptime()*float64(time.Second))
		return now
	}
	return t
}
