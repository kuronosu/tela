// Portions of this file follow React Aria's useMenuTrigger and
// react-stately's useMenuTriggerState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/menu and
// packages/@react-stately/menu), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. The behavior also follows Radix UI's
// ContextMenu (MIT; no code copied). See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// ContextMenuTriggerOptions configures [UseContextMenuTrigger].
type ContextMenuTriggerOptions struct {
	// Open points to the open state of a controlled menu; nil leaves the
	// state to the hook, which starts closed. OnOpenChange is called with
	// the next state: for a controlled menu it is how the change is asked
	// for, for an uncontrolled one a notification. A menu opens at a point,
	// so there is no DefaultOpen.
	Open         *bool
	OnOpenChange func(open bool)
	// Disabled makes the area ignore the gestures that open the menu: the
	// browser's (here, the platform's) own, and an outer context menu's,
	// take over.
	Disabled bool
	// NoLongPress keeps a touch that stays down from opening the menu. The
	// long press is recognized for every pointer of the area, so a mouse
	// button held for 500 ms on the area, or on a child that is not itself
	// a long-press target, does not click; turn it off for an area whose
	// content is used that way.
	NoLongPress bool
	// Ref is the area's ref and MenuRef the menu's, for hooks used together
	// with others on the same element; the zero value makes the hook
	// create its own.
	Ref, MenuRef tela.ElementRef
}

// ContextMenuTriggerResult is what [UseContextMenuTrigger] returns.
type ContextMenuTriggerResult struct {
	// TriggerProps go to the area that has the context menu: the ref and
	// the gestures that open the menu (see [UseContextMenuTrigger]).
	TriggerProps tela.Prop
	// MenuProps go to the menu's element: the ref and a focus scope that
	// gives the focus back when the menu closes. The menu's other hooks and
	// [UsePopover] share MenuRef. The menu has no name of its own: give
	// [MenuOptions].AriaLabel one.
	MenuProps tela.Prop
	// Open is whether the menu is open.
	Open bool
	// FocusStrategy is where the focus goes when the menu opens:
	// [FocusFirst] for a key or a long press, "" for the menu itself
	// (opened with a mouse). Pass it to [MenuOptions].FocusStrategy.
	FocusStrategy string
	// Anchor returns the virtual rectangle the menu is placed at, in window
	// coordinates: a point where the pointer was or the area's top-left
	// corner. It is [AnchorRect]'s argument, for a menu placed with
	// [UseFloating] instead of [UsePopover].
	Anchor func() tela.Rect
	// OpenAt opens the menu at a point of the window (a click handler of
	// your own, a "More" button). Close closes it, and a menu closes itself
	// when an item is chosen by calling Close. SetOpen asks for a state.
	OpenAt  func(x, y float32)
	SetOpen func(open bool)
	Close   func()
	// Ref is the area's ref and MenuRef the menu's.
	Ref, MenuRef tela.ElementRef
}

// PopoverOptions returns the options of [UsePopover] for the menu: anchored
// at the point the menu was opened with ("bottom-start", flipped and
// shifted to stay in the window). The popover is not modal, so that a
// right click on the area while the menu is open reaches it and moves the
// menu to the new point; a primary press outside the menu closes it.
// Change what you need in the value returned (Offset, Floating).
func (r ContextMenuTriggerResult) PopoverOptions() PopoverOptions {
	return PopoverOptions{
		Ref:       r.MenuRef,
		Open:      r.Open,
		OnClose:   r.Close,
		Placement: "bottom-start",
		Floating:  []FloatingOption{AnchorRect(r.Anchor)},
	}
}

// UseContextMenuTrigger holds the open state of a context menu and returns
// the props of its area, which opens the menu at the pointer (the
// contextmenu event of the platform, Radix UI's ContextMenu):
//
//   - a right click, at the pointer (the focus stays where it was, and the
//     menu itself takes it),
//   - a touch that stays down for 500 ms (h.OnLongPress), at the finger,
//     with the first item focused,
//   - Shift+F10 and the ContextMenu key, with the first item focused, at
//     the top-left corner of the area.
//
// The event's default is prevented and its propagation stopped, so the
// innermost area with a context menu wins. The menu is rendered by the
// caller, in a Portal, and [UsePopover] places it at the point with
// [AnchorRect], flipped and shifted to stay in the window; Escape, a
// primary press outside, the focus leaving and choosing an item close it,
// and the focus goes back to where it was. A right click on the area while
// the menu is open moves it to the new point; one outside the area does
// nothing (the overlay stack only dismisses for primary presses).
//
//	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
//	return Fragment(
//	    Div(cm.TriggerProps, Class("h-48 w-full rounded-md border"), Text("Right click here")),
//	    Portal(If(cm.Open, fileMenu{cm})))
//
//	type fileMenu struct{ cm aria.ContextMenuTriggerResult }
//
//	func (m fileMenu) Build() tela.Node {
//	    pop := aria.UsePopover(m.cm.PopoverOptions())
//	    menu := aria.UseMenu(aria.MenuOptions[string]{
//	        Keys: actions, OnAction: run, OnClose: m.cm.Close, AriaLabel: "Actions",
//	        AutoFocus: true, FocusStrategy: m.cm.FocusStrategy, Ref: m.cm.MenuRef,
//	    })
//	    return Div(pop.Props, m.cm.MenuProps, menu.Props, Class("w-48 rounded-md border bg-popover p-1"), ...)
//	}
//
// The menu is the one of a dropdown menu: [UseMenu] takes the same results.
// The area is not focusable by itself; the keys reach it from the element
// inside it that has the focus, and a keyboard menu opens at the area's
// corner, not at that element.
func UseContextMenuTrigger(o ContextMenuTriggerOptions) ContextMenuTriggerResult {
	open, st := useBoolState(o.Open, false, o.OnOpenChange)
	strategy, setStrategy := tela.UseState("")
	pos := tela.UseValue(tela.Rect{})
	ownArea, ownMenu := tela.UseElementRef(), tela.UseElementRef()
	ref, menuRef := o.Ref, o.MenuRef
	if ref == (tela.ElementRef{}) {
		ref = ownArea
	}
	if menuRef == (tela.ElementRef{}) {
		menuRef = ownMenu
	}
	openAt := func(x, y float32, s string) {
		if o.Disabled {
			return
		}
		setStrategy.Set(s)
		pos.Set(tela.Rect{X: x, Y: y})
		st.request(true)
	}
	ps := []tela.Prop{
		h.Ref(ref),
		h.OnContextMenu(func(e tela.MouseEvent) {
			if o.Disabled {
				return
			}
			e.PreventDefault()
			e.StopPropagation()
			openAt(e.X, e.Y, "")
		}),
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if o.Disabled || e.Ctrl || e.Meta || e.Alt {
				return
			}
			if e.Key != tela.KeyContextMenu && !(e.Key == tela.KeyF10 && e.Shift) {
				return
			}
			e.PreventDefault()
			e.StopPropagation()
			if e.Repeat {
				return
			}
			b := ref.Bounds()
			openAt(b.X, b.Y, FocusFirst)
		}),
	}
	if !o.NoLongPress && !o.Disabled {
		ps = append(ps, h.OnLongPress(func(e tela.PointerEvent) {
			if e.Kind != tela.PointerTouch {
				return
			}
			e.StopPropagation()
			openAt(e.X, e.Y, FocusFirst)
		}))
	}
	return ContextMenuTriggerResult{
		TriggerProps: core.MergeProps(ps...),
		MenuProps: core.MergeProps(h.Ref(menuRef),
			core.Scope(core.ScopeOptions{Restore: true}),
			// A right click on the menu is not the area's.
			h.OnContextMenu(func(e tela.MouseEvent) {
				e.PreventDefault()
				e.StopPropagation()
			})),
		Open:          open,
		FocusStrategy: strategy,
		Anchor:        pos.Get,
		OpenAt:        func(x, y float32) { openAt(x, y, "") },
		SetOpen:       func(v bool) { st.request(v) },
		Close:         func() { st.request(false) },
		Ref:           ref,
		MenuRef:       menuRef,
	}
}
