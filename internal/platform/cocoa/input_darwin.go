//go:build darwin

package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// Keyboard, pointer and scroll input: the NSResponder methods of TelaView
// (the content view, which is the window's first responder), the
// NSApplication subclass that forwards the key releases AppKit swallows,
// and the glue to the pure rules of input.go and internal/keyboard.
//
// Quirks handled, mostly as winit does (macOS view.rs and app.rs):
//
//   - AppKit sends no keyUp: for a key released while Command is held:
//     -[NSApplication sendEvent:] keeps it. TelaApplication overrides
//     sendEvent: and hands those releases to the key window, as winit's
//     WinitApplication does (GLFW uses a local event monitor instead,
//     which events sent through -[NSApp sendEvent:] bypass).
//   - Key events with Command or Control are first offered to
//     performKeyEquivalent:, and some (Control+Tab, Control+Escape,
//     Command+.) never reach keyDown: after it (GLFW claims those three).
//     The view claims every Command and Control key down there, after the
//     main menu had its chance, so each arrives once and in one place.
//   - The modifier keys arrive as flagsChanged:, which does not say
//     whether the key went down or up: the window keeps the previous
//     modifierFlags (reset to +[NSEvent modifierFlags] when it gains the
//     focus) for keyboard.MacFlagsChanged.
//   - The view overrides keyDown: and keyUp: without calling super, so an
//     unhandled key never walks up to NSWindow, which would beep.
//   - The view accepts the first mouse click (acceptsFirstMouse:), so the
//     click that activates a window also reaches the app, as in winit.
//   - Enter, leave and moves come from one tracking area (InVisibleRect,
//     so it follows the view's size; ActiveAlways, so hover works in
//     inactive windows as on the other platforms; EnabledDuringMouseDrag,
//     whose exits during a drag become the deferred leave of the contract).
//
// Positions come from locationInWindow converted into the flipped view
// (points, top-left origin); events without a window (built from a
// CGEvent) carry screen coordinates, which go through the window first.
// Times are the NSEvent timestamps mapped to the backend's clock.

// isel holds the selectors of the input code.
var isel = struct {
	typ, modifierFlags, keyCode, characters, charactersIgnoring, isARepeat  darwin.SEL
	timestamp, locationInWindow, window, buttonNumber, trackingArea         darwin.SEL
	scrollingDeltaX, scrollingDeltaY, hasPrecise, phase, momentumPhase      darwin.SEL
	convertPointFromView, convertPointToView, convertToScreen, convertFrom  darwin.SEL
	keyWindow, sendEvent, mainMenu, performKeyEquivalent, set, hide, unhide darwin.SEL
	initTrackingArea, addTrackingArea, processInfo, systemUptime            darwin.SEL
}{
	typ:                  darwin.Sel("type"),
	modifierFlags:        darwin.Sel("modifierFlags"),
	keyCode:              darwin.Sel("keyCode"),
	characters:           darwin.Sel("characters"),
	charactersIgnoring:   darwin.Sel("charactersIgnoringModifiers"),
	isARepeat:            darwin.Sel("isARepeat"),
	timestamp:            darwin.Sel("timestamp"),
	locationInWindow:     darwin.Sel("locationInWindow"),
	window:               darwin.Sel("window"),
	buttonNumber:         darwin.Sel("buttonNumber"),
	trackingArea:         darwin.Sel("trackingArea"),
	scrollingDeltaX:      darwin.Sel("scrollingDeltaX"),
	scrollingDeltaY:      darwin.Sel("scrollingDeltaY"),
	hasPrecise:           darwin.Sel("hasPreciseScrollingDeltas"),
	phase:                darwin.Sel("phase"),
	momentumPhase:        darwin.Sel("momentumPhase"),
	convertPointFromView: darwin.Sel("convertPoint:fromView:"),
	convertPointToView:   darwin.Sel("convertPoint:toView:"),
	convertToScreen:      darwin.Sel("convertPointToScreen:"),
	convertFrom:          darwin.Sel("convertPointFromScreen:"),
	keyWindow:            darwin.Sel("keyWindow"),
	sendEvent:            darwin.Sel("sendEvent:"),
	mainMenu:             darwin.Sel("mainMenu"),
	performKeyEquivalent: darwin.Sel("performKeyEquivalent:"),
	set:                  darwin.Sel("set"),
	hide:                 darwin.Sel("hide"),
	unhide:               darwin.Sel("unhide"),
	initTrackingArea:     darwin.Sel("initWithRect:options:owner:userInfo:"),
	addTrackingArea:      darwin.Sel("addTrackingArea:"),
	processInfo:          darwin.Sel("processInfo"),
	systemUptime:         darwin.Sel("systemUptime"),
}

// trackingOptions are the options of the view's tracking area (see the
// comment at the top of the file).
const trackingOptions = darwin.NSTrackingMouseEnteredAndExited | darwin.NSTrackingMouseMoved |
	darwin.NSTrackingActiveAlways | darwin.NSTrackingInVisibleRect | darwin.NSTrackingEnabledDuringMouseDrag

// addTrackingArea gives the view its tracking area; the view keeps it.
func (w *Window) addTrackingArea() {
	area := darwin.Send(darwin.SendClass(app.trackingArea, sel.alloc), isel.initTrackingArea,
		darwin.CGRect{}, uint(trackingOptions), w.view, darwin.ID(0))
	if area == 0 {
		return
	}
	darwin.Send(w.view, isel.addTrackingArea, area)
	w.tracking = area
	darwin.Release(area)
}

// uptime returns the system uptime in seconds, the base of NSEvent
// timestamps.
func uptime() float64 {
	return darwin.SendFloat(darwin.SendClass(app.processInfo, isel.processInfo), isel.systemUptime)
}

// eventTime returns the time of ev in the backend's clock.
func (b *Backend) eventTime(ev darwin.ID) time.Duration {
	return b.clock.time(darwin.SendFloat(ev, isel.timestamp), b.Now(), uptime)
}

// eventPos returns the position of ev in the view: points, top-left
// origin.
func (w *Window) eventPos(ev darwin.ID) platform.Point {
	loc := darwin.SendPoint(ev, isel.locationInWindow)
	if win := darwin.Send(ev, isel.window); win != w.ns {
		// Another window's coordinates, or the screen's for an event
		// without a window.
		if win != 0 {
			loc = darwin.SendPoint(win, isel.convertToScreen, loc)
		}
		loc = darwin.SendPoint(w.ns, isel.convertFrom, loc)
	}
	p := darwin.SendPoint(w.view, isel.convertPointFromView, loc, darwin.ID(0))
	return platform.Point{X: float32(p.X), Y: float32(p.Y)}
}

func eventMods(ev darwin.ID) platform.Modifiers {
	return keyboard.MacModifiers(darwin.SendUint(ev, isel.modifierFlags))
}

// --- keyboard ---

// keyStamp identifies a key event, so that one offered both to
// performKeyEquivalent: and keyDown: is handled once.
type keyStamp struct {
	ev darwin.ID
	ts float64
}

// key handles a keyDown: (down) or keyUp: event.
func (w *Window) key(ev darwin.ID, down bool) {
	stamp := keyStamp{ev, darwin.SendFloat(ev, isel.timestamp)}
	if stamp == w.lastKey {
		return
	}
	w.lastKey = stamp
	k := keyboard.MacKey{
		KeyCode:                     uint16(darwin.SendUint(ev, isel.keyCode)),
		Flags:                       darwin.SendUint(ev, isel.modifierFlags),
		Characters:                  darwin.GoString(darwin.Send(ev, isel.characters)),
		CharactersIgnoringModifiers: darwin.GoString(darwin.Send(ev, isel.charactersIgnoring)),
		ISO:                         darwin.KeyboardISO(),
		Time:                        w.b.eventTime(ev),
	}
	var e platform.KeyEvent
	var ok bool
	if down {
		k.Repeat = darwin.SendBool(ev, isel.isARepeat)
		e, ok = keyboard.MacKeyDown(k)
		if ok && w.imeKeys(k.Flags) {
			w.interpretKey(ev, e)
			return
		}
	} else {
		e, ok = keyboard.MacKeyUp(k)
	}
	if ok {
		w.deliverKey(e)
	}
}

// flagsChanged handles a flagsChanged: event. characters must not be read:
// AppKit raises an exception for events of this type.
func (w *Window) flagsChanged(ev darwin.ID) {
	flags := darwin.SendUint(ev, isel.modifierFlags)
	code := uint16(darwin.SendUint(ev, isel.keyCode))
	for _, e := range keyboard.MacFlagsChanged(&w.held, code, w.prevFlags, flags, w.b.eventTime(ev)) {
		w.deliverKey(e)
	}
	w.prevFlags = flags
}

// deliverKey queues e if the window has the keyboard focus and the held
// set lets it through (no release without a delivered press), and reports
// whether it did.
func (w *Window) deliverKey(e platform.KeyEvent) bool {
	if w.focused && w.held.Accept(e) {
		w.b.enqueue(w, e)
		return true
	}
	return false
}

// keyEquivalent answers performKeyEquivalent:: a key down with Command or
// Control in the focused window is delivered here, after the main menu
// (none until the macOS desktop phase) had its chance.
func (w *Window) keyEquivalent(ev darwin.ID) bool {
	if !w.focused || darwin.SendUint(ev, isel.typ) != darwin.NSEventTypeKeyDown {
		return false
	}
	if darwin.SendUint(ev, isel.modifierFlags)&(darwin.NSEventModifierFlagCommand|darwin.NSEventModifierFlagControl) == 0 {
		return false
	}
	if menu := darwin.Send(app.ns, isel.mainMenu); menu != 0 && darwin.SendBool(menu, isel.performKeyEquivalent, ev) {
		return true
	}
	w.key(ev, true)
	return true
}

// appSendEvent is -[TelaApplication sendEvent:]: a key release with Command
// held goes straight to the key window, which AppKit's own sendEvent:
// would not do (winit's WinitApplication); every other event takes the
// normal path.
func appSendEvent(self darwin.ID, _ darwin.SEL, ev darwin.ID) {
	if darwin.SendUint(ev, isel.typ) == darwin.NSEventTypeKeyUp &&
		darwin.SendUint(ev, isel.modifierFlags)&darwin.NSEventModifierFlagCommand != 0 {
		if kw := darwin.Send(self, isel.keyWindow); kw != 0 {
			darwin.Send(kw, isel.sendEvent, ev)
		}
		return
	}
	darwin.SendSuper(self, app.appSuper, isel.sendEvent, ev)
}

// --- pointer ---

// emitter returns the function through which the pointer rules queue the
// events of ev, completed with its modifiers and time. Moves coalesce.
func (w *Window) emitter(ev darwin.ID) func(platform.PointerEvent) {
	mods, t := eventMods(ev), w.b.eventTime(ev)
	return func(pe platform.PointerEvent) {
		pe.Kind = platform.PointerMouse
		pe.Mods = mods
		pe.Time = t
		if pe.Action == platform.PointerMove {
			w.b.loop.QueuePointer(w, pe)
		} else {
			w.b.enqueue(w, pe)
		}
	}
}

// mouseButton handles a press or release of btn (ButtonNone: the event's
// buttonNumber says which).
//
// A press in a caption region of the app's title bar, and the rest of its
// sequence, are the window's (captionPress, windowctl_darwin.go); a press
// in a window whose modal child is shown brings the child forward (the
// press is still delivered, and core drops it).
func (w *Window) mouseButton(ev darwin.ID, btn platform.PointerButton, down bool) {
	if btn == platform.ButtonNone {
		btn = macButton(darwin.SendInt(ev, isel.buttonNumber))
	}
	if btn == platform.ButtonNone || w.dragTakes(ev) {
		return
	}
	pos := w.eventPos(ev)
	defer w.keepPress(ev) // the press StartDrag starts from (dnd_darwin.go)
	if down {
		if w.ctl.blocked {
			w.raiseModal()
		}
		if w.captionPress(ev, btn, pos) {
			return
		}
	} else if w.ctl.caption.release(btn) {
		return
	}
	w.ptr.button(btn, down, pos, w.emitter(ev))
	w.updateCursorHidden()
}

// mouseMove handles mouseMoved: and the drags. The drags of a caption
// press are not delivered; a move without buttons ends every caption
// sequence (the window server may keep a release).
func (w *Window) mouseMove(ev darwin.ID) {
	if w.dragTakes(ev) {
		return
	}
	defer w.keepPress(ev)
	if darwin.SendUint(ev, isel.typ) == darwin.NSEventTypeMouseMoved {
		w.ctl.caption.moved()
	} else if w.ctl.caption.drag(w.ptr.held) {
		return
	}
	pos := w.eventPos(ev)
	b := darwin.SendRect(w.view, sel.bounds)
	w.ptr.move(pos, inBounds(pos, platform.Size{W: float32(b.Size.Width), H: float32(b.Size.Height)}), w.emitter(ev))
	w.updateCursorHidden()
}

// mouseCross handles mouseEntered: and mouseExited: of the view's tracking
// area (events of other tracking areas, such as AppKit's own, are not
// ours; events built without one are taken).
func (w *Window) mouseCross(ev darwin.ID, entered bool) {
	if area := darwin.Send(ev, isel.trackingArea); area != 0 && area != w.tracking || w.dragTakes(ev) {
		return
	}
	if entered {
		w.ptr.entered(w.eventPos(ev), w.emitter(ev))
	} else {
		w.ptr.exited(w.eventPos(ev), w.emitter(ev))
	}
	w.updateCursorHidden()
}

// scroll handles scrollWheel:.
func (w *Window) scroll(ev darwin.ID) {
	delta, steps, phase, ok := w.gesture.event(macScroll{
		DX:       darwin.SendFloat(ev, isel.scrollingDeltaX),
		DY:       darwin.SendFloat(ev, isel.scrollingDeltaY),
		Precise:  darwin.SendBool(ev, isel.hasPrecise),
		Phase:    darwin.SendUint(ev, isel.phase),
		Momentum: darwin.SendUint(ev, isel.momentumPhase),
	})
	if !ok {
		return
	}
	w.b.enqueue(w, platform.ScrollEvent{
		Kind:  platform.PointerMouse,
		Pos:   w.eventPos(ev),
		Delta: delta,
		Steps: steps,
		Phase: phase,
		Mods:  eventMods(ev),
		Time:  w.b.eventTime(ev),
	})
}

// updateCursorHidden hides the cursor while CursorNone is set and the
// pointer is in the view, and shows it again otherwise. +[NSCursor hide]
// and unhide nest, so each window balances its own.
func (w *Window) updateCursorHidden() {
	want := w.cursor == platform.CursorNone && w.ptr.inside && !w.Closed()
	if want == w.cursorHidden {
		return
	}
	w.cursorHidden = want
	if want {
		darwin.SendClass(app.cursor, isel.hide)
	} else {
		darwin.SendClass(app.cursor, isel.unhide)
	}
}

// inputMethods are the input methods of TelaView. Each finds its window by
// the view, handles the event and runs a delivery pass.
func inputMethods() []darwin.Method {
	on := func(name string, f func(w *Window, ev darwin.ID)) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL, ev darwin.ID) {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					f(w, ev)
					w.b.pump()
				})
			}
		}}
	}
	button := func(name string, btn platform.PointerButton, down bool) darwin.Method {
		return on(name, func(w *Window, ev darwin.ID) { w.mouseButton(ev, btn, down) })
	}
	move := func(w *Window, ev darwin.ID) { w.mouseMove(ev) }
	return []darwin.Method{
		on("keyDown:", func(w *Window, ev darwin.ID) { w.key(ev, true) }),
		on("keyUp:", func(w *Window, ev darwin.ID) { w.key(ev, false) }),
		on("flagsChanged:", (*Window).flagsChanged),
		{Sel: darwin.Sel("performKeyEquivalent:"), Fn: func(self darwin.ID, _ darwin.SEL, ev darwin.ID) bool {
			handled := false
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					handled = w.keyEquivalent(ev)
					w.b.pump()
				})
			}
			return handled
		}},
		{Sel: darwin.Sel("acceptsFirstMouse:"), Fn: func(darwin.ID, darwin.SEL, darwin.ID) bool { return true }},
		button("mouseDown:", platform.ButtonPrimary, true),
		button("mouseUp:", platform.ButtonPrimary, false),
		button("rightMouseDown:", platform.ButtonSecondary, true),
		button("rightMouseUp:", platform.ButtonSecondary, false),
		button("otherMouseDown:", platform.ButtonNone, true),
		button("otherMouseUp:", platform.ButtonNone, false),
		on("mouseMoved:", move),
		on("mouseDragged:", move),
		on("rightMouseDragged:", move),
		on("otherMouseDragged:", move),
		on("mouseEntered:", func(w *Window, ev darwin.ID) { w.mouseCross(ev, true) }),
		on("mouseExited:", func(w *Window, ev darwin.ID) { w.mouseCross(ev, false) }),
		on("scrollWheel:", (*Window).scroll),
	}
}
