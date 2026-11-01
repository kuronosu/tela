// Portions of this file follow shadcn/ui, menubar.tsx (apps/v4/registry/new-york-v4/ui) at the commit
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

// MenubarStyle is the recipe of [Menubar] (shadcn/ui's Menubar,
// MenubarTrigger and MenubarContent with the items they hold).
type MenubarStyle struct {
	// Root is the bar.
	Root string
	// Trigger is a menu's button in the bar; aria-expanded: styles the one
	// whose menu is open.
	Trigger string
	// Menu is the recipe of the menus and their items.
	Menu MenuStyle
}

func init() {
	menu := defaultMenuStyle()
	menu.Content = "z-50 min-w-48 overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md outline-hidden " +
		"transition duration-150 starting:scale-95 starting:opacity-0"
	menu.CheckboxItem = "relative flex cursor-default items-center gap-2 rounded-xs py-1.5 pe-2 ps-8 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"
	menu.RadioItem = menu.CheckboxItem
	defaultStyle.Menubar = MenubarStyle{
		Root: "flex h-9 items-center gap-1 rounded-md border border-border bg-background p-1 shadow-xs",
		Trigger: "flex items-center rounded-sm px-2 py-1 text-sm font-medium outline-hidden select-none focus:bg-accent focus:text-accent-foreground " +
			"aria-expanded:bg-accent aria-expanded:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50",
		Menu: menu,
	}
}

// MenubarMenu is one menu of a [Menubar]: the label on the bar and the
// items it opens.
type MenubarMenu[K comparable] struct {
	// Key identifies the menu (what Menubar.Open and OnOpenChange name).
	Key K
	// Label is the text of the menu's button in the bar.
	Label string
	// Disabled makes the menu impossible to open.
	Disabled bool
	// Items are the entries of the menu.
	Items []MenuItem[K]
}

// Menubar is a row of menus, such as the File, Edit and View of an
// application, shadcn/ui's Menubar, built on aria.UseMenuBar: the bar is one
// tab stop, ArrowLeft and ArrowRight move between the menus' buttons (and,
// with a menu open, open the neighbor's), Home and End jump, ArrowDown,
// Enter and Space open the menu on its first item and ArrowUp on its last;
// once a menu is open, hovering another button opens that one. A press on
// the open menu's button, Escape, a press outside and Tab close it.
//
//	kit.Menubar[string]{
//	    AriaLabel: "Application",
//	    Menus: []kit.MenubarMenu[string]{
//	        {Key: "file", Label: "File", Items: []kit.MenuItem[string]{
//	            {Key: "new", Label: "New Tab", Shortcut: "⌘T"},
//	            {Key: "window", Label: "New Window", Shortcut: "⌘N"},
//	            {Kind: kit.MenuSeparator},
//	            {Key: "print", Label: "Print...", Shortcut: "⌘P"},
//	        }},
//	        {Key: "edit", Label: "Edit", Items: edit},
//	    },
//	    OnSelect: func(menu, item string) { run(menu, item) },
//	}
//
// The items are the [MenuItem]s of [DropdownMenu], with no submenus yet
// (ADR-141). The keys of the menus and of the items have the same type K;
// an item key needs to be distinct only within its menu. The open menu
// follows the kit's value rule with Open (a pointer to the key of the
// open menu; a key no menu has, such as the zero K, means all closed),
// DefaultOpen and OnOpenChange.
//
// # Native menu bar
//
// The bar is drawn in the kit's look unless Native (kit.NativeOn), or
// tela.App.NativeMenus when Native is left at kit.NativeAuto, asks for the
// OS's window menu bar (ADR-215): the menus are given to the window
// (desktop.MenuBar) and the component draws nothing where it is. A window
// has a native bar on Windows when it has the OS's title bar; a frameless
// window (kit.TitleBar draws its own), Linux, and a window of a tree with no
// running app have none, and there the drawn bar replaces it: the native
// attempt reports it after the component's first frame, so the drawn bar
// appears one frame later. A bar's own Native setting wins over the app's.
//
// Each menu is a top-level entry with its items, converted as a native
// ContextMenu's are: Label, Shortcut (shown as text; handle the keys
// yourself), Disabled, separators, check items for checkbox and radio items;
// icons, the destructive Variant, Inset, KeepOpen, Class, Direction, the
// classes of the bar, buttons and panels, and the check and radio icons
// are not shown, a label becomes a disabled item and a group appears without
// it. Choosing an item does what it does in the drawn menu. Open,
// DefaultOpen and OnOpenChange belong to the drawn bar: the OS owns a
// native one and none of them is called, and the keyboard navigation
// is the OS's.
type Menubar[K comparable] struct {
	// Menus are the bar's menus, in order.
	Menus []MenubarMenu[K]
	// OnSelect is called with the key of the menu and of the plain item
	// that was chosen, after the item's own OnSelect.
	OnSelect func(menu, item K)
	// AriaLabel names the bar.
	AriaLabel string
	// Native chooses between the OS's menu bar and the drawn one; the
	// zero value, kit.NativeAuto, follows tela.App.NativeMenus.
	Native NativeMode
	// Direction is "ltr" or "rtl"; empty is the direction at the component's
	// position (tela.UseDirection: the nearest h.Dir above it, left to right
	// without one). Under "rtl" ArrowLeft goes to the next menu and ArrowRight to the previous one; a Direction given here also
	// sets the direction of the layout.
	Direction string
	// CheckIcon and RadioIcon replace the check of a checkbox item and the
	// dot of a radio item.
	CheckIcon, RadioIcon tela.Icon

	// Open points to the open menu of a controlled bar; nil leaves it to
	// the component, which starts with DefaultOpen (nil: all closed).
	// OnOpenChange is called with the menu that opens (open true) or the one
	// that closes; moving from one menu to another is one call, with the one
	// that opens.
	Open         *K
	DefaultOpen  *K
	OnOpenChange func(menu K, open bool)

	// Class is added after the recipe's Root classes, to the bar;
	// TriggerClass does the same for the menus' buttons and ContentClass for
	// the menus' panels.
	Class        string
	TriggerClass string
	ContentClass string
	// Props are more props for the bar element.
	Props tela.Child
}

// Build returns the OS's menu bar, which draws nothing here, or the drawn
// bar and, in a portal, its open menu.
func (m Menubar[K]) Build() tela.Node {
	store := useMenuStore[K]()
	native := useNative("Menubar", m.Native)
	failed, setFailed := tela.UseState(false)
	if native && !failed {
		return nativeMenubar[K]{
			menus: m.Menus, onSelect: m.OnSelect, store: store,
			// The platform has no such bar (or it cannot be shown): draw it.
			onError: func(error) { setFailed.Set(true) },
		}
	}
	return drawnMenubar[K]{bar: m, store: store}
}

// nativeMenubar is the OS's menu bar of a Menubar: desktop.MenuBar with the
// menus converted.
type nativeMenubar[K comparable] struct {
	menus    []MenubarMenu[K]
	onSelect func(menu, item K)
	store    *menuStore[K]
	onError  func(error)
}

func (n nativeMenubar[K]) Build() tela.Node {
	menus := make([]desktop.MenuItem, len(n.menus))
	for i, mm := range n.menus {
		k := mm.Key
		flat := flattenMenu("Menubar", mm.Items)
		p := menuPanel[K]{items: mm.Items, store: n.store}
		if onSelect := n.onSelect; onSelect != nil {
			p.onSelect = func(item K) { onSelect(k, item) }
		}
		menus[i] = desktop.MenuItem{
			Label: mm.Label, Disabled: mm.Disabled,
			Submenu: nativeItems(mm.Items, n.store, func(item K) { p.choose(flat, item) }),
		}
	}
	return desktop.MenuBar{Menus: menus, OnError: n.onError}
}

// drawnMenubar is the bar the kit draws, with its menus in a portal.
type drawnMenubar[K comparable] struct {
	bar   Menubar[K]
	store *menuStore[K]
}

func (d drawnMenubar[K]) Build() tela.Node {
	m, store := d.bar, d.store
	st := &UseStyle().Menubar
	keys := make([]K, len(m.Menus))
	var disabled []K
	for i, mm := range m.Menus {
		keys[i] = mm.Key
		if mm.Disabled {
			disabled = append(disabled, mm.Key)
		}
	}
	dir, rootDir := useDirection("Menubar", m.Direction)
	bar := aria.UseMenuBar(aria.MenuBarOptions[K]{
		Keys: keys, DisabledKeys: disabled, Open: m.Open, DefaultOpen: m.DefaultOpen, OnOpenChange: m.OnOpenChange,
		AriaLabel: m.AriaLabel, Direction: dir,
	})

	kids := []tela.Child{bar.Props, cls(st.Root, m.Class), rootDir, m.Props}
	for _, mm := range m.Menus {
		kids = append(kids, h.Button(bar.TriggerProps(mm.Key), h.Key(mm.Key), cls(st.Trigger, m.TriggerClass), h.Text(mm.Label)))
	}

	var open tela.Child
	if bar.Open {
		for _, mm := range m.Menus {
			if mm.Key != bar.OpenKey {
				continue
			}
			k := bar.OpenKey
			pop := bar.PopoverOptions(k)
			// shadcn's sideOffset 8 and alignOffset -4: the menu starts 4 px
			// before its button's edge.
			pop.Offset = 8
			pop.Floating = append(menuFloating(), aria.Offset(0, aria.OffsetOptions{CrossAxis: -4}))
			panel := menuPanel[K]{
				component: "Menubar", style: &st.Menu, items: mm.Items, store: store,
				popover: pop, menuProps: bar.MenuProps(k), ref: bar.MenuRef(k), strategy: bar.FocusStrategy, close: bar.Close,
				checkIcon: m.CheckIcon, radioIcon: m.RadioIcon, class: m.ContentClass,
			}
			if onSelect := m.OnSelect; onSelect != nil {
				panel.onSelect = func(item K) { onSelect(k, item) }
			}
			// Each menu mounts anew, so it focuses its first item (ADR-133).
			open = tela.WithKey(panel, k)
			break
		}
	}
	return h.Fragment(h.Div(kids...), h.Portal(h.Presence(open)))
}
