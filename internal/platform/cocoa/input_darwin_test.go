//go:build darwin

package cocoa

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// Input tests: events are built in-process (+[NSEvent keyEventWithType:...],
// mouseEventWithType:..., enterExitEventWithType:..., and CGEvents turned
// into NSEvents with eventWithCGEvent:), never posted to the system, and
// handed to AppKit through -[NSApp sendEvent:] (keys and clicks: the path
// of real events, with TelaApplication's override and the key window's
// dispatch) or straight to the view's NSResponder methods (moves, enter and
// leave, which AppKit routes through the tracking area, and events built
// from CGEvents, which have no window). The tests check the platform
// events that come out, and log them all.

var tsel = struct {
	mouseEvent, enterExit, withCGEvent, screens, objectAtIndex, frame  darwin.SEL
	activate, activateIgnoring, makeKeyWindow, respondsTo, isKeyWindow darwin.SEL
}{
	mouseEvent:       darwin.Sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
	enterExit:        darwin.Sel("enterExitEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:trackingNumber:userData:"),
	withCGEvent:      darwin.Sel("eventWithCGEvent:"),
	screens:          darwin.Sel("screens"),
	objectAtIndex:    darwin.Sel("objectAtIndex:"),
	frame:            darwin.Sel("frame"),
	activate:         darwin.Sel("activate"),
	activateIgnoring: darwin.Sel("activateIgnoringOtherApps:"),
	makeKeyWindow:    darwin.Sel("makeKeyWindow"),
	respondsTo:       darwin.Sel("respondsToSelector:"),
	isKeyWindow:      darwin.Sel("isKeyWindow"),
}

// session runs a backend with one shown window, then calls script on the
// main thread from inside the handler and records the window's events
// that follow (frames excluded).
type session struct {
	t      *testing.T
	b      *Backend
	w      *Window
	events []platform.Event
	// key reports that the window was AppKit's key window when the script
	// ran; simulated, that it was not and the test gave it the focus by
	// calling its delegate's windowDidBecomeKey: (key events then go to
	// the window directly, not through NSApp).
	key, simulated bool
	ts             float64
}

// runSession runs script once the window is shown and, if needFocus, has
// the keyboard focus (it asks for it a few times, then simulates it). It
// returns nil when the window never showed.
func runSession(t *testing.T, title string, needFocus bool, script func(s *session)) *session {
	return runSteps(t, title, needFocus, 250*time.Millisecond, script)
}

// runSteps is runSession with several scripts, each run from its own
// handler call, pause after the one before (so the run loop turns in
// between: an input method switch takes effect, events are delivered).
func runSteps(t *testing.T, title string, needFocus bool, pause time.Duration, steps ...func(s *session)) *session {
	b := newBackend(t)
	s := &session{t: t, b: b}
	onMain(func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: title, Size: platform.Size{W: 200, H: 150}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		s.w = pw.(*Window)
		time.AfterFunc(20*time.Second+time.Duration(len(steps))*pause, b.Quit) // a failure, not a hang
		stage, tries, scheduled := 0, 0, false
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				presentSolid(t, ew, color.RGBA{0x80, 0x80, 0x90, 0xff})
				if !scheduled && s.w.shown {
					scheduled = true
					time.AfterFunc(150*time.Millisecond, b.Wake)
				}
				return
			case platform.WakeEvent:
				switch stage {
				case 0:
					if needFocus && !s.w.focused && tries < 30 {
						tries++
						if tries%5 == 1 {
							s.askFocus()
						}
						time.AfterFunc(100*time.Millisecond, b.Wake)
						return
					}
					pool := darwin.PoolPush()
					s.key = darwin.SendBool(s.w.ns, tsel.isKeyWindow) && darwin.Send(app.ns, isel.keyWindow) == s.w.ns
					if needFocus && !s.w.focused {
						s.simulated = true
						darwin.Send(s.w.delegate, darwin.Sel("windowDidBecomeKey:"), darwin.ID(0))
					}
					t.Logf("NSApp is a %s; key window %v, focus simulated %v; pointer at %v (screen)",
						darwin.GoString(darwin.Send(app.ns, darwin.Sel("className"))), s.key, s.simulated,
						darwin.SendPoint(darwin.ID(app.event), darwin.Sel("mouseLocation")))
					stage = 1
					s.ts = uptime()
					steps[0](s)
					darwin.PoolPop(pool)
					time.AfterFunc(pause, b.Wake)
				default:
					if stage < len(steps) {
						pool := darwin.PoolPush()
						steps[stage](s)
						darwin.PoolPop(pool)
						stage++
						time.AfterFunc(pause, b.Wake)
						return
					}
					stage = len(steps) + 1
					b.Quit()
				}
				return
			}
			if stage >= 1 && stage <= len(steps) && ew == platform.Window(s.w) {
				s.events = append(s.events, e)
			}
		}))
	})
	if s.w == nil {
		return nil
	}
	return s
}

// askFocus activates the app and makes the window key (macOS 14 restricts
// activation; the runner's session usually lets a new app be active).
func (s *session) askFocus() {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if darwin.SendBool(app.ns, tsel.respondsTo, tsel.activate) {
		darwin.Send(app.ns, tsel.activate)
	} else {
		darwin.Send(app.ns, tsel.activateIgnoring, true)
	}
	darwin.Send(s.w.ns, sel.makeKeyAndOrderFront, darwin.ID(0))
	darwin.Send(s.w.ns, tsel.makeKeyWindow)
	s.t.Logf("asked for the focus: key window %v", darwin.SendBool(s.w.ns, tsel.isKeyWindow))
}

// at returns a timestamp step ms after the session's start.
func (s *session) at(ms int) float64 { return s.ts + float64(ms)/1000 }

func (s *session) windowNumber() int { return darwin.SendInt(s.w.ns, sel.windowNumber) }

// windowPoint returns the window coordinates (bottom-left origin) of the
// view point p (top-left origin, points).
func (s *session) windowPoint(p platform.Point) darwin.CGPoint {
	return darwin.SendPoint(s.w.view, isel.convertPointToView, darwin.CGPoint{X: float64(p.X), Y: float64(p.Y)}, darwin.ID(0))
}

// globalPoint returns the CoreGraphics global coordinates (top-left origin
// of the main display) of the view point p.
func (s *session) globalPoint(p platform.Point) darwin.CGPoint {
	screen := darwin.SendPoint(s.w.ns, isel.convertToScreen, s.windowPoint(p))
	main := darwin.Send(darwin.SendClass(darwin.GetClass("NSScreen"), tsel.screens), tsel.objectAtIndex, uint(0))
	h := darwin.SendRect(main, tsel.frame).Size.Height
	return darwin.CGPoint{X: screen.X, Y: h - screen.Y}
}

func (s *session) send(ev darwin.ID) {
	if ev == 0 {
		s.t.Errorf("building an event failed (nil NSEvent)")
		return
	}
	darwin.Send(app.ns, isel.sendEvent, ev)
}

// sendKey hands a key event to NSApp when the window is AppKit's key
// window (NSApp sends key events to that window only), else to the window.
func (s *session) sendKey(ev darwin.ID) {
	if s.key {
		s.send(ev)
		return
	}
	if ev == 0 {
		s.t.Errorf("building an event failed (nil NSEvent)")
		return
	}
	darwin.Send(s.w.ns, isel.sendEvent, ev)
}

func (s *session) toView(selector string, ev darwin.ID) {
	if ev == 0 {
		s.t.Errorf("building an event for %s failed (nil NSEvent)", selector)
		return
	}
	darwin.Send(s.w.view, darwin.Sel(selector), ev)
}

// keyEvent builds a keyDown (down) or keyUp NSEvent from a CGEvent with
// the key code, flags and characters given (AppKit derives
// charactersIgnoringModifiers itself). +[NSEvent keyEventWithType:...]
// cannot be used through purego on arm64: its isARepeat (BOOL) and keyCode
// (unsigned short) go on the stack, which Darwin packs into one slot and
// the message send does not, so every key read as keyCode 0 (see the
// package documentation of internal/darwin).
func (s *session) keyEvent(down bool, code uint16, flags uint64, chars string) darwin.ID {
	cg := darwin.CGEventCreateKeyboardEvent(0, code, down)
	if cg == 0 {
		return 0
	}
	defer darwin.CFRelease(cg)
	darwin.CGEventSetFlags(cg, flags)
	if u := utf16.Encode([]rune(chars)); len(u) > 0 {
		darwin.CGEventKeyboardSetUnicodeString(cg, uint(len(u)), &u[0])
	}
	ev := darwin.SendClass(app.event, tsel.withCGEvent, cg)
	if ev != 0 {
		s.t.Logf("built key event: type %d keyCode %#x flags %#x characters %q ignoring modifiers %q",
			darwin.SendUint(ev, isel.typ), uint16(darwin.SendUint(ev, isel.keyCode)), darwin.SendUint(ev, isel.modifierFlags),
			darwin.GoString(darwin.Send(ev, isel.characters)), darwin.GoString(darwin.Send(ev, isel.charactersIgnoring)))
	}
	return ev
}

// flagsEvent builds a flagsChanged NSEvent from a CGEvent (keyEventWithType:
// documents only key down and up).
func (s *session) flagsEvent(code uint16, flags uint64) darwin.ID {
	cg := darwin.CGEventCreateKeyboardEvent(0, code, true)
	if cg == 0 {
		return 0
	}
	defer darwin.CFRelease(cg)
	darwin.CGEventSetType(cg, darwin.CGEventFlagsChanged)
	darwin.CGEventSetFlags(cg, flags)
	return darwin.SendClass(app.event, tsel.withCGEvent, cg)
}

// mouseEvent builds a mouse NSEvent at the view point p.
func (s *session) mouseEvent(typ int, p platform.Point, flags uint64, ms, clicks int) darwin.ID {
	return darwin.SendClass(app.event, tsel.mouseEvent, uint(typ), s.windowPoint(p), uint(flags), s.at(ms),
		s.windowNumber(), darwin.ID(0), 0, clicks, float32(1))
}

// crossEvent builds a mouseEntered or mouseExited NSEvent of the view's
// tracking area (its tracking number is the area, as AppKit's are).
func (s *session) crossEvent(typ int, p platform.Point, ms int) darwin.ID {
	return darwin.SendClass(app.event, tsel.enterExit, uint(typ), s.windowPoint(p), uint(0), s.at(ms),
		s.windowNumber(), darwin.ID(0), 0, int(s.w.tracking), uintptr(0))
}

// otherMouseEvent builds an otherMouseDown or otherMouseUp NSEvent of
// button (2 middle, 3 back) at the view point p, from a CGEvent: it has
// no window, and screen coordinates.
func (s *session) otherMouseEvent(down bool, button uint32, p platform.Point) darwin.ID {
	typ := uint32(darwin.CGEventOtherMouseUp)
	if down {
		typ = darwin.CGEventOtherMouseDown
	}
	cg := darwin.CGEventCreateMouseEvent(0, typ, s.globalPoint(p), button)
	if cg == 0 {
		return 0
	}
	defer darwin.CFRelease(cg)
	return darwin.SendClass(app.event, tsel.withCGEvent, cg)
}

func modsString(m platform.Modifiers) string {
	var parts []string
	for _, x := range []struct {
		m platform.Modifiers
		s string
	}{{platform.ModShift, "Shift"}, {platform.ModCtrl, "Ctrl"}, {platform.ModAlt, "Alt"}, {platform.ModSuper, "Super"}} {
		if m&x.m != 0 {
			parts = append(parts, x.s)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "+")
}

func keyString(e platform.KeyEvent) string {
	s := fmt.Sprintf("%v %v key=%q text=%q mods=%s", e.Action, e.Code, e.Key, e.Text, modsString(e.Mods))
	if e.Repeat {
		s += " repeat"
	}
	if e.Synthetic {
		s += " synthetic"
	}
	return s
}

// TestKeyboard: keys with and without modifiers and their releases,
// modifier keys through flagsChanged:, a Command shortcut (delivered from
// performKeyEquivalent:) and its release (which only TelaApplication's
// sendEvent: override delivers), and the synthetic release of a key held
// when the window loses the focus.
func TestKeyboard(t *testing.T) {
	const (
		kvkA, kvkS, kvkB = 0x00, 0x01, 0x0B
		kvkShift, kvkCmd = 0x38, 0x37
		shift            = keyboard.MacFlagShift | keyboard.MacDeviceLeftShift
		cmd              = keyboard.MacFlagCommand | keyboard.MacDeviceLeftCommand
	)
	s := runSession(t, "tela keyboard", true, func(s *session) {
		s.t.Logf("ISO keyboard: %v", darwin.KeyboardISO())
		s.sendKey(s.keyEvent(true, kvkA, 0, "a"))
		s.sendKey(s.keyEvent(false, kvkA, 0, "a"))
		s.sendKey(s.flagsEvent(kvkShift, shift))
		s.sendKey(s.keyEvent(true, kvkA, shift, "A"))
		s.sendKey(s.keyEvent(false, kvkA, shift, "A"))
		s.sendKey(s.flagsEvent(kvkShift, 0))
		s.sendKey(s.flagsEvent(kvkCmd, cmd))
		s.sendKey(s.keyEvent(true, kvkS, cmd, "s"))
		s.sendKey(s.keyEvent(false, kvkS, cmd, "s"))
		s.sendKey(s.flagsEvent(kvkCmd, 0))
		s.sendKey(s.keyEvent(true, kvkB, 0, "b"))
		// The window loses the focus with B held, and gets it back.
		darwin.Send(s.w.delegate, darwin.Sel("windowDidResignKey:"), darwin.ID(0))
		darwin.Send(s.w.delegate, darwin.Sel("windowDidBecomeKey:"), darwin.ID(0))
		// The release of a key pressed before the focus came back.
		s.sendKey(s.keyEvent(false, kvkB, 0, "b"))
	})
	if s == nil {
		return
	}
	if s.key {
		t.Logf("events went through -[NSApp sendEvent:]: Command+S came through performKeyEquivalent: and its release through TelaApplication's sendEvent:")
	} else {
		t.Logf("note: the window was not AppKit's key window; events went to -[NSWindow sendEvent:], so the Command release forwarding of TelaApplication is not covered by this run")
	}
	var got []string
	for _, e := range s.events {
		switch e := e.(type) {
		case platform.KeyEvent:
			got = append(got, keyString(e))
		case platform.FocusEvent:
			got = append(got, fmt.Sprintf("focus %v", e.Focused))
		}
	}
	if s.simulated && len(got) > 0 && got[0] == "focus true" {
		got = got[1:] // the simulated focus, given before the script
	}
	want := []string{
		`press KeyA key="a" text="a" mods=-`,
		`release KeyA key="a" text="" mods=-`,
		`press ShiftLeft key="Shift" text="" mods=Shift`,
		`press KeyA key="A" text="A" mods=Shift`,
		`release KeyA key="A" text="" mods=Shift`,
		`release ShiftLeft key="Shift" text="" mods=-`,
		`press MetaLeft key="Meta" text="" mods=Super`,
		`press KeyS key="s" text="s" mods=Super`,
		`release KeyS key="s" text="" mods=Super`,
		`release MetaLeft key="Meta" text="" mods=-`,
		`press KeyB key="b" text="b" mods=-`,
		`release KeyB key="b" text="" mods=- synthetic`,
		`focus false`,
		`focus true`,
	}
	for _, g := range got {
		t.Logf("got: %s", g)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("key events differ:\n got:\n  %s\n want:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	for _, e := range s.events {
		if k, ok := e.(platform.KeyEvent); ok && !k.Synthetic {
			if d := k.Time - s.b.Now(); d > time.Second || d < -30*time.Second {
				t.Errorf("key event time %v is far from the backend clock (now %v)", k.Time, s.b.Now())
			}
		}
	}
}

func pointerString(e platform.PointerEvent) string {
	s := fmt.Sprintf("%v %.0f,%.0f", e.Action, e.Pos.X, e.Pos.Y)
	if e.Button != platform.ButtonNone {
		s += fmt.Sprintf(" button=%d", e.Button)
	}
	if e.Buttons != 0 {
		s += fmt.Sprintf(" held=%d", e.Buttons)
	}
	if e.Mods != 0 {
		s += " mods=" + modsString(e.Mods)
	}
	return s
}

// TestPointer: enter, moves, clicks of the left, right, middle and back
// buttons with modifiers, a double click by the shared rule (ADR-192), a
// drag out of the window whose leave waits for the release, and leave.
func TestPointer(t *testing.T) {
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	const (
		lDown, lUp, rDown, rUp = darwin.NSEventTypeLeftMouseDown, darwin.NSEventTypeLeftMouseUp, darwin.NSEventTypeRightMouseDown, darwin.NSEventTypeRightMouseUp
		moved, lDragged        = darwin.NSEventTypeMouseMoved, darwin.NSEventTypeLeftMouseDragged
		entered, exited        = darwin.NSEventTypeMouseEntered, darwin.NSEventTypeMouseExited
	)
	var hidden []bool
	var limit time.Duration
	s := runSession(t, "tela pointer", false, func(s *session) {
		limit = s.b.Settings().DoubleClickTime()
		// The real pointer may be over the window (it opens centered):
		// start from a pointer outside, as the script assumes.
		s.w.ptr = pointerState{}
		s.toView("mouseEntered:", s.crossEvent(entered, pt(10, 10), 0))
		s.toView("mouseMoved:", s.mouseEvent(moved, pt(20, 20), 0, 10, 0))
		s.send(s.mouseEvent(lDown, pt(30, 30), 0, 20, 1))
		s.send(s.mouseEvent(lUp, pt(30, 30), 0, 40, 1))
		s.send(s.mouseEvent(lDown, pt(31, 30), 0, 220, 2))
		s.send(s.mouseEvent(lUp, pt(31, 30), 0, 240, 2))
		s.send(s.mouseEvent(rDown, pt(40, 40), keyboard.MacFlagShift, 300, 1))
		s.send(s.mouseEvent(rUp, pt(40, 40), keyboard.MacFlagShift, 320, 1))
		s.toView("otherMouseDown:", s.otherMouseEvent(true, 2, pt(50, 50)))
		s.toView("otherMouseUp:", s.otherMouseEvent(false, 2, pt(50, 50)))
		s.toView("otherMouseDown:", s.otherMouseEvent(true, 3, pt(50, 50)))
		s.toView("otherMouseUp:", s.otherMouseEvent(false, 3, pt(50, 50)))
		// A drag out of the window: the exit waits for the release.
		s.send(s.mouseEvent(lDown, pt(60, 60), 0, 400, 1))
		s.toView("mouseExited:", s.crossEvent(exited, pt(-1, 60), 410))
		s.toView("mouseDragged:", s.mouseEvent(lDragged, pt(-10, 60), 0, 420, 0))
		s.toView("mouseUp:", s.mouseEvent(lUp, pt(-10, 60), 0, 430, 1))
		// Back in, with the cursor hidden while inside.
		s.w.SetCursor(platform.CursorNone)
		s.toView("mouseEntered:", s.crossEvent(entered, pt(5, 5), 500))
		hidden = append(hidden, s.w.cursorHidden)
		s.toView("mouseExited:", s.crossEvent(exited, pt(-1, 5), 510))
		hidden = append(hidden, s.w.cursorHidden)
		s.w.SetCursor(platform.CursorDefault)
	})
	if s == nil {
		return
	}
	var got []string
	var downs []platform.PointerEvent
	for _, e := range s.events {
		if p, ok := e.(platform.PointerEvent); ok {
			got = append(got, pointerString(p))
			if p.Action == platform.PointerDown && p.Button == platform.ButtonPrimary {
				downs = append(downs, p)
			}
			if p.Kind != platform.PointerMouse {
				t.Errorf("%v: Kind %v, want mouse", pointerString(p), p.Kind)
			}
		}
	}
	for _, g := range got {
		t.Logf("got: %s", g)
	}
	want := []string{
		"enter 10,10",
		"move 20,20",
		"down 30,30 button=1 held=1",
		"up 30,30 button=1",
		"down 31,30 button=1 held=1",
		"up 31,30 button=1",
		"down 40,40 button=2 held=2 mods=Shift",
		"up 40,40 button=2 mods=Shift",
		"down 50,50 button=3 held=4",
		"up 50,50 button=3",
		"down 50,50 button=4 held=8",
		"up 50,50 button=4",
		"down 60,60 button=1 held=1",
		"move -10,60 held=1",
		"up -10,60 button=1",
		"leave -10,60",
		"enter 5,5",
		"leave -1,5",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("pointer events differ:\n got:\n  %s\n want:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// The second click is a double click by the platform's rule: the
	// layers above count clicks from the times and positions.
	if len(downs) >= 2 {
		dt := downs[1].Time - downs[0].Time
		dx, dy := float64(downs[1].Pos.X-downs[0].Pos.X), float64(downs[1].Pos.Y-downs[0].Pos.Y)
		t.Logf("double click: %v apart (limit %v), %.1f points apart (slop %d)", dt, limit, math.Hypot(dx, dy), platform.DoubleClickSlop)
		if dt <= 0 || dt > limit || math.Hypot(dx, dy) > platform.DoubleClickSlop {
			t.Errorf("the second press, %v and %.1f points after the first, is not a double click by the shared rule", dt, math.Hypot(dx, dy))
		}
	}
	if len(hidden) != 2 {
		t.Fatalf("the script did not run to its end")
	}
	t.Logf("cursor hidden while inside with CursorNone: %v, after the exit: %v", hidden[0], hidden[1])
	if !hidden[0] || hidden[1] {
		t.Errorf("CursorNone: hidden inside %v, after leaving %v; want true, false", hidden[0], hidden[1])
	}
}

// TestScroll: wheel notches (Steps), touchpad gestures (precise Delta with
// phases), the momentum AppKit adds after the fingers lift (dropped: the
// layers above fling), and horizontal scrolling. The events are CGEvents
// turned into NSEvents, handed to the view.
func TestScroll(t *testing.T) {
	type step struct {
		name             string
		pixel            bool
		wheel1, wheel2   int32
		phase, momentum  int64
		wantPhase        int64 // NSEvent phase the step should have
		wantMomentum     int64
		want             string // the ScrollEvent, "" for none
		nsPhase, nsMomen uint64
		nsPrecise        bool
		nsDX, nsDY       float64
	}
	steps := []step{
		{name: "wheel up", wheel1: 1, want: "steps 0,-1 phase none"},
		{name: "wheel down 2", wheel1: -2, want: "steps 0,2 phase none"},
		{name: "gesture began", pixel: true, wheel1: -10, phase: darwin.CGScrollPhaseBegan, wantPhase: darwin.NSEventPhaseBegan, want: "delta 0,10 phase update"},
		{name: "gesture changed", pixel: true, wheel1: -20, phase: darwin.CGScrollPhaseChanged, wantPhase: darwin.NSEventPhaseChanged, want: "delta 0,20 phase update"},
		{name: "gesture ended", pixel: true, phase: darwin.CGScrollPhaseEnded, wantPhase: darwin.NSEventPhaseEnded, want: "delta 0,0 phase end"},
		{name: "momentum begin", pixel: true, wheel1: -15, momentum: darwin.CGMomentumScrollPhaseBegin, wantMomentum: darwin.NSEventPhaseBegan},
		{name: "momentum continue", pixel: true, wheel1: -5, momentum: darwin.CGMomentumScrollPhaseContinue, wantMomentum: darwin.NSEventPhaseChanged},
		{name: "momentum end", pixel: true, momentum: darwin.CGMomentumScrollPhaseEnd, wantMomentum: darwin.NSEventPhaseEnded},
		{name: "horizontal", pixel: true, wheel2: -7, want: "delta 7,0 phase none"},
	}
	pos := platform.Point{X: 50, Y: 40}
	s := runSession(t, "tela scroll", false, func(s *session) {
		for i := range steps {
			st := &steps[i]
			units := uint32(darwin.CGScrollEventUnitLine)
			if st.pixel {
				units = darwin.CGScrollEventUnitPixel
			}
			cg := darwin.CGEventCreateScrollWheelEvent2(0, units, 2, st.wheel1, st.wheel2, 0)
			if cg == 0 {
				s.t.Errorf("%s: CGEventCreateScrollWheelEvent2 failed", st.name)
				continue
			}
			darwin.CGEventSetLocation(cg, s.globalPoint(pos))
			if st.phase != 0 {
				darwin.CGEventSetIntegerValueField(cg, darwin.CGScrollWheelEventScrollPhase, st.phase)
			}
			if st.momentum != 0 {
				darwin.CGEventSetIntegerValueField(cg, darwin.CGScrollWheelEventMomentumPhase, st.momentum)
			}
			ev := darwin.SendClass(app.event, tsel.withCGEvent, cg)
			darwin.CFRelease(cg)
			if ev == 0 {
				s.t.Errorf("%s: eventWithCGEvent: returned nil", st.name)
				continue
			}
			st.nsPhase = darwin.SendUint(ev, isel.phase)
			st.nsMomen = darwin.SendUint(ev, isel.momentumPhase)
			st.nsPrecise = darwin.SendBool(ev, isel.hasPrecise)
			st.nsDX = darwin.SendFloat(ev, isel.scrollingDeltaX)
			st.nsDY = darwin.SendFloat(ev, isel.scrollingDeltaY)
			s.toView("scrollWheel:", ev)
		}
	})
	if s == nil {
		return
	}
	var got []platform.ScrollEvent
	for _, e := range s.events {
		if sc, ok := e.(platform.ScrollEvent); ok {
			got = append(got, sc)
		}
	}
	str := func(e platform.ScrollEvent) string {
		if e.Steps != (platform.Point{}) {
			return fmt.Sprintf("steps %g,%g phase %v", e.Steps.X, e.Steps.Y, e.Phase)
		}
		return fmt.Sprintf("delta %g,%g phase %v", e.Delta.X, e.Delta.Y, e.Phase)
	}
	// What AppKit made of each CGEvent, translated by the pure rules
	// (tested on every OS), is what the view must deliver; that it is
	// what each step meant is logged.
	var want []string
	for _, st := range steps {
		t.Logf("%s: NSEvent phase %d momentumPhase %d precise %v scrollingDelta %g,%g", st.name, st.nsPhase, st.nsMomen, st.nsPrecise, st.nsDX, st.nsDY)
		derived := ""
		if d, sp, ph, ok := macScrollEvent(macScroll{DX: st.nsDX, DY: st.nsDY, Precise: st.nsPrecise, Phase: st.nsPhase, Momentum: st.nsMomen}); ok {
			derived = str(platform.ScrollEvent{Delta: d, Steps: sp, Phase: ph})
			want = append(want, derived)
		}
		if derived != st.want {
			t.Logf("  note: AppKit made the step's event into %q, not the %q it meant (phase %d, momentum %d, precise %v)",
				derived, st.want, st.wantPhase, st.wantMomentum, st.pixel)
		}
	}
	var gotS []string
	for _, e := range got {
		gotS = append(gotS, str(e))
		t.Logf("got: %s at %.1f,%.1f mods %s", str(e), e.Pos.X, e.Pos.Y, modsString(e.Mods))
		if dx, dy := e.Pos.X-pos.X, e.Pos.Y-pos.Y; dx*dx+dy*dy > 4 {
			t.Errorf("scroll position %v, want about %v (screen coordinates of a window-less event converted into the view)", e.Pos, pos)
		}
	}
	if strings.Join(gotS, "\n") != strings.Join(want, "\n") {
		t.Errorf("scroll events differ:\n got:\n  %s\n want:\n  %s", strings.Join(gotS, "\n  "), strings.Join(want, "\n  "))
	}
}
