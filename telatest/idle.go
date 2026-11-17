package telatest

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/platform/headless"
)

// Pending names what keeps the app from being idle (ADR-291): events
// waiting to be delivered, a frame request or a frame owed to a window, a
// paced commit of a window without frames, and in each window's tree
// queued updates, changed elements, running animations (endless ones
// too), scroll animations, and the deadlines of the tree: every
// UseInterval and UseTimeout running (with its component and, in dev mode,
// where it is called), a caret blink, a long press, overlay scrollbars
// waiting to fade. Items of a window other than this App's are prefixed
// with its title. It is empty when the app is idle: an idle app asks for
// no frame and arms no timer (ARCHITECTURE §3.5, "0% CPU when idle").
//
// The app is settled first, as a query is: the work due is handled and,
// unless the app was mounted with [NoSettle], finite animations run to
// their end. The clock is not moved to deadlines, so a UseTimeout that
// has not fired yet is pending.
func (a *App) Pending() []string {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	return a.pending()
}

// ExpectIdle fails the test, naming what is pending ([App.Pending]),
// unless the app is idle once settled: nothing asks for a frame and
// nothing waits for a deadline. A screen that nobody touches should cost
// nothing; a forgotten UseInterval, an endless animation left mounted or
// a toast's timer show here, though they do not show in [App.Stats] until
// the clock reaches them.
//
//	app := telatest.Mount(t, tela.C(Clock))
//	app.ExpectIdle()
//	// telatest: ExpectIdle: the app is not idle: a UseInterval every 1s
//	// in <Clock> (called at clock.go:14), next in 1s
//
// To check that a screen does no work over time as well, measure a
// stretch of the clock:
//
//	if st := app.Measure(func() { app.Advance(5 * time.Second) }); st.Frames != 0 {
//		t.Errorf("%d frames at rest", st.Frames)
//	}
func (a *App) ExpectIdle() {
	a.tb.Helper()
	if why := a.Pending(); len(why) > 0 {
		a.tb.Fatalf("telatest: ExpectIdle: the app is not idle: %s", strings.Join(why, "; "))
	}
}

// pending is Pending without settling.
func (a *App) pending() []string {
	var why []string
	if a.backend.Pending() {
		why = append(why, "events waiting to be delivered")
	}
	if a.host.NeedsFrame() {
		why = append(why, "a window whose frame has not run since it was configured")
	}
	if at, ok := a.host.PausedWork(); ok {
		why = append(why, fmt.Sprintf("a paced commit of a window without frames (hidden or minimized) in %v", max(at-a.backend.Now(), 0)))
	}
	now := a.backend.Now()
	for _, w := range a.host.Windows() {
		hw := w.(*headless.Window)
		prefix := ""
		if hw != a.win {
			prefix = fmt.Sprintf("window %q: ", hw.Title())
		}
		if drawn(hw) && hw.FrameRequested() {
			why = append(why, prefix+"a frame request")
		}
		for _, s := range a.host.TreeOf(w).Pending(now) {
			why = append(why, prefix+s)
		}
	}
	return why
}
