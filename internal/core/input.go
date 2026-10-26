package core

import (
	"time"

	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Pointer input (ARCHITECTURE section 14): platform pointer events become
// DOM-style events dispatched to the elements under the pointer, the
// gestures of the arena (internal/gesture) and the interaction states of
// the hover:, active: and disabled: variants.
//
// # Targets and propagation
//
// The target of an event is the element hit-tested under the pointer
// (hit.go). Events bubble from the target to the root through the
// primitive ancestors (components are transparent), unless a handler calls
// StopPropagation (the other handlers of that element still run). There
// is no capture phase. Enter and leave events do not bubble: they are
// fired on every element the pointer enters or leaves, leaves from the
// innermost outward, enters from the outermost inward, as in the DOM.
//
// A disabled element (Disabled(true)) and its descendants run no pointer
// handler, and their events do not reach it; its ancestors still receive
// them. No gesture recognizer is created for them, and they do not become
// active.
//
// # Pointer and mouse events
//
// Pointer events (OnPointerDown, OnPointerMove, OnPointerUp,
// OnPointerCancel, OnPointerEnter, OnPointerLeave) fire for every pointer.
// Mouse events (OnMouseDown, OnMouseMove, OnMouseUp, OnMouseEnter,
// OnMouseLeave, OnClick, OnDoubleClick, OnContextMenu) are the
// compatibility view of the primary pointer: the mouse, the pen and the
// first touch contact. For one platform event the order is Chromium's
// (the Pointer Events spec's compatibility mapping): pointer leave and
// enter events, mouse leave and enter events, the pointer event, then
// the mouse event; a release is followed by click and, for the second
// click of a sequence, double click. Pointerdown and
// pointerup fire when the first button is pressed and the last released; a
// button pressed or released while another is held fires a pointer move
// (DOM chorded buttons), while mousedown and mouseup fire for each button.
// PreventDefault on pointerdown suppresses the mouse events of the press,
// not the click.
//
// # Click, double click, context menu
//
// A press with the primary button starts a gesture arena with a tap
// recognizer (when the press's path has click or double-click handlers),
// a drag recognizer per element with drag handlers and a long-press
// recognizer per element with OnLongPress (for a mouse press, not the
// elements whose only long-press handlers are OnTouchLongPress's),
// innermost first. The click
// fires on release, on the nearest common ancestor of the press and
// release targets (DOM), however far the pointer moved, unless a drag or
// long press won the arena. ClickCount is the DOM's detail
// (gesture.ClickCounter: 500 ms and 5 px for mice and pens, 300 ms and
// 100 px for touch); a double click fires after the click whose count is
// 2. OnContextMenu fires on the release of the secondary button.
//
// Drags (OnDragStart, OnDrag, OnDragEnd) and long presses go to the
// element whose recognizer won, without bubbling.
//
// Scrolling (scroll.go, scrollbar.go): a touch press also adds the pan
// recognizer of each scroll container on the path that can scroll, which
// competes like a drag (the inner elements' drags first); when it wins,
// the pressed element gets pointercancel and nothing more of that touch is
// dispatched (no moves, pointerup, mouseup or end of an interact-outside
// press) until it is released, as in a browser; a primary press
// on a visible scrollbar takes no part in the arena of the content: the
// thumb's eager drag or a page of the track, without click or focus
// change. A press stops the flings, and pointer movement ends the latched
// wheel transaction.
//
// # Capture
//
// While a touch or pen is down, its events target the element it pressed
// (implicit pointer capture, as in the DOM). A mouse keeps targeting the
// element under it; drags follow the pointer outside the element and the
// window, as backends keep delivering moves while a button is held.
//
// # Interaction states
//
// An element is hovered while a mouse or pen pointer is over it or a
// descendant (touch does not hover), active while the primary button
// pressed on it or a descendant is held (a touch until a scroll takes it
// over), and disabled with Disabled(true).
// A change restyles only the elements whose classes test it: the element
// for hover:, active:, disabled: and enabled:, the descendants of a group
// element (class group) for group-*, the following siblings of a peer
// (class peer) for peer-*. group-* tests every group ancestor and peer-*
// every preceding peer sibling, as Tailwind's selectors do.
//
// # Builds between events
//
// After each discrete event (pointer down and up, mouse down and up,
// click, double click, context menu, long press, drag start and end) the
// queued updates are applied and the build phase runs (React's flush of
// discrete events), so the handlers of the next event see the closures of
// the new build; the rest of the pipeline waits for the frame. Continuous
// events (moves, enter, leave, drag updates) are batched until the frame.
// Hit-testing keeps using the geometry of the last frame.
//
// # Time
//
// The long-press timeout runs on the backend clock: NextDeadline reports
// when the arena needs Tick, and the Host arms one wake-up for it only
// while a press waits.

// pointerKey identifies a platform pointer.
type pointerKey struct {
	kind platform.PointerKind
	id   platform.PointerID
}

// pointerState is the state of one pointer.
type pointerState struct {
	key     pointerKey
	primary bool
	pos     gfx.Point
	buttons platform.Buttons
	mods    platform.Modifiers
	inside  bool // over the window (mouse and pen)

	// target is the latest target; over is the boundary chain (target up
	// to the root) whose enter events fired.
	target *Element
	over   []*Element
	// capture is the press target of a touch or pen (implicit capture).
	capture *Element

	// The primary-button press.
	pressing   bool
	gid        gesture.PointerID
	down       *Element   // its target
	active     []*Element // the elements it made active
	clickCount int
	// noMouse: pointerdown was cancelled, so the mouse events wait for
	// all buttons to be up.
	noMouse bool
	// outside are the interact-outside listeners the press started
	// outside of, and noClick reports that one cancelled its click
	// (overlay.go).
	outside []outsidePress
	noClick bool
	// panned: a touch pan took the press over and cancelled the pointer
	// (scroll.go's panRecognizer), and the pointer left its boundary
	// chain. As in a browser, which sends nothing after pointercancel and
	// the pointerout and pointerleave that follow it, its later moves and
	// its release are not dispatched to elements, nor enter or leave the
	// elements it passes over (the gestures still see them: the pan
	// scrolls and flings), until the press ends.
	panned bool
}

// inputState is the pointer input state of a Tree (UI thread).
type inputState struct {
	ptrs    map[pointerKey]*pointerState
	arena   gesture.Manager
	clicks  gesture.ClickCounter
	nextGID gesture.PointerID
	hit     hitter
	chain   []*Box
	leaves  []*Element
	enters  []*Element
	// bar is the scrollbar part the last hit test found (scrollbar.go).
	bar barHit
	// fdrag is the selection dragged in a text field (fieldkeys.go).
	fdrag *fieldDrag
	// rehover is set while rehover fires boundary events (Rehovered).
	rehover bool
}

// HandlePointer dispatches a platform pointer event (see the comment at
// the top of input.go). Call it on the UI thread.
func (t *Tree) HandlePointer(e platform.PointerEvent) {
	in := &t.in
	key := pointerKey{e.Kind, e.ID}
	ps := in.ptrs[key]
	if ps == nil {
		ps = &pointerState{key: key, primary: true}
		if e.Kind == platform.PointerTouch {
			for k := range in.ptrs {
				if k.kind == platform.PointerTouch {
					ps.primary = false
					break
				}
			}
		}
		if in.ptrs == nil {
			in.ptrs = map[pointerKey]*pointerState{}
		}
		in.ptrs[key] = ps
	}
	prev := ps.buttons
	moved := ps.pos != gfx.Point{X: e.Pos.X, Y: e.Pos.Y}
	ps.pos = gfx.Point{X: e.Pos.X, Y: e.Pos.Y}
	ps.buttons, ps.mods = e.Buttons, e.Mods
	t.scr.now = max(t.scr.now, e.Time)
	if moved || e.Action == platform.PointerDown {
		// A moved pointer ends a wheel transaction (scroll.go).
		t.breakLatch()
	}
	switch e.Action {
	case platform.PointerEnter, platform.PointerMove:
		t.pointerMove(ps, e)
	case platform.PointerLeave:
		t.pointerLeave(ps)
	case platform.PointerDown:
		t.pointerDown(ps, e, prev)
	case platform.PointerUp:
		t.pointerUp(ps, e)
	case platform.PointerCancel:
		t.pointerCancel(ps, e)
	}
	clear(in.leaves)
	clear(in.enters)
	in.leaves, in.enters = in.leaves[:0], in.enters[:0]
}

// pointerTarget returns the element pointer events of ps target: the
// capture target, else the element under the pointer.
func (t *Tree) pointerTarget(ps *pointerState) *Element {
	if c := ps.capture; c != nil {
		if c.mounted {
			return c
		}
		ps.capture = nil // removed: the capture is lost (DOM)
	}
	if !ps.inside && ps.key.kind != platform.PointerTouch {
		return nil
	}
	return t.hitTest(ps.pos)
}

func (t *Tree) pointerMove(ps *pointerState, e platform.PointerEvent) {
	ps.inside = true
	target := t.pointerTarget(ps)
	ps.target = target
	if !ps.panned {
		// A panned touch left its chain at the pointercancel
		// (panRecognizer) and enters nothing until it lifts.
		if ps.key.kind != platform.PointerTouch || ps.buttons != 0 {
			t.updateOver(ps, target)
		}
		t.fireBoundary(ps)
		fire(t, evPointerMove, target, t.pointerEvent(ps, platform.ButtonNone), true)
		if ps.primary && !ps.noMouse {
			fire(t, evMouseMove, target, t.mouseEvent(ps, platform.ButtonNone, 0), true)
		}
	}
	if ps.pressing {
		t.in.arena.Move(t.gestureEvent(ps, e.Time))
	}
	t.fieldPointerMove(ps)
	t.hoverBars(ps)
}

func (t *Tree) pointerLeave(ps *pointerState) {
	ps.inside = false
	t.updateOver(ps, nil)
	t.fireBoundary(ps)
	ps.target = nil
	t.hoverBars(ps)
	if ps.key.kind == platform.PointerTouch && ps.buttons == 0 && !ps.pressing {
		delete(t.in.ptrs, ps.key)
	}
}

func (t *Tree) pointerDown(ps *pointerState, e platform.PointerEvent, prev platform.Buttons) {
	if prev == 0 && ps.pressing {
		// The release of the previous press never came: drop it.
		ps.pressing, ps.panned = false, false
		t.in.arena.Cancel(t.gestureEvent(ps, e.Time))
		t.clearActive(ps)
	}
	ps.inside = true
	target := t.pointerTarget(ps)
	bar := t.in.bar
	if ps.capture != nil {
		bar = barHit{}
	}
	ps.target = target
	t.updateOver(ps, target)
	t.fireBoundary(ps)
	// A press stops kinetic scrolling.
	t.stopFlings()
	if prev == 0 {
		if ps.key.kind != platform.PointerMouse {
			ps.capture = target
		}
		t.noteMouse()
		pe := t.pointerEvent(ps, e.Button)
		st := &eventState{}
		ps.noClick = false
		if outsideButton(ps, e.Button) {
			// The listeners of presses outside, as in the capture phase
			// of the document (overlay.go): a right or middle click too.
			t.outsideDown(ps, target, pe, st)
		}
		if !st.stopped {
			dispatchWith(t, evPointerDown, target, pe, true, true, st)
		}
		ps.noMouse = st.defaultPrevented()
		t.flushDiscrete()
	} else {
		fire(t, evPointerMove, target, t.pointerEvent(ps, e.Button), true)
	}
	if ps.primary {
		ps.clickCount = t.in.clicks.Press(gesture.Kind(ps.key.kind), uint8(e.Button),
			gesture.Point{X: ps.pos.X, Y: ps.pos.Y}, e.Time)
		prevented := ps.noMouse
		if !ps.noMouse {
			st := fire(t, evMouseDown, target, t.mouseEvent(ps, e.Button, ps.clickCount), true)
			prevented = st.defaultPrevented()
			t.flushDiscrete()
		}
		// The default action of a primary mousedown moves the focus
		// (focus.go); cancelling pointerdown or mousedown prevents it. A
		// press on a scrollbar does not move it.
		if prev == 0 && e.Button == platform.ButtonPrimary && !prevented && bar.s == nil {
			t.mouseFocus(target)
			// A press in a text field puts its caret (fieldkeys.go).
			t.fieldPress(ps, target)
		}
	}
	if e.Button != platform.ButtonPrimary || ps.pressing {
		return
	}
	ps.pressing = true
	ps.down = target
	t.setActive(ps, target)
	t.in.nextGID++
	ps.gid = t.in.nextGID
	if bar.s != nil {
		// A scrollbar press: the thumb's drag or a page, and no click.
		t.in.arena.Down(t.gestureEvent(ps, e.Time), t.barPress(bar))
		return
	}
	t.in.arena.Down(t.gestureEvent(ps, e.Time), t.recognizers(ps, target)...)
}

func (t *Tree) pointerUp(ps *pointerState, e platform.PointerEvent) {
	target := t.pointerTarget(ps)
	ps.target = target
	switch {
	case ps.panned:
		// Cancelled by a touch pan: the release only ends the gestures.
	case e.Buttons == 0:
		pe := t.pointerEvent(ps, e.Button)
		st := &eventState{}
		t.outsideUp(ps, target, pe, st)
		if st.defaultPrevented() {
			ps.noClick = true
		}
		if !st.stopped {
			dispatchWith(t, evPointerUp, target, pe, true, true, st)
		}
		t.flushDiscrete()
	default:
		fire(t, evPointerMove, target, t.pointerEvent(ps, e.Button), true)
	}
	if ps.primary && !ps.noMouse && !ps.panned {
		fire(t, evMouseUp, target, t.mouseEvent(ps, e.Button, ps.clickCount), true)
		t.flushDiscrete()
	}
	if e.Button == platform.ButtonPrimary && ps.pressing {
		ps.pressing = false
		t.fieldPointerUp(ps)
		// Routes the release: the click (tap), the end of a drag.
		t.in.arena.Up(t.gestureEvent(ps, e.Time))
		t.clearActive(ps)
		ps.down = nil
	}
	if e.Button == platform.ButtonSecondary && ps.primary && !ps.panned {
		fire(t, evContextMenu, target, t.mouseEvent(ps, e.Button, ps.clickCount), true)
		t.flushDiscrete()
	}
	if e.Buttons == 0 {
		t.endPress(ps)
	}
}

func (t *Tree) pointerCancel(ps *pointerState, e platform.PointerEvent) {
	target := t.pointerTarget(ps)
	ps.target = target
	ps.buttons = 0
	if !ps.panned { // a pan cancelled it already
		fire(t, evPointerCancel, target, t.pointerEvent(ps, platform.ButtonNone), true)
	}
	clear(ps.outside)
	ps.outside = ps.outside[:0]
	if ps.pressing {
		ps.pressing = false
		t.fieldPointerUp(ps)
		t.in.arena.Cancel(t.gestureEvent(ps, e.Time))
		t.in.clicks.Reset()
		t.clearActive(ps)
		ps.down = nil
	}
	t.flushDiscrete()
	t.endPress(ps)
}

// endPress ends the capture once every button is up: a touch contact is
// gone (it leaves everything), a pen goes back to hovering.
func (t *Tree) endPress(ps *pointerState) {
	ps.noMouse, ps.panned = false, false
	captured := ps.capture != nil
	ps.capture = nil
	switch {
	case ps.key.kind == platform.PointerTouch:
		t.updateOver(ps, nil)
		t.fireBoundary(ps)
		ps.target = nil
		delete(t.in.ptrs, ps.key)
	case captured && ps.inside:
		target := t.hitTest(ps.pos)
		ps.target = target
		t.updateOver(ps, target)
		t.fireBoundary(ps)
	}
}

// flushDiscrete applies the queued updates and builds what they marked,
// after a discrete event (React's flush): the next event's handlers see
// the new closures.
func (t *Tree) flushDiscrete() {
	t.ApplyUpdates()
	t.buildPhase()
	if t.foc.auto != nil || len(t.foc.restores) > 0 || len(t.foc.scopeAuto) > 0 || t.foc.lost != nil {
		t.focusWork()
	}
}

// ---------------------------------------------------------------------
// Gestures.

func (t *Tree) gestureEvent(ps *pointerState, at time.Duration) gesture.Event {
	return gesture.Event{Pointer: ps.gid, Kind: gesture.Kind(ps.key.kind),
		Pos: gesture.Point{X: ps.pos.X, Y: ps.pos.Y}, Time: at}
}

// recognizers returns the recognizers of a primary-button press on
// target: a long press and a drag for each element of the path with those
// handlers (innermost first), then a tap when the path has click handlers
// and the pointer is primary. Disabled elements and their descendants
// take no part.
func (t *Tree) recognizers(ps *pointerState, target *Element) []gesture.Recognizer {
	var rs []gesture.Recognizer
	tap := false
	inert := outermostDisabled(target)
	for el := target; el != nil; el = el.styleParent() {
		if inert != nil {
			if el == inert {
				inert = nil
			}
			continue
		}
		if s := el.box.scr; s != nil && ps.key.kind == platform.PointerTouch &&
			(s.userX && s.max.X > 0 || s.userY && s.max.Y > 0) {
			// Touch scrolling: the container's pan competes with the
			// drags of the elements inside it (scroll.go).
			rs = append(rs, t.panRecognizer(s, ps))
		}
		m := el.node.d.evMask
		if m&evLongPress.mask() != 0 && (ps.key.kind != platform.PointerMouse || el.node.d.mouseLongPress()) {
			// A touch-only long press (OnTouchLongPress) leaves a mouse
			// press to the click.
			rs = append(rs, &gesture.LongPress{OnLongPress: func(ge gesture.Event) { t.longPress(ps, el, ge) }})
		}
		if m&dragMask != 0 {
			rs = append(rs, &gesture.Drag{
				OnStart:  func(u gesture.DragUpdate) { t.drag(ps, el, evDragStart, u) },
				OnUpdate: func(u gesture.DragUpdate) { t.drag(ps, el, evDrag, u) },
				OnEnd:    func(u gesture.DragUpdate) { t.drag(ps, el, evDragEnd, u) },
			})
		}
		if m&clickMask != 0 || el.box.kind == style.KindButton || el.box.kind == style.KindLabel {
			// Buttons and labels have activation behavior (form.go).
			tap = true
		}
	}
	if tap && ps.primary {
		rs = append(rs, &gesture.Tap{
			OnTap:    func(gesture.Event) { t.click(ps) },
			OnReject: func() { t.in.clicks.Reset() },
		})
	}
	return rs
}

// click fires the click (and the double click) of a press of ps that the
// tap recognizer won: on the nearest common ancestor of the press and
// release targets.
func (t *Tree) click(ps *pointerState) {
	ct := commonAncestor(ps.down, ps.target)
	if ct == nil || ps.noClick {
		return // or an interact-outside listener cancelled it (overlay.go)
	}
	n := ps.clickCount
	t.dispatchClick(ct, t.mouseEvent(ps, platform.ButtonPrimary, n))
	if n == 2 {
		fire(t, evDoubleClick, ct, t.mouseEvent(ps, platform.ButtonPrimary, n), true)
		t.flushDiscrete()
	}
}

func (t *Tree) longPress(ps *pointerState, el *Element, ge gesture.Event) {
	t.in.clicks.Reset()
	if !el.mounted {
		return
	}
	e := t.pointerEvent(ps, platform.ButtonPrimary)
	e.X, e.Y = ge.Pos.X, ge.Pos.Y
	// The press rides along, for KeepOutsidePress (overlay.go).
	dispatchWith(t, evLongPress, el, e, false, true, &eventState{press: ps})
	t.flushDiscrete()
}

func (t *Tree) drag(ps *pointerState, el *Element, k eventKind, u gesture.DragUpdate) {
	if k == evDragStart {
		t.in.clicks.Reset()
	}
	if !el.mounted {
		return
	}
	e := DragEvent{
		X: u.Pos.X, Y: u.Pos.Y,
		DeltaX: u.Delta.X, DeltaY: u.Delta.Y,
		OffsetX: u.Offset.X, OffsetY: u.Offset.Y,
		VelocityX: u.Velocity.X, VelocityY: u.Velocity.Y,
		Button: MouseButtonLeft,
	}
	e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(ps.mods)
	fire(t, k, el, e, false)
	if k != evDrag {
		t.flushDiscrete()
	}
}

// NextDeadline reports when the tree needs TickGestures, in the backend
// clock: a long press waiting for its timeout, overlay scrollbars
// waiting to fade, a caret waiting to blink, a UseInterval due. ok is
// false when nothing waits: an idle app arms no timer.
func (t *Tree) NextDeadline() (at time.Duration, ok bool) {
	at, ok = t.in.arena.NextDeadline()
	if s, has := t.scrollDeadline(); has && (!ok || s < at) {
		at, ok = s, true
	}
	if c, has := t.caretDeadline(); has && (!ok || c < at) {
		at, ok = c, true
	}
	if c, has := t.intervalDeadline(); has && (!ok || c < at) {
		at, ok = c, true
	}
	return at, ok
}

// TickGestures lets what waits for a deadline at or before now (the
// backend clock) act: a long press is recognized and its handler runs,
// overlay scrollbars start fading, the caret blinks, the functions of the
// UseInterval hooks due run.
func (t *Tree) TickGestures(now time.Duration) {
	t.scr.now = max(t.scr.now, now)
	t.in.arena.Tick(now)
	t.tickScroll(now)
	t.tickCaret(now)
	t.tickIntervals(now)
}

// resetInput forgets every pointer: presses end without events (the
// tree's content is gone).
func (t *Tree) resetInput() {
	t.in.arena = gesture.Manager{}
	clear(t.in.ptrs)
	t.in.clicks.Reset()
	t.in.bar = barHit{}
	t.in.fdrag = nil
	t.scr.wheel = wheelLatch{}
}

// ---------------------------------------------------------------------
// Boundaries and states.

// chainOf appends to out the boundary chain of target: target and its
// primitive ancestors up to the root.
func chainOf(out []*Element, target *Element) []*Element {
	for el := target; el != nil; el = el.styleParent() {
		out = append(out, el)
	}
	return out
}

// updateOver makes target's chain the boundary chain of ps: the elements
// left (innermost first) go to in.leaves and the elements entered
// (outermost first) to in.enters, for fireBoundary; hover states follow
// for mice and pens.
func (t *Tree) updateOver(ps *pointerState, target *Element) {
	in := &t.in
	clear(in.leaves)
	clear(in.enters)
	in.leaves, in.enters = in.leaves[:0], in.enters[:0]
	if len(ps.over) > 0 && ps.over[0] == target {
		return
	}
	nw := chainOf(nil, target)
	old := ps.over
	// The common tail (shared ancestors, up to the root).
	i, j := len(old), len(nw)
	for i > 0 && j > 0 && old[i-1] == nw[j-1] {
		i--
		j--
	}
	hovers := ps.key.kind != platform.PointerTouch
	for _, el := range old[:i] {
		in.leaves = append(in.leaves, el)
		if hovers {
			el.hoverN--
			if el.hoverN <= 0 {
				el.hoverN = 0
				t.setState(el, style.StateHover, false)
			}
		}
	}
	for k := j - 1; k >= 0; k-- {
		el := nw[k]
		in.enters = append(in.enters, el)
		if hovers {
			el.hoverN++
			t.setState(el, style.StateHover, true)
		}
	}
	ps.over = nw
	if t.dbg.flags&DebugLayout != 0 {
		t.debugTouch() // the layout overlay emphasizes the hovered element
	}
}

// fireBoundary fires the leave and enter events of the last updateOver:
// the pointer's, then the mouse's for a primary pointer (Chromium's order,
// the Pointer Events spec's compatibility mapping), before the event that
// moved the pointer.
func (t *Tree) fireBoundary(ps *pointerState) {
	in := &t.in
	if len(in.leaves) == 0 && len(in.enters) == 0 {
		return
	}
	for _, el := range in.leaves {
		fire(t, evPointerLeave, el, t.pointerEvent(ps, platform.ButtonNone), false)
	}
	for _, el := range in.enters {
		fire(t, evPointerEnter, el, t.pointerEvent(ps, platform.ButtonNone), false)
	}
	if ps.primary {
		for _, el := range in.leaves {
			fire(t, evMouseLeave, el, t.mouseEvent(ps, platform.ButtonNone, 0), false)
		}
		for _, el := range in.enters {
			fire(t, evMouseEnter, el, t.mouseEvent(ps, platform.ButtonNone, 0), false)
		}
	}
	clear(in.leaves)
	clear(in.enters)
	in.leaves, in.enters = in.leaves[:0], in.enters[:0]
}

// setActive makes target and its ancestors active for the press of ps,
// except disabled elements and their descendants.
func (t *Tree) setActive(ps *pointerState, target *Element) {
	inert := outermostDisabled(target)
	for el := target; el != nil; el = el.styleParent() {
		if inert != nil {
			if el == inert {
				inert = nil
			}
			continue
		}
		ps.active = append(ps.active, el)
		el.activeN++
		t.setState(el, style.StateActive, true)
	}
}

// clearActive ends the active state of the press of ps.
func (t *Tree) clearActive(ps *pointerState) {
	for _, el := range ps.active {
		el.activeN--
		if el.activeN <= 0 {
			el.activeN = 0
			t.setState(el, style.StateActive, false)
		}
	}
	clear(ps.active)
	ps.active = ps.active[:0]
}

// setState turns an interaction state of primitive el on or off and
// restyles the elements whose classes test it: el (hover:...), the
// descendants of a group (group-*), the following siblings of a peer
// (peer-*).
func (t *Tree) setState(el *Element, flag style.StateFlags, on bool) {
	old := el.istate
	if on {
		el.istate |= flag
	} else {
		el.istate &^= flag
	}
	changed := old ^ el.istate
	if changed == 0 || !el.mounted {
		return
	}
	bx := el.box
	if bx.stateUse&changed != 0 {
		t.markStyle(el)
	}
	if bx.isGroup && el.groupKids {
		t.markGroupUsers(el, changed)
	}
	if bx.isPeer {
		t.markPeerUsers(el, changed)
	}
}

// markGroupUsers restyles the descendants of el whose classes test the
// changed states of a group ancestor.
func (t *Tree) markGroupUsers(el *Element, changed style.StateFlags) {
	for _, c := range el.children {
		if c.kind == elemPrimitive && c.box.groupUse&changed != 0 {
			t.markStyle(c)
		}
		if c.groupKids {
			t.markGroupUsers(c, changed)
		}
	}
}

// markPeerUsers restyles the siblings after el whose classes test the
// changed states of a peer.
func (t *Tree) markPeerUsers(el *Element, changed style.StateFlags) {
	sp := el.styleParent()
	if sp == nil {
		return
	}
	after := false
	eachPrimitiveChild(sp, func(c *Element) bool {
		if c == el {
			after = true
		} else if after && c.box.peerUse&changed != 0 {
			t.markStyle(c)
		}
		return true
	})
}

// groupState returns the states of el's group ancestors (Tailwind's
// group-* selectors test any ancestor with the class group).
func groupState(el *Element) style.StateFlags {
	var s style.StateFlags
	for a := el.styleParent(); a != nil; a = a.styleParent() {
		if a.box.isGroup {
			s |= a.istate
		}
	}
	return s
}

// peerState returns the states of el's preceding peer siblings
// (Tailwind's peer-* selectors test any preceding sibling with the class
// peer).
func peerState(el *Element) style.StateFlags {
	sp := el.styleParent()
	if sp == nil {
		return 0
	}
	var s style.StateFlags
	eachPrimitiveChild(sp, func(c *Element) bool {
		if c == el {
			return false
		}
		if c.box.isPeer {
			s |= c.istate
		}
		return true
	})
	return s
}

// eachPrimitiveChild calls f on the primitive children of p in order,
// looking through components, until f returns false.
func eachPrimitiveChild(p *Element, f func(c *Element) bool) bool {
	for _, c := range p.children {
		switch c.kind {
		case elemPrimitive:
			if !f(c) {
				return false
			}
		case elemComponent:
			if !eachPrimitiveChild(c, f) {
				return false
			}
		}
	}
	return true
}

// outermostDisabled returns the outermost disabled element among target
// and its ancestors, or nil: it and everything below it are inert.
func outermostDisabled(target *Element) *Element {
	var d *Element
	for el := target; el != nil; el = el.styleParent() {
		if el.istate&style.StateDisabled != 0 {
			d = el
		}
	}
	return d
}

// commonAncestor returns the nearest common ancestor of a and b among
// the mounted primitives (an unmounted element counts as its nearest
// mounted ancestor).
func commonAncestor(a, b *Element) *Element {
	a, b = mountedAncestor(a), mountedAncestor(b)
	if a == nil || b == nil {
		return nil
	}
	for x := a; x != nil; x = x.styleParent() {
		for y := b; y != nil; y = y.styleParent() {
			if x == y {
				return x
			}
		}
	}
	return nil
}

func mountedAncestor(el *Element) *Element {
	for el != nil && !el.mounted {
		el = el.styleParent()
	}
	return el
}

// ---------------------------------------------------------------------
// Events and dispatch.

func modifiers(m platform.Modifiers) (shift, ctrl, alt, meta bool) {
	return m&platform.ModShift != 0, m&platform.ModCtrl != 0, m&platform.ModAlt != 0, m&platform.ModSuper != 0
}

// publicPointerID numbers pointers for PointerEvent.PointerID: unique
// across kinds, the mouse is 1.
func publicPointerID(k pointerKey) int { return int(k.id)*3 + int(k.kind) + 1 }

func (t *Tree) pointerEvent(ps *pointerState, button platform.PointerButton) PointerEvent {
	e := PointerEvent{
		X: ps.pos.X, Y: ps.pos.Y,
		Button: MouseButton(button), Buttons: MouseButtons(ps.buttons),
		Kind: PointerKind(ps.key.kind), PointerID: publicPointerID(ps.key),
		IsPrimary: ps.primary, rehover: t.in.rehover,
	}
	if ps.buttons != 0 {
		e.Pressure = 0.5
	}
	e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(ps.mods)
	return e
}

func (t *Tree) mouseEvent(ps *pointerState, button platform.PointerButton, count int) MouseEvent {
	e := MouseEvent{
		X: ps.pos.X, Y: ps.pos.Y,
		Button: MouseButton(button), Buttons: MouseButtons(ps.buttons),
		ClickCount: count,
	}
	e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(ps.mods)
	return e
}

// event is the set of handler event types.
type event interface {
	MouseEvent | PointerEvent | DragEvent | KeyboardEvent | FocusEvent | WheelEvent | ScrollEvent |
		InputEvent | ChangeEvent | CompositionEvent | SelectEvent | SubmitEvent
}

// eventPtr gives fire access to an event's dispatch fields: prepare sets
// the shared dispatch state and the fields that depend on the element
// whose handlers run (the local position of pointer events).
type eventPtr[E event] interface {
	*E
	prepare(t *Tree, st *eventState, cur *Element)
}

func (e *MouseEvent) prepare(t *Tree, st *eventState, cur *Element) {
	l := t.localPoint(cur, gfx.Point{X: e.X, Y: e.Y})
	e.st, e.LocalX, e.LocalY = st, l.X, l.Y
}

func (e *PointerEvent) prepare(t *Tree, st *eventState, cur *Element) {
	l := t.localPoint(cur, gfx.Point{X: e.X, Y: e.Y})
	e.st, e.LocalX, e.LocalY = st, l.X, l.Y
}

func (e *DragEvent) prepare(t *Tree, st *eventState, cur *Element) {
	l := t.localPoint(cur, gfx.Point{X: e.X, Y: e.Y})
	e.st, e.LocalX, e.LocalY = st, l.X, l.Y
}

func (e *WheelEvent) prepare(t *Tree, st *eventState, cur *Element) {
	l := t.localPoint(cur, gfx.Point{X: e.X, Y: e.Y})
	e.st, e.LocalX, e.LocalY = st, l.X, l.Y
}

func (e *ScrollEvent) prepare(_ *Tree, st *eventState, _ *Element)   { e.st = st }
func (e *KeyboardEvent) prepare(_ *Tree, st *eventState, _ *Element) { e.st = st }
func (e *FocusEvent) prepare(_ *Tree, st *eventState, _ *Element)    { e.st = st }
func (e *InputEvent) prepare(_ *Tree, st *eventState, _ *Element)    { e.st = st }
func (e *ChangeEvent) prepare(_ *Tree, st *eventState, _ *Element)   { e.st = st }
func (e *CompositionEvent) prepare(_ *Tree, st *eventState, _ *Element) {
	e.st = st
}
func (e *SelectEvent) prepare(_ *Tree, st *eventState, _ *Element) { e.st = st }
func (e *SubmitEvent) prepare(_ *Tree, st *eventState, _ *Element) { e.st = st }

// fire runs the handlers of kind k for an event at target: bubbling from
// target to the root until a handler stops it, or on target alone.
// Disabled elements and their descendants are skipped. It returns the
// dispatch state (whether a handler prevented the default).
func fire[E event, P eventPtr[E]](t *Tree, k eventKind, target *Element, e E, bubbles bool) *eventState {
	return dispatch[E, P](t, k, target, e, bubbles, true)
}

// dispatch is fire; skipInert false also runs the handlers of disabled
// elements (the blur of an element that became disabled).
func dispatch[E event, P eventPtr[E]](t *Tree, k eventKind, target *Element, e E, bubbles, skipInert bool) *eventState {
	return dispatchWith[E, P](t, k, target, e, bubbles, skipInert, &eventState{})
}

// dispatchWith is dispatch with the state st of a dispatch begun by the
// listeners of the capture phase (overlay.go). Every handler of an
// element runs, in the order its props were given: several props may
// handle the same event (package aria merges its props with the user's).
func dispatchWith[E event, P eventPtr[E]](t *Tree, k eventKind, target *Element, e E, bubbles, skipInert bool, st *eventState) *eventState {
	if target == nil {
		return st
	}
	t.handlerDepth++
	defer func() { t.handlerDepth-- }()
	var inert *Element
	if skipInert {
		inert = outermostDisabled(target)
	}
	for cur := target; cur != nil; cur = cur.styleParent() {
		if inert != nil {
			if cur == inert {
				inert = nil
			}
		} else if cur.mounted && cur.node.d.evMask&k.mask() != 0 {
			P(&e).prepare(t, st, cur)
			for i := range cur.node.d.events {
				if ep := &cur.node.d.events[i]; ep.kind == k {
					ep.Handler.(func(E))(e)
				}
			}
			if st.stopped {
				break
			}
		}
		if !bubbles {
			break
		}
	}
	return st
}

// ---------------------------------------------------------------------
// Cursor.

// cursorOf maps the style cursors to the platform's (by CSS name).
var cursorOf = func() []platform.Cursor {
	var m []platform.Cursor
	for i := 0; ; i++ {
		name := style.Cursor(i).String()
		if name == "?" {
			return m
		}
		c, ok := platform.CursorFromName(name)
		if !ok {
			c = platform.CursorDefault // auto
		}
		m = append(m, c)
	}
}()

// Cursor returns the cursor the window should show: the cursor style
// (inherited) of the element under the mouse or pen, default when there
// is none. "auto" shows the default arrow (text is not selectable yet).
func (t *Tree) Cursor() platform.Cursor {
	var ps *pointerState
	for k, p := range t.in.ptrs {
		if p.inside && k.kind != platform.PointerTouch && (ps == nil || k.kind == platform.PointerMouse) {
			ps = p
		}
	}
	if ps == nil {
		return platform.CursorDefault
	}
	el := mountedAncestor(ps.target)
	if el == nil || el.box.cs == nil {
		return platform.CursorDefault
	}
	if c := int(el.box.cs.Cursor); c < len(cursorOf) {
		return cursorOf[c]
	}
	return platform.CursorDefault
}
