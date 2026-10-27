package loop

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// WindowState is the loop's state of one window. A backend embeds it by
// value in its window type, which gives the window the platform.Window
// methods Metrics and RequestFrame and lets the Loop find the state through
// LoopState. With Options.Lock set, every method must be called with the
// lock held.
type WindowState struct {
	metrics   platform.Metrics // delivered by the latest ConfigureEvent
	closed    bool
	hidden    bool // the backend vetoes frames (X11: iconified)
	requested bool // RequestFrame since the last FrameEvent
	forced    bool // a ConfigureEvent was delivered since the last FrameEvent
	hasFramed bool // a FrameEvent was delivered
	last      time.Duration
	needsFull bool // the next Present is full (after a ConfigureEvent)

	vsyncPending bool // a display signal (frame callback) is outstanding
	vsyncDone    bool // a display signal arrived since the last FrameEvent

	configureQueued bool             // a coalesced ConfigureEvent is queued
	configure       platform.Metrics // its metrics (see Loop.Reconfigure)

	state       platform.WindowState // delivered by the latest WindowStateEvent
	stateQueued bool                 // a coalesced WindowStateEvent is queued
	nextState   platform.WindowState // its state (see Loop.UpdateState)
}

// LoopState returns s; it lets the Loop reach the state embedded in a
// backend window.
func (s *WindowState) LoopState() *WindowState { return s }

// Metrics returns the metrics of the latest delivered ConfigureEvent (the
// zero Metrics before the first one), as platform.Window requires.
func (s *WindowState) Metrics() platform.Metrics { return s.metrics }

// State returns the state of the latest delivered WindowStateEvent (the
// zero WindowState before the first one), as platform.Window requires.
func (s *WindowState) State() platform.WindowState { return s.state }

// RequestFrame asks for a FrameEvent; requests coalesce until the next
// FrameEvent. No-op once the window is closed.
func (s *WindowState) RequestFrame() {
	if !s.closed {
		s.requested = true
	}
}

// Closed reports whether the window was removed from its Loop.
func (s *WindowState) Closed() bool { return s.closed }

// FramePending reports whether a FrameEvent is owed: requested, or forced
// by a delivered ConfigureEvent.
func (s *WindowState) FramePending() bool { return s.requested || s.forced }

// LastFrame returns the Time of the latest FrameEvent (0 before the first).
func (s *WindowState) LastFrame() time.Duration { return s.last }

// NeedsFull reports whether the next Present must be treated as a full
// frame: a ConfigureEvent was delivered since the last full Present.
func (s *WindowState) NeedsFull() bool { return s.needsFull }

// ClearNeedsFull records that a full frame was presented.
func (s *WindowState) ClearNeedsFull() { s.needsFull = false }

// SetHidden pauses (true) or resumes frame delivery for a window the OS
// does not show (X11: iconified; any backend: hidden by
// platform.Window.SetVisible). Requests made while hidden stay pending. A
// backend with several reasons passes their union (hidden by the app or
// iconified).
func (s *WindowState) SetHidden(hidden bool) { s.hidden = hidden }

// Hidden reports the value of the latest SetHidden.
func (s *WindowState) Hidden() bool { return s.hidden }

// VsyncRequested records that the backend requested a display signal (a
// Wayland frame callback): requested frames wait for it.
func (s *WindowState) VsyncRequested() { s.vsyncPending = true }

// VsyncDone records that the display signal arrived: the display is ready
// for a new frame, so a requested frame is due. No-op once closed.
func (s *WindowState) VsyncDone() {
	if !s.closed {
		s.vsyncPending = false
		s.vsyncDone = true
	}
}

// VsyncPending reports whether a display signal is outstanding.
func (s *WindowState) VsyncPending() bool { return s.vsyncPending }

// visible reports whether frames can be delivered: delivered metrics with a
// non-empty physical size, not closed and not hidden.
func (s *WindowState) visible() bool {
	if s.closed || s.hidden || s.metrics.Scale == 0 {
		return false
	}
	p := s.metrics.Physical()
	return p.X > 0 && p.Y > 0
}

// due is the frame-due rule of the package documentation.
//
// The interval branch alone must never allow two FrameEvents in a row
// without a display signal or an interval between them: a handler that
// requests a frame without presenting (an animation tick with no pixel
// change) would otherwise make the loop spin at 100% CPU.
func (s *WindowState) due(now, interval time.Duration) bool {
	if !s.visible() {
		return false
	}
	if s.forced {
		return true
	}
	if !s.requested || s.vsyncPending {
		return false
	}
	return s.vsyncDone || !s.hasFramed || now-s.last >= interval
}

// framed records a FrameEvent at now.
func (s *WindowState) framed(now time.Duration) {
	s.requested, s.forced, s.vsyncDone = false, false, false
	s.last, s.hasFramed = now, true
}

// prepare is called for a queued window item right before delivery. It
// applies a ConfigureEvent (metrics, full Present, forced frame) or a
// WindowStateEvent (state) and returns the event to deliver; ok is false
// if the item must be skipped (closed window, or a coalesced configure or
// state that did not change).
func (s *WindowState) prepare(ev platform.Event, configure, restate bool) (platform.Event, bool) {
	if s.closed {
		return nil, false
	}
	if configure {
		s.configureQueued = false
		if s.configure == s.metrics {
			return nil, false
		}
		ev = platform.ConfigureEvent{Metrics: s.configure}
	}
	if restate {
		s.stateQueued = false
		if s.nextState == s.state {
			return nil, false
		}
		ev = platform.WindowStateEvent{State: s.nextState}
	}
	switch e := ev.(type) {
	case platform.ConfigureEvent:
		s.metrics = e.Metrics
		s.needsFull = true
		s.forced = true
	case platform.WindowStateEvent:
		s.state = e.State
	}
	return ev, true
}
