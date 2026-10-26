// Portions of this file follow React Aria's useOverlay, useModalOverlay and
// useOverlayTrigger (https://github.com/adobe/react-spectrum,
// packages/@react-aria/overlays) and react-stately's useOverlayTriggerState,
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// OverlayOptions configures [UseOverlay].
type OverlayOptions struct {
	// Open is whether the overlay is open. A closed overlay is not on the
	// overlay stack: its Props do nothing. Render the overlay only while
	// it is open, and pass the same value here, or true when the
	// component that calls the hook exists only while the overlay does.
	Open bool
	// OnClose is called when the user asks to close the overlay: Escape,
	// a press outside it (with Dismissable) or the focus leaving it (with
	// ShouldCloseOnBlur). The caller closes it, by changing the state that
	// Open comes from.
	OnClose func()
	// Dismissable closes the overlay with a press outside it: at the
	// pointer down, the press is kept from the elements under it, and at
	// the release OnClose is called and the click the press would make is
	// cancelled (React Aria's isDismissable). Any mouse button counts, the
	// secondary and middle ones too, as in Radix's DismissableLayer (React
	// Aria counts the primary one only); a right click's contextmenu event
	// still reaches the element under the pointer after the release. An
	// alert dialog is not dismissable.
	Dismissable bool
	// KeyboardDismissDisabled keeps Escape from closing the overlay. The
	// overlay still takes the key while it is the top one: Escape does not
	// reach the overlays below it nor the page.
	KeyboardDismissDisabled bool
	// ShouldCloseOnBlur closes the overlay when the focus moves from
	// inside it to another element outside it (not when it goes nowhere,
	// such as the window losing the focus).
	ShouldCloseOnBlur bool
	// ShouldCloseOnInteractOutside decides, for a press outside the
	// overlay, whether it closes it; nil closes it for every press. The
	// event carries the position of the press in the window: a popover
	// excludes its trigger by comparing it with the trigger's bounds. It
	// applies to Dismissable overlays.
	ShouldCloseOnInteractOutside func(tela.PointerEvent) bool
	// Modal makes everything outside the overlay inert while it is open
	// and the topmost modal overlay (the overlays opened from it stay
	// usable): it cannot be pressed or focused, is out of the Tab order and
	// is hidden from the aria snapshot (React Aria's ariaHideOutside).
	// A dialog is modal; a popover, a menu or a toast usually is not.
	Modal bool
}

// OverlayResult is what [UseOverlay] returns.
type OverlayResult struct {
	// Props go to the overlay's element: they put it on the overlay stack
	// while it is mounted and Open, so that Escape and presses outside
	// reach only the top overlay, and make it modal if asked.
	Props tela.Prop
}

// UseOverlay returns the props that make an element an overlay (React
// Aria's useOverlay and, with Modal, useModalOverlay): the topmost one is
// closed by Escape and by a press outside it, and a modal one makes the
// rest of the window inert. Overlays opened from inside another count as
// inside it (a popover in a dialog, in a Portal or not), so pressing in the
// popover does not close the dialog, and Escape closes the popover first.
//
//	func Confirm(open bool, close func()) tela.Node {
//	    ov := aria.UseOverlay(aria.OverlayOptions{Open: open, OnClose: close, Dismissable: true, Modal: true})
//	    return Portal(Presence(If(open,
//	        Div(ov.Props, Exit("opacity-0"), Class("... transition starting:opacity-0"), ...))))
//	}
//
// Use [UseDialog] on the same element for the dialog role and the focus,
// and [UseOverlayTrigger] for the state and the trigger. Rendering it
// inside a Presence lets it fade out: an exiting overlay is inert and no
// longer on the stack, so the page below is usable at once and a second
// Escape reaches the next overlay.
//
// The hook has no ref: [FocusScopeOptions.Ref] and the other hooks that
// share the element take theirs as usual.
func UseOverlay(o OverlayOptions) OverlayResult {
	return OverlayResult{Props: overlayProp(o, nil)}
}

// overlayProp is the prop of UseOverlay. blurIgnored, when set, says that
// a focus change to the element now focused is not a reason to close
// (UsePopover: the trigger).
func overlayProp(o OverlayOptions, blurIgnored func() bool) tela.Prop {
	if !o.Open {
		return tela.Prop{}
	}
	closeIt := func() {
		if o.OnClose != nil {
			o.OnClose()
		}
	}
	should := func(e tela.PointerEvent) bool {
		return o.ShouldCloseOnInteractOutside == nil || o.ShouldCloseOnInteractOutside(e)
	}
	eo := core.OverlayOptions{Modal: o.Modal}
	if !o.KeyboardDismissDisabled {
		eo.OnEscape = func(core.KeyboardEvent) { closeIt() }
	}
	if o.Dismissable {
		eo.OnInteractOutsideStart = func(e tela.PointerEvent) {
			if should(e) {
				e.StopPropagation()
				e.PreventDefault()
			}
		}
		eo.OnInteractOutside = func(e tela.PointerEvent) {
			if should(e) {
				e.StopPropagation()
				e.PreventDefault()
				closeIt()
			}
		}
	}
	if o.ShouldCloseOnBlur {
		eo.OnFocusOutside = func() {
			if blurIgnored == nil || !blurIgnored() {
				closeIt()
			}
		}
	}
	return core.Overlay(eo)
}

// OverlayTriggerOptions configures [UseOverlayTrigger].
type OverlayTriggerOptions struct {
	// Type is what the trigger opens: "dialog" (the default), "menu",
	// "listbox", "tree" or "grid" (the values of aria-haspopup). Another
	// value panics in dev mode with the closest valid one.
	Type string
	// Open points to the open state of a controlled overlay; nil leaves the
	// state to the hook, starting at DefaultOpen. OnOpenChange is called
	// with the next state when the trigger or the overlay asks for a change:
	// for a controlled overlay it is how the change is asked for, for an
	// uncontrolled one a notification.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Disabled makes the trigger inert and disabled.
	Disabled bool
	// Ref is the trigger's ref, for a hook used together with others on the
	// same element; the zero value makes the hook create its own.
	Ref tela.ElementRef
	// OverlayRef is the overlay's ref, which the trigger's aria-controls
	// points to; the zero value makes the hook create its own. The hooks of
	// the overlay's element (UsePopover, UseDialog, UseFocusScope...) must
	// share it as their Ref option.
	OverlayRef tela.ElementRef
}

// OverlayTriggerResult is what [UseOverlayTrigger] returns.
type OverlayTriggerResult struct {
	// TriggerProps go to the trigger: a press that toggles the overlay,
	// the "button" role, a tab stop, aria-haspopup, aria-expanded and,
	// while the overlay is open, aria-controls. Override the role on an
	// element that is not a button.
	TriggerProps tela.Prop
	// OverlayProps go to the overlay's element when no other hook of the
	// element attaches the overlay's ref: they hold just that ref.
	OverlayProps tela.Prop
	// Open is the current state, controlled or not.
	Open bool
	// SetOpen asks for a state (a controlled overlay only reports it to
	// OnOpenChange), Toggle for the other one, and Close for closed; Open
	// for open is SetOpen(true).
	SetOpen func(open bool)
	Toggle  func()
	Close   func()
	// Pressed is true while the trigger is pressed.
	Pressed bool
	// Ref is the trigger's ref and OverlayRef the overlay's.
	Ref, OverlayRef tela.ElementRef
}

// popupTypes are the values of OverlayTriggerOptions.Type.
var popupTypes = []string{"dialog", "menu", "listbox", "tree", "grid"}

func checkPopupType(typ string) string {
	if typ == "" {
		return "dialog"
	}
	for _, p := range popupTypes {
		if typ == p {
			return typ
		}
	}
	if core.DevNow() {
		panic(fmt.Sprintf("aria: OverlayTriggerOptions.Type %q: unknown popup type; use one of %s%s",
			typ, strings.Join(popupTypes, ", "), didYouMean(typ, popupTypes)))
	}
	return "dialog"
}

// didYouMean returns " (did you mean "x"?)" for the member of list nearest
// to s, or "".
func didYouMean(s string, list []string) string {
	if c := closest(s, list); c != "" {
		return fmt.Sprintf(" (did you mean %q?)", c)
	}
	return ""
}

// UseOverlayTrigger holds the open state of an overlay and returns the
// props of its trigger (React Aria's useOverlayTrigger with
// useOverlayTriggerState): a press toggles the overlay, and the trigger
// says what it opens (aria-haspopup), whether it is open (aria-expanded)
// and which element it controls (aria-controls).
//
//	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{Type: "dialog"})
//	ov := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: true})
//	return Fragment(
//	    Button(trig.TriggerProps, Class("rounded-md border px-3 py-1.5"), Text("Share")),
//	    Portal(If(trig.Open,
//	        Div(ov.Props, trig.OverlayProps, Role("dialog"), Text("Link copied")))))
//
// The state is uncontrolled by default; pass Open for a controlled one.
// Pair it with [UsePopover] or [UseOverlay] and [UseDialog] for the
// overlay (sharing OverlayRef), and with [UseMenuTrigger] for a menu.
func UseOverlayTrigger(o OverlayTriggerOptions) OverlayTriggerResult {
	popup := checkPopupType(o.Type)
	open, st := useBoolState(o.Open, o.DefaultOpen, o.OnOpenChange)
	ownTrigger, ownOverlay := tela.UseElementRef(), tela.UseElementRef()
	triggerRef, overlayRef := o.Ref, o.OverlayRef
	if triggerRef == (tela.ElementRef{}) {
		triggerRef = ownTrigger
	}
	if overlayRef == (tela.ElementRef{}) {
		overlayRef = ownOverlay
	}
	toggle := func() {
		if !o.Disabled {
			st.request(!st.cur)
		}
	}
	props, pressed, _ := usePressCore(pressConfig{
		PressOptions: PressOptions{OnPress: func(PressEvent) { toggle() }, Disabled: o.Disabled, Ref: triggerRef},
		enter:        true,
	})
	ps := []tela.Prop{props, buttonProps(o.Disabled), h.AriaHasPopup(popup), h.AriaExpanded(open)}
	if open {
		ps = append(ps, h.AriaControls(overlayRef))
	}
	return OverlayTriggerResult{
		TriggerProps: core.MergeProps(ps...),
		OverlayProps: h.Ref(overlayRef),
		Open:         open,
		SetOpen:      func(v bool) { st.request(v) },
		Toggle:       toggle,
		Close:        func() { st.request(false) },
		Pressed:      pressed,
		Ref:          triggerRef,
		OverlayRef:   overlayRef,
	}
}
