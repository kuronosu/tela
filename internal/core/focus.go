package core

import (
	"math"

	"github.com/kuronosu/tela/internal/style"
)

// Focus (ARCHITECTURE section 14) follows the HTML focus model.
//
// # Focusable elements
//
// A Button, an Input and a Textarea are focusable, as are the primitives
// with a TabIndex; the others are not. An element is not focusable while
// it or an ancestor is disabled (Disabled) or display: none, while it is
// visibility: hidden or display: contents, inert (an exiting Presence
// child, Inert) or outside the active modal overlay (overlay.go), or
// before it has a computed style (it has no box yet). The sequential
// focus order (Tab) holds the focusable elements with a tab index of 0 or
// more (a Button's is 0): those with a positive index first, in ascending
// order, then those with 0, in tree order (HTML's rules); -1 makes an
// element focusable by click and by ElementRef.Focus only.
//
// # Moving the focus
//
// The focus moves by keyboard (Tab and Shift+Tab, keyboard.go), by mouse
// (a press of the primary button focuses the nearest focusable element at
// or above the target; a press elsewhere blurs the focused element, unless
// a containing focus scope holds it (Containment, below), and
// PreventDefault on OnPointerDown or OnMouseDown keeps the focus where it
// is), by script (ElementRef.Focus and Blur, AutoFocus, the restoration
// of a focus scope) and by fix-up: a focused element that stops being
// focusable (disabled, hidden) is blurred, with OnBlur, as Chromium does.
// Unmounting the focused element leaves nothing focused, without OnBlur
// (React), except inside a containing focus scope (Containment, below).
// Tab with nothing focused starts from the point the last press
// blurred the focus at, or from the element blurred last (HTML's
// sequential focus navigation starting point).
//
// A change fires OnBlur on the element losing the focus, then OnFocus on
// the element gaining it; both bubble (React's onFocus and onBlur, the
// DOM's focusout and focusin). A handler of OnBlur that moves the focus
// elsewhere cancels the rest of the change. After each event the queued
// updates are applied and components rebuilt, as after other discrete
// events (outside a frame).
//
// # Window focus
//
// While the window does not have keyboard focus, the focused element stays
// the tree's (ElementRef.Focused reports it) but does not match focus:,
// focus-visible: or focus-within:. Losing the window focus fires OnBlur on
// it and gaining it fires OnFocus again, as in browsers. A Host starts
// with an unfocused window (the backend's FocusEvent turns it on); a Tree
// alone starts with a focused one.
//
// # Variants
//
// The focused element matches focus:, it and its ancestors focus-within:,
// and it matches focus-visible: following Chromium's heuristic: unless the
// last focus that was not programmatic came from a pointer press, or a
// key that is not a modifier was pressed (without Ctrl, Alt or Meta) since
// the last pointer press. So Tab shows it, a click does not, a key typed
// after the click does, and a programmatic focus shows it when the last
// interaction was the keyboard (or there was none). group-focus:,
// peer-focus: and their -visible and -within forms follow (input.go's
// interaction states). A change restyles only the elements whose classes
// test the states that changed.
//
// # AutoFocus
//
// The first element mounted with AutoFocus(true) in a commit takes the
// focus after the style phase of the frame that shows it (before layout
// effects, like React's autoFocus at commit), unless another element
// gained the focus between its mount and then. A focus change by a
// Frame (fix-up, AutoFocus, scope restoration) fires its handlers inside
// the frame; their state updates apply in the next one.
//
// # Focus scopes
//
// A focus scope (Scope and FocusScope, reserved for package aria's
// UseFocusScope) on a primitive contains the focus inside its subtree
// while it is mounted (the most recently mounted containing scope that is
// not inert nor outside the active modal is the active one), restores the
// focus to the element focused when it mounted when it unmounts, if the
// focus was inside it (or nowhere) then, and with AutoFocus focuses the
// first tabbable element of its subtree (else the first focusable one)
// after the style phase of the frame that mounts it, unless the focus is
// already inside (React Aria's FocusScope). The focus manager of
// focusmgr.go moves the focus within a subtree for package aria's hooks.
//
// # Containment
//
// Inside the active containing scope means inside its subtree in the
// element tree, so the content of a Portal rendered from inside it (a
// popover or a select opened from a dialog) and a nested scope count. Tab
// and Shift+Tab wrap among its elements (moveFocus). Every other focus
// change is checked too (React Aria's useFocusContainment, ADR-160):
//
//   - A primary press on nothing focusable (the scrim of an alert dialog,
//     which hits the root, or plain content) leaves the focus where it is
//     while it is inside the scope, as if the pointerdown had been
//     prevented (Radix's AlertDialog prevents it): no OnBlur, and the
//     focus-visible state stays the one the heuristic gives a prevented
//     press (Chromium keeps the element's state when the focus does not
//     change).
//   - A focus change to an element outside the scope, by a press on a
//     focusable element that is still hit (an overlay alive above a modal,
//     or any element around a scope that is not modal), a Label, or a
//     script (ElementRef.Focus, the focus manager, AutoFocus, the
//     restoration of another scope), does not happen: the focus stays
//     where it is when it is inside the scope, and otherwise goes to the
//     element focused last inside it while it can take the focus, else to
//     the first tabbable element of the scope, else to the first
//     focusable one. React Aria lets the focus reach the outside element
//     and moves it back in its focusin listener; Tela checks before the
//     change, so the handlers never see the focus go out and come back.
//   - ElementRef.Blur on an element inside the scope does nothing (React
//     Aria's blur listener focuses it again).
//
// The window losing the focus is not a change (the element stays
// focused), and the fix-up blur of a focused element that stops being
// focusable still leaves nothing focused: the next Tab starts inside the
// scope again (Radix leaves a disabled element's blur to the browser too).
//
// When the focused element unmounts (or starts an exit, presence.go)
// while it is inside the active containing scope, the focus goes to the
// scope's root element when it can take the focus, as Radix's
// FocusScope (trapped) does with its container (aria.UseDialog gives the
// dialog element TabIndex(-1)), else to the first tabbable element of the
// scope, else to the first focusable one (React Aria's contain, which
// focuses the first tabbable element), else nowhere. It happens once
// nothing else of the commit can focus (lostWork; Radix moves the focus
// from a MutationObserver, after React's layout effects): after AutoFocus
// and the restoration of the scopes that ended, so a scope that ends in
// the same commit (a dialog that closes) restores the focus as before and
// an AutoFocus element mounted by the commit wins, and after the layout
// effects of the commit, so a layout effect that focuses (the next step
// of a wizard focusing its input) makes the only focus change. That is at
// once after a discrete event whose commit has no layout effect pending,
// else in the frame, after its layout effects and before paint (the
// states it changes are styled in that frame); a key pressed before that
// frame resolves it first, so Escape and shortcuts reach the scope.
// Passive effects run after paint and are not waited for. Any focus
// change meanwhile cancels it, so a later blur (the fix-up of an element
// focused since) leaves nothing focused; a press on nothing focusable
// meanwhile does not, since the scope holds the focus against it as it
// does once the focus is back (Containment, above). It is a script
// focus: no OnBlur (the element is gone), OnFocus on the element, and
// focus-visible from the last interaction, so after a pointer press
// nothing shows a ring. Neither the root nor the fallback element is
// scrolled into view (Radix focuses with preventScroll). A scope without
// Contain keeps the browser's outcome: nothing focused.

// focusType is how the focus moved.
type focusType uint8

const (
	focusNone     focusType = iota // no focus change yet
	focusKeyboard                  // sequential navigation (Tab)
	focusMouse                     // a pointer press
	focusScript                    // ElementRef.Focus, AutoFocus, scopes, fix-up
	// focusQuiet is a script focus that does not scroll the element into
	// view: a containing scope taking back the focus of an element that
	// unmounted (focusLost), for its root or the fallback element.
	focusQuiet
)

// focusState is the focus of a Tree (UI thread).
type focusState struct {
	// el is the focused element, nil for none.
	el *Element
	// window reports that the window has keyboard focus: el matches the
	// focus variants and receives focus events.
	window bool
	// visible reports that the focused element matches focus-visible:
	// (lastType != focusMouse || hadKey).
	visible  bool
	lastType focusType
	hadKey   bool
	// gen counts the elements that gained the focus (AutoFocus checks
	// that none did since its mount).
	gen uint64
	// shown is the element whose focus states are set, shownVis whether
	// focus-visible is among them.
	shown    *Element
	shownVis bool
	// start is the sequential navigation starting point while nothing is
	// focused.
	start *Element
	// auto is the AutoFocus element of the pending commit, mounted when gen
	// was autoGen.
	auto    *Element
	autoGen uint64
	// scopes are the mounted focus scopes, in mount order; restores the
	// elements to restore the focus to after unmounted scopes; scopeAuto
	// the scopes mounted with AutoFocus waiting for their first style.
	scopes    []*focusScope
	restores  []*Element
	scopeAuto []*focusScope
	// lost is the containing scope that held the focused element when it
	// unmounted, for lostWork to give the focus back inside (focusLost);
	// any focus change clears it.
	lost *focusScope
	// space is the Button whose Space press waits for its release.
	space *Element
	// tab is the scratch list of tab order walks.
	tab []tabEntry
}

// Focused returns the focused element, or nil.
func (t *Tree) Focused() *Element { return t.foc.el }

// WindowFocused reports whether the tree's window has keyboard focus.
func (t *Tree) WindowFocused() bool { return t.foc.window }

// SetWindowFocused records whether the window has keyboard focus (a
// platform FocusEvent): losing it fires OnBlur on the focused element and
// removes its focus states, gaining it gives them back with OnFocus.
func (t *Tree) SetWindowFocused(focused bool) {
	f := &t.foc
	if f.window == focused {
		return
	}
	if focused {
		// An element that stopped being focusable while the window was
		// inactive is blurred silently: it never received OnFocus.
		t.fixFocus()
	}
	f.window = focused
	if fld := t.focusedField(); fld != nil {
		// The caret shows and the selection's colors follow the window.
		if focused {
			t.caretReset(fld)
		} else {
			fld.markPaint()
		}
	}
	if !focused {
		t.cancelSpace()
		t.syncFocus()
		if el := f.el; el != nil && el.mounted {
			dispatch(t, evBlur, el, FocusEvent{}, true, false)
			t.afterFocusEvent()
		}
		return
	}
	t.syncFocus()
	if el := f.el; el != nil && el.mounted {
		dispatch(t, evFocus, el, FocusEvent{}, true, false)
		t.afterFocusEvent()
	}
}

// afterFocusEvent applies the updates of focus handlers and builds, as
// after other discrete events; inside a frame the frame goes on with the
// focus states and the updates wait for the next one. A focus change made
// by code that runs as a handler (ElementRef.Focus, the focus manager or a
// focus scope from a handler, an overlay callback or a passive effect)
// leaves them to the event, the frame or the host's next step, as
// flushNested does: the code after the call runs before the build.
func (t *Tree) afterFocusEvent() { t.flushNested() }

// ---------------------------------------------------------------------
// Focusability.

// focusTarget reports whether el takes the focus at all: a Button or an
// element with a TabIndex.
func focusTarget(el *Element) bool {
	return el.kind == elemPrimitive && (el.node.d.hasTab || nativeFocusable(el.box.kind))
}

// nativeFocusable reports whether elements of kind k take the focus
// without a TabIndex: Buttons and text fields.
func nativeFocusable(k style.Kind) bool { return k == style.KindButton || k.IsTextField() }

// tabRank returns the position group of el in the sequential focus order
// (a positive tab index, or MaxInt32 for 0) and whether it is in it.
func tabRank(el *Element) (int32, bool) {
	d := el.node.d
	ti := int32(0)
	if d.hasTab {
		ti = d.tabIndex
	} else if !nativeFocusable(el.box.kind) {
		return 0, false
	}
	switch {
	case ti < 0:
		return 0, false
	case ti == 0:
		return math.MaxInt32, true
	}
	return ti, true
}

// focusable reports whether el can take the focus now (see the comment at
// the top of focus.go).
func (t *Tree) focusable(el *Element) bool {
	if el == nil || !el.mounted || el.parent == nil || !focusTarget(el) {
		return false
	}
	cs := el.box.cs
	if cs == nil || cs.Display == style.DisplayContents || cs.Visibility != style.VisibilityVisible {
		return false
	}
	if el.inert || el.outside != outsideNo {
		// In an exiting Presence child (presence.go), under Inert, or
		// outside the active modal overlay (overlay.go).
		return false
	}
	for a := el; a != nil; a = a.styleParent() {
		acs := a.box.cs
		if acs == nil || acs.Display == style.DisplayNone || a.istate&style.StateDisabled != 0 {
			return false
		}
	}
	return true
}

// fixFocus blurs the focused element when it is no longer focusable
// (disabled, hidden), firing OnBlur as Chromium does.
// A focus given back by tela dev before its first style phase waits for
// the frame's fix-up (devFocusWaits; release builds have no such focus).
func (t *Tree) fixFocus() {
	if el := t.foc.el; el != nil && !devFocusWaits(el) && !t.focusable(el) {
		t.setFocus(nil, focusScript)
	}
}

// ---------------------------------------------------------------------
// Focus changes.

// setFocus moves the focus to el (nil: blur), as typ says: the steps at
// the top of focus.go.
func (t *Tree) setFocus(el *Element, typ focusType) {
	f := &t.foc
	if el == f.el {
		return
	}
	// A focus change, or a blur, after the unmount of the focused element
	// replaces the containing scope's recovery (lostWork).
	f.lost = nil
	if typ != focusScript && typ != focusQuiet {
		f.lastType = typ
	}
	f.visible = f.visibleFor(el)
	t.cancelSpace()
	old := f.el
	g := f.gen
	if old != nil {
		// The states follow at the end (syncFocus), so the ancestors that
		// keep focus-within: are not restyled.
		f.el = nil
		f.start = old
		if fld := textField(old); fld != nil && old.mounted {
			t.fieldBlurred(fld)
			if f.gen != g || f.el != nil {
				return // a handler moved the focus
			}
		}
		if f.window && old.mounted {
			dispatch(t, evBlur, old, FocusEvent{}, true, false)
			t.afterFocusEvent()
			if f.gen != g || f.el != nil {
				return // a handler moved the focus
			}
		}
	}
	if el == nil || !el.mounted {
		t.syncFocus()
		return
	}
	f.gen++
	f.el, f.start = el, nil
	for _, s := range f.scopes {
		if s.opts.contain && isInside(el, s.el) {
			s.last = el
		}
	}
	t.syncFocus()
	t.focusMoved(old, el)
	t.revealFocused(el, typ)
	if fld := textField(el); fld != nil {
		t.fieldFocused(fld, typ)
	}
	if f.window {
		dispatch(t, evFocus, el, FocusEvent{}, true, false)
		t.afterFocusEvent()
	}
}

// revealFocused scrolls the focused element into view after the next
// layout (HTML's focusing steps), in every scroll container around it,
// except for pointer focus, whose target is already under the pointer.
func (t *Tree) revealFocused(el *Element, typ focusType) {
	if typ == focusMouse || typ == focusQuiet {
		return
	}
	t.scr.reveal = el
	t.requestRepaint()
}

// updateVisible recomputes focus-visible: after a key or a pointer press.
func (t *Tree) updateVisible() {
	f := &t.foc
	if v := f.visibleFor(f.el); v != f.visible {
		f.visible = v
		t.syncFocus()
	}
}

// visibleFor reports whether focus on el matches focus-visible: unless a
// pointer press focused it and no key came since. Text fields match it
// however they were focused (Chromium).
func (f *focusState) visibleFor(el *Element) bool {
	return f.lastType != focusMouse || f.hadKey || isTextFieldEl(el)
}

// noteMouse records a pointer press for the focus-visible heuristic.
func (t *Tree) noteMouse() {
	if t.foc.hadKey {
		t.foc.hadKey = false
		t.updateVisible()
	}
}

// syncFocus makes the focus states (focus:, focus-visible:, focus-within:)
// match the focused element, changing only the elements whose states
// differ.
func (t *Tree) syncFocus() {
	f := &t.foc
	var want *Element
	if f.window && f.el != nil && f.el.mounted {
		want = f.el
	}
	vis := want != nil && f.visible
	if want == f.shown {
		if want != nil && vis != f.shownVis {
			t.setState(want, style.StateFocusVisible, vis)
		}
		f.shownVis = vis
		return
	}
	ca := commonAncestor(f.shown, want)
	if f.shown != nil {
		t.focusStates(f.shown, false, false, ca)
	}
	if want != nil {
		t.focusStates(want, true, vis, ca)
	}
	f.shown, f.shownVis = want, vis
}

// focusStates turns the focus states of el on or off: focus: and
// focus-visible: (vis) on el, focus-within: on el and its ancestors below
// stop.
func (t *Tree) focusStates(el *Element, on, vis bool, stop *Element) {
	t.setState(el, style.StateFocus, on)
	t.setState(el, style.StateFocusVisible, on && vis)
	for a := el; a != nil && a != stop; a = a.styleParent() {
		t.setState(a, style.StateFocusWithin, on)
	}
}

// mouseFocus is the focus change of a primary press on target: the
// nearest focusable element at or above it takes the focus; with none,
// the focused element is blurred and the press becomes the starting point
// of sequential navigation.
func (t *Tree) mouseFocus(target *Element) {
	el := mountedAncestor(target)
	if el == nil {
		return
	}
	for x := el; x != nil; x = x.styleParent() {
		if t.focusable(x) {
			if s := t.trapping(x); s != nil {
				t.keepInScope(s)
				return
			}
			t.setFocus(x, focusMouse)
			return
		}
	}
	if t.holdsFocus() || t.lostPending() {
		// Nothing focusable under the press, and the active containing
		// scope holds the focus, or is about to take it back (the focused
		// element just unmounted, lostWork): it stays, as after a
		// prevented pointerdown (no lastType change, so focus-visible
		// keeps what noteMouse left), and the recovery goes on, as when
		// it ran before the press.
		return
	}
	t.setFocus(nil, focusMouse)
	t.foc.lastType = focusMouse
	t.updateVisible()
	if t.foc.el == nil {
		t.foc.start = el
	}
}

// focusElement focuses el programmatically (ElementRef.Focus): nothing
// happens when el is not focusable.
func (t *Tree) focusElement(el *Element) {
	if !t.focusable(el) {
		return
	}
	if s := t.trapping(el); s != nil {
		t.keepInScope(s)
		return
	}
	t.setFocus(el, focusScript)
}

// ---------------------------------------------------------------------
// Sequential navigation.

// tabEntry is a focusable element of the sequential order: its rank (tab
// index group) and its position in tree order.
type tabEntry struct {
	el        *Element
	rank, idx int32
}

func (a tabEntry) less(b tabEntry) bool {
	return a.rank < b.rank || a.rank == b.rank && a.idx < b.idx
}

// tabWalker collects the sequential order of a subtree.
type tabWalker struct {
	out      []tabEntry
	n        int32
	start    *Element
	startIdx int32
	found    bool
	// top is the element being walked from (walkAll).
	top *Element
}

// walkAll walks root, then the portals inside it in mount order: their
// content follows the root's, like content appended to the body.
func (w *tabWalker) walkAll(t *Tree, root *Element) {
	w.top = root
	w.walk(root, false, false)
	for _, p := range t.portalsIn(root, nil) {
		w.top = p
		w.walk(p, false, false)
	}
}

func (w *tabWalker) walk(el *Element, hidden, inert bool) {
	switch el.kind {
	case elemText:
		return
	case elemComponent:
		for _, c := range el.children {
			w.walk(c, hidden, inert)
		}
		return
	}
	if !el.mounted || el != w.top && isPortal(el) {
		return // a portal's content comes after the root's (walkAll)
	}
	idx := w.n
	w.n++
	if el == w.start {
		w.startIdx, w.found = idx, true
	}
	cs := el.box.cs
	if cs == nil || cs.Display == style.DisplayNone {
		hidden = true
	}
	if el.istate&style.StateDisabled != 0 || el.inert || el.outside == outsideYes {
		inert = true
	}
	if !hidden && !inert && el.outside == outsideNo && el.parent != nil && cs.Display != style.DisplayContents &&
		cs.Visibility == style.VisibilityVisible {
		if r, ok := tabRank(el); ok {
			w.out = append(w.out, tabEntry{el, r, idx})
		}
	}
	if hidden && (w.start == nil || w.found) {
		return // nothing in it is focusable, and the start is elsewhere
	}
	for _, c := range el.children {
		w.walk(c, hidden, inert)
	}
}

// moveFocus moves the focus to the next element of the sequential order
// (the previous one when backward), wrapping around, inside the active
// containing focus scope if any.
func (t *Tree) moveFocus(backward bool) {
	f := &t.foc
	root := t.root
	if s := t.activeScope(); s != nil {
		root = s.el
	}
	start := f.el
	if start == nil {
		start = f.start
	}
	if start != nil && !start.mounted {
		start = nil
	}
	w := tabWalker{out: f.tab[:0], start: start}
	w.walkAll(t, root)
	f.tab = w.out
	defer func() {
		clear(f.tab)
		f.tab = f.tab[:0]
	}()
	if len(w.out) == 0 {
		return
	}
	var next *Element
	if w.found {
		// The starting point's key: its own rank when it is in the order,
		// else as a tab index of 0 (HTML).
		key := tabEntry{rank: math.MaxInt32, idx: w.startIdx}
		if r, ok := tabRank(start); ok {
			key.rank = r
		}
		var best *tabEntry
		for i := range w.out {
			e := &w.out[i]
			if backward {
				if e.less(key) && (best == nil || best.less(*e)) {
					best = e
				}
			} else if key.less(*e) && (best == nil || e.less(*best)) {
				best = e
			}
		}
		if best != nil {
			next = best.el
		}
	}
	if next == nil { // wrap around, or no starting point
		best := w.out[0]
		for _, e := range w.out[1:] {
			if backward == best.less(e) {
				best = e
			}
		}
		next = best.el
	}
	t.setFocus(next, focusKeyboard)
}

// ---------------------------------------------------------------------
// Focus work of a commit: fix-up, AutoFocus, scope restoration, and the
// recovery of a lost focus (lostWork).

// focusWork does the focus work queued by the builds: it blurs a focused
// element that stopped being focusable, focuses the AutoFocus element of
// the commit once it has a style, and restores the focus of unmounted
// scopes. The recovery of a focus lost inside a containing scope waits
// for the layout effects (lostWork).
func (t *Tree) focusWork() {
	f := &t.foc
	t.fixFocus()
	if a := f.auto; a != nil {
		switch {
		case !a.mounted:
			f.auto = nil
		case a.box.cs == nil:
			// Not styled yet (mounted by a discrete event's build): wait
			// for the frame.
		default:
			f.auto = nil
			if f.gen == f.autoGen {
				t.focusElement(a)
			}
		}
	}
	if len(f.scopeAuto) > 0 {
		t.scopeAutoFocus()
	}
	for len(f.restores) > 0 {
		el := f.restores[0]
		f.restores = f.restores[1:]
		if f.el == nil {
			t.focusElement(el)
		}
	}
	f.restores = nil
}

// lostWork gives the focus back inside the containing scope whose focused
// element unmounted (focusLost), when one did. Its callers run it once
// nothing else of the commit can focus: after focusWork and the layout
// effects (Frame, virtualRelayout), after a discrete event's build when
// no layout effect is pending (flushDiscrete), and before a keydown is
// dispatched (HandleKey), so that the key reaches the scope.
func (t *Tree) lostWork() {
	if t.foc.lost != nil {
		t.focusLost()
	}
}

// noteLost records that el, the focused element, loses the focus by an
// unmount (or the start of an exit) while it is inside the active
// containing scope (scopes ending with it are already gone), so that
// lostWork gives the focus back inside that scope.
func (t *Tree) noteLost(el *Element) {
	if s := t.activeScope(); s != nil && isInside(el, s.el) {
		t.foc.lost = s
	}
}

// focusLost gives the focus back inside the scope that held the focused
// element when it unmounted, unless something has the focus by now (an
// AutoFocus, a restoration, a layout effect) or that scope has ended: to
// the active containing scope's root element when it is focusable
// (Radix's container), else its first tabbable element, else its first
// focusable one (see Containment at the top of focus.go), without
// scrolling either into view. It waits while an AutoFocus of the commit
// or the scope's root has no style yet (mounted by a discrete event's
// build), so the frame decides.
// lostPending reports whether a recovery of the focus (focusLost) is
// pending for a scope that can still take it: mounted, still its element's
// scope, and inside the active containing scope.
func (t *Tree) lostPending() bool {
	s := t.foc.lost
	if s == nil || t.foc.el != nil || !s.el.mounted || s.el.fscope != s {
		return false
	}
	a := t.activeScope()
	return a != nil && isInside(a.el, s.el)
}

func (t *Tree) focusLost() {
	f := &t.foc
	s := f.lost
	if f.el != nil || !s.el.mounted || s.el.fscope != s {
		f.lost = nil
		return
	}
	a := t.activeScope()
	if a == nil || !isInside(a.el, s.el) {
		f.lost = nil // a scope outside s holds the focus now, or none does
		return
	}
	if f.auto != nil || len(f.scopeAuto) > 0 || a.el.box.cs == nil {
		return // not styled yet: the frame's focus work decides
	}
	f.lost = nil
	if t.focusable(a.el) {
		t.setFocus(a.el, focusQuiet)
		return
	}
	// The first tabbable element, else the first focusable one, not
	// scrolled into view either (focusFirstIn would reveal it): a scrolled
	// list whose focused row is deleted stays where it is. They are inside
	// a, the active scope, so containment has nothing to check.
	cands, _, _ := t.focusCands(a.el, true, nil)
	if len(cands) == 0 {
		cands, _, _ = t.focusCands(a.el, false, nil)
	}
	if len(cands) > 0 {
		t.setFocus(cands[0].el, focusQuiet)
	}
}

// scopeAutoFocus focuses the first tabbable element (else the first
// focusable one) of each focus scope mounted with AutoFocus once it has a
// style, unless the focus is already inside it (React Aria's FocusScope
// autoFocus).
func (t *Tree) scopeAutoFocus() {
	f := &t.foc
	q := f.scopeAuto
	f.scopeAuto = nil
	var wait []*focusScope
	for _, s := range q {
		switch {
		case !s.el.mounted || s.el.fscope != s:
		case s.el.box.cs == nil:
			wait = append(wait, s) // mounted by a discrete event's build
		case t.foc.el == nil || !isInside(t.foc.el, s.el):
			if !t.focusFirstIn(s.el, true) {
				t.focusFirstIn(s.el, false)
			}
		}
	}
	f.scopeAuto = append(f.scopeAuto, wait...)
}

// focusPending reports whether focusWork has anything to do.
func (t *Tree) focusPending() bool {
	return t.foc.el != nil || t.foc.auto != nil || len(t.foc.restores) > 0 || len(t.foc.scopeAuto) > 0
}

// mountFocus records the focus props of a primitive being mounted.
func (t *Tree) mountFocus(el *Element) {
	d := el.node.d
	if d.ref != nil {
		d.ref.el = el
		if len(d.ref.follow) > 0 {
			t.queueRefSync(d.ref)
		}
	}
	if d.autoFocus && t.foc.auto == nil {
		t.foc.auto, t.foc.autoGen = el, t.foc.gen
	}
	if d.scope != nil {
		t.addScope(el, *d.scope, true)
	}
}

// updateFocus records the focus props of a primitive updated from old.
func (t *Tree) updateFocus(el *Element, old *desc) {
	d := el.node.d
	if old.ref != d.ref {
		if old.ref != nil && old.ref.el == el {
			old.ref.el = nil
		}
		if d.ref != nil {
			d.ref.el = el
			if len(d.ref.follow) > 0 {
				t.queueRefSync(d.ref)
			}
		}
	}
	switch {
	case d.scope != nil && el.fscope == nil:
		t.addScope(el, *d.scope, true)
	case d.scope == nil && el.fscope != nil:
		t.removeScope(el.fscope)
		el.fscope = nil
	case d.scope != nil:
		el.fscope.opts = *d.scope
	}
}

// unmountFocus handles the unmount of primitive el: its ElementRef is
// detached, its focus scope ends (queueing the restoration), and when it
// is focused nothing is (no OnBlur; React), until the containing scope
// around it, if any, takes the focus back (noteLost).
func (t *Tree) unmountFocus(el *Element) {
	f := &t.foc
	if r := el.node.d.ref; r != nil && r.el == el {
		r.el = nil
	}
	if s := el.fscope; s != nil {
		t.removeScope(s)
		el.fscope = nil
		if s.opts.restore && s.prev != nil && (f.el == nil || isInside(f.el, el)) {
			f.restores = append(f.restores, s.prev)
		}
	}
	if f.space == el {
		f.space = nil
	}
	if f.auto == el {
		f.auto = nil
	}
	if f.start == el {
		f.start = nil
	}
	if f.el == el {
		t.noteLost(el)
		f.el = nil
		t.syncFocus()
	}
}

// resetFocus forgets the focus (the tree's content is gone).
func (t *Tree) resetFocus() {
	f := &t.foc
	t.cancelSpace()
	f.el = nil
	t.syncFocus()
	f.start, f.auto, f.restores, f.scopes, f.scopeAuto, f.lost = nil, nil, nil, nil, nil, nil
}

// isInside reports whether el is anc or one of its descendants.
func isInside(el, anc *Element) bool {
	for x := el; x != nil; x = x.parent {
		if x == anc {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Focus scopes (reserved for package aria).

// scopeOpts are the options of a focus scope.
type scopeOpts struct {
	contain, restore, autoFocus bool
}

// focusScope is a mounted focus scope.
type focusScope struct {
	el   *Element
	opts scopeOpts
	// prev was focused when the scope mounted.
	prev *Element
	// last is the element focused last inside a containing scope (where
	// keepInScope brings the focus back), or nil.
	last *Element
}

// FocusScope returns a prop that makes a primitive a focus scope while it
// is mounted: with contain, Tab and Shift+Tab move the focus only among
// the focusable elements of its subtree (wrapping around inside it; the
// most recently mounted containing scope wins), no press or script
// takes the focus out of it, and when the focused element inside it
// unmounts the focus goes to the scope's element if it is focusable, else
// to its first tabbable element (Containment at the top of focus.go); with
// restore, when it unmounts while the focus is inside it (or nowhere), the
// focus returns to the element focused when it mounted, if that element
// is still focusable.
//
// Reserved for package aria (UseFocusScope); it is not exposed by tela
// or h. It is Scope without AutoFocus.
func FocusScope(contain, restore bool) Prop {
	return Scope(ScopeOptions{Contain: contain, Restore: restore})
}

// ScopeOptions configures Scope: React Aria's FocusScope props.
type ScopeOptions struct {
	// Contain keeps the focus inside the scope's subtree: Tab and
	// Shift+Tab wrap inside it, presses and scripts do not take the focus
	// out, and the unmount of the focused element gives the focus to the
	// scope's element when it is focusable, else to its first tabbable
	// element (Containment at the top of focus.go).
	Contain bool
	// Restore gives the focus back, when the scope unmounts, to the
	// element focused when it mounted.
	Restore bool
	// AutoFocus focuses the first tabbable element of the subtree (the
	// first focusable one when none is tabbable) after the style phase of
	// the frame that mounts the scope, unless the focus is already inside
	// it.
	AutoFocus bool
}

// Scope returns a prop that makes a primitive a focus scope while it is
// mounted (see FocusScope); with AutoFocus the first tabbable element of
// its subtree takes the focus when it mounts. Reserved for package aria
// (UseFocusScope).
func Scope(o ScopeOptions) Prop {
	so := &scopeOpts{contain: o.Contain, restore: o.Restore, autoFocus: o.AutoFocus}
	return Prop{name: "FocusScope", apply: func(d *desc) { d.scope = so }}
}

// addScope makes el a focus scope; auto queues its AutoFocus (a scope
// given back by a cancelled exit does not focus again).
func (t *Tree) addScope(el *Element, o scopeOpts, auto bool) {
	s := &focusScope{el: el, opts: o, prev: t.foc.el}
	el.fscope = s
	t.foc.scopes = append(t.foc.scopes, s)
	if auto && o.autoFocus {
		t.foc.scopeAuto = append(t.foc.scopeAuto, s)
	}
}

func (t *Tree) removeScope(s *focusScope) {
	ss := t.foc.scopes
	for i, x := range ss {
		if x == s {
			copy(ss[i:], ss[i+1:])
			ss[len(ss)-1] = nil
			t.foc.scopes = ss[:len(ss)-1]
			return
		}
	}
}

// activeScope returns the most recently mounted containing scope, or nil.
func (t *Tree) activeScope() *focusScope {
	ss := t.foc.scopes
	for i := len(ss) - 1; i >= 0; i-- {
		if s := ss[i]; s.opts.contain && s.el.mounted && !s.el.inert && s.el.outside != outsideYes {
			return s
		}
	}
	return nil
}

// trapping returns the active containing scope when focusing el would take
// the focus out of it, or nil when the change is allowed (no active
// containing scope, or el inside it).
func (t *Tree) trapping(el *Element) *focusScope {
	s := t.activeScope()
	if s == nil || isInside(el, s.el) {
		return nil
	}
	return s
}

// holdsFocus reports whether the focused element is inside the active
// containing scope, which then keeps it from going nowhere.
func (t *Tree) holdsFocus() bool {
	el := t.foc.el
	if el == nil || !el.mounted {
		return false
	}
	s := t.activeScope()
	return s != nil && isInside(el, s.el)
}

// keepInScope replaces a focus change that would take the focus out of the
// active containing scope s (see Containment at the top of focus.go): the
// focus stays where it is when it is inside s, otherwise it goes to the
// element focused last inside s while it is focusable, else to the first
// tabbable element of s, else to the first focusable one. At most one
// focus change happens, to an element inside s, so it cannot come back
// here.
func (t *Tree) keepInScope(s *focusScope) {
	f := &t.foc
	if f.el != nil && f.el.mounted && isInside(f.el, s.el) {
		return
	}
	if l := s.last; l != nil && isInside(l, s.el) && t.focusable(l) {
		t.setFocus(l, focusScript)
		return
	}
	if !t.focusFirstIn(s.el, true) {
		t.focusFirstIn(s.el, false)
	}
}

// ---------------------------------------------------------------------
// Props.

// TabIndex makes a primitive focusable and sets its place in the
// sequential focus order (see the comment at the top of focus.go): 0 in
// tree order, -1 out of it (focusable by click and ElementRef.Focus
// only), a positive index before every 0, in ascending order.
func TabIndex(i int) Prop {
	v := int32(max(min(i, math.MaxInt32-1), math.MinInt32))
	return Prop{name: "TabIndex", apply: func(d *desc) { d.tabIndex, d.hasTab = v, true }}
}

// AutoFocus focuses the element after the commit that mounts it, when on
// (see the comment at the top of focus.go).
func AutoFocus(on bool) Prop {
	return Prop{name: "AutoFocus", apply: func(d *desc) { d.autoFocus = on }}
}

// ---------------------------------------------------------------------
// Element references.

// elemRef is the slot of one UseElementRef call: the primitive the ref is
// attached to while it is mounted.
type elemRef struct {
	el *Element
	// follow are the UseScroll slots that follow el's scroll offsets.
	follow []*scrollFollow
}

// ElementRef is a handle on the primitive it is attached to (with
// AttachRef; h.Ref in the public API), returned by UseElementRef: the same
// comparable value on every build of the hook slot. Its methods act on
// the element while it is mounted and do nothing (or report false)
// otherwise, or for the zero ElementRef. Call them on the UI thread: in
// event handlers and effects.
type ElementRef struct {
	r *elemRef
}

// element returns the mounted element of r, or nil.
func (r ElementRef) element() *Element {
	if r.r == nil || r.r.el == nil || !r.r.el.mounted {
		return nil
	}
	return r.r.el
}

// Focus focuses the element, if it is focusable (HTML's focus()): OnBlur
// runs on the element that loses the focus and OnFocus on this one,
// before it returns. Called from a handler or an effect, the updates those
// handlers make are applied and built by the event or the frame that is
// running (after the passive effects, by the host's next step), as for
// Click.
func (r ElementRef) Focus() {
	if el := r.element(); el != nil {
		el.tree.focusElement(el)
	}
}

// Blur removes the focus from the element if it has it (HTML's blur()):
// nothing is focused afterwards. Inside an active containing focus scope
// it does nothing: the scope keeps the focus (React Aria's FocusScope
// contain focuses the element again).
func (r ElementRef) Blur() {
	if el := r.element(); el != nil && el.tree.foc.el == el && !el.tree.holdsFocus() {
		el.tree.setFocus(nil, focusScript)
	}
}

// Focused reports whether the element is the focused element of its
// window (also while the window itself does not have keyboard focus).
func (r ElementRef) Focused() bool {
	el := r.element()
	return el != nil && el.tree.foc.el == el
}

// UseElementRef returns the ElementRef of this hook slot, the same on
// every build. Attach it to a primitive with AttachRef.
func UseElementRef() ElementRef {
	el, i := nextSlot(hookElementRef)
	if r, ok := existingSlot[*elemRef](el, i, "UseElementRef"); ok {
		return ElementRef{r}
	}
	r := &elemRef{}
	el.hooks = append(el.hooks, r)
	return ElementRef{r}
}

// NewElementRef returns an ElementRef that belongs to no hook slot: the
// zero state of what UseElementRef returns, for code that needs one ref
// per item of a collection whose size changes between builds (package
// aria keeps one per key of a list box, menu or tab list), where a hook
// per item is not possible. Attach it with AttachRef like any other; it
// follows the element while it is mounted and keeps working after the
// element unmounts and mounts again.
func NewElementRef() ElementRef { return ElementRef{&elemRef{}} }

// AttachRef attaches r to the primitive (h.Ref): while it is mounted, r's
// methods act on it. A ref attached to several elements follows the last
// one mounted or updated with it. It panics on the zero ElementRef.
//
// An element holds one ref. Two different refs on one element panic in
// dev mode (naming both call sites and the fix: the hooks of package aria
// take the shared ref as their Ref option); outside dev mode the last one
// wins. The same ref twice is fine.
func AttachRef(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: Ref(zero ElementRef): pass the ElementRef returned by tela.UseElementRef")
	}
	var site *callStack
	if devNow() {
		site = new(callStack)
		site.capture(0)
	}
	return Prop{name: "Ref", apply: func(d *desc) {
		if site != nil && d.ref != nil && d.ref != r.r && d.refSite != nil {
			panic(twoRefsMessage(d.refSite.site(), site.site()))
		}
		d.ref, d.refSite = r.r, site
	}}
}

// twoRefsMessage explains an element that got two different refs.
func twoRefsMessage(first, second string) string {
	return "tela: an element received two different refs, one from " + first + " and one from " + second +
		". An element has one h.Ref: make it once with tela.UseElementRef and pass the same ref everywhere, " +
		"to the hooks of package aria as their Ref option (aria.PressOptions{Ref: ref}, aria.FocusRingOptions{Ref: ref}) " +
		"and to h.Ref(ref)"
}
