package core

import (
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// Overlays, interacting outside and modality (ADR-096, ADR-191): the
// engine under package aria's UseOverlay, UseDialog, UsePopover, UseMenu
// and UseTooltip, after React Aria's useInteractOutside, useOverlay and
// ariaHideOutside and HTML's inert attribute.
//
// # Inside and outside
//
// Inside an element means inside its subtree in the element tree, as in
// React: the content of a Portal rendered from inside it, and an overlay
// opened from inside it, are inside it wherever they are painted.
//
// # The overlay stack
//
// Overlay(OverlayOptions) on a primitive puts it on the overlay stack
// while it is mounted, in mount order: the last one mounted that is not
// inert (an overlay whose Presence child exits is closing) is the top
// overlay (React Aria's isTopLayer). Only the top overlay is told about
// presses outside it and about Escape. An EscapeOnly overlay (a tooltip)
// takes Escape like the others, topmost first, but is not told of
// presses and does not hide them from the overlay below, which stays the
// top overlay for them.
//
// # Pressing outside
//
// OnInteractOutsideStart runs at the pointerdown of a press whose target
// is outside the top overlay, and OnInteractOutside at the pointerup of
// that press when its target is outside it too and the overlay was the
// top at the pointerdown (React Aria's useInteractOutside). A press is
// the mouse's primary, secondary or middle button, a touch contact or a
// pen tip; a pen's barrel buttons and the mouse's back and forward
// buttons do not count. React Aria counts the primary button only;
// Radix's DismissableLayer, which shadcn/ui's overlays use, counts a
// pointerdown with any button, so a right click outside an open menu
// closes it (ADR-167). The pointerdown is the press of the first button
// and the pointerup the release of the last one (chorded buttons count
// as one press, input.go); a press that starts outside and ends inside
// ends nothing. The contextmenu event of a right click comes after that
// pointerup, so a right click that closes a context menu can open it
// again at the new point (package aria's UseContextMenuTrigger); a
// context menu's long press, which comes before the release, takes its
// menu out of the press with KeepOutsidePress instead. Both callbacks run
// before the target's handlers, like listeners in the capture phase of
// the document, with the event of the dispatch: StopPropagation there
// keeps the pointerdown (pointerup) from reaching the target's handlers,
// PreventDefault on the pointerdown cancels its default (the mouse
// events of the press and the focus change, input.go), and on the
// pointerup it cancels the click of the press (React Aria stops that
// click).
//
// # Escape and the focus
//
// An Escape keydown (not a repeat) that no handler prevented has the
// default action of calling the top overlay's OnEscape (HTML's close
// requests, which also close the topmost dialog or popover only); while
// there is a top overlay, Escape does nothing else, even when it has no
// OnEscape (it cannot be dismissed with the keyboard). An element inside
// an overlay that handles Escape itself (a combobox closing its list)
// prevents the keydown. OnFocusOutside tells an overlay that the focus
// moved from inside it to another element.
//
// The callbacks of an overlay run as handlers do (Tree.handlerDepth):
// the updates of a click they make (ElementRef.Click, a button hook's
// activation) are applied and built by the event that is running, after
// its dispatch, not in the middle of it.
//
// # Modality
//
// An overlay with Modal makes everything outside it inert, except the
// overlays above it on the stack (a popover opened from a dialog, a
// toast): the topmost modal overlay that is not inert, and every live
// overlay above it, are the subtrees left alive. Every element outside
// them (Element.outside) is not hit by the pointer (a press there hits
// the root, which is outside every overlay), cannot take the focus (a
// focused one is blurred by the focus fix-up), is out of the Tab order,
// and is hidden in the semantics tree (React Aria's ariaHideOutside and
// inert); the ancestors of the subtrees left alive are not hit nor
// focusable themselves, but they still show in the semantics tree, as
// ariaHideOutside leaves the ancestors of the modal alone, and events
// from inside still bubble through them (React). The marks are computed
// again, walking the element tree, when the stack changes (after the
// build phase); elements mounted later take their parent's.
//
// # Inert
//
// Inert(true) makes a primitive and its subtree inert, like HTML's inert
// attribute and an exiting Presence child (presence.go): not hit, not
// focusable (a focused element in it is blurred), out of the Tab order
// and hidden in the semantics tree. Playwright 1.61 does not know the
// inert attribute, so its aria snapshot of the equivalent HTML would
// still show the subtree; Tela's leaves it out, like the browser's
// accessibility tree (and like Playwright for aria-hidden).

// Element.outside values.
const (
	outsideNo       uint8 = iota // no modal, or inside the subtrees left alive
	outsideYes                   // outside them: inert
	outsideAncestor              // an ancestor of one of them
)

// inheritOutside returns the outside mark of a child mounted under p.
func inheritOutside(p *Element) uint8 {
	if p.outside == outsideNo {
		return outsideNo
	}
	return outsideYes
}

// OverlayOptions configures Overlay.
type OverlayOptions struct {
	// OnEscape runs as the default action of an Escape keydown while the
	// element is the top overlay. Nil: the overlay is not dismissed with
	// the keyboard (and Escape does nothing).
	OnEscape func(KeyboardEvent)
	// OnInteractOutsideStart runs at the pointerdown of a press outside
	// the element (a right click too) while it is the top overlay;
	// OnInteractOutside at the pointerup of that press, when its target
	// is outside too. Both run before the target's handlers (see "Pressing
	// outside" at the top of overlay.go for what counts as a press and
	// for StopPropagation and PreventDefault).
	OnInteractOutsideStart func(PointerEvent)
	OnInteractOutside      func(PointerEvent)
	// Modal makes everything outside the overlay, and outside the
	// overlays above it, inert while it is the topmost modal overlay.
	Modal bool
	// OnFocusOutside runs when the focus moves from inside the element to
	// another element outside it (React Aria's shouldCloseOnBlur), not
	// when it goes nowhere (the window losing the focus).
	OnFocusOutside func()
	// EscapeOnly is for a tooltip: the overlay takes Escape while it is
	// the topmost one on the stack (closing it first, as React Aria's
	// tooltips stop the key before the overlays below), but it is not
	// told of presses outside it and does not hide them from the overlay
	// below, which stays the top overlay for them.
	EscapeOnly bool
}

// Overlay returns a prop that puts a primitive on the overlay stack while
// it is mounted (see the comment at the top of overlay.go): only the top
// overlay is dismissed by Escape and outside presses, and a Modal one
// makes the rest of the window inert. For package aria (UseOverlay,
// UseDialog, UsePopover); it is not exposed by tela or h.
func Overlay(o OverlayOptions) Prop {
	opts := &o
	return Prop{name: "Overlay", apply: func(d *desc) { d.overlay = opts }}
}

// Inert makes a primitive and its subtree inert when on, like HTML's
// inert attribute (h.Inert): not hit by the pointer, not focusable and
// hidden from the semantics tree.
func Inert(on bool) Prop {
	return Prop{name: "Inert", apply: func(d *desc) { d.inert = on }}
}

// overlayState is the overlays of a Tree (UI thread).
type overlayState struct {
	// stack are the primitives with an Overlay prop, in mount order.
	stack []*Element
	// alive are the subtrees left alive by the active modal (the topmost
	// modal overlay and the overlays above it), nil without one; dirty
	// asks syncModal to compute them again.
	alive []*Element
	dirty bool
}

// updateOverlay records the Overlay prop of primitive el, whose node was
// old (nil at mount).
func (t *Tree) updateOverlay(el *Element, old *desc) {
	O := &t.ovl
	d := el.node.d
	wasO, isO := old != nil && old.overlay != nil, d.overlay != nil
	switch {
	case isO && !wasO:
		O.stack = append(O.stack, el)
		O.dirty = true
	case wasO && !isO:
		O.stack = removeElement(O.stack, el)
		O.dirty = true
	case isO && old.overlay.Modal != d.overlay.Modal:
		O.dirty = true
	}
}

// removeOverlay forgets primitive el, an overlay unmounted.
func (t *Tree) removeOverlay(el *Element) {
	O := &t.ovl
	O.stack = removeElement(O.stack, el)
	O.dirty = true
}

func removeElement(s []*Element, el *Element) []*Element {
	if i := slices.Index(s, el); i >= 0 {
		return slices.Delete(s, i, i+1)
	}
	return s
}

// topOverlay returns the top overlay for presses, or nil: the last live
// overlay that is not EscapeOnly.
func (t *Tree) topOverlay() *Element {
	s := t.ovl.stack
	for i := len(s) - 1; i >= 0; i-- {
		if el := s[i]; el.mounted && !el.inert && !el.node.d.overlay.EscapeOnly {
			return el
		}
	}
	return nil
}

// topEscapeOverlay returns the overlay that takes Escape, or nil: the
// last live overlay, EscapeOnly ones included.
func (t *Tree) topEscapeOverlay() *Element {
	s := t.ovl.stack
	for i := len(s) - 1; i >= 0; i-- {
		if el := s[i]; el.mounted && !el.inert {
			return el
		}
	}
	return nil
}

// focusMoved tells the overlays that contain old but not now, where the
// focus went, that it left them.
func (t *Tree) focusMoved(old, now *Element) {
	if old == nil || now == nil || len(t.ovl.stack) == 0 {
		return
	}
	t.handlerDepth++ // the callbacks run as handlers (flushNested)
	defer func() { t.handlerDepth-- }()
	// A copy: the callbacks may unmount overlays (in later builds).
	for _, el := range slices.Clone(t.ovl.stack) {
		f := el.node.d.overlay.OnFocusOutside
		if f != nil && el.mounted && !el.inert && isInside(old, el) && !isInside(now, el) {
			f()
		}
	}
}

// outsideButton reports whether a pointerdown of ps with button b is a
// press for the top overlay's outside callbacks: the mouse's primary,
// secondary and middle buttons (Radix's DismissableLayer), a touch
// contact and a pen tip.
func outsideButton(ps *pointerState, b platform.PointerButton) bool {
	switch b {
	case platform.ButtonPrimary:
		return true
	case platform.ButtonSecondary, platform.ButtonMiddle:
		return ps.key.kind == platform.PointerMouse
	}
	return false
}

// outsideDown runs the top overlay's OnInteractOutsideStart for a press
// at target (outsideButton) outside it, at its pointerdown, before the
// dispatch that shares st, and records the overlay in ps for outsideUp.
func (t *Tree) outsideDown(ps *pointerState, target *Element, e PointerEvent, st *eventState) {
	ps.outside = nil
	top := t.topOverlay()
	if top == nil || target != nil && isInside(target, top) {
		return
	}
	ps.outside = top
	if f := top.node.d.overlay.OnInteractOutsideStart; f != nil {
		t.handlerDepth++ // it runs as a handler (flushNested)
		defer func() { t.handlerDepth-- }()
		(&e).prepare(t, st, top)
		f(e)
	}
}

// outsideUp runs OnInteractOutside of the overlay a press started outside
// of, at its pointerup at target, before the dispatch that shares st
// (PreventDefault there cancels the click of the press: pointerUp).
func (t *Tree) outsideUp(ps *pointerState, target *Element, e PointerEvent, st *eventState) {
	el := ps.outside
	ps.outside = nil
	if el == nil || !el.mounted || el.inert || target != nil && isInside(target, el) {
		return
	}
	// The element may have lost its Overlay prop since the pointerdown.
	if o := el.node.d.overlay; o != nil && o.OnInteractOutside != nil {
		t.handlerDepth++ // it runs as a handler (flushNested)
		defer func() { t.handlerDepth-- }()
		(&e).prepare(t, st, el)
		o.OnInteractOutside(e)
	}
}

// KeepOutsidePress takes the element r is attached to (and its subtree,
// and the element holding it) out of the outside press that the long
// press e belongs to: at the release of that press, the OnInteractOutside
// of the overlay the press started outside of does not run when that
// overlay is among them; an overlay elsewhere (another area's menu) still
// gets the release. Reserved for package aria: a context menu's long
// press that opens or moves the menu while it is open
// (UseContextMenuTrigger), whose pointerdown was outside the menu, so
// that its release does not close the menu it has just placed (Radix
// keeps it open at the new point; a right click does not need this, as
// its contextmenu event comes after the release). It does nothing for
// another event than a long press, or when r is not attached.
func KeepOutsidePress(e PointerEvent, r ElementRef) {
	keep := r.element()
	if e.st == nil || e.st.press == nil || keep == nil {
		return
	}
	ps := e.st.press
	if el := ps.outside; el != nil && (isInside(el, keep) || isInside(keep, el)) {
		ps.outside = nil
	}
}

// escapeOverlay runs the Escape default action: the top overlay's
// OnEscape. It reports whether there is a top overlay (which takes the
// key whether or not it has OnEscape). st is the keydown's state, nil
// when no keydown handler ran.
func (t *Tree) escapeOverlay(ke KeyboardEvent, st *eventState) bool {
	top := t.topEscapeOverlay()
	if top == nil {
		return false
	}
	if f := top.node.d.overlay.OnEscape; f != nil {
		if st == nil {
			st = &eventState{}
		}
		(&ke).prepare(t, st, top)
		t.handlerDepth++ // it runs as a handler (flushNested)
		func() {
			defer func() { t.handlerDepth-- }()
			f(ke)
		}()
		t.flushDiscrete()
	}
	return true
}

// syncModal computes the subtrees left alive by the active modal again
// when the overlay stack changed, and marks the elements outside them.
func (t *Tree) syncModal() {
	O := &t.ovl
	if !O.dirty {
		return
	}
	O.dirty = false
	var alive []*Element
	for i := len(O.stack) - 1; i >= 0; i-- {
		el := O.stack[i]
		if !el.mounted || el.inert || !el.node.d.overlay.Modal {
			continue
		}
		for _, x := range O.stack[i:] {
			if x.mounted && !x.inert {
				alive = append(alive, x)
			}
		}
		break
	}
	if slices.Equal(alive, O.alive) {
		return
	}
	O.alive = alive
	t.markOutside()
	t.dirty() // the focus fix-up and the hit-testing of the next frame
}

// markOutside sets Element.outside on the whole tree from ovl.alive.
// perf: a walk of the element tree per change of the active modal.
func (t *Tree) markOutside() {
	if t.root == nil {
		return
	}
	alive := t.ovl.alive
	in := make(map[*Element]bool, len(alive))
	above := map[*Element]bool{}
	for _, a := range alive {
		in[a] = true
		for x := a.parent; x != nil && !above[x]; x = x.parent {
			above[x] = true
		}
	}
	var walk func(el *Element, out bool)
	walk = func(el *Element, out bool) {
		if in[el] {
			out = false
		}
		switch {
		case !out:
			el.outside = outsideNo
		case above[el]:
			el.outside = outsideAncestor
		default:
			el.outside = outsideYes
		}
		for _, c := range el.children {
			walk(c, out)
		}
	}
	walk(t.root, len(alive) > 0)
}

// blockedBox reports whether hit-testing skips box bx and its subtree:
// its element is outside the active modal's subtrees.
func blockedBox(bx *Box) bool {
	return bx.owner != nil && bx.owner.outside == outsideYes
}
