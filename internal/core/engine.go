package core

import "slices"

// Small engine pieces of package aria (ADR-096): MergeProps for the Props
// its hooks return, pointer capture (UsePress) and the focus-visible
// query (UseFocusRing).

// MergeProps returns one prop that applies ps in order, for a hook that
// returns several props as one (React Aria's mergeProps): event handlers
// accumulate (every handler of an event runs, in order), the others are
// set by the last prop that sets them, as when they are given one after
// the other. The zero Prop in ps is skipped. It is for primitives:
// Fragment, Presence and the Key of components take Key only.
func MergeProps(ps ...Prop) Prop {
	ps = slices.Clone(ps)
	return Prop{name: "MergeProps", apply: func(d *desc) {
		for _, p := range ps {
			if p.apply != nil {
				p.apply(d)
			}
		}
	}}
}

// SetPointerCapture makes the element r is attached to the target of the
// events of pointer pointerID (PointerEvent.PointerID) until all its
// buttons are released or ReleasePointerCapture (the DOM's
// setPointerCapture): its moves, release and cancel go to the element
// wherever the pointer is, and its enter and leave events follow as if
// the pointer stayed over it. A touch contact and a pen are captured by
// their press target already (implicit capture). It reports whether the
// pointer is pressed (only a pressed pointer can be captured), on the UI
// thread: call it from a pointerdown or pointermove handler.
func SetPointerCapture(r ElementRef, pointerID int) bool {
	el := r.element()
	if el == nil {
		return false
	}
	ps := el.tree.pointerByID(pointerID)
	if ps == nil || ps.buttons == 0 {
		return false
	}
	ps.capture = el
	return true
}

// ReleasePointerCapture ends the capture of pointer pointerID by the
// element r is attached to (the DOM's releasePointerCapture): its next
// events go to the element under it. It also releases the implicit
// capture of a touch contact or a pen, so that enter and leave events
// follow the contact (React Aria's usePress does that).
func ReleasePointerCapture(r ElementRef, pointerID int) {
	el := r.element()
	if el == nil {
		return
	}
	if ps := el.tree.pointerByID(pointerID); ps != nil && ps.capture == el {
		ps.capture = nil
	}
}

// HasPointerCapture reports whether the element r is attached to captures
// pointer pointerID.
func HasPointerCapture(r ElementRef, pointerID int) bool {
	el := r.element()
	if el == nil {
		return false
	}
	ps := el.tree.pointerByID(pointerID)
	return ps != nil && ps.capture == el
}

// pointerByID returns the state of the pointer whose PointerEvent
// .PointerID is id, or nil.
func (t *Tree) pointerByID(id int) *pointerState {
	for k, ps := range t.in.ptrs {
		if publicPointerID(k) == id {
			return ps
		}
	}
	return nil
}

// FocusVisible reports whether the element r is attached to has the focus
// and shows it as focus-visible: does (the window has the keyboard focus,
// and Chromium's heuristic of focus.go says the last interaction was the
// keyboard): React Aria's isFocusVisible. It changes without a focus event
// when a key is pressed after a pointer focus; such a key's keydown
// reaches the focused element after the change.
func FocusVisible(r ElementRef) bool {
	el := r.element()
	if el == nil {
		return false
	}
	f := &el.tree.foc
	return f.window && f.el == el && f.visible
}
