// Portions of this file follow shadcn/ui, dropdown-menu.tsx, context-menu.tsx and menubar.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
)

// This file is what [DropdownMenu], [Menubar] and [ContextMenu] share: the
// model of a menu's items, the recipe of the menu's panel and the element
// tree of an open menu.

// MenuKind is what a [MenuItem] is: an item that does something, a
// checkbox item, a group of radio items, a label, a separator or a group.
type MenuKind int

// The kinds of [MenuItem]. MenuPlain is the zero value, so an item that
// only has a Key and a Label is an ordinary one.
const (
	// MenuPlain is an item that does something when it is chosen
	// (shadcn's DropdownMenuItem): Key, Label, Icon, Shortcut, Variant,
	// Disabled, Inset, OnSelect.
	MenuPlain MenuKind = iota
	// MenuCheckbox is an item that is on or off (DropdownMenuCheckboxItem):
	// Key, Label, Shortcut, Disabled and Checked/DefaultChecked/
	// OnCheckedChange.
	MenuCheckbox
	// MenuRadioGroup is a set of exclusive items (DropdownMenuRadioGroup
	// with its RadioItems): Key names the group and Items are the radio
	// items, whose Key is their value. Value/DefaultValue/OnValueChange say
	// which is chosen.
	MenuRadioGroup
	// MenuLabel is a heading (DropdownMenuLabel): Label and Inset.
	MenuLabel
	// MenuSeparator is a line between items (DropdownMenuSeparator).
	MenuSeparator
	// MenuGroup gathers Items under the role "group" (DropdownMenuGroup);
	// Label, when set, is its accessible name (it is not drawn: put a
	// MenuLabel in Items for a visible heading).
	MenuGroup
	// MenuSub is an item that opens a submenu (DropdownMenuSub, with its
	// SubTrigger and SubContent): Key, Label, Icon, Inset and Disabled are
	// the trigger's, Items the submenu's entries, of any kind (a MenuSub
	// too). It is not chosen: a click, Enter, Space or ArrowRight
	// (ArrowLeft under right-to-left) opens the submenu, and so does the
	// pointer resting on it for aria.SubmenuDelay.
	MenuSub
)

// MenuItem is one entry of a menu, whatever its kind: the model that
// [DropdownMenu], [Menubar] and [ContextMenu] share. K is the type of the
// keys, which name the items that can be chosen. A menu is a slice of them:
//
//	[]kit.MenuItem[string]{
//	    {Kind: kit.MenuLabel, Label: "My account"},
//	    {Key: "profile", Label: "Profile", Shortcut: "⇧⌘P", OnSelect: openProfile},
//	    {Key: "billing", Label: "Billing"},
//	    {Kind: kit.MenuSeparator},
//	    {Kind: kit.MenuCheckbox, Key: "bars", Label: "Status bar", DefaultChecked: true},
//	    {Kind: kit.MenuSeparator},
//	    {Kind: kit.MenuRadioGroup, Key: "theme", DefaultValue: new("light"), Items: []kit.MenuItem[string]{
//	        {Key: "light", Label: "Light"}, {Key: "dark", Label: "Dark"}}},
//	    {Key: "logout", Label: "Log out", Variant: kit.Destructive},
//	}
//
// Every item that can be chosen (a plain item, a checkbox item, a radio
// item) and every submenu trigger needs a Key distinct from the others of
// its menu, its submenus included; it is what the menu's OnSelect reports
// and what the keyboard's typeahead and the accessibility tree tell items
// apart by. A menu with two items of the same Key panics in dev mode.
// Fields that a kind does not use are ignored.
//
// # Submenus
//
// A MenuSub item opens a submenu beside it (on the right, on the left under
// right-to-left), whose Items are any entries, submenus too:
//
//	{Kind: kit.MenuSub, Key: "share", Label: "Share", Items: []kit.MenuItem[string]{
//	    {Key: "email", Label: "Email link"},
//	    {Key: "messages", Label: "Messages"},
//	}},
//
// ArrowLeft (ArrowRight under right-to-left) and Escape close the submenu
// and give the focus back to its item; the focus moving to another item of
// the parent menu closes it, except while the mouse moves toward it (React
// Aria's pointer grace); choosing an item in it, or a press outside every
// menu, closes the whole menu. Each submenu has its own typeahead. In a
// native menu (ContextMenu and Menubar with Native, ADR-215) it is the OS's
// submenu.
//
// # Roles
//
// A plain item with a Role is one of the standard items of desktop.Role
// (Electron's roles): Undo, Redo, Cut, Copy, Paste, Select All, Minimize,
// Zoom and Close. An empty Label is the role's English one and an empty
// Shortcut the role's keys, so an Edit menu can mix the app's items with
// the standard ones:
//
//	{Key: "undo", Role: desktop.RoleUndo},
//	{Key: "redo", Role: desktop.RoleRedo},
//	{Kind: kit.MenuSeparator},
//	{Key: "cut", Role: desktop.RoleCut},
//	{Key: "copy", Role: desktop.RoleCopy},
//	{Key: "paste", Role: desktop.RolePaste},
//	{Key: "find", Label: "Find…", Shortcut: "CmdOrCtrl+F", OnSelect: find},
//
// Choosing a role item without an OnSelect of its own carries the role out,
// and neither the item's nor the menu's OnSelect is called (macOS's own
// items never report the choice either): in a native menu the OS or
// package desktop does it (ADR-226), and in a drawn one the menu closes and
// desktop.PerformRole does it once the focus is back where the menu was
// opened from (the element that had it before the trigger's press took it;
// a context menu's area keeps it), so Copy acts on the text field the user
// was in. An item's own OnSelect replaces the role, as in desktop.MenuItem.
//
// # Shortcuts
//
// Shortcut is shown as written, with two exceptions that follow the
// platform of the window's key conventions (tela.UsePlatformKeys): a
// shortcut that names CmdOrCtrl ("CmdOrCtrl+Shift+Z") is shown as a Mac
// shows it ("⇧⌘Z") under "darwin" and as "Ctrl+Shift+Z" elsewhere, and a
// role item's default keys are written that way. Assistive technologies
// get the keys in ARIA's syntax (aria-keyshortcuts: "Shift+Meta+Z") from
// the glyphs of a Mac ("⇧⌘Z", "⌃⌥⌘K") and from the usual spelling
// ("Ctrl+Shift+T").
//
// Choosing a plain item calls its OnSelect and the menu's, and closes the
// menu; a checkbox item toggles and a radio item chooses, and they close
// the menu too (Radix's default) unless KeepOpen is set. A disabled item
// is skipped by the keyboard and dimmed.
//
// Checkbox items and radio groups follow the kit's value rule: Checked (a
// pointer to your state) with OnCheckedChange controls a checkbox item,
// DefaultChecked starts an uncontrolled one; Value with OnValueChange
// controls a radio group, DefaultValue starts one. An uncontrolled state is
// kept by the menu component, so it survives the menu closing and opening.
type MenuItem[K comparable] struct {
	// Kind is what the entry is; the zero value is a plain item.
	Kind MenuKind
	// Key identifies a plain, checkbox or radio item, and a radio group.
	Key K
	// Label is the item's text, a label's text, or a group's accessible name.
	Label string
	// Icon is drawn before the label of a plain item.
	Icon tela.Icon
	// Shortcut is text shown at the end of the item, such as "⌘T" or
	// "CmdOrCtrl+T" (see "Shortcuts"). It is only shown: binding the keys
	// is the app's.
	Shortcut string
	// Role makes a plain item one of the standard items (see "Roles"):
	// desktop.RoleUndo, RoleCopy, RoleMinimize... The zero value,
	// desktop.RoleNone, is an item of the app's own. A menu role
	// (RoleEditMenu...) is a MenubarMenu's, and panics here in dev mode.
	Role desktop.Role
	// Variant is Default or Destructive (a plain item, as shadcn's
	// variant="destructive").
	Variant Variant
	// Disabled makes the item (for a radio group or a group, all of its
	// items) impossible to choose.
	Disabled bool
	// Inset pads a plain item or a label like the ones with a check or an
	// icon, so that their texts line up (shadcn's inset).
	Inset bool
	// KeepOpen keeps the menu open when this item is chosen, such as a
	// checkbox toggled in a row.
	KeepOpen bool
	// OnSelect is called when a plain item is chosen.
	OnSelect func()

	// Checked points to the state of a controlled checkbox item; nil leaves
	// it to the menu, which starts at DefaultChecked. OnCheckedChange is
	// called with the next state.
	Checked         *bool
	DefaultChecked  bool
	OnCheckedChange func(checked bool)

	// Value points to the chosen item of a controlled radio group; nil
	// leaves it to the menu, which starts at DefaultValue (an item whose
	// Key it equals; nil: none chosen, a pointer so that the zero K can be
	// a key). OnValueChange is called with the Key of the item the user
	// chose.
	Value         *K
	DefaultValue  *K
	OnValueChange func(K)

	// Items are the radio items of a radio group, the entries of a group,
	// or the entries of a submenu.
	Items []MenuItem[K]

	// Class is added after the recipe's classes, to the item.
	Class string
}

// MenuStyle is the recipe of the panel of a menu and its items, shared by
// [DropdownMenu] and [ContextMenu] (shadcn/ui's DropdownMenuContent and
// ContextMenuContent and what they hold) and used by [Menubar] for its
// menus.
type MenuStyle struct {
	// Content is the panel: surface, border, shadow, padding, its minimum
	// width and the entrance. It scrolls when there is no room for it.
	Content string
	// Exit is what the panel transitions to as it closes.
	Exit string
	// Sides adds classes for the side of the trigger the panel is on
	// ("top", "right", "bottom" or "left"): the transform origin and the
	// slide it enters with, as [PopoverStyle].Sides.
	Sides map[string]string
	// Item is a plain item, ItemVariants adds the classes of a Variant
	// (Destructive, and the empty key for the default), ItemInset the
	// padding of an inset item.
	Item         string
	ItemVariants map[Variant]string
	ItemInset    string
	// ItemIcon is the icon of a plain item and ItemIconVariants its color
	// per Variant.
	ItemIcon         string
	ItemIconVariants map[Variant]string
	// CheckboxItem and RadioItem are the items with an indicator at the
	// start; Indicator is the box that holds it, CheckIcon the check and
	// RadioIcon the dot.
	CheckboxItem string
	RadioItem    string
	Indicator    string
	CheckIcon    string
	RadioIcon    string
	// Label is a heading, LabelInset its padding when inset.
	Label      string
	LabelInset string
	// Separator is the line between items.
	Separator string
	// Shortcut is the text at the end of an item.
	Shortcut string
	// SubTrigger is an item that opens a submenu (shadcn's SubTrigger;
	// aria-expanded: styles it while its submenu is open), SubIcon the
	// chevron at its end and SubContent the submenu's panel, which enters
	// and exits as Content does (Exit, Sides).
	SubTrigger string
	SubIcon    string
	SubContent string
}

// defaultMenuStyle is shadcn/ui's DropdownMenuContent and the items it
// holds, at the pinned commit. A menu's open and close animation are the
// popover's.
func defaultMenuStyle() MenuStyle {
	const indicatorItem = "relative flex cursor-default items-center gap-2 rounded-sm py-1.5 pe-2 ps-8 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"
	return MenuStyle{
		Content: "z-50 min-w-32 overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md outline-hidden " +
			"transition duration-150 starting:scale-95 starting:opacity-0",
		Exit: "scale-95 opacity-0",
		Sides: map[string]string{
			"top":    "origin-bottom starting:translate-y-2",
			"right":  "origin-left starting:-translate-x-2",
			"bottom": "origin-top starting:-translate-y-2",
			"left":   "origin-right starting:translate-x-2",
		},
		Item: "relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground " +
			"aria-disabled:pointer-events-none aria-disabled:opacity-50",
		ItemVariants: map[Variant]string{
			"":          "",
			Destructive: "text-destructive focus:bg-destructive/10 focus:text-destructive dark:focus:bg-destructive/20",
		},
		ItemInset: "ps-8",
		ItemIcon:  "pointer-events-none size-4 shrink-0",
		ItemIconVariants: map[Variant]string{
			"":          "text-muted-foreground",
			Destructive: "text-destructive",
		},
		CheckboxItem: indicatorItem,
		RadioItem:    indicatorItem,
		Indicator:    "pointer-events-none absolute start-2 flex size-3.5 items-center justify-center",
		CheckIcon:    "size-4",
		RadioIcon:    "size-2 fill-current",
		Label:        "px-2 py-1.5 text-sm font-medium",
		LabelInset:   "ps-8",
		Separator:    "-mx-1 my-1 h-px bg-border",
		Shortcut:     "ms-auto text-xs tracking-widest text-muted-foreground",
		SubTrigger: "flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground " +
			"aria-expanded:bg-accent aria-expanded:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50",
		SubIcon: "pointer-events-none ms-auto size-4 shrink-0 rtl:rotate-180",
		SubContent: "z-50 min-w-32 overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-lg outline-hidden " +
			"transition duration-150 starting:scale-95 starting:opacity-0",
	}
}

// menuChevronRight is Lucide's chevron-right (ISC), the end of a submenu's
// trigger.
var menuChevronRight = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`}

// menuRowKey keys the entries of a menu that have no Key of their own.
type menuRowKey struct {
	kind  MenuKind
	index int
}

// menuFloating limits the height of a menu (or the list of a select) to the
// room the window has left, so that a long one scrolls (shadcn's
// max-h-(--radix-...-available-height)).
//
// The limit takes no margin from the window's edge, as Radix's (its
// collisionPadding is 0): it has to agree with the Flip that UsePopover
// puts before it, which keeps no margin either, or a menu that Flip finds
// room for would be limited anyway, to the room minus the margin, and
// scroll for nothing (ADR-163).
func menuFloating() []aria.FloatingOption {
	return []aria.FloatingOption{aria.Size(func(a aria.Available) { a.MaxHeight(a.Height) })}
}

// menuStore keeps the state of the uncontrolled checkbox items and radio
// groups of a menu. It belongs to the component that is always mounted
// (the dropdown, the bar, the area), because the panel exists only while
// the menu is open.
type menuStore[K comparable] struct {
	checked map[K]bool
	radio   map[K]K
	rebuild func()
	// role is the role of the item chosen last, carried out once the
	// drawn menu has closed (desktop.PerformRole), in origin when there
	// is one: the element that had the focus before the trigger's press
	// took it.
	role      desktop.Role
	origin    tela.ElementRef
	hasOrigin bool
}

// useMenuStore is the hook that returns the store of a menu component. It
// carries out a chosen role after the build that closed the menu, when the
// panel's focus scope has given the focus back.
func useMenuStore[K comparable]() *menuStore[K] {
	_, bump := tela.UseState(0)
	w := tela.UseWindow()
	s := tela.UseOnce(func() *menuStore[K] {
		return &menuStore[K]{checked: map[K]bool{}, radio: map[K]K{}}
	})
	s.rebuild = func() { bump.Update(func(n int) int { return n + 1 }) }
	tela.UseEffect(func() func() {
		if r := s.role; r != desktop.RoleNone {
			s.role = desktop.RoleNone
			if s.hasOrigin {
				s.origin.Focus()
			}
			s.hasOrigin = false
			desktop.PerformRole(w, r)
		}
		return nil
	})
	return s
}

// captureOrigin remembers the element that has the focus before a press
// on a menu's trigger moves it there, for the roles of the menu's items
// (menuStore.role). A key on the trigger forgets it: the focus is the
// trigger's already.
func (s *menuStore[K]) captureOrigin(w tela.WindowHandle, open func() bool) tela.Prop {
	return tela.MergeProps(
		h.OnPointerDown(func() {
			if !open() {
				s.origin, s.hasOrigin = w.ActiveElement()
			}
		}),
		h.OnKeyDown(func() {
			if !open() {
				s.hasOrigin = false
			}
		}),
	)
}

func (s *menuStore[K]) isChecked(it MenuItem[K]) bool {
	if it.Checked != nil {
		return *it.Checked
	}
	if v, ok := s.checked[it.Key]; ok {
		return v
	}
	return it.DefaultChecked
}

// chosen reports whether k is the chosen item of the radio group g: the
// controlled Value, else what the user chose, else DefaultValue. A group
// with none of them has nothing chosen, even for the zero K.
func (s *menuStore[K]) chosen(g MenuItem[K], k K) bool {
	switch {
	case g.Value != nil:
		return *g.Value == k
	default:
		if v, ok := s.radio[g.Key]; ok {
			return v == k
		}
	}
	return g.DefaultValue != nil && *g.DefaultValue == k
}

// menuEntry is a chosen-able item with the group it is in.
type menuEntry[K comparable] struct {
	item  MenuItem[K]
	group *MenuItem[K] // the radio group, nil for others
}

// menuFlat is the items of a menu that can be chosen, and the items that
// open its submenus, in order (one level: a submenu has its own).
type menuFlat[K comparable] struct {
	keys     []K
	disabled []K
	subs     []K
	entries  map[K]menuEntry[K]
	text     map[K]string
}

// flattenMenu lists the items that can be chosen. component names the
// caller for the dev-mode message about duplicate keys.
func flattenMenu[K comparable](component string, items []MenuItem[K]) *menuFlat[K] {
	f := &menuFlat[K]{entries: map[K]menuEntry[K]{}, text: map[K]string{}}
	groups := map[K]bool{}
	// deep holds, in dev mode, the keys of the submenus' entries at every
	// depth: a key is unique across the whole menu, or a native menu,
	// whose choices come from any level (flattenDeep), would run the
	// wrong item.
	deep := map[K]bool{}
	var walk func(items []MenuItem[K], radio *MenuItem[K], off bool)
	add := func(it MenuItem[K], radio *MenuItem[K], off bool) {
		if devMode() {
			if _, dup := f.entries[it.Key]; dup {
				panic(fmt.Sprintf("kit: %s has two items with the key %v; give each item that can be chosen its own Key", component, it.Key))
			}
			if deep[it.Key] {
				panic(fmt.Sprintf("kit: %s has two items with the key %v (one in a submenu); give each item that can be chosen, in every submenu, its own Key", component, it.Key))
			}
		}
		f.entries[it.Key] = menuEntry[K]{item: it, group: radio}
		f.keys = append(f.keys, it.Key)
		f.text[it.Key] = it.Label
		if it.Disabled || off {
			f.disabled = append(f.disabled, it.Key)
		}
	}
	walk = func(items []MenuItem[K], radio *MenuItem[K], off bool) {
		for i := range items {
			it := items[i]
			switch it.Kind {
			case MenuSub:
				add(it, nil, off)
				f.subs = append(f.subs, it.Key)
				if devMode() {
					checkSubKeys(component, it.Items, f.entries, deep)
				}
			case MenuPlain, MenuCheckbox:
				if it.Role != desktop.RoleNone && devMode() {
					checkItemRole(component, it)
				}
				add(it, nil, off)
			case MenuRadioGroup:
				if groups[it.Key] && devMode() {
					panic(fmt.Sprintf("kit: %s has two radio groups with the key %v; give each its own Key", component, it.Key))
				}
				groups[it.Key] = true
				g := it
				for _, ri := range it.Items {
					add(ri, &g, off || it.Disabled)
				}
			case MenuGroup:
				walk(it.Items, nil, off || it.Disabled)
			}
		}
	}
	walk(items, nil, false)
	for k, e := range f.entries {
		if e.item.Label == "" && e.item.Role != desktop.RoleNone {
			f.text[k] = e.item.Role.Label()
		}
	}
	return f
}

// checkSubKeys panics, in dev mode, when an entry of a submenu (at any
// depth) has the Key of an item of the menu (seen) or of another submenu
// entry met before it (deep, which it adds the submenu's keys to): the
// menu's OnSelect could not tell them apart.
func checkSubKeys[K comparable](component string, items []MenuItem[K], seen map[K]menuEntry[K], deep map[K]bool) {
	var walk func(items []MenuItem[K])
	walk = func(items []MenuItem[K]) {
		for _, it := range items {
			switch it.Kind {
			case MenuPlain, MenuCheckbox, MenuSub:
				if _, dup := seen[it.Key]; dup || deep[it.Key] {
					panic(fmt.Sprintf("kit: %s has two items with the key %v (one in a submenu); give each item that can be chosen, in every submenu, its own Key", component, it.Key))
				}
				deep[it.Key] = true
				if it.Kind == MenuSub {
					walk(it.Items)
				}
			case MenuRadioGroup:
				walk(it.Items)
			case MenuGroup:
				walk(it.Items)
			}
		}
	}
	walk(items)
}

// checkItemRole panics when an item has a Role that is not the role of an
// item.
func checkItemRole[K comparable](component string, it MenuItem[K]) {
	switch it.Role {
	case desktop.RoleUndo, desktop.RoleRedo, desktop.RoleCut, desktop.RoleCopy, desktop.RolePaste,
		desktop.RoleSelectAll, desktop.RoleMinimize, desktop.RoleZoom, desktop.RoleClose:
		if it.Kind != MenuPlain {
			panic(fmt.Sprintf("kit: %s item %q has Role %v and is not a plain item; only plain items have a role", component, it.Label, it.Role))
		}
	default:
		panic(fmt.Sprintf("kit: %s item %q has Role %v, the role of a menu; an item's Role is desktop.RoleUndo, RoleRedo, RoleCut, RoleCopy, RolePaste, RoleSelectAll, RoleMinimize, RoleZoom or RoleClose (a menu's role goes in MenubarMenu.Role)", component, it.Label, it.Role))
	}
}

// menuPanel is an open menu: the popover that places it, the aria menu
// that moves through its items and the items. It is a component of its own
// so that it exists only while the menu is open, which is what makes the
// menu focus an item each time it opens (aria.UseMenu).
type menuPanel[K comparable] struct {
	component string
	style     *MenuStyle
	items     []MenuItem[K]
	store     *menuStore[K]
	onSelect  func(K)
	// popover places the menu; menuProps, ref, strategy and close come from
	// the trigger hook (dropdown, bar or context area).
	popover   aria.PopoverOptions
	menuProps tela.Prop
	ref       tela.ElementRef
	strategy  string
	close     func()
	ariaLabel string
	checkIcon tela.Icon
	subIcon   tela.Icon
	radioIcon tela.Icon
	class     string
	props     tela.Child
	// dir is the direction of the menu ("ltr" or "rtl"), for its
	// submenus' side and keys; "" is the one at the panel's position.
	dir string
	// sub is the submenu this panel is, the zero value for a menu's own
	// panel: its popover, props, ref and close come from it.
	sub   aria.SubmenuTriggerResult
	isSub bool
}

func (p menuPanel[K]) Build() tela.Node {
	st := p.style
	// UseDirection is a hook: read it on every build, then prefer p.dir.
	dir := tela.UseDirection()
	if p.dir != "" {
		dir = p.dir
	}
	content := st.Content
	if p.isSub {
		p.popover = p.sub.PopoverOptions()
		p.popover.Floating = menuFloating()
		p.menuProps, p.ref, p.close, content = p.sub.MenuProps, p.sub.MenuRef, p.sub.CloseAll, st.SubContent
	}
	pop := aria.UsePopover(p.popover)
	flat := flattenMenu(p.component, p.items)
	keepOpen := false
	o := aria.MenuOptions[K]{
		Keys: flat.keys, DisabledKeys: flat.disabled, SubmenuKeys: flat.subs, Direction: dir,
		TextValue: func(k K) string { return flat.text[k] },
		// Each item decides whether the menu closes (KeepOpen).
		CloseOnSelect: &keepOpen,
		OnAction:      func(k K) { p.choose(flat, k) },
		Ref:           p.ref, AriaLabel: p.ariaLabel,
	}
	if p.isSub {
		o.Submenu = p.sub
	} else {
		// CloseAll of the submenus; the items close the menu themselves.
		o.OnClose, o.AutoFocus, o.FocusStrategy = p.close, true, p.strategy
	}
	menu := aria.UseMenu(o)
	r := menuRenderer[K]{component: p.component, st: st, menu: menu, store: p.store, checkIcon: p.checkIcon, radioIcon: p.radioIcon, subIcon: p.subIcon,
		mac: tela.UsePlatformKeys() == "darwin"}
	kids := []tela.Child{pop.Props, p.menuProps, menu.Props, h.Exit(st.Exit),
		cls(content, st.Sides[pop.Side], when(!p.isSub, p.class))}
	if !p.isSub {
		kids = append(kids, p.props)
	}
	kids = append(kids, r.entries(p.items)...)
	// The submenus are children of the panel, not of their item: the item
	// would get the submenu's pointer events (aria.MenuResult.SubmenuTrigger).
	for _, k := range flat.subs {
		sub := menu.SubmenuTrigger(k)
		inner := menuPanel[K]{
			component: p.component, style: st, items: flat.entries[k].item.Items, store: p.store, onSelect: p.onSelect,
			checkIcon: p.checkIcon, radioIcon: p.radioIcon, subIcon: p.subIcon, dir: dir, sub: sub, isSub: true,
		}
		kids = append(kids, h.Portal(h.Key(menuSubKey[K]{k}), h.Presence(h.If(sub.Open, inner))))
	}
	return h.Div(kids...)
}

// menuSubKey keys the portal of a submenu among the entries of its menu.
type menuSubKey[K comparable] struct{ k K }

// choose runs what choosing the item with key k does.
func (p menuPanel[K]) choose(flat *menuFlat[K], k K) {
	e, ok := flat.entries[k]
	if !ok {
		return
	}
	it := e.item
	switch {
	case e.group != nil:
		g := *e.group
		if !p.store.chosen(g, k) {
			if g.Value == nil {
				p.store.radio[g.Key] = k
				p.store.rebuild()
			}
			if g.OnValueChange != nil {
				g.OnValueChange(k)
			}
		}
	case it.Kind == MenuCheckbox:
		next := !p.store.isChecked(it)
		if it.Checked == nil {
			p.store.checked[k] = next
			p.store.rebuild()
		}
		if it.OnCheckedChange != nil {
			it.OnCheckedChange(next)
		}
	case it.Kind == MenuSub:
		return // aria opens the submenu; it is not chosen
	case it.Role != desktop.RoleNone && it.OnSelect == nil:
		// Carried out once the menu has closed and the focus is back
		// (useMenuStore).
		p.store.role = it.Role
		p.store.rebuild()
		if p.close != nil {
			p.close()
		}
		return
	default:
		if it.OnSelect != nil {
			it.OnSelect()
		}
		if p.onSelect != nil {
			p.onSelect(k)
		}
	}
	if !it.KeepOpen && p.close != nil {
		p.close()
	}
}

// menuRenderer draws the entries of a menu from the aria menu's props.
type menuRenderer[K comparable] struct {
	component string
	st        *MenuStyle
	menu      aria.MenuResult[K]
	store     *menuStore[K]
	checkIcon tela.Icon
	subIcon   tela.Icon
	radioIcon tela.Icon
	// mac: the window follows the Mac's key conventions (shortcuts).
	mac bool
}

// entries draws a list of entries.
func (r menuRenderer[K]) entries(items []MenuItem[K]) []tela.Child {
	var out []tela.Child
	for i, it := range items {
		switch it.Kind {
		case MenuPlain, MenuCheckbox, MenuSub:
			out = append(out, r.item(it, nil))
		case MenuRadioGroup:
			g := it
			var kids []tela.Child
			kids = append(kids, h.Role("group"), h.Key(menuRowKey{it.Kind, i}))
			if it.Label != "" {
				kids = append(kids, h.AriaLabel(it.Label))
			}
			if it.Disabled {
				kids = append(kids, h.AriaDisabled(true))
			}
			for _, ri := range it.Items {
				ri.Disabled = ri.Disabled || it.Disabled
				kids = append(kids, r.item(ri, &g))
			}
			out = append(out, h.Div(kids...))
		case MenuLabel:
			out = append(out, h.Div(h.Key(menuRowKey{it.Kind, i}), h.Role("presentation"),
				cls(r.st.Label, when(it.Inset, r.st.LabelInset), it.Class), h.Text(it.Label)))
		case MenuSeparator:
			out = append(out, h.Div(h.Key(menuRowKey{it.Kind, i}), h.Role("separator"), cls(r.st.Separator, it.Class)))
		case MenuGroup:
			kids := []tela.Child{h.Role("group"), h.Key(menuRowKey{it.Kind, i}), cls(it.Class)}
			if it.Label != "" {
				kids = append(kids, h.AriaLabel(it.Label))
			}
			if it.Disabled {
				kids = append(kids, h.AriaDisabled(true))
			}
			out = append(out, h.Div(append(kids, r.entries(it.Items)...)...))
		}
	}
	return out
}

// item draws an item that can be chosen: a radio item when radio is the
// group it is in.
func (r menuRenderer[K]) item(it MenuItem[K], radio *MenuItem[K]) tela.Child {
	st := r.st
	kids := []tela.Child{r.menu.ItemProps(it.Key), h.Key(it.Key)}
	switch {
	case radio != nil:
		on := r.store.chosen(*radio, it.Key)
		kids = append(kids, h.Role("menuitemradio"), h.AriaChecked(on),
			cls(st.RadioItem, it.Class),
			h.Span(cls(st.Indicator), h.If(on, icon(orIcon(r.radioIcon, iconCircle), st.RadioIcon))))
	case it.Kind == MenuCheckbox:
		on := r.store.isChecked(it)
		kids = append(kids, h.Role("menuitemcheckbox"), h.AriaChecked(on),
			cls(st.CheckboxItem, it.Class),
			h.Span(cls(st.Indicator), h.If(on, icon(orIcon(r.checkIcon, iconCheck), st.CheckIcon))))
	case it.Kind == MenuSub:
		return h.Div(append(kids, cls(st.SubTrigger, when(it.Inset, st.ItemInset), it.Class),
			icon(it.Icon, st.ItemIcon, st.ItemIconVariants[""]), h.Text(it.Label),
			icon(orIcon(r.subIcon, menuChevronRight), st.SubIcon))...)
	default:
		// The icon's color falls back to the default one for a variant
		// that has none of its own.
		iconVariant, ok := st.ItemIconVariants[it.Variant]
		if !ok {
			iconVariant = st.ItemIconVariants[""]
		}
		kids = append(kids, cls(st.Item, r.itemVariant(it.Variant), when(it.Inset, st.ItemInset), it.Class),
			icon(it.Icon, st.ItemIcon, iconVariant))
	}
	label, shortcut := it.Label, it.Shortcut
	if it.Role != desktop.RoleNone && it.Kind == MenuPlain {
		if label == "" {
			label = it.Role.Label()
		}
		if shortcut == "" {
			shortcut = roleShortcut(it.Role)
		}
	}
	kids = append(kids, h.Text(label))
	if shortcut != "" {
		shown, keys := formatShortcut(shortcut, r.mac)
		if keys != "" {
			kids = append(kids, h.AriaKeyShortcuts(keys))
		}
		kids = append(kids, h.Span(cls(st.Shortcut), h.AriaHidden(true), h.Text(shown)))
	}
	return h.Div(kids...)
}

// roleShortcut is the keys a role item shows when it has no Shortcut: the
// edit roles' (desktop's own), none for the window roles.
func roleShortcut(r desktop.Role) string {
	switch r {
	case desktop.RoleUndo:
		return "CmdOrCtrl+Z"
	case desktop.RoleRedo:
		return "CmdOrCtrl+Shift+Z"
	case desktop.RoleCut:
		return "CmdOrCtrl+X"
	case desktop.RoleCopy:
		return "CmdOrCtrl+C"
	case desktop.RolePaste:
		return "CmdOrCtrl+V"
	case desktop.RoleSelectAll:
		return "CmdOrCtrl+A"
	}
	return ""
}

// formatShortcut returns how a menu item shows its Shortcut and the keys in
// ARIA's syntax for h.AriaKeyShortcuts ("" when it is not a key
// combination). A shortcut that names CmdOrCtrl follows the window's key
// conventions (mac): "⇧⌘Z" for "CmdOrCtrl+Shift+Z" on a Mac, "Ctrl+Shift+Z"
// elsewhere; any other is shown as written, the glyphs of a Mac ("⇧⌘Z")
// and the usual spelling ("Ctrl+Shift+T") both giving ARIA's keys
// ("Shift+Meta+Z", "Control+Shift+T").
func formatShortcut(shortcut string, mac bool) (shown, aria string) {
	mods, key, ok := parseGlyphs(shortcut)
	if ok {
		return shortcut, ariaKeys(mods, key)
	}
	if strings.ContainsAny(shortcut, "⌃⌥⇧⌘") {
		return shortcut, "" // a glyph alone, or glyphs among words
	}
	text, named := resolveCmdOrCtrl(shortcut, mac)
	keys, err := desktop.ParseKeys(text)
	if err != nil {
		return shortcut, ""
	}
	mods, key = splitKeys(keys)
	shown = shortcut
	if named {
		if mac {
			shown = macGlyphs(mods, key)
		} else {
			shown = keys
		}
	}
	return shown, ariaKeys(mods, key)
}

// resolveCmdOrCtrl replaces CmdOrCtrl (CommandOrControl) in a shortcut by
// the primary modifier of the key conventions (Super on a Mac, Ctrl
// elsewhere), and reports whether it was there: desktop.ParseKeys would
// take the host's.
func resolveCmdOrCtrl(s string, mac bool) (string, bool) {
	primary := "Ctrl"
	if mac {
		primary = "Super"
	}
	tail := ""
	t := s
	if strings.HasSuffix(t, "++") {
		t, tail = t[:len(t)-2], "++"
	}
	parts := strings.Split(t, "+")
	named := false
	for i, p := range parts {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "cmdorctrl", "commandorcontrol":
			parts[i], named = primary, true
		}
	}
	return strings.Join(parts, "+") + tail, named
}

// splitKeys splits desktop.ParseKeys's canonical form ("Ctrl+Shift+K",
// "Ctrl++") into its modifiers and key.
func splitKeys(keys string) (mods []string, key string) {
	switch {
	case keys == "+":
		return nil, "+"
	case strings.HasSuffix(keys, "++"):
		return strings.Split(keys[:len(keys)-2], "+"), "+"
	}
	parts := strings.Split(keys, "+")
	return parts[:len(parts)-1], parts[len(parts)-1]
}

// The glyphs of a Mac's modifiers, in the order its menus show them.
var macModGlyphs = []struct {
	glyph rune
	mod   string // desktop.ParseKeys's name
}{{'⌃', "Ctrl"}, {'⌥', "Alt"}, {'⇧', "Shift"}, {'⌘', "Super"}}

// macKeyGlyphs are the glyphs of named keys in a Mac's menus.
var macKeyGlyphs = map[string]string{
	"Enter": "↩", "Backspace": "⌫", "Delete": "⌦", "Escape": "⎋", "Tab": "⇥",
	"ArrowLeft": "←", "ArrowRight": "→", "ArrowUp": "↑", "ArrowDown": "↓",
	"PageUp": "⇞", "PageDown": "⇟", "Home": "↖", "End": "↘", "Space": "Space",
}

// parseGlyphs reads a shortcut written with a Mac's modifier glyphs
// ("⇧⌘Z", "⌃⌥⌘K", "⌘↩"): one or more modifiers, then the key. ok is false
// for anything else.
func parseGlyphs(s string) (mods []string, key string, ok bool) {
	rest := strings.TrimSpace(s)
	seen := map[string]bool{}
	for rest != "" {
		r, n := utf8.DecodeRuneInString(rest)
		found := false
		for _, g := range macModGlyphs {
			if g.glyph == r {
				seen[g.mod], found = true, true
			}
		}
		if !found {
			break
		}
		rest = rest[n:]
	}
	if len(seen) == 0 || rest == "" {
		return nil, "", false
	}
	for name, glyph := range macKeyGlyphs {
		if rest == glyph && glyph != "Space" {
			key = name
		}
	}
	if key == "" {
		if rest == "␣" {
			key = "Space"
		} else {
			k, err := desktop.ParseKeys(rest)
			if err != nil {
				return nil, "", false
			}
			key = k
		}
	}
	for _, g := range macModGlyphs {
		if seen[g.mod] {
			mods = append(mods, g.mod)
		}
	}
	return mods, key, true
}

// macGlyphs writes modifiers and a key as a Mac's menus show them: "⇧⌘Z".
func macGlyphs(mods []string, key string) string {
	var b strings.Builder
	for _, g := range macModGlyphs {
		if slices.Contains(mods, g.mod) {
			b.WriteRune(g.glyph)
		}
	}
	if g, ok := macKeyGlyphs[key]; ok {
		key = g
	}
	b.WriteString(key)
	return b.String()
}

// ariaKeys writes modifiers and a key (desktop.ParseKeys's names) in
// ARIA's syntax for aria-keyshortcuts: "Shift+Meta+Z", "Control+Plus".
func ariaKeys(mods []string, key string) string {
	var out []string
	for _, m := range []string{"Ctrl", "Alt", "Shift", "Super"} {
		if !slices.Contains(mods, m) {
			continue
		}
		switch m {
		case "Ctrl":
			m = "Control"
		case "Super":
			m = "Meta"
		}
		out = append(out, m)
	}
	if key == "+" {
		key = "Plus"
	}
	return strings.Join(append(out, key), "+")
}

// itemVariant returns the classes of a plain item's Variant. One the
// recipe does not define panics in dev mode, as [pick] does, with the name
// of the recipe field that holds the variants of a menu.
func (r menuRenderer[K]) itemVariant(v Variant) string {
	if c, ok := r.st.ItemVariants[v]; ok {
		return c
	}
	if v != "" && devMode() {
		names := make([]string, 0, len(r.st.ItemVariants))
		for n := range r.st.ItemVariants {
			names = append(names, string(n))
		}
		sort.Strings(names)
		field := r.component
		if field == "Menubar" {
			field = "Menubar.Menu"
		}
		msg := fmt.Sprintf("kit: %s item Variant %q is not defined by the active style, which defines %s", r.component, string(v), quoteNames(names))
		if c := closest(string(v), names); c != "" {
			msg += fmt.Sprintf("; did you mean %q?", c)
		} else {
			msg += fmt.Sprintf("; add it to Style.%s.ItemVariants (and ItemIconVariants for its icon's color; see kit.DefaultStyle) or use one of them", field)
		}
		panic(msg)
	}
	return r.st.ItemVariants[""]
}

// orIcon returns ic, or def when ic is the zero Icon.
func orIcon(ic, def tela.Icon) tela.Icon {
	if ic == (tela.Icon{}) {
		return def
	}
	return ic
}
