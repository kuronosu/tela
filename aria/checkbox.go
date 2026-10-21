// Portions of this file follow React Aria's useCheckbox, useSwitch and
// useToggle (https://github.com/adobe/react-spectrum,
// packages/@react-aria/checkbox, packages/@react-aria/switch and
// packages/@react-aria/toggle), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// UseCheckbox returns the props of a checkbox (React Aria's useCheckbox):
// press (a click or a tap toggles it, and so does Space, but not Enter,
// as in HTML), the "checkbox" role, `aria-checked`, a tab stop, the
// disabled and read-only states and an ID for a Label. The `checked:`
// variant, and `peer-checked:` and `group-checked:`, match it while it is
// selected (so do `aria-checked:` and its peer and group forms).
//
//	box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
//	return Div(Class("flex items-center gap-2"),
//	    Button(box.Props, Class("size-4 rounded border checked:bg-primary")),
//	    Label(For(box.ID), Text("Notifications")))
//
// Render a checkbox with a Button, so that a Label can name it and toggle
// it when it is clicked: Tela's Label does that for an Input, a Textarea
// or a Button.
//
// Selected, DefaultSelected and OnChange follow [ToggleOptions]. There is
// no indeterminate state yet.
func UseCheckbox(o ToggleOptions) ToggleResult { return useCheck(o, "checkbox") }

// UseSwitch returns the props of a switch (React Aria's useSwitch): the
// same as [UseCheckbox] with the "switch" role.
//
//	sw := aria.UseSwitch(aria.ToggleOptions{Selected: on, OnChange: setOn.Set})
//	return Button(sw.Props, Class("h-6 w-11 rounded-full bg-muted checked:bg-primary"),
//	    Span(Class("block size-5 rounded-full bg-white transition checked:translate-x-5")))
func UseSwitch(o ToggleOptions) ToggleResult { return useCheck(o, "switch") }

func useCheck(o ToggleOptions, role string) ToggleResult {
	selected, toggle := useToggleState(o)
	id := useToggleID(o)
	props, pressed, ref := usePressCore(pressConfig{
		PressOptions: PressOptions{OnPress: func(PressEvent) { toggle() }, Disabled: o.Disabled, Ref: o.Ref},
		// Enter does nothing on a checkbox: only Space and clicks toggle.
	})
	ps := []tela.Prop{h.Role(role), h.Disabled(o.Disabled), h.ID(id), h.AriaChecked(selected), readOnlyProp(o.ReadOnly)}
	if !o.Disabled {
		ps = append(ps, h.TabIndex(0))
	}
	return ToggleResult{
		Selected: selected, Pressed: pressed, ID: id, Ref: ref,
		Props: core.MergeProps(props, core.MergeProps(ps...)),
	}
}
