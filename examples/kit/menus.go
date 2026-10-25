package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the menus. DropdownMenu, Menubar and ContextMenu take the
// same model of items, kit.MenuItem, and draw the same panel.

func dropdownMenuPage() tela.Node {
	last, setLast := tela.UseState("")
	cols, setCols := tela.UseState(true)
	return grid(
		demo("My account", "Enter, Space or the arrow keys open it on an item; Home, End and typing jump; Escape closes and the focus goes back to the trigger.", colStart,
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text("Open")},
				Align:   "start",
				Items: []kit.MenuItem[string]{
					{Kind: kit.MenuLabel, Label: "My account"},
					{Kind: kit.MenuSeparator},
					{Key: "profile", Label: "Profile", Icon: icons.User, Shortcut: "Shift+Cmd+P"},
					{Key: "billing", Label: "Billing", Icon: icons.CreditCard, Shortcut: "Cmd+B"},
					{Key: "settings", Label: "Settings", Icon: icons.Settings, Shortcut: "Cmd+,"},
					{Key: "keys", Label: "Keyboard shortcuts", Disabled: true},
					{Kind: kit.MenuSeparator},
					{Key: "logout", Label: "Log out", Icon: icons.LogOut, Variant: kit.Destructive},
				},
				OnSelect: func(key string) { setLast.Set(key) },
			},
			readout(map[bool]string{true: "Choose an item.", false: "Chose: " + last}[last == ""])),
		demo("Checkboxes and radios", "A checkbox item toggles and a radio group chooses; both follow the value rule and keep their state while the menu is closed.", colStart,
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text("View")},
				Align:   "start",
				Items: []kit.MenuItem[string]{
					{Kind: kit.MenuLabel, Label: "Appearance"},
					{Kind: kit.MenuCheckbox, Key: "bars", Label: "Status bar", Checked: &cols, OnCheckedChange: setCols.Set, KeepOpen: true},
					{Kind: kit.MenuCheckbox, Key: "activity", Label: "Activity bar", DefaultChecked: false, KeepOpen: true},
					{Kind: kit.MenuCheckbox, Key: "panel", Label: "Panel", Disabled: true},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuLabel, Label: "Panel position"},
					{Kind: kit.MenuRadioGroup, Key: "position", DefaultValue: new("bottom"), Items: []kit.MenuItem[string]{
						{Key: "top", Label: "Top"}, {Key: "bottom", Label: "Bottom"}, {Key: "right", Label: "Right"}}},
				},
			},
			readout(map[bool]string{true: "Status bar: shown", false: "Status bar: hidden"}[cols])),
		demo("Row actions", "An icon button as the trigger; Align end opens the menu under its right edge.", "",
			Div(Class("flex w-full max-w-sm items-center justify-between gap-4 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text("quarterly-report.pdf")), muted("2.4 MB, edited today")),
				kit.DropdownMenu[string]{
					Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Ellipsis, AriaLabel: "Actions for quarterly-report.pdf"},
					Align:   "end",
					Items: []kit.MenuItem[string]{
						{Key: "open", Label: "Open"},
						{Key: "download", Label: "Download", Icon: icons.Download},
						{Key: "share", Label: "Share", Icon: icons.Share2},
						{Kind: kit.MenuSeparator},
						{Key: "delete", Label: "Delete", Icon: icons.Trash2, Variant: kit.Destructive},
					},
				})),
		demo("Placement", "Placement and Align choose the side and the position along it.", "",
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Below, centered")}, Items: simpleItems},
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Right")}, Placement: "right", Align: "start", Items: simpleItems},
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Above, at the end")}, Placement: "top", Align: "end", Items: simpleItems}),
	)
}

var simpleItems = []kit.MenuItem[string]{{Key: "one", Label: "First item"}, {Key: "two", Label: "Second item"}, {Key: "three", Label: "Third item"}}

func menubarPage() tela.Node {
	last, setLast := tela.UseState("")
	return grid(
		demo("Application menu", "One tab stop; the arrows move between the menus and, with one open, open the neighbor's. Hovering another button opens that one.", colStart,
			kit.Menubar[string]{
				AriaLabel: "Application",
				OnSelect:  func(menu, item string) { setLast.Set(menu + " > " + item) },
				Menus: []kit.MenubarMenu[string]{
					{Key: "file", Label: "File", Items: []kit.MenuItem[string]{
						{Key: "new-tab", Label: "New Tab", Shortcut: "Cmd+T"},
						{Key: "new-window", Label: "New Window", Shortcut: "Cmd+N"},
						{Key: "incognito", Label: "New Incognito Window", Disabled: true},
						{Kind: kit.MenuSeparator},
						{Key: "print", Label: "Print...", Shortcut: "Cmd+P"},
					}},
					{Key: "edit", Label: "Edit", Items: []kit.MenuItem[string]{
						{Key: "undo", Label: "Undo", Shortcut: "Cmd+Z"},
						{Key: "redo", Label: "Redo", Shortcut: "Shift+Cmd+Z"},
						{Kind: kit.MenuSeparator},
						{Key: "cut", Label: "Cut"}, {Key: "copy", Label: "Copy"}, {Key: "paste", Label: "Paste"},
					}},
					{Key: "view", Label: "View", Items: []kit.MenuItem[string]{
						{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Always Show Bookmarks Bar"},
						{Kind: kit.MenuCheckbox, Key: "urls", Label: "Always Show Full URLs", DefaultChecked: true},
						{Kind: kit.MenuSeparator},
						{Key: "reload", Label: "Reload", Shortcut: "Cmd+R", Inset: true},
						{Key: "force", Label: "Force Reload", Shortcut: "Shift+Cmd+R", Inset: true, Disabled: true},
					}},
					{Key: "profiles", Label: "Profiles", Items: []kit.MenuItem[string]{
						{Kind: kit.MenuRadioGroup, Key: "profile", DefaultValue: new("benoit"), Items: []kit.MenuItem[string]{
							{Key: "andy", Label: "Andy"}, {Key: "benoit", Label: "Benoit"}, {Key: "luis", Label: "Luis"}}},
						{Kind: kit.MenuSeparator},
						{Key: "edit-profile", Label: "Edit...", Inset: true},
						{Key: "add-profile", Label: "Add Profile...", Inset: true},
					}},
				},
			},
			readout(map[bool]string{true: "Pick something from the menus.", false: "Chose: " + last}[last == ""])),
	)
}

func contextMenuPage() tela.Node {
	last, setLast := tela.UseState("")
	return grid(
		demo("Right click", "A right click opens it at the pointer; a long press on touch; Shift+F10 or the context menu key open it from the keyboard.", col,
			kit.ContextMenu[string]{
				AriaLabel: "Page",
				Class:     "flex h-40 w-72 items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground",
				Children:  Text("Right click here"),
				OnSelect:  func(key string) { setLast.Set(key) },
				Items: []kit.MenuItem[string]{
					{Key: "back", Label: "Back", Shortcut: "Cmd+["},
					{Key: "forward", Label: "Forward", Shortcut: "Cmd+]", Disabled: true},
					{Key: "reload", Label: "Reload", Shortcut: "Cmd+R"},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Show Bookmarks", DefaultChecked: true},
					{Kind: kit.MenuCheckbox, Key: "urls", Label: "Show Full URLs"},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuLabel, Label: "People"},
					{Kind: kit.MenuRadioGroup, Key: "person", DefaultValue: new("pedro"), Items: []kit.MenuItem[string]{
						{Key: "pedro", Label: "Pedro Duarte"}, {Key: "colm", Label: "Colm Tuite"}}},
				},
			},
			readout(map[bool]string{true: "Right click the box.", false: "Chose: " + last}[last == ""])),
		demo("On a list row", "Each row is its own area, so the innermost menu wins.", col,
			Div(Class("flex w-72 flex-col overflow-hidden rounded-md border border-border text-sm"),
				fileRow("budget.xlsx", "1.2 MB"), fileRow("notes.md", "4 KB"), fileRow("photo.png", "3.8 MB"))),
	)
}

func fileRow(name, size string) tela.Node {
	return Fragment(kit.ContextMenu[string]{
		Props: Key(name), AriaLabel: name,
		Class:    "flex items-center justify-between border-b border-border px-3 py-2.5 last:border-b-0 hover:bg-accent/50",
		Children: Fragment(Span(Class("font-medium"), Text(name)), Span(Class("text-muted-foreground"), Text(size))),
		Items: []kit.MenuItem[string]{
			{Key: "open", Label: "Open"}, {Key: "rename", Label: "Rename"}, {Key: "duplicate", Label: "Duplicate"},
			{Kind: kit.MenuSeparator},
			{Key: "delete", Label: "Delete", Variant: kit.Destructive},
		},
	})
}
