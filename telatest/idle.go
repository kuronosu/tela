package telatest

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Pending is one thing that keeps the app from being idle ([App.Pending]).
type Pending struct {
	// Kind says what it is, for a tool that tells work that can still
	// change the screen from work that only redraws it.
	Kind PendingKind
	// Window is the title of the window it belongs to when that is not the
	// App's own window, else "".
	Window string
	// What describes it as ExpectIdle prints it: "a UseInterval every 1s
	// in <Clock> (called at clock.go:14), next in 1s".
	What string
	// In is the distance of its deadline on the app's clock (a timer, a
	// caret blink, a long press, a scrollbar fade, a paced commit, a drag's
	// autoscroll), 0 for work due now or without a deadline.
	In time.Duration
}

// String returns What, prefixed with the window when it is another one:
// `window "Settings": a frame request`.
func (p Pending) String() string {
	if p.Window != "" {
		return fmt.Sprintf("window %q: %s", p.Window, p.What)
	}
	return p.What
}

// PendingKind is the kind of a [Pending] item.
type PendingKind uint8

// The kinds of [Pending] items. Those that only redraw what is on the
// screen, without changing its elements, are [PendingEndless],
// [PendingScrollbar], [PendingCaret] and [PendingDebug] (a frame request
// beside them is theirs).
const (
	// PendingEvents: input or window events waiting to be delivered.
	PendingEvents PendingKind = iota + 1
	// PendingFrame: a frame request, or a frame owed to a window that was
	// configured.
	PendingFrame
	// PendingCommit: a paced commit of a window without frames (hidden or
	// minimized).
	PendingCommit
	// PendingUpdate: state or Value updates, or functions given to
	// tela.Post, queued and not yet applied.
	PendingUpdate
	// PendingChange: changed elements or boxes waiting for a frame.
	PendingChange
	// PendingAnimation: a transition or animation that ends.
	PendingAnimation
	// PendingEndless: only endless animations run (animate-spin...).
	PendingEndless
	// PendingScroll: a scroll animation or a fling.
	PendingScroll
	// PendingScrollbar: overlay scrollbars fading, or waiting to.
	PendingScrollbar
	// PendingDebug: the debug overlays (TELA_DEBUG) asking for frames.
	PendingDebug
	// PendingLongPress: the deadline of a long press.
	PendingLongPress
	// PendingCaret: the blink of the focused text field's caret.
	PendingCaret
	// PendingTimer: a UseInterval or UseTimeout running.
	PendingTimer
	// PendingAutoscroll: the autoscroll of a drag.
	PendingAutoscroll
	// PendingAsync: a UseAsync run whose function has not returned.
	PendingAsync
	// PendingGo: a UseGo goroutine whose function has not returned. A
	// goroutine that waits (for a channel, the network) costs nothing, so
	// [App.ExpectIdle] does not count it; a result it posts can still
	// change the screen.
	PendingGo
)

var pendingKindNames = [...]string{
	PendingEvents: "events", PendingFrame: "frame", PendingCommit: "commit",
	PendingUpdate: "update", PendingChange: "change", PendingAnimation: "animation",
	PendingEndless: "endless animation", PendingScroll: "scroll", PendingScrollbar: "scrollbar",
	PendingDebug: "debug overlay", PendingLongPress: "long press", PendingCaret: "caret",
	PendingTimer: "timer", PendingAutoscroll: "autoscroll", PendingAsync: "UseAsync", PendingGo: "UseGo",
}

// String names the kind: "timer", "endless animation", "UseAsync".
func (k PendingKind) String() string {
	if int(k) < len(pendingKindNames) && pendingKindNames[k] != "" {
		return pendingKindNames[k]
	}
	return fmt.Sprintf("PendingKind(%d)", uint8(k))
}

// coreKinds maps the kinds of a tree's items.
var coreKinds = map[core.PendingKind]PendingKind{
	core.PendingUpdate: PendingUpdate, core.PendingChange: PendingChange,
	core.PendingAnimation: PendingAnimation, core.PendingEndless: PendingEndless,
	core.PendingScroll: PendingScroll, core.PendingScrollbar: PendingScrollbar,
	core.PendingDebug: PendingDebug, core.PendingLongPress: PendingLongPress,
	core.PendingCaret: PendingCaret, core.PendingTimer: PendingTimer,
	core.PendingAutoscroll: PendingAutoscroll, core.PendingAsync: PendingAsync,
	core.PendingGo: PendingGo,
}

// Pending names what keeps the app from being idle (ADR-291, ADR-308):
// events waiting to be delivered, a frame request or a frame owed to a
// window, a paced commit of a window without frames, and in each window's
// tree queued updates, changed elements, running animations (endless ones
// too), scroll animations, the deadlines of the tree (every UseInterval
// and UseTimeout running, with its component and, in dev mode, where it
// is called; a caret blink, a long press, overlay scrollbars waiting to
// fade) and the goroutines of UseAsync runs and of UseGo that have not
// returned. Each item has a [PendingKind]; the items of a window other
// than this App's name it. It is empty when the app is idle: an idle app
// asks for no frame and arms no timer (ARCHITECTURE §3.5, "0% CPU when
// idle").
//
// The app is settled first, as a query is: the work due is handled and,
// unless the app was mounted with [NoSettle], finite animations run to
// their end. The clock is not moved to deadlines, so a UseTimeout that
// has not fired yet is pending.
//
//	for _, p := range app.Pending() {
//		if p.Kind == telatest.PendingTimer {
//			t.Log(p) // a UseTimeout of 3s in <Toast> (called at toast.go:20), next in 3s
//		}
//	}
func (a *App) Pending() []Pending {
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
// the clock reaches them. A UseGo goroutine ([PendingGo]) does not count:
// one that waits costs nothing.
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
	var why []string
	for _, p := range a.Pending() {
		if p.Kind != PendingGo {
			why = append(why, p.String())
		}
	}
	if len(why) > 0 {
		a.tb.Fatalf("telatest: ExpectIdle: the app is not idle: %s", strings.Join(why, "; "))
	}
}

// pending is Pending without settling.
func (a *App) pending() []Pending {
	var why []Pending
	now := a.backend.Now()
	if a.backend.Pending() {
		why = append(why, Pending{Kind: PendingEvents, What: "events waiting to be delivered"})
	}
	if a.host.NeedsFrame() {
		why = append(why, Pending{Kind: PendingFrame, What: "a window whose frame has not run since it was configured"})
	}
	if at, ok := a.host.PausedWork(); ok {
		in := max(at-now, 0)
		why = append(why, Pending{Kind: PendingCommit, In: in, What: fmt.Sprintf("a paced commit of a window without frames (hidden or minimized) in %v", in)})
	}
	for _, w := range a.host.Windows() {
		hw := w.(*headless.Window)
		title := ""
		if hw != a.win {
			title = hw.Title()
		}
		if drawn(hw) && hw.FrameRequested() {
			why = append(why, Pending{Kind: PendingFrame, Window: title, What: "a frame request"})
		}
		for _, p := range a.host.TreeOf(w).Pending(now) {
			why = append(why, Pending{Kind: coreKinds[p.Kind], Window: title, What: p.What, In: p.In})
		}
	}
	return why
}

// asyncRuns names the UseAsync runs in flight in the trees of the app's
// windows.
func (a *App) asyncRuns() []string {
	var out []string
	for _, w := range a.host.Windows() {
		out = append(out, a.host.TreeOf(w).AsyncRuns()...)
	}
	return out
}

// asyncInFlight reports whether a UseAsync run is in flight in the app.
func (a *App) asyncInFlight() bool {
	for _, w := range a.host.Windows() {
		if a.host.TreeOf(w).AsyncInFlight() {
			return true
		}
	}
	return false
}
