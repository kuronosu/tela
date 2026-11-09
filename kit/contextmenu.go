// Portions of this file follow shadcn/ui, context-menu.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/desktop"
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
// right click (a Control+click on a Mac) opens it at the pointer, a touch
// or a pen that stays down opens it under the finger, and Shift+F10 or the
// ContextMenu key opens it, on its first item, at the element that has
// the focus (its center, or a text field's caret, as Chromium does). The
// menu closes with Escape,
// a press outside it with any mouse button or choosing an item, and the
// focus returns to where it was. A right click elsewhere in the area while
// the menu is open closes it and opens a new one at the new point (Radix),
// a right click in another area opens that area's menu instead, and in
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
// The items are the [MenuItem]s of [DropdownMenu], submenus and roles
// included (Cut, Copy and Paste in the context menu of a text field act on
// the field). The root element is the area, a div that holds Children.
// Open and OnOpenChange are the usual controlled pair (a menu opens at a
// point, so there is no DefaultOpen).
//
// # The keyboard
//
// The area is not focusable: kit adds no tab stop (Radix's model). The
// keys reach it from the element inside it that has the focus, as in the
// browser, and the menu opens at that element. An area with nothing
// focusable inside (a canvas, a drop zone) opens from the keyboard only
// when it can take the focus itself: give it a tab stop, a role and a name
// in Props, and a focus ring in Class:
//
//	kit.ContextMenu[string]{
//	    Props: tela.MergeProps(h.TabIndex(0), h.Role("region"), h.AriaLabel("Canvas")),
//	    Class: "rounded-md border outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
//	    ...
//	}
//
// The menu then opens at the area's center, inside it, however large the
// area is. A list of items with selection
// inside the area (aria.UseListBox) selects the item that is right-clicked
// before the menu opens, unless it is part of the selection already, as
// Finder does.
//
// # Native context menu
//
// The menu is drawn in the kit's look unless Native (kit.NativeOn), or
// tela.App.NativeMenus when Native is left at kit.NativeAuto, asks for the
// OS's context menu (ADR-215). It then opens where the drawn one would
// (the pointer, or the focused element for Shift+F10 and the Menu key),
// and
// where the platform has no native context menu (Linux) the drawn menu
// opens instead, in the same event and frame. The area's own Native
// setting wins over the app's, so one menu can stay drawn while the
// others are native (kit.NativeOff).
//
// A native menu shows each item's Label, Shortcut (as its accelerator:
// text only), Disabled state and separators; checkbox items are check
// items and the items of a radio group check items with the chosen one
// checked. Icons, the destructive Variant, Inset, KeepOpen (the OS's menu
// always closes), Class, ContentClass and the check and radio icons are
// not shown, a label becomes a disabled item, and a group's items appear
// without it. Choosing an item does what it does in the drawn menu
// (OnSelect, OnCheckedChange, OnValueChange and the menu's OnSelect). While
// a native menu is up the drawn one does not open, and Open and
// OnOpenChange are not involved: the OS owns the menu and reports only the
// choice.
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
	// NoLongPress keeps a touch or a pen that stays down from opening the
	// menu, so that it clicks what it is on (a held mouse button always
	// does: see aria.ContextMenuTriggerOptions).
	NoLongPress bool
	// AriaLabel names the menu, which has no name of its own: the area is
	// not a label.
	AriaLabel string
	// Native chooses between the OS's context menu and the drawn one; the
	// zero value, kit.NativeAuto, follows tela.App.NativeMenus.
	Native NativeMode
	// CheckIcon and RadioIcon replace the check of a checkbox item and the
	// dot of a radio item, SubIcon the chevron of a submenu's item.
	CheckIcon, RadioIcon, SubIcon tela.Icon

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
	w := tela.UseWindow()
	mac := tela.UsePlatformKeys() == "darwin"
	var intercept func(x, y float32) bool
	if useNative("ContextMenu", c.Native) {
		intercept = func(x, y float32) bool { return c.popup(w, store, mac, tela.Point{X: x, Y: y}) }
	}
	cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{
		Open: c.Open, Disabled: c.Disabled, NoLongPress: c.NoLongPress, Intercept: intercept,
		OnOpenChange: c.OnOpenChange,
	})
	pop := cm.PopoverOptions()
	pop.Floating = append(pop.Floating, menuFloating()...)
	panel := menuPanel[K]{
		component: "ContextMenu", style: st, items: c.Items, store: store, onSelect: c.OnSelect,
		popover: pop, menuProps: cm.MenuProps, ref: cm.MenuRef, strategy: cm.FocusStrategy, close: cm.Close,
		ariaLabel: c.AriaLabel, checkIcon: c.CheckIcon, radioIcon: c.RadioIcon, subIcon: c.SubIcon, class: c.ContentClass,
	}
	// The panel is keyed by the opening: a right click that closes the
	// menu and opens it again at another point (in the same event) mounts
	// a new panel, which takes the focus as any opening does, while the
	// old one fades out where it was (the anchor of its last build is its
	// opening's: aria's Anchor). Without the key the Presence would bring
	// the exiting panel back as it was. A long press while the menu is
	// open is not an opening: it moves the panel.
	return h.Fragment(
		h.Div(cm.TriggerProps, cls(c.Class), c.Props, c.Children),
		h.Portal(h.Presence(h.If(cm.Open, tela.WithKey(panel, cm.Opening)))),
	)
}

// popup shows the menu as the OS's context menu at the point; false means
// there is none (an error, such as desktop.ErrUnsupported on Linux), and the
// drawn menu opens instead.
func (c ContextMenu[K]) popup(w tela.WindowHandle, store *menuStore[K], mac bool, at tela.Point) bool {
	flat := flattenDeep("ContextMenu", c.Items)
	p := menuPanel[K]{items: c.Items, store: store, onSelect: c.OnSelect}
	items := nativeItems(c.Items, store, mac, func(k K) { p.choose(flat, k) })
	if len(items) == 0 {
		return false
	}
	return desktop.PopupMenu(w, at, items, nil) == nil
}
