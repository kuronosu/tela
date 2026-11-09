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
	// NoLongPress keeps a touch or a pen that stays down from opening the
	// menu. The long press is recognized for touch contacts and pens only
	// (a mouse button held on the area, or on a child, clicks as usual),
	// and it wins over the click of that touch: turn it off for an area
	// whose content is held down with a finger for something else.
	NoLongPress bool
	// Intercept, if not nil, is asked before the menu opens (a right
	// click, a long press, the keys, OpenAt), with the point it would open
	// at in window coordinates: the pointer, or for the keys the
	// bottom-left corner of the focused element (the area's top-left
	// corner when the area has no focus). When it returns true it has taken the opening over, by
	// showing the OS's own context menu there: the hook then neither opens
	// the menu nor calls OnOpenChange. When it returns false, the menu
	// opens as usual, in the same event. A menu that is open already is
	// only moved, without asking. kit's ContextMenu uses it for native
	// menus (ADR-215).
	Intercept func(x, y float32) (handled bool)
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
	// (opened with a mouse); a move of the open menu keeps it. Pass it to
	// [MenuOptions].FocusStrategy.
	FocusStrategy string
	// Opening numbers the openings of the menu: it changes each time the
	// menu opens, by a gesture, OpenAt or SetOpen(true) (a right click
	// elsewhere while it is open closes it and opens it again, as a new
	// opening) and not when an open menu is moved
	// (a long press or OpenAt while it is open). Key the menu's component
	// with it, tela.WithKey(menu, cm.Opening), so that each opening mounts
	// a new menu that takes the focus while the previous one exits in a
	// Presence at its own point.
	Opening int
	// Anchor returns the virtual rectangle the menu is placed at, in window
	// coordinates: a point where the pointer was or the area's top-left
	// corner. It is [AnchorRect]'s argument, for a menu placed with
	// [UseFloating] instead of [UsePopover]. Each build's Anchor belongs to
	// the opening that build saw: it follows the menu while it is moved
	// (a long press or OpenAt while it is open) and keeps its point once
	// the menu closes, so that a menu exiting in a Presence stays where it
	// was when the next one opens elsewhere.
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
// shifted to stay in the window), through [ContextMenuTriggerResult.Anchor],
// so a menu exiting in a Presence stays at its own point. The popover is
// not modal, so that a right click on the area while the menu is open
// reaches it: the press closes the menu, as any press outside it does (a
// right click too), and the right click opens it again at the new point.
// Change what you need in the value returned (Offset; append to Floating,
// whose first option is the anchor).
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
//   - a right click, or a Control+click under the Mac flavor, at the
//     pointer (the focus stays where it was, and the menu itself takes
//     it),
//   - a touch or a pen that stays down for 500 ms, at the finger, with the
//     first item focused (Radix's rule; a mouse button held down is not a
//     long press here, so it clicks what it is on as usual),
//   - Shift+F10 and the ContextMenu key, with the first item focused, at
//     the top-left corner of the area.
//
// A key opens the menu at the element that has the focus, the area itself
// or an element inside it: below it (its bounds are the anchor, cut to the
// area's), as a browser opens the menu of the keyboard at the focused
// element; the area's corner when nothing in it has the focus.
//
// The event's default is prevented and its propagation stopped, so the
// innermost area with a context menu wins. The menu is rendered by the
// caller, in a Portal, and [UsePopover] places it at the point with
// [AnchorRect], flipped and shifted to stay in the window; Escape, a
// press outside it with any mouse button (Radix's DismissableLayer), the
// focus leaving and choosing an item close it, and the focus goes back to
// where it was. A right click elsewhere on the area while the menu is
// open closes it and opens it again at the new point, as a new menu: it
// takes the focus as when it opens (the right click's contextmenu event
// comes after the release that closes the menu). A touch or a pen that
// stays down elsewhere on the area while the menu is open moves the open
// menu there, and its release does not close it (Radix); a quick tap
// there closes it, as any press outside does. A right click or a long
// press in another area with a context menu closes this one and opens
// that one, and a right click outside every area just closes it. A menu
// rendered in a Presence (to fade out) comes back as the same element
// when it opens again while it exits, at the new point and without taking
// the focus: key it with the result's Opening, as kit's ContextMenu does,
// and the exiting one fades out at its own point while a new one opens and
// takes the focus.
//
//	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
//	return Fragment(
//	    Div(cm.TriggerProps, Class("h-48 w-full rounded-md border"), Text("Right click here")),
//	    Portal(Presence(If(cm.Open, tela.WithKey(fileMenu{cm}, cm.Opening)))))
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
// The area is not focusable by itself (Radix's model); the keys reach it
// from the element inside it that has the focus. An area that should be
// reached with Tab is made focusable by the caller: h.TabIndex(0) with a
// role and a name (h.Role("region"), h.AriaLabel("Canvas")) and a
// focus-visible: ring.
func UseContextMenuTrigger(o ContextMenuTriggerOptions) ContextMenuTriggerResult {
	open, st := useBoolState(o.Open, false, o.OnOpenChange)
	strategy, setStrategy := tela.UseState("")
	// pos is the point of the menu and the opening it belongs to: a Value,
	// so that the float pass places the menu in the frame that shows it
	// and a move while it is open re-places it without a build.
	pos := tela.UseValue(ctxPoint{})
	ownArea, ownMenu := tela.UseElementRef(), tela.UseElementRef()
	ref, menuRef := o.Ref, o.MenuRef
	if ref == (tela.ElementRef{}) {
		ref = ownArea
	}
	if menuRef == (tela.ElementRef{}) {
		menuRef = ownMenu
	}
	openAt := func(at tela.Rect, s string) {
		if o.Disabled {
			return
		}
		if !st.cur && o.Intercept != nil && o.Intercept(at.X, at.Y+at.Height) {
			return
		}
		p := pos.Latest()
		if !st.cur {
			// A new opening (a right click elsewhere closed the menu at its
			// release, before its contextmenu event): an exiting menu keeps
			// the point of its own. A move of the open menu (a long press
			// or OpenAt while it is open) keeps the opening and its focus.
			p.gen++
			setStrategy.Set(s)
		}
		p.at = at
		pos.Set(p)
		st.request(true)
	}
	point := func(x, y float32) tela.Rect { return tela.Rect{X: x, Y: y} }
	ps := []tela.Prop{
		h.Ref(ref),
		h.OnContextMenu(func(e tela.MouseEvent) {
			if o.Disabled {
				return
			}
			e.PreventDefault()
			e.StopPropagation()
			openAt(point(e.X, e.Y), "")
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
			openAt(keyAnchor(ref), FocusFirst)
		}),
	}
	if !o.NoLongPress && !o.Disabled {
		// Touch and pen only, and a mouse press does not even take part in
		// the gesture arena, so a held mouse button still clicks
		// (h.OnLongPress would recognize the mouse too).
		ps = append(ps, core.OnTouchLongPress(func(e tela.PointerEvent) {
			e.StopPropagation()
			openAt(point(e.X, e.Y), FocusFirst)
			// With the menu open, the pointerdown of this press was outside
			// it, so its release would close the menu just moved here: the
			// press is not an interact-outside one for the menu (Radix
			// keeps it open at the new point). The other overlays it
			// started outside of, another area's menu among them, still
			// close at the release.
			core.KeepOutsidePress(e, menuRef)
		}))
	}
	anchor := openingAnchor(pos)
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
		Opening:       int(pos.Latest().gen),
		Anchor:        anchor,
		OpenAt:        func(x, y float32) { openAt(point(x, y), "") },
		SetOpen: func(v bool) {
			if v && !st.cur {
				// A new opening at the last point, as OpenAt's: an exiting
				// menu is not revived as this one.
				p := pos.Latest()
				p.gen++
				pos.Set(p)
				setStrategy.Set("")
			}
			st.request(v)
		},
		Close:   func() { st.request(false) },
		Ref:     ref,
		MenuRef: menuRef,
	}
}

// keyAnchor is where a menu opened from the keyboard is placed: the
// bounds of the element that has the focus, the area or one inside it,
// cut to the area's (a browser opens the keyboard's context menu at the
// focused element); the area's top-left corner when the focus is
// elsewhere or the element is out of the area's view.
func keyAnchor(area tela.ElementRef) tela.Rect {
	a := area.Bounds()
	corner := tela.Rect{X: a.X, Y: a.Y}
	f, ok := core.ActiveElement(area)
	if !ok || !core.ContainsFocus(area) {
		return corner
	}
	b := f.Bounds()
	x0, y0 := max(b.X, a.X), max(b.Y, a.Y)
	x1, y1 := min(b.X+b.Width, a.X+a.Width), min(b.Y+b.Height, a.Y+a.Height)
	if x1 < x0 || y1 < y0 {
		return corner
	}
	return tela.Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

// ctxPoint is the point a context menu is placed at, and the number of
// the opening it was set by.
type ctxPoint struct {
	gen uint64
	at  tela.Rect
}

// openingAnchor returns the Anchor of a build of UseContextMenuTrigger:
// the point of the opening the build saw, which follows the menu while
// that opening lasts (moved by a long press or OpenAt) and then keeps its
// last value. A menu in a Presence keyed per opening (kit's ContextMenu)
// keeps the props of its last build while it exits, and so this Anchor:
// it fades out where it was, while the next opening's menu is placed at
// the new point. The read in it is a Get, so the float pass that calls it
// is redone when the point moves, with no build. Call it on the UI
// thread.
func openingAnchor(pos *tela.Value[ctxPoint]) func() tela.Rect {
	// Latest, not Get: reading the point in the build would build the
	// component that holds the area (its children are the area) every
	// time the menu moves.
	cur := pos.Latest()
	gen, at := cur.gen, cur.at
	return func() tela.Rect {
		if p := pos.Get(); p.gen == gen {
			at = p.at
		}
		return at
	}
}
