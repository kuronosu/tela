// Portions of this file follow React Aria's useMenu, useMenuItem and
// useMenuSection, and react-stately's useTreeState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/menu and
// packages/@react-stately/tree), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
)

// MenuOptions configures [UseMenu]. The items are keyed like those of a
// list box: see [ListBoxOptions] for Keys, TextValue, DisabledKeys,
// Selected, DefaultSelected, OnSelectionChange, ShouldFocusWrap and
// Direction.
type MenuOptions[K comparable] struct {
	Keys []K
	// TextValue is the text of an item, for typeahead; the key as
	// fmt.Sprint prints it when nil. See [ListBoxOptions].
	TextValue    func(K) string
	DisabledKeys []K
	// SelectionMode is "none" (the default: the items are commands,
	// "menuitem"), "single" (radio items, "menuitemradio", of which one
	// is selected) or "multiple" (checkbox items, "menuitemcheckbox").
	// A single selection keeps one item selected once there is one.
	SelectionMode     string
	Selected          Selection[K]
	DefaultSelected   Selection[K]
	OnSelectionChange func(Selection[K])
	// OnAction is called with the key of an item when it is chosen (a
	// click, Enter or Space), whether the menu selects or not.
	OnAction func(K)
	// OnClose is called when choosing an item should close the menu: pass
	// the trigger's Close. See CloseOnSelect.
	OnClose func()
	// CloseOnSelect decides whether choosing an item calls OnClose: nil
	// (the default) closes the menu unless the selection is "multiple",
	// where several items are toggled in a row; point to true or false to
	// decide.
	CloseOnSelect *bool
	// AutoFocus focuses the menu when it mounts, and FocusStrategy
	// (FocusFirst, FocusLast, from the trigger's result) the item that
	// takes the focus; a selected item wins. A menu that mounts when it
	// opens passes AutoFocus: true and the trigger's FocusStrategy; a
	// non-empty FocusStrategy alone implies AutoFocus.
	AutoFocus     bool
	FocusStrategy string
	// ShouldFocusWrap makes the arrow keys go around the ends.
	ShouldFocusWrap bool
	Direction       string
	// DisallowTypeAhead turns off typeahead.
	DisallowTypeAhead bool
	// AriaLabel names the menu; the trigger's MenuProps already name it
	// by the trigger.
	AriaLabel string
	// Ref is the menu's ref: pass the trigger's MenuRef, which its
	// MenuProps attach too.
	Ref tela.ElementRef

	// SubmenuKeys are the items that open a submenu (React Aria's
	// SubmenuTrigger, Radix's MenuSubTrigger): they are not chosen, they
	// open their submenu, whose state [MenuResult.SubmenuTrigger] returns.
	// ArrowRight (ArrowLeft under "rtl"), Enter, Space and a click open it,
	// the keyboard with its first item focused; the pointer resting on the
	// item for [SubmenuDelay] opens it too. The focus moving to another
	// item of this menu closes it, except on the way to the submenu: while
	// the mouse moves toward it, the items it crosses are not hovered
	// (React Aria's pointer grace).
	SubmenuKeys []K
	// Submenu makes this menu the submenu of a parent menu's item: pass the
	// parent's SubmenuTrigger(key). Ref, FocusStrategy and OnClose then
	// default to its MenuRef, FocusStrategy and CloseAll (choosing an item
	// closes the whole menu), and its own submenus chain to it. Direction
	// is still this menu's own: pass the parent's.
	Submenu SubmenuTriggerResult
	// ShouldCloseOnInteractOutside is, for a menu that is not a submenu,
	// its popover's rule for a press outside it: pass the
	// PopoverOptions.ShouldCloseOnInteractOutside of the menu's popover
	// (a menu bar's PopoverOptions has one). Its submenus follow it, so a
	// press it lets through, such as one on another button of a menu bar
	// (which opens that menu), reaches its target while a submenu is open
	// too, instead of only closing the menu.
	ShouldCloseOnInteractOutside func(tela.PointerEvent) bool
}

// MenuResult is what [UseMenu] returns.
type MenuResult[K comparable] struct {
	Collection[K]
	// Props go to the menu's element: the "menu" role, the ref, the key
	// handler and a tab stop for the menu itself while no item is
	// focused. Give them next to the trigger's MenuProps and the popover's
	// props.
	Props tela.Prop
	// Selected is the current selection.
	Selected Selection[K]
	// Ref is the ref attached to the menu by Props.
	Ref tela.ElementRef
}

// SubmenuTrigger returns the state of the submenu that the item k opens
// (an item of [MenuOptions].SubmenuKeys; the zero value for another):
// render the submenu while its Open is true, in a Portal that is a child
// of the menu's element (so that it is inside the menu for the focus and
// for presses; not inside the item's element, whose handlers would get the
// submenu's pointer events), with [UsePopover](sub.PopoverOptions()) and a
// [UseMenu] that has Submenu: sub.
//
//	sub := menu.SubmenuTrigger("share")
//	Div(menu.Props, ...,
//	    Div(menu.ItemProps("share"), Key("share"), Text("Share")),
//	    Portal(If(sub.Open, shareMenu{sub})))
//
//	type shareMenu struct{ sub aria.SubmenuTriggerResult }
//
//	func (m shareMenu) Build() tela.Node {
//	    pop := aria.UsePopover(m.sub.PopoverOptions())
//	    menu := aria.UseMenu(aria.MenuOptions[string]{Keys: targets, OnAction: share, Submenu: m.sub})
//	    return Div(pop.Props, m.sub.MenuProps, menu.Props, Class("w-40 rounded-md border bg-popover p-1"), ...)
//	}
func (r MenuResult[K]) SubmenuTrigger(k K) SubmenuTriggerResult { return r.c.submenuTrigger(k) }

// UseMenu holds the state of a menu and returns the props of its element
// and its items (React Aria's useMenu, useMenuItem and useMenuSection with
// useTreeState). It is the collection of a menu button: it takes the
// trigger's results, focuses the first or last item as the trigger says,
// chooses with Enter, Space and clicks, closes the menu, and the focus
// returns to the trigger by the scope of the trigger's MenuProps (UseMenu
// adds none).
//
// Call it in the component that is rendered while the menu is open: it
// focuses an item when it mounts, so a menu that stays mounted while closed
// would not focus again.
//
//	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
//	pop := aria.UsePopover(aria.PopoverOptions{
//	    Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
//	    Placement: "bottom-start", Modal: true,
//	})
//	return Fragment(
//	    Button(mt.TriggerProps, Text("Actions")),
//	    Portal(If(mt.Open, actionMenu{mt, pop})))
//
//	type actionMenu struct{ mt aria.MenuTriggerResult; pop aria.PopoverResult }
//
//	func (m actionMenu) Build() tela.Node {
//	    menu := aria.UseMenu(aria.MenuOptions[string]{
//	        Keys: actions, TextValue: strings.ToUpper,
//	        OnAction: run, OnClose: m.mt.Close,
//	        AutoFocus: true, FocusStrategy: m.mt.FocusStrategy, Ref: m.mt.MenuRef,
//	    })
//	    return Div(m.pop.Props, m.mt.MenuProps, menu.Props, Class("w-48 rounded-md border bg-popover p-1"),
//	        h.Range(len(actions), func(i int) tela.Node {
//	            return Div(menu.ItemProps(actions[i]), Key(actions[i]),
//	                Class("rounded px-2 py-1.5 focus-visible:bg-accent aria-disabled:opacity-50"), Text(actions[i]))
//	        }))
//	}
//
// Items with SelectionMode "single" or "multiple" are radio and checkbox
// items: they print as `menuitemradio "Name" [checked]` and
// `menuitemcheckbox "Name" [checked]`, and `aria-checked:` styles them. The
// mouse focuses the item under it, as in a native menu. Disabled items are
// skipped. Items in SubmenuKeys open submenus (see
// [MenuResult.SubmenuTrigger]).
func UseMenu[K comparable](o MenuOptions[K]) MenuResult[K] {
	const hook = "UseMenu: MenuOptions."
	mode := checkChoice(hook, "SelectionMode", o.SelectionMode, "none", "none", "single", "multiple")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	strategy := checkChoice(hook, "FocusStrategy", o.FocusStrategy, "", FocusFirst, FocusLast)
	auto, onClose, ref := o.AutoFocus, o.OnClose, o.Ref
	if sub := o.Submenu; sub.lvl != nil {
		if ref == (tela.ElementRef{}) {
			ref = sub.MenuRef
		}
		if strategy == "" {
			strategy = sub.FocusStrategy
		}
		if onClose == nil {
			onClose = sub.CloseAll
		}
	}
	c := useCollection(collConfig[K]{
		kind: kindMenu, role: "menu",
		keys: o.Keys, textValue: o.TextValue, disabled: o.DisabledKeys,
		mode: mode, behavior: "toggle",
		selected: o.Selected, defaultSelected: o.DefaultSelected, onSelectionChange: o.OnSelectionChange,
		// A radio item can not be unchecked by choosing it again.
		disallowEmpty: mode == "single",
		wrap:          o.ShouldFocusWrap, orientation: "vertical", rtl: dir == "rtl",
		disableType:  o.DisallowTypeAhead,
		focusOnHover: true,
		autoFocus:    auto, strategy: strategy,
		onAction: o.OnAction, onClose: onClose, closeOnSelect: o.CloseOnSelect,
		subKeys: o.SubmenuKeys, parent: o.Submenu, outside: o.ShouldCloseOnInteractOutside,
		label: o.AriaLabel, ref: ref,
	})
	return MenuResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref}
}
