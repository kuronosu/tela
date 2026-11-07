package win32

import (
	"fmt"
	"sync"
	"time"
)

// A bound on the calls a drop makes into the drag's source (ADR-234).
//
// A drop target reads the dropped data with IDataObject calls (EnumFormatEtc,
// GetData, and the Read of a stream) on the UI thread, in the middle of
// IDropTarget::Drop or DragEnter. When the data object is another process's,
// each is a COM call across processes, and a source that hangs (a busy or
// suspended program, a virtual file that waits for a network) blocks the UI
// thread of the app for as long as the source does.
//
// The bound is COM's own call cancellation: the thread enables it
// (CoEnableCallCancellation), a timer arms a stallGuard, and when no call
// completes within dropStallTimeout the timer cancels the call pending on the
// thread (CoCancelCall from the timer's goroutine), which returns
// RPC_E_CALL_CANCELED. The thread never waits on another thread, so there is
// no apartment to marshal the data object to and nothing to deadlock: the
// drop target stays on the STA that owns it. The choice against reading on a
// worker thread: a proxy of the source marshaled to another apartment may be
// routed back through the STA that made it, which would be the blocked
// thread. The timeout is per call, not per drop: every call that completes
// (a GetData, a chunk of a stream) starts it again, so a large slow transfer
// that progresses is not cut, and a source that stops answering is.

// dropStallTimeout is how long a call into the source may take before it is
// cancelled (a variable for the tests).
var dropStallTimeout = 5 * time.Second

// errDropStalled is the error of a drop whose data could not be read in
// time. The drop completes without data (DragEvent.Data is empty).
func errDropStalled() error {
	return fmt.Errorf("win32: the drag source did not answer within %v; the drop completes without data", dropStallTimeout)
}

// stallGuard cancels a pending call when none completes for a duration. It
// is armed when created; progress restarts the wait, stop ends it. A nil
// guard guards nothing. cancel runs on the timer's goroutine.
//
// cancel may run just after the call it was for returned on its own; it is
// then a cancellation of nothing (COM answers that no call is pending). The
// guard stops it from running after stop returns only for a timer that has
// not fired yet.
type stallGuard struct {
	mu      sync.Mutex
	d       time.Duration
	timer   *time.Timer
	last    time.Time // of the latest progress
	stalled bool
	done    bool
	cancel  func()
}

// newStallGuard returns a guard that calls cancel when d passes without a
// call to progress.
func newStallGuard(d time.Duration, cancel func()) *stallGuard {
	g := &stallGuard{d: d, cancel: cancel, last: time.Now()}
	g.timer = time.AfterFunc(d, g.fire)
	return g
}

func (g *stallGuard) fire() {
	g.mu.Lock()
	if g.done || g.stalled {
		g.mu.Unlock()
		return
	}
	if idle := time.Since(g.last); idle < g.d {
		// A progress raced with the timer: wait out the rest.
		g.timer.Reset(g.d - idle)
		g.mu.Unlock()
		return
	}
	g.stalled = true
	g.mu.Unlock()
	g.cancel()
}

// progress notes that a call completed: the wait starts again.
func (g *stallGuard) progress() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if !g.done && !g.stalled {
		g.last = time.Now()
		g.timer.Reset(g.d)
	}
	g.mu.Unlock()
}

// Stalled reports whether the guard cancelled (or is cancelling) a call.
func (g *stallGuard) Stalled() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stalled
}

// stop disarms the guard and reports whether it had fired.
func (g *stallGuard) stop() (stalled bool) {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.done = true
	g.timer.Stop()
	return g.stalled
}
