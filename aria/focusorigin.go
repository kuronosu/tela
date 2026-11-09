package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// focusOrigin remembers the element that had the focus before a menu's
// trigger (a menu button, a menu bar) took it: the element the standard
// edit items of the menu act on, as a native menu, which takes no focus,
// acts on the window's focused field (ADR-255). The focus coming from
// outside the trigger and its menus sets it (Tab, a press, the menu bar's
// F10), the focus leaving them for another element clears it, and a move
// between the trigger and its menus keeps it.
type focusOrigin struct {
	ref tela.ElementRef
	has bool
	// inside reports whether an element is the trigger's or its menus'.
	inside func(tela.ElementRef) bool
}

// get returns the element, if there is one.
func (o *focusOrigin) get() (tela.ElementRef, bool) { return o.ref, o.has }

// set records where the focus was when it is about to move to the trigger
// (a press, a key that moves it there): the element that has it, if it is
// not the trigger's or its menus' already; none when nothing has it. at is
// any ref of the window.
func (o *focusOrigin) set(at tela.ElementRef) {
	f, ok := core.ActiveElement(at)
	switch {
	case !ok:
		o.has = false
	case !o.inside(f):
		o.ref, o.has = f, true
	}
}

// triggerProps are the handlers of the trigger's element: focusProp, the
// blur and pressProp.
func (o *focusOrigin) triggerProps(at tela.ElementRef) tela.Prop {
	return core.MergeProps(o.focusProp(), h.OnBlur(o.blur), o.pressProp(at))
}

// focusProp is the focus handler of the trigger's element (or of an
// element around the triggers, as the focus events bubble): the focus that
// comes from elsewhere records where it came from.
func (o *focusOrigin) focusProp() tela.Prop {
	return h.OnFocus(func(e tela.FocusEvent) {
		if r, ok := core.RelatedTarget(e); ok && !o.inside(r) {
			o.ref, o.has = r, true
		}
	})
}

// pressProp is the pointerdown handler of a trigger: a press focuses the
// trigger (or nothing) after its pointerdown, so it is merged before the
// press's handlers, which may move the focus.
func (o *focusOrigin) pressProp(at tela.ElementRef) tela.Prop {
	return h.OnPointerDown(func() {
		if !o.inside(activeOr(at)) {
			o.set(at)
		}
	})
}

// blur is the blur handler of the trigger and its menus: the focus that
// goes to another element ends the origin.
func (o *focusOrigin) blur(e tela.FocusEvent) {
	if r, ok := core.RelatedTarget(e); ok && !o.inside(r) {
		o.has = false
	}
}

// activeOr returns the element that has the focus in at's window, or the
// zero ref.
func activeOr(at tela.ElementRef) tela.ElementRef {
	f, _ := core.ActiveElement(at)
	return f
}
