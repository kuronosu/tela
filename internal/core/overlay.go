package core

import "slices"

// Overlays, interacting outside and modality (ADR-096): the engine under
// package aria's UseOverlay, UseDialog, UsePopover, UseMenu and
// UseTooltip, after React Aria's useInteractOutside, useOverlay and
// ariaHideOutside and HTML's inert attribute.
//
// # Inside and outside
//
// Inside an element means inside its subtree in the element tree, as in
// React: the content of a Portal rendered from inside it, and an overlay
// opened from inside it, are inside it wherever they are painted.
//
// # Interacting outside
//
// InteractOutside(OutsideOptions) on a primitive reports pointer presses
// outside it: OnStart at the pointerdown of a primary press (the left
// button, a touch contact or a pen tip: React Aria's rule, which ignores
// the other buttons) whose target is outside it, and OnEnd at the
// pointerup of that press when its target is outside it too. They run
// before the target's handlers, like listeners in the capture phase of
// the document, with the event of the dispatch: StopPropagation there
// keeps the pointerdown (pointerup) from reaching the target's handlers,
// PreventDefault on the pointerdown cancels its default (the mouse
// events of the press and the focus change, input.go), and on the
// pointerup it cancels the click of the press (React Aria stops that
// click). Inert listeners are skipped.
//
// # The overlay stack
//
// Overlay(OverlayOptions) on a primitive puts it on the overlay stack
// while it is mounted, in mount order: the last one mounted that is not
// inert (an overlay whose Presence child exits is closing) is the top
// overlay (React Aria's isTopLayer). Only the top overlay is told about
// presses outside it (OnInteractOutsideStart at the pointerdown, and
// OnInteractOutside at the pointerup when it was the top at the
// pointerdown, both with the semantics of InteractOutside) and about
// Escape: an Escape keydown (not a repeat) that no handler prevented has
// the default action of calling the top overlay's OnEscape (HTML's close
// requests, which also close the topmost dialog or popover only); while
// there is a top overlay, Escape does nothing else, even when it has no
// OnEscape (it cannot be dismissed with the keyboard). An element inside
// an overlay that handles Escape itself (a combobox closing its list)
// prevents the keydown. An EscapeOnly overlay (a tooltip) takes Escape
// like the others, topmost first, but is not told of presses and does not
// hide them from the overlay below; OnFocusOutside tells an overlay that
// the focus moved from inside it to another element.
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

// OutsideOptions configures InteractOutside.
type OutsideOptions struct {
	// OnStart runs at the pointerdown of a primary press whose target is
	// outside the element, before the target's handlers.
	OnStart func(PointerEvent)
	// OnEnd runs at the pointerup of a press that started outside, when
	// its target is outside too, before the target's handlers;
	// PreventDefault there cancels the click of the press.
	OnEnd func(PointerEvent)
}

// InteractOutside returns a prop that reports pointer presses outside a
// primitive's subtree (see the comment at the top of overlay.go): React
// Aria's useInteractOutside, for package aria; it is not exposed by tela
// or h.
func InteractOutside(o OutsideOptions) Prop {
	opts := &o
	return Prop{name: "InteractOutside", apply: func(d *desc) { d.outside = opts }}
}

// OverlayOptions configures Overlay.
type OverlayOptions struct {
	// OnEscape runs as the default action of an Escape keydown while the
	// element is the top overlay. Nil: the overlay is not dismissed with
	// the keyboard (and Escape does nothing).
	OnEscape func(KeyboardEvent)
	// OnInteractOutsideStart runs at the pointerdown of a primary press
	// outside the element while it is the top overlay; OnInteractOutside
	// at the pointerup of that press, when its target is outside too (see
	// InteractOutside for StopPropagation and PreventDefault).
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
	// below, which stays the top overlay for them (IsTopOverlay).
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

// IsTopOverlay reports whether the element r is attached to is the top
// overlay: the last mounted primitive with an Overlay prop that is not
// inert. Call it on the UI thread.
func IsTopOverlay(r ElementRef) bool {
	el := r.element()
	return el != nil && el.tree.topOverlay() == el
}

// overlayState is the overlays and interact-outside listeners of a Tree
// (UI thread).
type overlayState struct {
	// listeners are the primitives with an InteractOutside or an Overlay
	// prop, stack those with an Overlay, both in mount order.
	listeners []*Element
	stack     []*Element
	// alive are the subtrees left alive by the active modal (the topmost
	// modal overlay and the overlays above it), nil without one; dirty
	// asks syncModal to compute them again.
	alive []*Element
	dirty bool
}

// outsidePress is a listener that a press started outside of: plain for
// its InteractOutside prop, top for its Overlay prop (it was the top
// overlay).
type outsidePress struct {
	el         *Element
	plain, top bool
}

// updateListener records the InteractOutside and Overlay props of
// primitive el, whose node was old (nil at mount).
func (t *Tree) updateListener(el *Element, old *desc) {
	O := &t.ovl
	d := el.node.d
	was := old != nil && (old.overlay != nil || old.outside != nil)
	is := d.overlay != nil || d.outside != nil
	switch {
	case is && !was:
		O.listeners = append(O.listeners, el)
	case was && !is:
		O.listeners = removeElement(O.listeners, el)
	}
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

// removeListener forgets primitive el, unmounted.
func (t *Tree) removeListener(el *Element) {
	O := &t.ovl
	O.listeners = removeElement(O.listeners, el)
	if el.node.d.overlay != nil {
		O.stack = removeElement(O.stack, el)
		O.dirty = true
	}
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
	// A copy: the callbacks may unmount overlays (in later builds).
	for _, el := range slices.Clone(t.ovl.stack) {
		f := el.node.d.overlay.OnFocusOutside
		if f != nil && el.mounted && !el.inert && isInside(old, el) && !isInside(now, el) {
			f()
		}
	}
}

// outsideDown runs the interact-outside listeners of a primary press at
// target, at its pointerdown, before the dispatch that shares st, and
// records in ps those it started for outsideUp.
func (t *Tree) outsideDown(ps *pointerState, target *Element, e PointerEvent, st *eventState) {
	clear(ps.outside)
	ps.outside = ps.outside[:0]
	O := &t.ovl
	if len(O.listeners) == 0 {
		return
	}
	top := t.topOverlay()
	// A copy: the handlers may mount and unmount listeners (in later
	// builds; discrete events build after the dispatch).
	for _, el := range slices.Clone(O.listeners) {
		if !el.mounted || el.inert || target != nil && isInside(target, el) {
			continue
		}
		d := el.node.d
		op := outsidePress{el: el, plain: d.outside != nil, top: d.overlay != nil && el == top}
		if !op.plain && !op.top {
			continue
		}
		ps.outside = append(ps.outside, op)
		if op.plain && d.outside.OnStart != nil {
			(&e).prepare(t, st, el)
			d.outside.OnStart(e)
		}
		if op.top && d.overlay.OnInteractOutsideStart != nil {
			(&e).prepare(t, st, el)
			d.overlay.OnInteractOutsideStart(e)
		}
	}
}

// outsideUp runs the listeners a press started outside of at its
// pointerup at target, before the dispatch that shares st (PreventDefault
// there cancels the click of the press: pointerUp).
func (t *Tree) outsideUp(ps *pointerState, target *Element, e PointerEvent, st *eventState) {
	if len(ps.outside) == 0 {
		return
	}
	presses := slices.Clone(ps.outside)
	clear(ps.outside)
	ps.outside = ps.outside[:0]
	for _, op := range presses {
		el := op.el
		if !el.mounted || el.inert || target != nil && isInside(target, el) {
			continue
		}
		d := el.node.d
		if op.plain && d.outside != nil && d.outside.OnEnd != nil {
			(&e).prepare(t, st, el)
			d.outside.OnEnd(e)
		}
		if op.top && d.overlay != nil && d.overlay.OnInteractOutside != nil {
			(&e).prepare(t, st, el)
			d.overlay.OnInteractOutside(e)
		}
	}
}

// escapeOverlay runs the Escape default action: the top overlay's
// OnEscape. It reports whether there is a top overlay (which takes the
// key whether or not it has OnEscape).
func (t *Tree) escapeOverlay(ke KeyboardEvent, st *eventState) bool {
	top := t.topEscapeOverlay()
	if top == nil {
		return false
	}
	if f := top.node.d.overlay.OnEscape; f != nil {
		(&ke).prepare(t, st, top)
		f(ke)
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
