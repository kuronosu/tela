package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
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
	menu.CheckboxItem = "relative flex cursor-default items-center gap-2 rounded-xs py-1.5 pr-2 pl-8 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"
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
type Menubar[K comparable] struct {
	// Menus are the bar's menus, in order.
	Menus []MenubarMenu[K]
	// OnSelect is called with the key of the menu and of the plain item
	// that was chosen, after the item's own OnSelect.
	OnSelect func(menu, item K)
	// AriaLabel names the bar.
	AriaLabel string
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

// Build returns the bar and, in a portal, the open menu.
func (m Menubar[K]) Build() tela.Node {
	st := &UseStyle().Menubar
	store := useMenuStore[K]()
	keys := make([]K, len(m.Menus))
	var disabled []K
	for i, mm := range m.Menus {
		keys[i] = mm.Key
		if mm.Disabled {
			disabled = append(disabled, mm.Key)
		}
	}
	bar := aria.UseMenuBar(aria.MenuBarOptions[K]{
		Keys: keys, DisabledKeys: disabled, Open: m.Open, DefaultOpen: m.DefaultOpen, OnOpenChange: m.OnOpenChange,
		AriaLabel: m.AriaLabel,
	})

	kids := []tela.Child{bar.Props, cls(st.Root, m.Class), m.Props}
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
