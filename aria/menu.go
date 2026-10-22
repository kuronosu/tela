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
//	    Portal(If(mt.Open, tela.C(actionMenu, mt, pop))))
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
// skipped. Submenus are not part of this hook.
func UseMenu[K comparable](o MenuOptions[K]) MenuResult[K] {
	const hook = "UseMenu: MenuOptions."
	mode := checkChoice(hook, "SelectionMode", o.SelectionMode, "none", "none", "single", "multiple")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	strategy := checkChoice(hook, "FocusStrategy", o.FocusStrategy, "", FocusFirst, FocusLast)
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
		autoFocus:    o.AutoFocus, strategy: strategy,
		onAction: o.OnAction, onClose: o.OnClose, closeOnSelect: o.CloseOnSelect,
		label: o.AriaLabel, ref: o.Ref,
	})
	return MenuResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref}
}
