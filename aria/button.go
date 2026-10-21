// Portions of this file follow React Aria's useButton and useToggleButton
// (https://github.com/adobe/react-spectrum, packages/@react-aria/button),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
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
// every press, it prevents the default action of the click, so it does
// not submit or reset its Form: do that from OnPress.
func UseButton(o ButtonOptions) ButtonResult {
	props, pressed, ref := usePressCore(pressConfig{PressOptions: o.press(), enter: true})
	return ButtonResult{Props: core.MergeProps(props, buttonProps(o.Disabled)), Pressed: pressed, Ref: ref}
}

// ToggleOptions configures [UseToggleButton], [UseCheckbox] and
// [UseSwitch]: a two-state control. Its state is uncontrolled by default
// (the hook keeps it, starting at DefaultSelected) and controlled when
// OnChange is set: the control then shows Selected and only asks OnChange
// for the next value, as an Input with Value and OnChange does.
//
//	// Uncontrolled: the hook keeps the state.
//	aria.ToggleOptions{DefaultSelected: true}
//	// Controlled: the caller keeps it.
//	on, setOn := tela.UseState(false)
//	aria.ToggleOptions{Selected: on, OnChange: setOn.Set}
//
// To follow an uncontrolled control, read the Selected field of its
// result.
type ToggleOptions struct {
	// Selected is the state of a controlled control; it is used only when
	// OnChange is set.
	Selected bool
	// DefaultSelected is the initial state of an uncontrolled control.
	DefaultSelected bool
	// OnChange is called with the next state when the user toggles the
	// control. Setting it makes the control controlled.
	OnChange func(selected bool)
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
	internal, setInternal := tela.UseState(o.DefaultSelected)
	controlled := o.OnChange != nil
	selected = internal
	if controlled {
		selected = o.Selected
	}
	toggle = func() {
		if o.Disabled || o.ReadOnly {
			return
		}
		if !controlled {
			setInternal.Update(func(v bool) bool { return !v })
			return
		}
		o.OnChange(!selected)
	}
	return selected, toggle
}

// UseToggleButton returns the props of a button that is on or off (React
// Aria's useToggleButton): [UseButton] plus the state, shown as
// `aria-pressed`, so the `aria-pressed:` variants style it. Enter, Space,
// a click or a tap toggles it.
//
//	bold, setBold := tela.UseState(false)
//	tb := aria.UseToggleButton(aria.ToggleOptions{Selected: bold, OnChange: setBold.Set})
//	return Button(tb.Props, Class("rounded px-2 aria-pressed:bg-accent"), Text("B"))
func UseToggleButton(o ToggleOptions) ToggleResult {
	selected, toggle := useToggleState(o)
	id := useToggleID(o)
	props, pressed, ref := usePressCore(pressConfig{
		PressOptions: PressOptions{OnPress: func(PressEvent) { toggle() }, Disabled: o.Disabled, Ref: o.Ref},
		enter:        true,
	})
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
