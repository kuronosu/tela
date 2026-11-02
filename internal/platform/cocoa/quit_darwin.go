//go:build darwin

package cocoa

import (
	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Quit requests. Cmd+Q, the app menu's Quit, the Dock's Quit and a logout
// or restart (the quit Apple event) all end in -[NSApplication
// terminate:], whose own implementation asks the delegate
// (applicationShouldTerminate:) and then calls exit(): Go code after Run
// would never run. The delegate's other answer, NSTerminateLater, runs
// the run loop in NSModalPanelRunLoopMode inside terminate: until
// replyToApplicationShouldTerminate:, whose YES calls exit() too; neither
// lets Run return.
//
// So TelaApplication overrides terminate:, as Chromium's
// BrowserCrApplication and Electron's ElectronApplication do, and decides
// with this rule, which needs no guess about what the app will do:
//
//   - The request is delivered at once: an app-scoped QuitRequestedEvent
//     and a delivery pass, inside terminate:.
//   - The app quits (Quit, from OnQuit or by any path that makes Run
//     return): terminate: returns without calling NSApplication's, so
//     AppKit never calls exit(); [NSApp run] returns at the end of the
//     event and so does Run. A logout or restart is not cancelled: the
//     quit Apple event is answered without an error and loginwindow waits
//     for the process to end, as it does for Chromium.
//   - The app keeps running: once the request's pass has ended with no
//     Quit, and nothing the app opened while answering is pending (a
//     dialog or alert opened since the request, a modal window shown since
//     then, an answer or event the loop has not delivered yet), the
//     backend calls NSApplication's terminate: with the delegate answering
//     NSTerminateCancel: AppKit's own refusal, which cancels a logout at
//     once.
//   - While something is pending (an unsaved-changes alert from OnQuit),
//     terminate: waits for the app's decision running the app's events
//     as [NSApp run] does (nextEventMatchingMask: in the default mode, then
//     sendEvent:), so the alert, its answer and the passes go on as usual;
//     each pass posts an application-defined event to make the wait look
//     again. The decision then follows the two rules above.
//   - A request that comes while one is waiting, or from inside a pass
//     (the handler sent terminate:), is delivered as a QuitRequestedEvent
//     in the next pass and answered as accepted: the waiting request, or
//     the app's own code, decides.
//
// If NSApp is not a TelaApplication (something in the process created
// NSApp first), the delegate delivers the request and answers
// NSTerminateCancel, as before: the app quits with Quit, and a logout is
// cancelled.

// quitState is the backend's state of the quit requests.
type quitState struct {
	waiting    bool // a terminate: waits for the app's decision
	wakePosted bool // the wait has an event coming to look again
	// requests counts the QuitRequestedEvents of terminate:, last is the
	// answer to the last one (tests).
	requests int
	last     quitAnswer
}

// quitAnswer is how terminate: answered.
type quitAnswer uint8

// The zero quitAnswer is "none": no request yet.
const (
	// quitAccepted: the app quit; terminate: returned without exit().
	quitAccepted quitAnswer = iota + 1
	// quitDeclined: the app kept running; AppKit's terminate: was told
	// NSTerminateCancel.
	quitDeclined
	// quitPassed: delivered for a later pass (a request while one waits,
	// or terminate: from inside a pass).
	quitPassed
)

func (a quitAnswer) String() string {
	return [...]string{"none", "accepted", "declined", "passed"}[a]
}

// quitDeclining is set while TelaApplication calls NSApplication's
// terminate: to refuse: the delegate then answers NSTerminateCancel.
var quitDeclining bool

// runningBackend is in launch_darwin.go.

// appTerminate is -[TelaApplication terminate:].
func appTerminate(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	decline := true // no running backend: nobody to ask
	if b := runningBackend(); b != nil {
		b.guard(func() { decline = b.quitRequested() })
	}
	if decline {
		quitDeclining = true
		darwin.SendSuper(self, app.appSuper, qsel.terminate, sender)
		quitDeclining = false
	}
}

// qsel holds the selectors of the quit code.
var qsel = struct {
	terminate, nextEvent, distantFuture darwin.SEL
}{
	terminate:     darwin.Sel("terminate:"),
	nextEvent:     darwin.Sel("nextEventMatchingMask:untilDate:inMode:dequeue:"),
	distantFuture: darwin.Sel("distantFuture"),
}

// quitSnapshot is what was open when a quit request came: what the app
// opens later while answering it is pending.
type quitSnapshot struct {
	dialogs map[*dialogRequest]bool
	windows map[*Window]bool
}

// quitRequested delivers a quit request and waits for the app's decision
// (see the top of the file). It reports whether terminate: must refuse.
func (b *Backend) quitRequested() (decline bool) {
	q := &b.menus.quit
	q.requests++
	if q.waiting || b.pumping {
		b.enqueue(nil, platform.QuitRequestedEvent{})
		b.notify()
		q.last = quitPassed
		return false
	}
	snap := quitSnapshot{dialogs: map[*dialogRequest]bool{}, windows: map[*Window]bool{}}
	for _, r := range b.desk.dialogs {
		snap.dialogs[r] = true
	}
	for _, w := range b.loop.Windows() {
		snap.windows[w] = true
	}
	q.waiting = true
	defer func() { q.waiting = false }()
	b.enqueue(nil, platform.QuitRequestedEvent{})
	b.pump()
	for !b.loop.Quitting() && !b.failed && b.quitPending(snap) {
		b.waitEvent()
	}
	if b.loop.Quitting() {
		q.last = quitAccepted
		// [NSApp run] notices stop: after an event: the wait may have
		// taken the one stopApp posted, and terminate: may have come from
		// outside an event (a run loop block).
		postAppEvent()
		return false
	}
	q.last = quitDeclined
	return true
}

// quitPending reports whether the app may still decide to quit: a dialog
// or alert opened since snap is open, a modal window opened since then is
// shown, or the loop has something to deliver.
func (b *Backend) quitPending(snap quitSnapshot) bool {
	for _, r := range b.desk.dialogs {
		if !snap.dialogs[r] {
			return true
		}
	}
	for _, w := range b.loop.Windows() {
		if !snap.windows[w] && w.opts.Modal && w.visible && !w.Closed() {
			return true
		}
	}
	return b.loop.Ready()
}

// waitEvent handles the app's next event as [NSApp run] would.
func (b *Backend) waitEvent() {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	mode := darwin.AppKitString("NSDefaultRunLoopMode")
	if mode == 0 {
		mode = darwin.AutoString("kCFRunLoopDefaultMode") // the same mode, toll-free bridged
	}
	future := darwin.SendClass(darwin.GetClass("NSDate"), qsel.distantFuture)
	ev := darwin.Send(app.ns, qsel.nextEvent, darwin.NSEventMaskAny, future, mode, true)
	b.menus.quit.wakePosted = false
	if ev != 0 {
		darwin.Send(app.ns, isel.sendEvent, ev)
	}
}

// wakeQuitWait posts an application-defined event after a pass while a
// quit request waits, so the wait looks at the app's state again (a pass
// is not an event: nextEventMatchingMask: would sleep on).
func (b *Backend) wakeQuitWait() {
	q := &b.menus.quit
	if !q.waiting || q.wakePosted {
		return
	}
	q.wakePosted = true
	postAppEvent()
}

// postAppEvent posts an application-defined event at the end of the
// queue; NSApp ignores it.
func postAppEvent() {
	ev := darwin.SendClass(app.event, sel.otherEvent,
		uintptr(darwin.NSEventTypeApplicationDefined), darwin.CGPoint{}, uintptr(0), float64(0),
		uintptr(0), uintptr(0), uintptr(0), uintptr(0), uintptr(0))
	darwin.Send(app.ns, sel.postEventAtStart, ev, false)
}
