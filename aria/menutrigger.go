// Portions of this file follow React Aria's useMenuTrigger and
// react-stately's useMenuTriggerState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/menu and
// packages/@react-stately/menu), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// Where the focus goes when a menu opens ([MenuTriggerResult].FocusStrategy).
const (
	// FocusFirst focuses the first item: the menu opened with the keyboard
	// (ArrowDown, Enter, Space), a long press or a virtual click.
	FocusFirst = "first"
	// FocusLast focuses the last item: the menu opened with ArrowUp.
	FocusLast = "last"
)

// MenuTriggerOptions configures [UseMenuTrigger].
type MenuTriggerOptions struct {
	// Trigger is how a pointer opens the menu: "press" (the default), at
	// the press of a mouse or a pen and at the release of a touch, or
	// "longPress", when the press lasts, which leaves the quick press of the
	// trigger to OnPress; the keyboard then opens it with Alt+ArrowDown and
	// Alt+ArrowUp only. Another value panics in dev mode.
	Trigger string
	// Type is what the trigger opens: "menu" (the default) or "listbox".
	Type string
	// Open points to the open state of a controlled menu; nil leaves the
	// state to the hook, starting at DefaultOpen. OnOpenChange is called
	// with the next state: for a controlled menu it is how the change is
	// asked for, for an uncontrolled one a notification.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Disabled makes the trigger inert and disabled.
	Disabled bool
	// OnPress is the trigger's own press with Trigger "longPress"; it does
	// not run after a long press that opened the menu.
	OnPress func(PressEvent)
	// Ref is the trigger's ref and MenuRef the menu's, for hooks used
	// together with others on the same element; the zero value makes the
	// hook create its own.
	Ref, MenuRef tela.ElementRef
}

// MenuTriggerResult is what [UseMenuTrigger] returns.
type MenuTriggerResult struct {
	// TriggerProps go to the trigger: the press (and long press), the
	// keyboard that opens the menu, the "button" role, a tab stop,
	// aria-haspopup, aria-expanded and, while open, aria-controls.
	TriggerProps tela.Prop
	// MenuProps go to the menu's element: the ref, aria-labelledby (the
	// trigger) and a focus scope that gives the focus back to the trigger
	// when the menu closes. The menu's other hooks and [UsePopover] share
	// MenuRef.
	MenuProps tela.Prop
	// Open is whether the menu is open.
	Open bool
	// FocusStrategy is where the focus goes when the menu opens: [FocusFirst],
	// [FocusLast], or "" for the menu itself (opened with a mouse or a
	// touch). The menu focuses accordingly when it mounts.
	FocusStrategy string
	// SetOpen asks for a state, Toggle for the other one and Close for
	// closed. A menu closes itself when an item is chosen by calling Close.
	SetOpen func(open bool)
	Toggle  func()
	Close   func()
	// Pressed is true while the trigger is pressed.
	Pressed bool
	// Ref is the trigger's ref and MenuRef the menu's.
	Ref, MenuRef tela.ElementRef
}

// UseMenuTrigger holds the open state of a menu and returns the props of
// its trigger (React Aria's useMenuTrigger with useMenuTriggerState): a
// press opens it, and so do Enter, Space and ArrowDown (focusing the first
// item) and ArrowUp (the last), or a long press with Trigger "longPress".
//
//	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
//	pop := aria.UsePopover(aria.PopoverOptions{
//	    Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
//	    Placement: "bottom-start", Modal: true,
//	})
//	return Fragment(
//	    Button(mt.TriggerProps, Text("Actions")),
//	    Portal(If(mt.Open, Div(pop.Props, mt.MenuProps, Role("menu"), ...))))
//
// The menu itself, its items and keyboard navigation belong to the menu
// collection hook (UseMenu); this hook gives it the focus strategy and
// Close, to call when an item is chosen.
func UseMenuTrigger(o MenuTriggerOptions) MenuTriggerResult {
	longPress := false
	switch o.Trigger {
	case "", "press":
	case "longPress":
		longPress = true
	default:
		if core.DevNow() {
			panic("aria: MenuTriggerOptions.Trigger " + quote(o.Trigger) + ": use \"press\" (the default) or \"longPress\"")
		}
	}
	popup := o.Type
	if popup == "" {
		popup = "menu"
	}
	if popup != "menu" && popup != "listbox" && core.DevNow() {
		panic("aria: MenuTriggerOptions.Type " + quote(o.Type) + ": use \"menu\" (the default) or \"listbox\"")
	}
	open, st := useBoolState(o.Open, o.DefaultOpen, o.OnOpenChange)
	strategy, setStrategy := tela.UseState("")
	ownTrigger, ownMenu := tela.UseElementRef(), tela.UseElementRef()
	ref, menuRef := o.Ref, o.MenuRef
	if ref == (tela.ElementRef{}) {
		ref = ownTrigger
	}
	if menuRef == (tela.ElementRef{}) {
		menuRef = ownMenu
	}
	opened := &tela.UseRef(false).Current // a long press opened the menu: its release is not a press
	openWith := func(s string) {
		if o.Disabled {
			return
		}
		setStrategy.Set(s)
		st.request(true)
	}
	toggleWith := func(s string) {
		if o.Disabled {
			return
		}
		setStrategy.Set(s)
		st.request(!st.cur)
	}
	pressOpts := PressOptions{Disabled: o.Disabled, Ref: ref}
	if longPress {
		pressOpts.OnPressStart = func(PressEvent) { *opened = false }
		pressOpts.OnPress = func(e PressEvent) {
			if !*opened && o.OnPress != nil {
				o.OnPress(e)
			}
		}
	} else {
		pressOpts.OnPressStart = func(e PressEvent) {
			// Like a native menu button, a mouse or a pen opens it on the
			// press; a touch on the release (OnPress). A virtual click is
			// an assistive technology: focus the first item.
			if e.PointerType == PointerTouch || e.PointerType == PointerKeyboard {
				return
			}
			s := ""
			if e.PointerType == PointerVirtual {
				s = FocusFirst
			}
			if st.cur {
				st.request(false) // a non-modal menu: the trigger toggles it
				return
			}
			openWith(s)
		}
		pressOpts.OnPress = func(e PressEvent) {
			if e.PointerType == PointerTouch {
				toggleWith("")
			}
		}
	}
	props, pressed, _ := usePressCore(pressConfig{PressOptions: pressOpts, enter: true})
	onKeyDown := func(e tela.KeyboardEvent) {
		if o.Disabled || !ref.Focused() {
			return
		}
		if longPress && !e.Alt {
			return
		}
		switch e.Key {
		case tela.KeyEnter, " ":
			if longPress {
				return
			}
			fallthrough
		case tela.KeyArrowDown:
			e.PreventDefault()
			e.StopPropagation()
			toggleWith(FocusFirst)
		case tela.KeyArrowUp:
			e.PreventDefault()
			e.StopPropagation()
			toggleWith(FocusLast)
		}
	}
	ps := []tela.Prop{props, buttonProps(o.Disabled), h.OnKeyDown(onKeyDown), h.AriaHasPopup(popup), h.AriaExpanded(open)}
	if longPress && !o.Disabled {
		ps = append(ps, h.OnLongPress(func() {
			*opened = true
			openWith(FocusFirst)
		}))
	}
	if open {
		ps = append(ps, h.AriaControls(menuRef))
	}
	return MenuTriggerResult{
		TriggerProps: core.MergeProps(ps...),
		MenuProps: core.MergeProps(h.Ref(menuRef), h.AriaLabelledBy(ref),
			core.Scope(core.ScopeOptions{Restore: true})),
		Open:          open,
		FocusStrategy: strategy,
		SetOpen:       func(v bool) { st.request(v) },
		Toggle:        func() { toggleWith("") },
		Close:         func() { st.request(false) },
		Pressed:       pressed,
		Ref:           ref,
		MenuRef:       menuRef,
	}
}
