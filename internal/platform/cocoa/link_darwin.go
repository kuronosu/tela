//go:build darwin

package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/darwin"
)

// Frame pacing with a display link (ARCHITECTURE §8; internal/loop, "How
// Win32 and Cocoa plug in"). A requested frame that the shared loop does
// not deliver at once goes to the loop's Pace hook, which starts the
// window's display link and records the outstanding display signal
// (VsyncRequested). Each tick of the link records its arrival (VsyncDone)
// and runs a delivery pass; after the pass, a link whose window no longer
// waits for a signal is stopped, so an idle app gets no ticks (0% CPU).
//
// Which link, chosen once per backend (linkKind):
//
//   - linkView: -[NSView displayLinkWithTarget:selector:] (macOS 14+), a
//     CADisplayLink that follows the screen the view is on and calls the
//     view back on the main run loop, added in the common modes so it
//     also ticks during live resize.
//   - linkCV: CVDisplayLink (macOS 13; deprecated since 15 but present),
//     one per window, set to the display of the window's screen when it is
//     created and when the window moves to another screen
//     (windowDidChangeScreen:). Its callback runs on a CoreVideo thread:
//     it only marks the window (an atomic flag) and signals the run loop
//     source, whose pass on the main thread records the signal.
//   - linkTimer: no link could be made (CoreVideo missing): the shared
//     loop's interval deadline arms the CFRunLoopTimer, as in the base
//     backend.
//
// A window that is not on screen yet (before its first Present) is paced by
// the timer: a link would not tick for it. A link that stops ticking while
// a frame waits (the window became occluded, its display went to sleep)
// does not stall the window: the timer, armed linkTimeout after the
// request, records the signal itself, so such a window gets frames at
// about 1/linkTimeout until the link ticks again.

// linkKind is the frame pacing source of a backend.
type linkKind uint8

const (
	linkView linkKind = iota + 1
	linkCV
	linkTimer
)

func (k linkKind) String() string {
	switch k {
	case linkView:
		return "NSView.displayLink"
	case linkCV:
		return "CVDisplayLink"
	case linkTimer:
		return "timer"
	}
	return "none"
}

// linkTimeout is how long a requested frame waits for the display link
// before the timer stands in for it.
const linkTimeout = 100 * time.Millisecond

// lsel holds the selectors of the display link code.
var lsel = struct {
	instancesRespond, displayLink, addToRunLoop, setPaused, invalidate, mainRunLoop darwin.SEL
	tick, screen, deviceDescription, objectForKey, unsignedIntValue                 darwin.SEL
}{
	instancesRespond:  darwin.Sel("instancesRespondToSelector:"),
	displayLink:       darwin.Sel("displayLinkWithTarget:selector:"),
	addToRunLoop:      darwin.Sel("addToRunLoop:forMode:"),
	setPaused:         darwin.Sel("setPaused:"),
	invalidate:        darwin.Sel("invalidate"),
	mainRunLoop:       darwin.Sel("mainRunLoop"),
	tick:              darwin.Sel("telaDisplayLinkTick:"),
	screen:            darwin.Sel("screen"),
	deviceDescription: darwin.Sel("deviceDescription"),
	objectForKey:      darwin.Sel("objectForKey:"),
	unsignedIntValue:  darwin.Sel("unsignedIntValue"),
}

// chooseLink returns the best pacing source this system has.
func chooseLink() linkKind {
	if darwin.SendBool(darwin.ID(app.nsView), lsel.instancesRespond, lsel.displayLink) {
		return linkView
	}
	if darwin.LoadCoreVideo() == nil {
		return linkCV
	}
	return linkTimer
}

// paceHook is the loop's Pace hook: a requested frame waits for the
// window's display link, which it starts. It returns false (the timer
// paces the frame) when the backend has no link, the window is not on
// screen yet, or the link cannot be started.
func (b *Backend) paceHook(w *Window, now time.Duration) bool {
	if b.linkKind == linkTimer || !w.shown || w.Closed() || !w.startLink() {
		return false
	}
	w.VsyncRequested()
	w.linkSince = now
	return true
}

// startLink makes the window's display link tick, creating it at the
// first use. It reports whether the link runs.
func (w *Window) startLink() bool {
	if w.linkRunning {
		return true
	}
	switch w.b.linkKind {
	case linkView:
		if w.link == 0 {
			l := darwin.Send(w.view, lsel.displayLink, w.view, lsel.tick)
			if l == 0 {
				return false
			}
			w.link = darwin.Retain(l)
			loop := darwin.SendClass(app.runLoop, lsel.mainRunLoop)
			darwin.Send(w.link, lsel.addToRunLoop, loop, darwin.ID(darwin.CFRunLoopCommonModes))
		}
		darwin.Send(w.link, lsel.setPaused, false)
	case linkCV:
		if w.cvLink == 0 {
			var l uintptr
			if darwin.CVDisplayLinkCreateWithActiveCGDisplays(&l) != 0 || l == 0 {
				return false
			}
			registry.Lock()
			registry.next++
			w.linkID = registry.next
			registry.links[w.linkID] = w
			registry.Unlock()
			darwin.CVDisplayLinkSetOutputCallback(l, callbacks.cvTick, w.linkID)
			w.cvLink = l
			w.followScreen()
		}
		if darwin.CVDisplayLinkStart(w.cvLink) != 0 {
			return false
		}
	default:
		return false
	}
	w.linkRunning = true
	return true
}

// stopLink stops the window's display link (no ticks while idle).
func (w *Window) stopLink() {
	if !w.linkRunning {
		return
	}
	w.linkRunning = false
	switch {
	case w.link != 0:
		darwin.Send(w.link, lsel.setPaused, true)
	case w.cvLink != 0:
		darwin.CVDisplayLinkStop(w.cvLink) // returns after a running callback
	}
	w.linkFired.Store(false)
}

// releaseLink stops and frees the window's display link (at Close).
func (w *Window) releaseLink() {
	w.stopLink()
	if w.link != 0 {
		// invalidate releases the target (the view); the link object is
		// released at the next pass, outside a tick that may be running.
		darwin.Send(w.link, lsel.invalidate)
		w.b.releaseLater(w.link)
		w.link = 0
	}
	if w.cvLink != 0 {
		darwin.CVDisplayLinkRelease(w.cvLink)
		w.cvLink = 0
		registry.Lock()
		delete(registry.links, w.linkID)
		registry.Unlock()
	}
}

// followScreen points a CVDisplayLink at the display of the window's
// screen (the NSView link follows it by itself).
func (w *Window) followScreen() {
	if w.cvLink == 0 {
		return
	}
	screen := darwin.Send(w.ns, lsel.screen)
	if screen == 0 {
		return
	}
	if screenNumberKey == 0 {
		screenNumberKey = darwin.NSString("NSScreenNumber") // kept for the process
	}
	num := darwin.Send(darwin.Send(screen, lsel.deviceDescription), lsel.objectForKey, screenNumberKey)
	if num == 0 {
		return
	}
	darwin.CVDisplayLinkSetCurrentCGDisplay(w.cvLink, uint32(darwin.SendUint(num, lsel.unsignedIntValue)))
}

var screenNumberKey darwin.ID

// linkTick records a tick of the window's NSView display link and runs a
// pass.
func (w *Window) linkTick() {
	w.ticks.Add(1)
	if w.VsyncPending() {
		w.VsyncDone()
	}
	w.b.pump()
}

// collectTicks records the CVDisplayLink ticks that arrived since the last
// pass (their flags are set on CoreVideo's thread).
func (b *Backend) collectTicks() {
	for _, w := range b.loop.Windows() {
		if w.linkFired.Swap(false) && w.VsyncPending() {
			w.VsyncDone()
		}
	}
}

// idleLinks stops the links of the windows that no longer wait for a
// display signal; it runs after every pass, once the Pace hook started the
// ones that do.
func (b *Backend) idleLinks() {
	for _, w := range b.loop.Windows() {
		if w.linkRunning && !w.VsyncPending() {
			w.stopLink()
		}
	}
}

// linkDeadline returns the earliest time the timer must stand in for a
// display link that has not ticked (see the comment at the top).
func (b *Backend) linkDeadline() (deadline time.Duration, ok bool) {
	for _, w := range b.loop.Windows() {
		if w.linkRunning && w.VsyncPending() {
			if t := w.linkSince + linkTimeout; !ok || t < deadline {
				deadline, ok = t, true
			}
		}
	}
	return deadline, ok
}

// standIn records the display signal of the windows whose link has not
// ticked for linkTimeout after their request.
func (b *Backend) standIn() {
	now := b.Now()
	for _, w := range b.loop.Windows() {
		if w.linkRunning && w.VsyncPending() && now-w.linkSince >= linkTimeout {
			w.stalls.Add(1)
			w.VsyncDone()
		}
	}
}

// cvTick is the CVDisplayLink output callback, on a CoreVideo thread: it
// marks the window and signals the run loop source. It touches only the
// window's atomic fields and the backend's notify, which are safe there.
func cvTick(_, _, _ uintptr, _ uint64, _, context uintptr) int32 {
	registry.Lock()
	w := registry.links[context]
	registry.Unlock()
	if w != nil {
		w.ticks.Add(1)
		w.linkFired.Store(true)
		w.b.notify()
	}
	return 0
}
