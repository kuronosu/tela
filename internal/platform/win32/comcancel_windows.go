//go:build windows

package win32

import (
	"time"

	"golang.org/x/sys/windows"
)

// COM call cancellation, the bound on the calls into a drag's source
// (stallguard.go has the design).

var (
	procCoEnableCallCancellation  = ole32.NewProc("CoEnableCallCancellation")
	procCoDisableCallCancellation = ole32.NewProc("CoDisableCallCancellation")
	procCoCancelCall              = ole32.NewProc("CoCancelCall")
)

// comGuard is a stallGuard for the COM calls the UI thread makes while it
// is open.
type comGuard struct{ *stallGuard }

// guardCOM lets the calling thread's pending COM call be cancelled and
// arms a guard that cancels it when no call completes for timeout. The
// caller ends it with end, on the same thread. Cancellation needs the
// thread to have COM (the UI thread has OLE for drag and drop); without it
// the calls simply are not bounded.
func guardCOM(timeout time.Duration) *comGuard {
	procCoEnableCallCancellation.Call(0)
	tid := windows.GetCurrentThreadId()
	return &comGuard{newStallGuard(timeout, func() {
		procCoCancelCall.Call(uintptr(tid), 0)
	})}
}

// end disarms the guard and the thread's call cancellation, and reports
// whether the guard cancelled a call.
func (g *comGuard) end() (stalled bool) {
	stalled = g.stop()
	procCoDisableCallCancellation.Call(0)
	return stalled
}

// guarded runs f, which makes COM calls into the drag's source, under a
// guard of dropStallTimeout, and reports whether it was cut short.
func guarded(f func(g *stallGuard)) (stalled bool) {
	g := guardCOM(dropStallTimeout)
	f(g.stallGuard)
	return g.end()
}
