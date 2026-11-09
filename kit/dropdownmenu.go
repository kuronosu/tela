// Portions of this file follow shadcn/ui, dropdown-menu.tsx (apps/v4/registry/new-york-v4/ui) at the commit
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
	defaultStyle.DropdownMenu = defaultMenuStyle()
}

// DropdownMenu is a menu of actions that opens from a button, shadcn/ui's
// DropdownMenu: a press on the trigger, or Enter, Space or ArrowDown (and
// ArrowUp for the last item) when it has the focus, opens it below; the
// arrow keys, Home and End move through the items, typing jumps to one,
// Enter, Space or a click chooses it and Escape closes it. It is built on
// aria.UseMenuTrigger, aria.UsePopover and aria.UseMenu. The page is inert
// while it is open (it is modal, as Radix's), and the focus returns to the
// trigger when it closes.
//
//	kit.DropdownMenu[string]{
//	    Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")},
//	    Align:   "start",
//	    Items: []kit.MenuItem[string]{
//	        {Kind: kit.MenuLabel, Label: "My account"},
//	        {Kind: kit.MenuSeparator},
//	        {Key: "profile", Label: "Profile", Shortcut: "⇧⌘P"},
//	        {Key: "billing", Label: "Billing", Shortcut: "⌘B"},
//	        {Kind: kit.MenuSeparator},
//	        {Key: "logout", Label: "Log out", Variant: kit.Destructive},
//	    },
//	    OnSelect: func(key string) { run(key) },
//	}
//
// The items are [MenuItem]s: plain items with an icon, a shortcut and a
// destructive variant, checkbox items, radio groups, labels, separators,
// groups, submenus (MenuSub) and standard items with a Role.
//
// The Trigger slot is a [Trigger] (see there): a [Button], or anything
// made one with [TriggerFunc]. Open, DefaultOpen and OnOpenChange are the
// usual controlled and uncontrolled pair; the panel exists only while the
// menu is open. It opens on the side Placement says ("bottom" by default,
// "top", "left" or "right") and Align puts it along that side ("start",
// "center" or "end"; Radix's default is the center), 4 px away unless
// Offset says otherwise, and it flips and shifts to stay in the window;
// a menu taller than the room left scrolls.
type DropdownMenu[K comparable] struct {
	// Trigger is the element that opens the menu.
	Trigger Trigger
	// Items are the entries of the menu, in order.
	Items []MenuItem[K]
	// OnSelect is called with the key of a plain item when it is chosen,
	// after the item's own OnSelect.
	OnSelect func(K)
	// Placement is the side of the trigger the menu opens on: "bottom"
	// (the default), "top", "left" or "right"; a Floating UI placement such
	// as "bottom-start" also works. Align is where it sits along that side:
	// "start", "center" (the default) or "end".
	Placement, Align string
	// Offset is the distance in pixels between the trigger and the menu;
	// zero is 4 and [NoOffset] puts them together.
	Offset float32
	// Disabled makes the trigger inert.
	Disabled bool
	// AriaLabel names the menu when the trigger's text is not enough; the
	// menu is otherwise named by its trigger.
	AriaLabel string
	// CheckIcon and RadioIcon replace the check of a checkbox item and the
	// dot of a radio item, SubIcon the chevron of a submenu's item.
	CheckIcon, RadioIcon, SubIcon tela.Icon

	// Open points to the open state of a controlled menu; nil leaves it to
	// the component, which starts closed unless DefaultOpen is set.
	// OnOpenChange is called with the next state.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)

	// Class is added after the recipe's Content classes, to the menu's
	// panel (the root element of a dropdown menu is its trigger, the app's).
	Class string
	// Props are more props for the panel element.
	Props tela.Child

	// outer holds the props of an overlay that has the menu as its
	// Trigger.
	outer outerTrigger
}

// WithTriggerProps makes DropdownMenu a [Trigger], so that it can be the
// trigger of another overlay: a Tooltip{Trigger: kit.DropdownMenu{Trigger:
// button, ...}} explains the button that opens the menu. The props of the
// overlay go to the root element of the menu's own Trigger, with the
// menu's.
func (d DropdownMenu[K]) WithTriggerProps(p tela.Prop) tela.Component {
	d.outer = outerTrigger{props: p, set: true}
	return d
}

func (d DropdownMenu[K]) withTrigger(p tela.Prop, ref tela.ElementRef) tela.Component {
	d.outer = outerTrigger{props: p, ref: ref, set: true}
	return d
}

// triggerBusy forwards its Trigger's busyTrigger, so that an overlay that
// holds this one in its slot opens nothing from a loading button either.
func (d DropdownMenu[K]) triggerBusy() bool { return isBusy(d.Trigger) }

// Build returns the trigger and the menu's portal.
func (d DropdownMenu[K]) Build() tela.Node {
	st := &UseStyle().DropdownMenu
	store := useMenuStore[K]()
	before := tela.UseOnce(func() *func() (tela.ElementRef, bool) { return new(func() (tela.ElementRef, bool)) })
	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{
		Open: d.Open, DefaultOpen: d.DefaultOpen, OnOpenChange: store.openChange(before, d.OnOpenChange), Disabled: d.Disabled || isBusy(d.Trigger), Ref: d.outer.ref,
	})
	// The element the roles of the items act on (menuStore.origin).
	*before = mt.FocusedBefore
	store.sync(mt.Open)
	panel := menuPanel[K]{
		component: "DropdownMenu", style: st, items: d.Items, store: store, onSelect: d.OnSelect,
		popover: aria.PopoverOptions{
			Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
			Placement: placementOf("DropdownMenu", d.Placement, d.Align, "bottom"), Offset: menuOffset(d.Offset), Modal: true,
			Floating: menuFloating(),
		},
		menuProps: mt.MenuProps, ref: mt.MenuRef, strategy: mt.FocusStrategy, close: mt.Close,
		ariaLabel: d.AriaLabel, checkIcon: d.CheckIcon, radioIcon: d.RadioIcon, subIcon: d.SubIcon, class: d.Class, props: d.Props,
	}
	props := d.outer.merge(mt.TriggerProps)
	return h.Fragment(
		triggerOf(d.Trigger, props, mt.Ref),
		h.Portal(h.Presence(h.If(mt.Open, panel))),
	)
}

// menuOffset is the distance a menu keeps from its trigger: 4 px for the
// zero value, as shadcn's sideOffset, and none for [NoOffset].
func menuOffset(offset float32) float32 {
	switch {
	case offset == 0:
		return 4
	case offset < 0:
		return 0
	}
	return offset
}
