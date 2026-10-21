// Portions of this file follow React Aria's useFocusRing, useFocusVisible
// and useFocusWithin
// (https://github.com/adobe/react-spectrum, packages/@react-aria/focus and
// packages/@react-aria/interactions), Copyright 2020 Adobe, licensed under
// the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// FocusRingOptions configures [UseFocusRing].
type FocusRingOptions struct {
	// Within makes the ring follow the focus anywhere inside the element,
	// not only on the element itself (CSS's :focus-within and
	// React Aria's within): a group that shows a ring around all its
	// parts.
	Within bool
	// Ref is the element's ref, for a hook that is used together with
	// others on one element (see the package documentation); the zero
	// value makes the hook create its own.
	Ref tela.ElementRef
}

// FocusRingResult is what [UseFocusRing] returns.
type FocusRingResult struct {
	// Props go to the element: the focus, blur, key and pointer handlers
	// that keep the state, and the ref.
	Props tela.Prop
	// Focused is true while the element has the keyboard focus (with
	// Within: or one of its descendants does), in an active window.
	Focused bool
	// FocusVisible is true while Focused and the focus came, or was last
	// moved, with the keyboard: Tela's focus-visible heuristic, Chromium's
	// (a text field matches it whatever moved the focus). A ring drawn
	// from FocusVisible does not flash when a button is clicked.
	FocusVisible bool
	// Ref is the ref attached to the element by Props.
	Ref tela.ElementRef
}

// UseFocusRing returns the props that track the focus of an element, and
// whether it should show a focus ring (React Aria's useFocusRing): the
// ring appears after the keyboard moves the focus and not after a click.
// The `focus:`, `focus-visible:` and `focus-within:` variants do the same
// for classes; use the hook when Go needs to know, or when a ring is made
// of several elements.
//
//	ring := aria.UseFocusRing(aria.FocusRingOptions{})
//	return Button(ring.Props, Class("rounded-md px-3 py-1.5"),
//	    If(ring.FocusVisible, Class("ring-2 ring-primary")), Text("Save"))
//
// The element must be focusable (a Button, an Input, or h.TabIndex).
func UseFocusRing(o FocusRingOptions) FocusRingResult {
	own := tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	focused, setFocused := tela.UseState(false)
	visible, setVisible := tela.UseState(false)
	// Reads the engine: the focus and its visibility change without a
	// focus event when a key is pressed after a click, or the other way
	// round.
	update := func() {
		var f, v bool
		if o.Within {
			f = core.ContainsFocus(ref)
			v = f && core.FocusVisibleWithin(ref)
		} else {
			f = ref.Focused()
			v = core.FocusVisible(ref)
		}
		setFocused.Set(f)
		setVisible.Set(v)
	}
	return FocusRingResult{
		Focused:      focused,
		FocusVisible: visible,
		Ref:          ref,
		Props: core.MergeProps(
			h.Ref(ref),
			h.OnFocus(update),
			// Moving the focus blurs and then focuses again: the focus
			// handler restores what is still true.
			h.OnBlur(func() { setFocused.Set(false); setVisible.Set(false) }),
			h.OnKeyDown(update),
			h.OnPointerDown(update),
		),
	}
}
