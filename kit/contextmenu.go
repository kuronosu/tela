// Portions of this file follow shadcn/ui, context-menu.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

func init() {
	s := defaultMenuStyle()
	// shadcn's ContextMenuLabel is text-foreground.
	s.Label += " text-foreground"
	defaultStyle.ContextMenu = s
}

// ContextMenu is a menu that opens where the user right-clicks inside an
// area, shadcn/ui's ContextMenu, built on aria.UseContextMenuTrigger: a
// right click opens it at the pointer, a touch that stays down opens it
// under the finger, and Shift+F10 or the ContextMenu key opens it, on its
// first item, at the area's top-left corner. The menu closes with Escape,
// a press outside it or choosing an item, and the focus returns to where
// it was. A right click inside the area while the menu is open moves it; in
// nested areas the innermost wins.
//
//	kit.ContextMenu[string]{
//	    AriaLabel: "Actions",
//	    Class:     "flex h-40 w-72 items-center justify-center rounded-md border border-dashed text-sm",
//	    Children:  h.Text("Right click here"),
//	    Items: []kit.MenuItem[string]{
//	        {Key: "back", Label: "Back", Shortcut: "⌘["},
//	        {Key: "reload", Label: "Reload", Shortcut: "⌘R"},
//	        {Kind: kit.MenuSeparator},
//	        {Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Show bookmarks", DefaultChecked: true},
//	    },
//	}
//
// The items are the [MenuItem]s of [DropdownMenu], with no submenus yet
// (ADR-141). The root element is the area, a div that holds Children; the
// area is not focusable by itself, the keys reach it from what inside it
// has the focus, and a keyboard menu opens at the area's corner, not at that
// element (ADR-133). Open and OnOpenChange are the usual controlled pair
// (a menu opens at a point, so there is no DefaultOpen).
type ContextMenu[K comparable] struct {
	// Children is the area: what a right click opens the menu on.
	Children tela.Component
	// Items are the entries of the menu, in order.
	Items []MenuItem[K]
	// OnSelect is called with the key of a plain item when it is chosen,
	// after the item's own OnSelect.
	OnSelect func(K)
	// Disabled leaves the area's right click to the platform and to an
	// outer context menu.
	Disabled bool
	// NoLongPress keeps a touch that stays down from opening the menu (a
	// mouse button held for half a second on the area otherwise does not
	// click either: see aria.ContextMenuTriggerOptions).
	NoLongPress bool
	// AriaLabel names the menu, which has no name of its own: the area is
	// not a label.
	AriaLabel string
	// CheckIcon and RadioIcon replace the check of a checkbox item and the
	// dot of a radio item.
	CheckIcon, RadioIcon tela.Icon

	// Open points to the open state of a controlled menu; nil leaves it to
	// the component. OnOpenChange is called with the next state.
	Open         *bool
	OnOpenChange func(open bool)

	// Class is added to the area (it has no classes of its own);
	// ContentClass is added after the recipe's Content classes, to the
	// menu's panel.
	Class        string
	ContentClass string
	// Props are more props for the area element.
	Props tela.Child
}

// Build returns the area and, in a portal, the menu.
func (c ContextMenu[K]) Build() tela.Node {
	st := &UseStyle().ContextMenu
	store := useMenuStore[K]()
	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{
		Open: c.Open, OnOpenChange: c.OnOpenChange, Disabled: c.Disabled, NoLongPress: c.NoLongPress,
	})
	pop := cm.PopoverOptions()
	pop.Floating = append(pop.Floating, menuFloating()...)
	panel := menuPanel[K]{
		component: "ContextMenu", style: st, items: c.Items, store: store, onSelect: c.OnSelect,
		popover: pop, menuProps: cm.MenuProps, ref: cm.MenuRef, strategy: cm.FocusStrategy, close: cm.Close,
		ariaLabel: c.AriaLabel, checkIcon: c.CheckIcon, radioIcon: c.RadioIcon, class: c.ContentClass,
	}
	return h.Fragment(
		h.Div(cm.TriggerProps, cls(c.Class), c.Props, c.Children),
		h.Portal(h.Presence(h.If(cm.Open, panel))),
	)
}
