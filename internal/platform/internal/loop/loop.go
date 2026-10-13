// Package loop is the OS-independent half of a platform backend's event
// loop: the part of the platform contract (see package platform, "Loop
// contract" and "Frames and pacing") that does not depend on how the OS
// reports events. Every backend (wayland, x11, headless, and later win32 and
// cocoa) owns one Loop and keeps only its OS-specific I/O.
//
// # What the Loop owns
//
//   - The FIFO queue of window events, app events (nil window) and deferred
//     callbacks (asynchronous clipboard replies), plus coalesced
//     ConfigureEvents whose metrics are taken when they are delivered.
//   - Wake and Quit: atomic flags plus the backend's Notify hook that
//     unblocks the OS wait. Many Wake calls yield one WakeEvent per pass.
//   - Per-window state ([WindowState], embedded in the backend's window):
//     the metrics of the latest delivered ConfigureEvent, closed, frame
//     requests, the last frame time, "the next Present is full", and an
//     optional display signal (vsync) gate.
//   - The delivery pass ([Loop.Deliver]): a WakeEvent, the queued items in
//     order with Quit checks between them, then one FrameEvent per window
//     whose frame is due. A ConfigureEvent updates the window's metrics
//     before the handler sees it and forces a frame. The pass is never
//     re-entered: a Deliver called from inside the handler (a Win32 WndProc
//     running during SetWindowPos) returns without delivering, and what it
//     queued waits for the next pass.
//   - Pacing: which windows get a FrameEvent now ([Loop.Ready]) and, for
//     requested frames that are not due yet, either a display signal the
//     backend arranges (its Pace hook) or a timer deadline ([Loop.Pace]).
//
// # Frame-due rule
//
// A window's frame is due when the window is visible (delivered metrics with
// a non-empty physical size, not closed, not hidden) and either a
// ConfigureEvent was delivered since its last FrameEvent (forced), or a
// frame was requested, no display signal is outstanding, and one of these
// holds: a display signal arrived since the last FrameEvent; the window never
// had a FrameEvent; the last one is at least one Interval old ("has not drawn
// recently": deliver on the next pass).
//
// Backends without display signals (X11 core protocol, headless) get plain
// interval pacing from this rule. Wayland uses wl_surface.frame callbacks as
// the display signal: Present and its Pace hook call
// [WindowState.VsyncRequested] when they request a callback, and the done
// event calls [WindowState.VsyncDone]. A requested frame that is not due and
// whose Pace hook arranges no signal (Wayland before the first buffer, or
// while a configure waits for its buffer; every request on X11 and headless)
// becomes due at lastFrame + Interval, the deadline Pace returns for the
// backend's poll timeout or timer. Nothing is armed while no frame is
// requested, so an idle loop never wakes up (0% CPU).
//
// # Reuse by future backends
//
// Win32: WndProc translates messages into Enqueue, Coalesce and Reconfigure
// calls; Notify posts a private message to a message-only window; the
// GetMessageW loop (and WndProc itself inside modal move/resize loops)
// calls Deliver after dispatching, then Pace to arm a SetTimer for the
// deadline (or, from a Pace hook, a DwmFlush-paced post that ends in
// VsyncDone). WM_SIZE becomes Reconfigure followed by a synchronous Deliver,
// which delivers the ConfigureEvent and its forced FrameEvent before
// DefWindowProc continues; if WM_SIZE arrives while the handler runs, the
// re-entrancy guard defers it to the next pass.
//
// Cocoa: the run loop source that Notify signals calls Deliver; the Pace
// hook starts the display link and calls VsyncRequested, and each link tick
// calls VsyncDone and signals the source. When Pace reports no outstanding
// request the link is stopped.
//
// # Threading
//
// Wake, Quit and Quitting are safe from any goroutine. Everything else runs
// on the UI thread. Backends whose state is also touched by other goroutines
// (headless, whose injection API is goroutine-safe) set Options.Lock: every
// method other than Wake, Quit, Quitting and Deliver must then be called
// with it held, and Deliver takes it itself and never holds it while the
// handler or a queued callback runs. The Now and Pace hooks run with the
// lock held.
package loop

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Window is the constraint on a backend's window type: a comparable pointer
// that implements platform.Window and embeds a [WindowState] (which provides
// LoopState, Metrics and RequestFrame).
type Window interface {
	comparable
	platform.Window
	LoopState() *WindowState
}

// Options configures a Loop. Now and Notify are required.
type Options[W Window] struct {
	// Now is the backend clock, the time base of FrameEvent.Time.
	Now func() time.Duration
	// Notify unblocks the backend's OS wait (an eventfd write, a channel
	// send, PostMessageW, CFRunLoopSourceSignal). It is called by Wake and
	// Quit from any goroutine, must not block, and may coalesce.
	Notify func()
	// Interval is the refresh interval of timer pacing and of the "has not
	// drawn recently" rule. Zero means 1/60 s.
	Interval time.Duration
	// Pace, if set, is offered every requested frame that is not due yet
	// (visible, not forced, no display signal outstanding). It returns true
	// if it arranged a display signal for it (and called VsyncRequested), so
	// no timer is needed; false falls back to the timer deadline.
	Pace func(w W, now time.Duration) bool
	// Lock, if set, guards the loop state together with the backend's own;
	// see the package documentation.
	Lock sync.Locker
}

// item is a queued unit of work: a window event (w non-zero), an app event
// (w zero), a deferred callback (fn non-nil), or a coalesced ConfigureEvent
// (configure; the metrics are in the window's state).
type item[W Window] struct {
	w         W
	ev        platform.Event
	fn        func()
	configure bool
}

// Loop is the shared event delivery and frame pacing state of one backend.
// Create it with New.
type Loop[W Window] struct {
	opts Options[W]

	wake atomic.Bool // a WakeEvent is owed
	quit atomic.Bool

	windows    []W // open windows in creation order
	queue      []item[W]
	delivering bool // a Deliver pass is running (re-entrancy guard)
}

// DefaultInterval is the Interval used when Options.Interval is zero.
const DefaultInterval = time.Second / 60

// New returns a Loop. It panics if Now or Notify is nil.
func New[W Window](opts Options[W]) *Loop[W] {
	if opts.Now == nil || opts.Notify == nil {
		panic("loop: Options.Now and Options.Notify are required")
	}
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	return &Loop[W]{opts: opts}
}

func (l *Loop[W]) lock() {
	if l.opts.Lock != nil {
		l.opts.Lock.Lock()
	}
}

func (l *Loop[W]) unlock() {
	if l.opts.Lock != nil {
		l.opts.Lock.Unlock()
	}
}

// Interval returns the pacing interval.
func (l *Loop[W]) Interval() time.Duration { return l.opts.Interval }

// --- any goroutine ---

// Wake makes the next pass deliver a WakeEvent and unblocks the OS wait.
// Safe from any goroutine; coalesces.
func (l *Loop[W]) Wake() {
	l.wake.Store(true)
	l.opts.Notify()
}

// Quit makes Deliver stop at the next handler boundary and Ready report
// true, and unblocks the OS wait. Safe from any goroutine; idempotent.
func (l *Loop[W]) Quit() {
	l.quit.Store(true)
	l.opts.Notify()
}

// Quitting reports whether Quit was called. Safe from any goroutine.
func (l *Loop[W]) Quitting() bool { return l.quit.Load() }

// --- windows ---

// Add registers a new window; FrameEvents are chosen in creation order.
func (l *Loop[W]) Add(w W) {
	l.windows = append(l.windows, w)
}

// Remove closes w for the loop: it is marked closed (its WindowState
// methods become no-ops and it is never delivered to again), it leaves the
// window list, and its queued events are dropped. Removing a window twice
// is harmless.
func (l *Loop[W]) Remove(w W) {
	s := w.LoopState()
	s.closed = true
	s.requested, s.forced = false, false
	l.windows = slices.DeleteFunc(l.windows, func(x W) bool { return x == w })
	l.queue = slices.DeleteFunc(l.queue, func(it item[W]) bool { return it.w == w })
}

// Windows returns the open windows in creation order. The slice is owned by
// the Loop: do not modify it, and do not keep it across Add or Remove.
func (l *Loop[W]) Windows() []W { return l.windows }

// --- queue ---

// Enqueue queues e for w, or an app-scoped event when w is the zero W (nil).
// Events for a closed window are dropped. Enqueue does not call Notify:
// the UI thread queues from its own I/O handling, and other goroutines
// notify themselves.
func (l *Loop[W]) Enqueue(w W, e platform.Event) {
	var zero W
	if w != zero && w.LoopState().closed {
		return
	}
	l.queue = append(l.queue, item[W]{w: w, ev: e})
}

// Coalesce queues e for w, unless the last queued item is an event for the
// same window for which replace(prev) reports true: then e replaces it in
// place (consecutive pointer motion becomes one move).
func (l *Loop[W]) Coalesce(w W, e platform.Event, replace func(prev platform.Event) bool) {
	if n := len(l.queue); n > 0 {
		last := &l.queue[n-1]
		if last.w == w && last.ev != nil && last.fn == nil && !last.configure && replace(last.ev) {
			last.ev = e
			return
		}
	}
	l.Enqueue(w, e)
}

// Reconfigure queues a coalesced ConfigureEvent for w with metrics m. Until
// it is delivered, later calls only update m, so a burst of OS resizes
// becomes one ConfigureEvent with the latest metrics, delivered at the
// position of the first. It is dropped if m equals the metrics already
// delivered.
func (l *Loop[W]) Reconfigure(w W, m platform.Metrics) {
	s := w.LoopState()
	if s.closed {
		return
	}
	s.configure = m
	if s.configureQueued {
		return
	}
	s.configureQueued = true
	l.queue = append(l.queue, item[W]{w: w, configure: true})
}

// Post queues fn to run on the UI thread during a later pass, in queue
// order (asynchronous replies such as Clipboard.ReadText).
func (l *Loop[W]) Post(fn func()) {
	l.queue = append(l.queue, item[W]{fn: fn})
}

// Discard drops every queued item (the backend is shutting down).
func (l *Loop[W]) Discard() {
	clear(l.queue)
	l.queue = nil
}

// --- pass ---

// Deliver runs one delivery pass and returns how many events and callbacks
// it delivered: a WakeEvent if Wake was called, the items queued before the
// pass (FIFO), then one FrameEvent per window whose frame is due, in
// creation order, all with the same Time. Items queued while the pass runs
// wait for the next one, except frame requests, which are evaluated after
// the items (an input event that requests a frame gets it in the same pass).
// It stops as soon as Quit is called, and returns 0 at once when called
// from inside its own handler.
func (l *Loop[W]) Deliver(h platform.Handler) int {
	if l.delivering || l.quit.Load() {
		return 0
	}
	l.delivering = true
	defer func() { l.delivering = false }()

	var zero W
	n := 0
	if l.wake.Swap(false) {
		h.HandleEvent(nil, platform.WakeEvent{})
		n++
	}

	l.lock()
	items := l.queue
	l.queue = nil
	l.unlock()
	for _, it := range items {
		if l.quit.Load() {
			return n
		}
		switch {
		case it.fn != nil:
			it.fn()
		case it.w == zero:
			h.HandleEvent(nil, it.ev)
		default:
			l.lock()
			ev, ok := it.w.LoopState().prepare(it.ev, it.configure)
			l.unlock()
			if !ok {
				continue
			}
			h.HandleEvent(it.w, ev)
		}
		n++
	}

	// Frames: decide which are due once, then deliver.
	l.lock()
	now := l.opts.Now()
	var due []W
	for _, w := range l.windows {
		if s := w.LoopState(); s.due(now, l.opts.Interval) {
			s.framed(now)
			due = append(due, w)
		}
	}
	l.unlock()
	for _, w := range due {
		if l.quit.Load() {
			return n
		}
		l.lock()
		closed := w.LoopState().closed
		l.unlock()
		if closed {
			continue
		}
		h.HandleEvent(w, platform.FrameEvent{Time: now})
		n++
	}
	return n
}

// Ready reports whether a pass would deliver something now (or Quit was
// called): the backend's OS wait must not block.
func (l *Loop[W]) Ready() bool {
	if l.quit.Load() || l.wake.Load() || len(l.queue) > 0 {
		return true
	}
	now := l.opts.Now()
	for _, w := range l.windows {
		if w.LoopState().due(now, l.opts.Interval) {
			return true
		}
	}
	return false
}

// Pace arranges for every requested frame that is not due yet to become due
// without busy-looping: it offers the frame to the Pace hook (a display
// signal), and otherwise returns the earliest time a frame becomes due by
// the interval rule, for the backend's poll timeout or timer. ok is false
// when no timer is needed, which is always the case while no frame is
// requested.
func (l *Loop[W]) Pace() (deadline time.Duration, ok bool) {
	now := l.opts.Now()
	for _, w := range l.windows {
		s := w.LoopState()
		if !s.visible() || s.forced || !s.requested || s.vsyncPending || s.due(now, l.opts.Interval) {
			continue
		}
		if l.opts.Pace != nil && l.opts.Pace(w, now) {
			continue
		}
		if t := s.last + l.opts.Interval; !ok || t < deadline {
			deadline, ok = t, true
		}
	}
	return deadline, ok
}
