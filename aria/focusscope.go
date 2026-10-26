// Portions of this file follow React Aria's FocusScope and useFocusManager
// (https://github.com/adobe/react-spectrum, packages/@react-aria/focus),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// FocusScopeOptions configures [UseFocusScope].
type FocusScopeOptions struct {
	// Contain keeps the focus inside the scope: Tab and Shift+Tab wrap
	// around its focusable elements instead of leaving, a press on
	// something that is not focusable (a scrim) leaves the focus where it
	// is, and a press or a script that would focus an element outside
	// keeps it inside instead (React Aria's contain). When the focused
	// element inside unmounts, the focus goes to the scope's element if it
	// is focusable (h.TabIndex(-1), as Radix's FocusScope focuses its
	// container), else to its first tabbable element. The content of
	// portals and overlays opened from inside the scope is inside it. The
	// most recently mounted containing scope wins.
	Contain bool
	// Restore gives the focus back when the scope unmounts, to the element
	// that had it when the scope mounted, if that element can still take
	// it.
	Restore bool
	// AutoFocus focuses the first tabbable element of the scope (the first
	// focusable one if none is tabbable) after the frame that mounts it,
	// unless the focus is already inside.
	AutoFocus bool
	// Ref is the element's ref, for a hook that is used together with
	// others on one element (see the package documentation); the zero
	// value makes the hook create its own.
	Ref tela.ElementRef
}

// FocusScopeResult is what [UseFocusScope] returns.
type FocusScopeResult struct {
	// Props go to the element that holds the scope: the scope itself and
	// the ref. The scope is the element's subtree (the content of the
	// portals rendered from inside it counts, after its own).
	Props tela.Prop
	// Manager moves the focus inside the scope.
	Manager FocusManager
	// Ref is the ref attached to the element by Props.
	Ref tela.ElementRef
}

// FocusOptions configures the moves of a [FocusManager].
type FocusOptions struct {
	// Tabbable limits the candidates to the elements in the sequential
	// focus order (h.TabIndex of 0 or more, Buttons and fields), leaving
	// out those that only a script can focus (h.TabIndex(-1)).
	Tabbable bool
	// Wrap goes around at the ends: FocusNext from the last candidate
	// focuses the first, FocusPrevious from the first focuses the last.
	// It applies to FocusNext and FocusPrevious.
	Wrap bool
	// From is where FocusNext and FocusPrevious start, an element of the
	// scope; the zero value starts from the focused element.
	From tela.ElementRef
}

// FocusManager moves the focus among the focusable elements of a scope
// (React Aria's FocusManager), in tree order. Hidden, disabled and inert
// elements, and those outside an open modal, are not candidates. Call its
// methods from event handlers and effects; each reports whether it found
// an element to focus. The zero FocusManager does nothing.
type FocusManager struct{ ref tela.ElementRef }

func (m FocusManager) opts(o FocusOptions) core.FocusOptions {
	return core.FocusOptions{Tabbable: o.Tabbable, Wrap: o.Wrap, From: o.From}
}

// FocusNext focuses the element after o.From (or after the focused
// element) in the scope.
func (m FocusManager) FocusNext(o FocusOptions) bool { return core.FocusNext(m.ref, m.opts(o)) }

// FocusPrevious focuses the element before o.From (or before the focused
// element) in the scope.
func (m FocusManager) FocusPrevious(o FocusOptions) bool {
	return core.FocusPrevious(m.ref, m.opts(o))
}

// FocusFirst focuses the first element of the scope.
func (m FocusManager) FocusFirst(o FocusOptions) bool { return core.FocusFirst(m.ref, m.opts(o)) }

// FocusLast focuses the last element of the scope.
func (m FocusManager) FocusLast(o FocusOptions) bool { return core.FocusLast(m.ref, m.opts(o)) }

// UseFocusScope returns the props that make an element a focus scope:
// contain Tab inside it (a dialog, a popover), restore the focus when it
// closes, focus its first element when it opens, and the manager that
// moves the focus inside it (React Aria's FocusScope and useFocusManager).
//
//	scope := aria.UseFocusScope(aria.FocusScopeOptions{Contain: true, Restore: true, AutoFocus: true})
//	return Div(scope.Props, Role("dialog"), Class("rounded-lg p-4"), ...)
//
// An arrow-key toolbar uses the manager from its key handler:
//
//	scope := aria.UseFocusScope(aria.FocusScopeOptions{})
//	return Div(scope.Props, Role("toolbar"), OnKeyDown(func(e tela.KeyboardEvent) {
//	    switch e.Key {
//	    case tela.KeyArrowRight:
//	        e.PreventDefault()
//	        scope.Manager.FocusNext(aria.FocusOptions{Wrap: true})
//	    case tela.KeyArrowLeft:
//	        e.PreventDefault()
//	        scope.Manager.FocusPrevious(aria.FocusOptions{Wrap: true})
//	    }
//	}), ...)
//
// The scope's element holds the hook's ref; see the package documentation
// when it must also hold other hooks'.
func UseFocusScope(o FocusScopeOptions) FocusScopeResult {
	own := tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	return FocusScopeResult{
		Ref:     ref,
		Manager: FocusManager{ref: ref},
		Props: core.MergeProps(
			h.Ref(ref),
			core.Scope(core.ScopeOptions{Contain: o.Contain, Restore: o.Restore, AutoFocus: o.AutoFocus}),
		),
	}
}
