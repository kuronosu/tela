// Portions of this file follow React Aria's useHover
// (https://github.com/adobe/react-spectrum, packages/@react-aria/interactions),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// HoverEvent is passed to the callbacks of [HoverOptions].
type HoverEvent struct {
	// PointerType is [PointerMouse] or [PointerPen]: a touch never hovers.
	PointerType PointerType
	// Modifier keys held during the event; Meta is the Windows, Command or
	// Super key.
	Shift, Ctrl, Alt, Meta bool
}

// HoverOptions configures [UseHover].
type HoverOptions struct {
	// OnHoverStart is called when a mouse or a pen enters the element.
	OnHoverStart func(HoverEvent)
	// OnHoverEnd is called when it leaves, or when the element becomes
	// Disabled while hovered.
	OnHoverEnd func(HoverEvent)
	// OnHoverChange is called with true on a start and false on an end.
	OnHoverChange func(hovered bool)
	// Disabled turns hovering off: no callbacks, Hovered stays false.
	Disabled bool
}

// HoverResult is what [UseHover] returns.
type HoverResult struct {
	// Props go to the element: the pointer enter and leave handlers.
	Props tela.Prop
	// Hovered is true while a mouse or a pen is over the element or one of
	// its descendants.
	Hovered bool
}

// UseHover returns the props that track whether a mouse or a pen is over
// the element (React Aria's useHover). Unlike the `hover:` variant it
// reaches Go: show a tooltip, start a preview, drive an animation. It
// ignores touch: a finger does not hover, and the enter that a touch press
// makes is not a hover.
//
//	hover := aria.UseHover(aria.HoverOptions{})
//	return Div(hover.Props, If(hover.Hovered, Class("ring-2")), Text("Card"))
//
// Like the `hover:` variant, the element is hovered while the pointer is
// over it or its descendants.
func UseHover(o HoverOptions) HoverResult {
	hovered, setHovered := tela.UseState(false)
	hovering := &tela.UseRef(false).Current
	end := func(e tela.PointerEvent) {
		if !*hovering {
			return
		}
		*hovering = false
		if f := o.OnHoverEnd; f != nil {
			f(hoverEvent(e))
		}
		if f := o.OnHoverChange; f != nil {
			f(false)
		}
		setHovered.Set(false)
	}
	tela.UseEffect(func() func() {
		if o.Disabled && *hovering {
			end(tela.PointerEvent{})
		}
		return nil
	}, o.Disabled)
	return HoverResult{
		Hovered: hovered && !o.Disabled,
		Props: core.MergeProps(
			h.OnPointerEnter(func(e tela.PointerEvent) {
				if o.Disabled || e.Kind == tela.PointerTouch || *hovering {
					return
				}
				*hovering = true
				if f := o.OnHoverStart; f != nil {
					f(hoverEvent(e))
				}
				if f := o.OnHoverChange; f != nil {
					f(true)
				}
				setHovered.Set(true)
			}),
			h.OnPointerLeave(func(e tela.PointerEvent) {
				if e.Kind == tela.PointerTouch {
					return
				}
				end(e)
			}),
		),
	}
}

func hoverEvent(e tela.PointerEvent) HoverEvent {
	return HoverEvent{PointerType: pointerTypeOf(e.Kind), Shift: e.Shift, Ctrl: e.Ctrl, Alt: e.Alt, Meta: e.Meta}
}
