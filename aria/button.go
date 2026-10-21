// Portions of this file follow React Aria's useButton and useToggleButton
// (https://github.com/adobe/react-spectrum, packages/@react-aria/button),
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

// ButtonOptions configures [UseButton]. The press fields are those of
// [PressOptions].
type ButtonOptions struct {
	// OnPress is called when the button is pressed and released over it,
	// with Enter, with Space, or by a virtual click.
	OnPress func(PressEvent)
	// OnPressStart is called when a press begins.
	OnPressStart func(PressEvent)
	// OnPressEnd is called when a press ends, whether it counts or not.
	OnPressEnd func(PressEvent)
	// OnPressChange is called with true when a press starts and false when
	// it ends.
	OnPressChange func(pressed bool)
	// OnPressUp is called when the pointer or the key is released over the
	// button.
	OnPressUp func(PressEvent)
	// Disabled makes the button inert and disabled for styling and
	// assistive technologies (h.Disabled): it cannot be focused or
	// pressed, and the `disabled:` variants apply.
	Disabled bool
	// ShouldCancelOnPointerExit ends the press for good when the pointer
	// leaves the button.
	ShouldCancelOnPointerExit bool
	// PreventFocusOnPress keeps the focus where it is when the button is
	// pressed with a pointer.
	PreventFocusOnPress bool
	// Type is what pressing the button does to the Form that contains it,
	// as HTML's type attribute: "button" (the default) nothing,
	// "submit" submits the Form (OnSubmit) and "reset" resets it, after
	// OnPress has run. A press has this effect whatever the element is, a
	// Button or a Div, and so does ElementRef.Click on it. Another value
	// panics in dev mode with the closest valid one.
	Type string
	// Ref is the element's ref, for a hook that is used together with
	// others on one element (see the package documentation); the zero
	// value makes the hook create its own.
	Ref tela.ElementRef
}

// ButtonResult is what [UseButton] and [UseToggleButton] return.
type ButtonResult struct {
	// Props go to the element: everything [UsePress] returns, the
	// "button" role, a tab stop and the disabled state.
	Props tela.Prop
	// Pressed is true while the button is pressed.
	Pressed bool
	// Ref is the ref attached to the element by Props.
	Ref tela.ElementRef
}

func (o ButtonOptions) press() PressOptions {
	return PressOptions{
		OnPressStart: o.OnPressStart, OnPressEnd: o.OnPressEnd, OnPressChange: o.OnPressChange,
		OnPressUp: o.OnPressUp, OnPress: o.OnPress, Disabled: o.Disabled,
		ShouldCancelOnPointerExit: o.ShouldCancelOnPointerExit, PreventFocusOnPress: o.PreventFocusOnPress,
		Ref: o.Ref,
	}
}

// checkButtonType returns the activation of a button type: "" for a
// button that does nothing, else "submit" or "reset". An unknown type
// panics in dev mode, with the closest valid one, and counts as "button"
// otherwise.
func checkButtonType(typ string) string {
	switch typ {
	case "", "button":
		return ""
	case "submit", "reset":
		return typ
	}
	if core.DevNow() {
		panic(fmt.Sprintf("aria: ButtonOptions.Type %q: unknown button type; use \"button\" (the default), \"submit\" or \"reset\"%s",
			typ, didYouMean(strings.ToLower(typ), []string{"button", "submit", "reset"})))
	}
	return ""
}

// buttonProps are what a button adds to its press: role, tab stop and
// disabled state.
func buttonProps(disabled bool) tela.Prop {
	ps := []tela.Prop{h.Role("button"), h.Disabled(disabled)}
	if !disabled {
		ps = append(ps, h.TabIndex(0))
	}
	return core.MergeProps(ps...)
}

// UseButton returns the props that make an element a button (React Aria's
// useButton): [UsePress] plus the "button" role, a tab stop and the
// disabled state. It works on a Button element and on any other, such as
// a Div or an A, that should behave as one.
//
//	btn := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { save() }})
//	return Button(btn.Props, Class("rounded-md bg-primary px-3 py-1.5 text-primary-foreground"),
//	    If(btn.Pressed, Class("scale-95")), Text("Save"))
//
// A Button that has the hook's props presses with Enter and Space. Like
// every press, it prevents the default action of the click, so the Form
// is not submitted or reset by the element's own activation: set Type to
// "submit" or "reset" and the hook does it when the press completes, on a
// Button or on any other element.
//
//	save := aria.UseButton(aria.ButtonOptions{Type: "submit"})
//	return Form(OnSubmit(func(tela.SubmitEvent) { send() }),
//	    Div(save.Props, Class("rounded-md bg-primary px-3 py-1.5"), Text("Send")))
func UseButton(o ButtonOptions) ButtonResult {
	props, pressed, ref := usePressCore(pressConfig{PressOptions: o.press(), enter: true, activate: checkButtonType(o.Type)})
	return ButtonResult{Props: core.MergeProps(props, buttonProps(o.Disabled)), Pressed: pressed, Ref: ref}
}

// ToggleOptions configures [UseToggleButton], [UseCheckbox] and
// [UseSwitch]: a two-state control. Its state is uncontrolled when
// Selected is nil (the hook keeps it, starting at DefaultSelected) and
// controlled when Selected points to the caller's state: the control then
// shows it and only asks OnChange for the next value, as an Input with
// Value and OnChange does. OnChange is also called for an uncontrolled
// control, to follow it.
//
//	// Uncontrolled: the hook keeps the state.
//	aria.ToggleOptions{DefaultSelected: true}
//	// Uncontrolled, and told of every change.
//	aria.ToggleOptions{DefaultSelected: true, OnChange: func(on bool) { save(on) }}
//	// Controlled: the caller keeps it.
//	on, setOn := tela.UseState(false)
//	aria.ToggleOptions{Selected: &on, OnChange: setOn.Set}
//
// To follow an uncontrolled control, read the Selected field of its
// result.
type ToggleOptions struct {
	// Selected points to the state of a controlled control; nil leaves the
	// state to the hook.
	Selected *bool
	// DefaultSelected is the initial state of an uncontrolled control.
	DefaultSelected bool
	// OnChange is called with the next state when the user toggles the
	// control: for a controlled control it is how the change is asked for,
	// for an uncontrolled one a notification.
	OnChange func(selected bool)
	// OnPress is called when the control is pressed, before the state
	// changes; the other press callbacks are those of [PressOptions].
	OnPress func(PressEvent)
	// OnPressStart is called when a press begins.
	OnPressStart func(PressEvent)
	// OnPressEnd is called when a press ends, whether it counts or not.
	OnPressEnd func(PressEvent)
	// OnPressChange is called with true when a press starts and false when
	// it ends.
	OnPressChange func(pressed bool)
	// OnPressUp is called when the pointer or the key is released over the
	// control.
	OnPressUp func(PressEvent)
	// Disabled makes the control inert and disabled for styling and
	// assistive technologies (h.Disabled).
	Disabled bool
	// ReadOnly keeps the control focusable and readable but not
	// changeable (aria-readonly).
	ReadOnly bool
	// ID is the id of the element, for a Label's For; the default is one
	// made with tela.UseID, returned in the ID field of the result.
	ID string
	// Ref is the element's ref, for a hook that is used together with
	// others on one element (see the package documentation); the zero
	// value makes the hook create its own.
	Ref tela.ElementRef
}

// ToggleResult is what [UseToggleButton], [UseCheckbox] and [UseSwitch]
// return.
type ToggleResult struct {
	// Props go to the element: the press, the role, the tab stop, the
	// checked or pressed state, the disabled and read-only states and the
	// ID.
	Props tela.Prop
	// Selected is the current state, controlled or not.
	Selected bool
	// Pressed is true while the control is pressed.
	Pressed bool
	// ID is the element's id: ToggleOptions.ID, or one made by the hook.
	// A Label(For(id)) names the control and toggles it when clicked, if
	// the control is a Button element.
	ID string
	// Ref is the ref attached to the element by Props.
	Ref tela.ElementRef
}

// useToggleState returns the current state and the function that asks for
// the next one.
func useToggleState(o ToggleOptions) (selected bool, toggle func()) {
	selected, st := useBoolState(o.Selected, o.DefaultSelected, o.OnChange)
	toggle = func() {
		if o.Disabled || o.ReadOnly {
			return
		}
		st.request(!st.cur)
	}
	return selected, toggle
}

// press returns the press options of a toggle: its callbacks, and the
// toggle after OnPress.
func (o ToggleOptions) press(toggle func()) PressOptions {
	return PressOptions{
		OnPressStart: o.OnPressStart, OnPressEnd: o.OnPressEnd, OnPressChange: o.OnPressChange, OnPressUp: o.OnPressUp,
		OnPress: func(e PressEvent) {
			if o.OnPress != nil {
				o.OnPress(e)
			}
			toggle()
		},
		Disabled: o.Disabled, Ref: o.Ref,
	}
}

// UseToggleButton returns the props of a button that is on or off (React
// Aria's useToggleButton): [UseButton] plus the state, shown as
// `aria-pressed`, so the `aria-pressed:` variants style it. Enter, Space,
// a click or a tap toggles it.
//
//	bold, setBold := tela.UseState(false)
//	tb := aria.UseToggleButton(aria.ToggleOptions{Selected: &bold, OnChange: setBold.Set})
//	return Button(tb.Props, Class("rounded px-2 aria-pressed:bg-accent"), Text("B"))
func UseToggleButton(o ToggleOptions) ToggleResult {
	selected, toggle := useToggleState(o)
	id := useToggleID(o)
	props, pressed, ref := usePressCore(pressConfig{PressOptions: o.press(toggle), enter: true})
	return ToggleResult{
		Selected: selected, Pressed: pressed, ID: id, Ref: ref,
		Props: core.MergeProps(props, buttonProps(o.Disabled), h.ID(id), h.AriaPressed(selected)),
	}
}

func useToggleID(o ToggleOptions) string {
	auto := tela.UseID()
	if o.ID != "" {
		return o.ID
	}
	return auto
}

// readOnlyProp is aria-readonly when set, for checkboxes and switches.
func readOnlyProp(readOnly bool) tela.Prop {
	if !readOnly {
		return tela.Prop{}
	}
	return h.AriaReadOnly(true)
}
