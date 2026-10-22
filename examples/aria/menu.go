package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// MenuItem is one item of a menu.
type MenuItem struct {
	Key, Label string
	// Shortcut is shown at the right edge.
	Shortcut string
	// Icon is shown at the left; the zero Icon shows nothing.
	Icon        tela.Icon
	Disabled    bool
	Destructive bool
}

// MenuGroup is a section of a menu: its items under a heading, with a
// separator before it unless it is the first.
type MenuGroup struct {
	Name  string
	Items []MenuItem
}

// MenuButton is a button that opens a menu: UseMenuTrigger holds the open
// state and the keys that open it, UsePopover places and dismisses the
// menu, and UseMenu (in MenuList, which exists only while the menu is open)
// holds the focused item and the selection.
//
// Mode is "none" for a menu of commands (OnAction runs the chosen one and
// the menu closes) or "multiple" for checkbox items (Checked is the state
// and OnChecked its change; the menu stays open while several are toggled).
type MenuButton struct {
	Label  string
	Icon   tela.Icon
	Groups []MenuGroup
	Mode   string
	// Placement is where the menu goes around the button.
	Placement string
	OnAction  func(key string)
	Checked   aria.Selection[string]
	OnChecked func(aria.Selection[string])

	// Open starts the menu open and Focus is the item it focuses when it
	// does ("first" or "last"): a preview's seed.
	Open  bool
	Focus string
}

func (m MenuButton) Build() tela.Node {
	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{DefaultOpen: m.Open})
	popover := aria.UsePopover(aria.PopoverOptions{
		Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
		Placement: m.Placement, Offset: 6, Modal: true,
	})
	return Fragment(
		Button(mt.TriggerProps, Class(button, outline, focusRing),
			If(m.Icon.SVG != "", ic(m.Icon, "size-4")),
			Text(m.Label),
			ic(icons.ChevronDown, "size-4 text-muted-foreground")),
		Portal(Presence(If(mt.Open, MenuList{Button: m, Trigger: mt, Popover: popover}))),
	)
}

// MenuList is the menu of a MenuButton. It is rendered only while the menu is
// open, so UseMenu focuses an item when it mounts.
type MenuList struct {
	Button  MenuButton
	Trigger aria.MenuTriggerResult
	Popover aria.PopoverResult
}

func (m MenuList) Build() tela.Node {
	b := m.Button
	var keys, disabled []string
	byKey := map[string]MenuItem{}
	for _, g := range b.Groups {
		for _, it := range g.Items {
			keys = append(keys, it.Key)
			byKey[it.Key] = it
			if it.Disabled {
				disabled = append(disabled, it.Key)
			}
		}
	}
	strategy := m.Trigger.FocusStrategy
	if strategy == "" {
		strategy = b.Focus
	}
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: keys, DisabledKeys: disabled,
		TextValue:     func(k string) string { return byKey[k].Label },
		SelectionMode: b.Mode, Selected: b.Checked, OnSelectionChange: b.OnChecked,
		OnAction: b.OnAction, OnClose: m.Trigger.Close,
		AutoFocus: true, FocusStrategy: strategy, Ref: m.Trigger.MenuRef,
	})

	origin := "origin-top-left"
	if m.Popover.Side == "top" {
		origin = "origin-bottom-left"
	}
	var groups []tela.Child
	for i, g := range b.Groups {
		sec := menu.Section(g.Name)
		if i > 0 {
			groups = append(groups, Div(Role("separator"), Class("-mx-1 my-1 h-px bg-border")))
		}
		var items []tela.Child
		for _, it := range g.Items {
			items = append(items, renderItem(menu, it, b.Mode))
		}
		groups = append(groups, Div(sec.Props,
			Div(sec.HeadingProps, Class("px-2 py-1.5 text-xs font-medium text-muted-foreground"), Text(g.Name)),
			Fragment(items...)))
	}
	return Div(m.Popover.Props, m.Trigger.MenuProps, menu.Props, Exit(exitPop),
		Class(surface, pop, origin, "min-w-52 outline-none"),
		Fragment(groups...))
}

// renderItem is one item: a roving focus (the item's element has the focus,
// and the mouse moves it), shown by focus:.
func renderItem(menu aria.MenuResult[string], it MenuItem, mode string) tela.Node {
	checkbox := mode == "multiple"
	return Div(menu.ItemProps(it.Key), Key(it.Key),
		Class("relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none",
			"focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"),
		If(checkbox, Class("pl-8")),
		If(it.Destructive, Class("text-destructive focus:bg-destructive/10 focus:text-destructive")),
		If(checkbox && menu.IsSelected(it.Key),
			Span(Class("absolute left-2 flex size-4 items-center justify-center"), ic(icons.Check, "size-4"))),
		If(!checkbox && it.Icon.SVG != "", ic(it.Icon, "size-4 text-muted-foreground")),
		Span(Class("flex-1"), Text(it.Label)),
		If(it.Shortcut != "", Span(Class("ml-6 text-xs tracking-widest text-muted-foreground"), AriaHidden(true), Text(it.Shortcut))),
	)
}
