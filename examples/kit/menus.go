package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the menus. DropdownMenu, Menubar and ContextMenu take the
// same model of items, kit.MenuItem, and draw the same panel. The labels of
// the menus are written with tr (English here, Spanish and Arabic in the
// catalogs); the role items (Undo, Copy...) take theirs from the catalogs'
// "tela.menu.*" messages.

// chose is the readout under a menu: what was chosen, or the prompt.
func chose(tr trFunc, last, prompt string) tela.Node {
	if last == "" {
		return readout(prompt)
	}
	return readout(tr("menu.chose", "Chose: {item}", "item", last))
}

// ExportMenu is a dropdown whose trigger is a TriggerFunc that draws a
// Button of its own. Choosing a format "exports" for two seconds, and the
// trigger is Loading meanwhile; kit.LoadingTrigger tells the menu so, since
// the Button's own Loading cannot reach the menu's hook through a function:
// pressing or opening it from the keyboard opens nothing until it is done.
type ExportMenu struct{}

func (ExportMenu) Build() tela.Node {
	tr := useTr()
	saving, start := useSaving()
	format, setFormat := tela.UseState("")
	return Div(Class("flex flex-col items-start gap-4"),
		kit.DropdownMenu[string]{
			Trigger: kit.LoadingTrigger(kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return kit.Button{Variant: kit.Outline, Icon: icons.Download, Loading: saving, Props: p, Children: Text(tr("menu.export", "Export"))}
			}), saving),
			Align: "start",
			Items: []kit.MenuItem[string]{
				{Key: "pdf", Label: "PDF"}, {Key: "csv", Label: "CSV"}, {Key: "json", Label: "JSON"}},
			OnSelect: func(key string) {
				setFormat.Set(key)
				start()
			},
		},
		readout(map[bool]string{
			true:  tr("menu.exporting", "Exporting {format}...", "format", strings.ToUpper(format)),
			false: tr("menu.chooseFormat", "Choose a format."),
		}[saving]))
}

func dropdownMenuPage() tela.Node {
	tr := useTr()
	last, setLast := tela.UseState("")
	invited, setInvited := tela.UseState("")
	cols, setCols := tela.UseState(true)
	simple := []kit.MenuItem[string]{
		{Key: "one", Label: tr("menu.first", "First item")}, {Key: "two", Label: tr("menu.second", "Second item")}, {Key: "three", Label: tr("menu.third", "Third item")}}
	return grid(
		demo("My account", "Enter, Space or the arrow keys open it on an item; Home, End and typing jump; Escape closes and the focus goes back to the trigger.", colStart,
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text(tr("menu.open", "Open"))},
				Align:   "start",
				Items: []kit.MenuItem[string]{
					{Kind: kit.MenuLabel, Label: tr("menu.account", "My account")},
					{Kind: kit.MenuSeparator},
					{Key: "profile", Label: tr("menu.profile", "Profile"), Icon: icons.User, Shortcut: "Shift+Cmd+P"},
					{Key: "billing", Label: tr("menu.billing", "Billing"), Icon: icons.CreditCard, Shortcut: "Cmd+B"},
					{Key: "settings", Label: tr("menu.settings", "Settings"), Icon: icons.Settings, Shortcut: "Cmd+,"},
					{Key: "keys", Label: tr("menu.keys", "Keyboard shortcuts"), Disabled: true},
					{Kind: kit.MenuSeparator},
					{Key: "logout", Label: tr("menu.logout", "Log out"), Icon: icons.LogOut, Variant: kit.Destructive},
				},
				OnSelect: func(key string) { setLast.Set(key) },
			},
			chose(tr, last, tr("menu.choose", "Choose an item."))),
		demo("Submenus", "A MenuSub opens a submenu beside it: ArrowRight (ArrowLeft under right-to-left), Enter or Space open it, and so does the pointer resting on it; ArrowLeft and Escape close one level. Submenus nest.", colStart,
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Icon: icons.UserPlus, Children: Text(tr("menu.team", "Team"))},
				Align:   "start",
				Items: []kit.MenuItem[string]{
					{Key: "profile2", Label: tr("menu.members", "Members"), Icon: icons.User},
					{Kind: kit.MenuSub, Key: "invite", Label: tr("menu.invite", "Invite users"), Icon: icons.UserPlus, Items: []kit.MenuItem[string]{
						{Key: "email", Label: tr("menu.email", "Email"), Icon: icons.Mail},
						{Key: "message", Label: tr("menu.message", "Message"), Icon: icons.MessageSquare},
						{Kind: kit.MenuSeparator},
						{Kind: kit.MenuSub, Key: "more", Label: tr("menu.more", "More..."), Items: []kit.MenuItem[string]{
							{Key: "slack", Label: "Slack"}, {Key: "teams", Label: "Teams"}, {Key: "discord", Label: "Discord"}}},
					}},
					{Key: "newTeam", Label: tr("menu.newTeam", "New team"), Icon: icons.Plus, Shortcut: "Cmd+T"},
				},
				OnSelect: func(key string) { setInvited.Set(key) },
			},
			chose(tr, invited, tr("menu.pickInvite", "Open Invite users."))),
		demo("Checkboxes and radios", "A checkbox item toggles and a radio group chooses; both follow the value rule and keep their state while the menu is closed.", colStart,
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text(tr("menu.view", "View"))},
				Align:   "start",
				Items: []kit.MenuItem[string]{
					{Kind: kit.MenuLabel, Label: tr("menu.appearance", "Appearance")},
					{Kind: kit.MenuCheckbox, Key: "bars", Label: tr("menu.statusBar", "Status bar"), Checked: &cols, OnCheckedChange: setCols.Set, KeepOpen: true},
					{Kind: kit.MenuCheckbox, Key: "activity", Label: tr("menu.activityBar", "Activity bar"), DefaultChecked: false, KeepOpen: true},
					{Kind: kit.MenuCheckbox, Key: "panel", Label: tr("menu.panel", "Panel"), Disabled: true},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuLabel, Label: tr("menu.panelPosition", "Panel position")},
					{Kind: kit.MenuRadioGroup, Key: "position", DefaultValue: new("bottom"), Items: []kit.MenuItem[string]{
						{Key: "top", Label: tr("menu.top", "Top")}, {Key: "bottom", Label: tr("menu.bottom", "Bottom")}, {Key: "right", Label: tr("menu.right", "Right")}}},
				},
			},
			readout(map[bool]string{true: tr("menu.barShown", "Status bar: shown"), false: tr("menu.barHidden", "Status bar: hidden")}[cols])),
		demo("Row actions", "An icon button as the trigger; Align end opens the menu under its right edge.", "",
			Div(Class("flex w-full items-center justify-between gap-4 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text("quarterly-report.pdf")), muted(tr("menu.fileInfo", "2.4 MB, edited today"))),
				kit.DropdownMenu[string]{
					Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Ellipsis, AriaLabel: tr("menu.actionsFor", "Actions for quarterly-report.pdf")},
					Align:   "end",
					Items: []kit.MenuItem[string]{
						{Key: "open", Label: tr("menu.openFile", "Open")},
						{Key: "download", Label: tr("menu.download", "Download"), Icon: icons.Download},
						{Key: "share", Label: tr("menu.share", "Share"), Icon: icons.Share2},
						{Kind: kit.MenuSeparator},
						{Key: "delete", Label: tr("menu.delete", "Delete"), Icon: icons.Trash2, Variant: kit.Destructive},
					},
				})),
		demo("A busy trigger", "A trigger drawn by a kit.TriggerFunc has its own Button, whose Loading the menu cannot see: kit.LoadingTrigger(trigger, loading) tells it. Choose a format: the button works for two seconds, opens nothing meanwhile (pressed or from the keyboard), and keeps its place in the tab order.", colStart,
			ExportMenu{}),
		demo("Placement", "Placement and Align choose the side and the position along it.", "",
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("menu.below", "Below, centered"))}, Items: simple},
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("menu.rightSide", "Right"))}, Placement: "right", Align: "start", Items: simple},
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("menu.aboveEnd", "Above, at the end"))}, Placement: "top", Align: "end", Items: simple}),
	)
}

func menubarPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	last, setLast := tela.UseState("")
	found, setFound := tela.UseState(false)
	return grid(
		demo("Application menu", "One tab stop; the arrows move between the menus and, with one open, open the neighbor's. Hovering another button opens that one.", colStart,
			kit.Menubar[string]{
				AriaLabel: tr("menubar.application", "Application"),
				OnSelect:  func(menu, item string) { setLast.Set(menu + " > " + item) },
				Menus: []kit.MenubarMenu[string]{
					{Key: "file", Label: tr("menubar.file", "File"), Items: []kit.MenuItem[string]{
						{Key: "new-tab", Label: tr("menubar.newTab", "New Tab"), Shortcut: "Cmd+T"},
						{Key: "new-window", Label: tr("menubar.newWindow", "New Window"), Shortcut: "Cmd+N"},
						{Key: "incognito", Label: tr("menubar.incognito", "New Incognito Window"), Disabled: true},
						{Kind: kit.MenuSeparator},
						{Key: "print", Label: tr("menubar.print", "Print..."), Shortcut: "Cmd+P"},
					}},
					{Key: "edit", Label: tr("menubar.edit", "Edit"), Items: []kit.MenuItem[string]{
						{Key: "undo", Label: tr("menubar.undo", "Undo"), Shortcut: "Cmd+Z"},
						{Key: "redo", Label: tr("menubar.redo", "Redo"), Shortcut: "Shift+Cmd+Z"},
						{Kind: kit.MenuSeparator},
						{Key: "cut", Label: tr("menubar.cut", "Cut")}, {Key: "copy", Label: tr("menubar.copy", "Copy")}, {Key: "paste", Label: tr("menubar.paste", "Paste")},
					}},
					{Key: "view", Label: tr("menubar.view", "View"), Items: []kit.MenuItem[string]{
						{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: tr("menubar.bookmarks", "Always Show Bookmarks Bar")},
						{Kind: kit.MenuCheckbox, Key: "urls", Label: tr("menubar.urls", "Always Show Full URLs"), DefaultChecked: true},
						{Kind: kit.MenuSeparator},
						{Key: "reload", Label: tr("menubar.reload", "Reload"), Shortcut: "Cmd+R", Inset: true},
						{Key: "force", Label: tr("menubar.force", "Force Reload"), Shortcut: "Shift+Cmd+R", Inset: true, Disabled: true},
					}},
					{Key: "profiles", Label: tr("menubar.profiles", "Profiles"), Items: []kit.MenuItem[string]{
						{Kind: kit.MenuRadioGroup, Key: "profile", DefaultValue: new("benoit"), Items: []kit.MenuItem[string]{
							{Key: "andy", Label: "Andy"}, {Key: "benoit", Label: "Benoit"}, {Key: "luis", Label: "Luis"}}},
						{Kind: kit.MenuSeparator},
						{Key: "edit-profile", Label: tr("menubar.editProfile", "Edit..."), Inset: true},
						{Key: "add-profile", Label: tr("menubar.addProfile", "Add Profile..."), Inset: true},
					}},
				},
			},
			chose(tr, last, tr("menubar.pick", "Pick something from the menus."))),
		demo("Edit menu with roles", "Items with a Role (desktop.RoleUndo, RoleCut...) are the standard ones: their keys come from the role, their label is the catalogs' tela.menu.* message (passed here from the page's formatter), and they act on the text field that had the focus. Find is the app's own. Select some text in the field, then use Edit.", colStart,
			Div(Class("flex w-full max-w-sm flex-col gap-3"),
				kit.Menubar[string]{
					AriaLabel: tr("menubar.editBar", "Edit"), Class: "self-start",
					OnSelect: func(menu, item string) {
						if item == "find" {
							setFound.Set(true)
						}
					},
					Menus: []kit.MenubarMenu[string]{
						{Key: "edit", Label: tr("menubar.edit", "Edit"), Items: []kit.MenuItem[string]{
							{Key: "undo", Label: f.Label("tela.menu.undo"), Role: desktop.RoleUndo},
							{Key: "redo", Label: f.Label("tela.menu.redo"), Role: desktop.RoleRedo},
							{Kind: kit.MenuSeparator},
							{Key: "cut", Label: f.Label("tela.menu.cut"), Role: desktop.RoleCut},
							{Key: "copy", Label: f.Label("tela.menu.copy"), Role: desktop.RoleCopy},
							{Key: "paste", Label: f.Label("tela.menu.paste"), Role: desktop.RolePaste},
							{Key: "selectAll", Label: f.Label("tela.menu.selectAll"), Role: desktop.RoleSelectAll},
							{Kind: kit.MenuSeparator},
							{Key: "find", Label: tr("menubar.find", "Find..."), Shortcut: "CmdOrCtrl+F"},
						}},
					},
				},
				kit.Input{DefaultValue: tr("menubar.sample", "Select this text, then cut or copy it."), AriaLabel: tr("menubar.text", "Text")}),
			readout(map[bool]string{true: tr("menubar.findChosen", "Find was chosen."), false: tr("menubar.findHint", "Find is the app's item: it calls its OnSelect.")}[found])),
		demo("A styled submenu", "ContentClass styles the panel of a MenuSub's submenu (shadcn's SubContent className), apart from the Class of its item: here Export as is wider than the Share submenu beside it.", colStart,
			kit.Menubar[string]{
				AriaLabel: tr("menubar.fileBar", "File"),
				Menus: []kit.MenubarMenu[string]{
					{Key: "file", Label: tr("menubar.file", "File"), Items: []kit.MenuItem[string]{
						{Key: "new", Label: tr("menubar.newTab", "New Tab"), Shortcut: "Cmd+T"},
						{Kind: kit.MenuSub, Key: "share", Label: tr("menubar.share", "Share"), Items: []kit.MenuItem[string]{
							{Key: "email", Label: tr("menubar.email", "Email link")}, {Key: "messages", Label: tr("menubar.messages", "Messages")}}},
						{Kind: kit.MenuSub, Key: "export", Label: tr("menubar.exportAs", "Export as"), ContentClass: "min-w-64", Items: []kit.MenuItem[string]{
							{Key: "pdf", Label: tr("menubar.pdf", "PDF document"), Shortcut: "Shift+Cmd+E"}, {Key: "png", Label: tr("menubar.png", "PNG image")},
							{Key: "html", Label: tr("menubar.html", "Web page")}}},
					}},
				},
			}),
		demo("The window's menu bar", "AppMenu makes a bar the app's: the OS's where native menus apply, drawn where it is declared otherwise (on Linux: in the title bar or at the top of the window). A drawn one is also the window's bar for the keyboard: F10 or a lone Alt focuses its first menu from anywhere in the window, and F10 or Escape gives the focus back. The Mac's key conventions have neither. The gallery's own window has no app bar, so the button opens a frameless window with one: press F10 or Alt in it.", col,
			Div(Class("flex"), DemoWindow{}),
			snippet("kit.TitleBar{\n\tChildren: kit.Menubar[string]{\n\t\tAppMenu: true, Menus: menus}}")),
	)
}

func contextMenuPage() tela.Node {
	tr := useTr()
	last, setLast := tela.UseState("")
	picked, setPicked := tela.UseState("")
	files := []string{"budget.xlsx", "notes.md", "photo.png", "report.pdf"}
	lb := aria.UseListBox(aria.ListBoxOptions[string]{
		Keys: files, SelectionMode: "multiple", SelectionBehavior: "replace", AriaLabel: tr("context.files", "Files"),
	})
	chosen := lb.Selected.In(files)
	labels := map[string]string{"open": tr("context.open", "Open"), "rename": tr("context.rename", "Rename"), "delete": tr("context.delete", "Delete")}
	var options []tela.Child
	for _, name := range files {
		options = append(options, Div(lb.ItemProps(name), Key(name),
			Class("group flex items-center justify-between border-b border-border px-3 py-2.5 last:border-b-0 hover:bg-accent/50 aria-selected:bg-accent aria-selected:font-medium focus-visible:bg-accent/50 focus-visible:outline-none"),
			Span(Text(name)), Span(Class("text-muted-foreground"), ic(icons.Check, "size-4 opacity-0 group-aria-selected:opacity-100"))))
	}
	return grid(
		demo("Right click", "A right click opens it at the pointer; a long press on touch. From the keyboard, Tab to the box (TabIndex 0 makes it focusable, with a role and a name) and press Shift+F10 or the context menu key: the menu opens below the box.", col,
			kit.ContextMenu[string]{
				AriaLabel: tr("context.page", "Page"),
				// Focusable for the keyboard, so it gets a role and a name
				// for screen readers, and the keys that open its menu.
				Props: tela.MergeProps(TabIndex(0), Role("region"), AriaLabel(tr("context.area", "Page area")),
					AriaKeyShortcuts("Shift+F10 ContextMenu")),
				Class: "flex h-40 w-72 items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground outline-none " +
					"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
				Children: Text(tr("context.here", "Right click here")),
				OnSelect: func(key string) { setLast.Set(key) },
				Items: []kit.MenuItem[string]{
					{Key: "back", Label: tr("context.back", "Back"), Shortcut: "Cmd+["},
					{Key: "forward", Label: tr("context.forward", "Forward"), Shortcut: "Cmd+]", Disabled: true},
					{Key: "reload", Label: tr("context.reload", "Reload"), Shortcut: "Cmd+R"},
					{Kind: kit.MenuSub, Key: "share", Label: tr("context.share", "Share"), Icon: icons.Share2, Items: []kit.MenuItem[string]{
						{Key: "email", Label: tr("context.emailLink", "Email link"), Icon: icons.Mail},
						{Key: "messages", Label: tr("context.messages", "Messages"), Icon: icons.MessageSquare},
					}},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: tr("context.bookmarks", "Show Bookmarks"), DefaultChecked: true},
					{Kind: kit.MenuCheckbox, Key: "urls", Label: tr("context.urls", "Show Full URLs")},
					{Kind: kit.MenuSeparator},
					{Kind: kit.MenuLabel, Label: tr("context.people", "People")},
					{Kind: kit.MenuRadioGroup, Key: "person", DefaultValue: new("pedro"), Items: []kit.MenuItem[string]{
						{Key: "pedro", Label: "Pedro Duarte"}, {Key: "colm", Label: "Colm Tuite"}}},
				},
			},
			chose(tr, last, tr("context.rightClick", "Right click the box."))),
		demo("A list box inside", "A right click on an option that is not selected selects it (and only it) before the menu opens, as Finder does; one on a selected option keeps the selection, so the menu acts on all of it. Ctrl+click or Shift+click first to select several.", col,
			kit.ContextMenu[string]{
				AriaLabel: tr("context.files", "Files"),
				Class:     "w-72 overflow-hidden rounded-md border border-border text-sm",
				Children:  Div(lb.Props, Class("outline-none"), Fragment(options...)),
				OnSelect: func(key string) {
					setPicked.Set(tr("context.action", "{action}: {files}", "action", labels[key], "files", strings.Join(chosen, ", ")))
				},
				Items: []kit.MenuItem[string]{
					{Key: "open", Label: labels["open"]},
					{Key: "rename", Label: labels["rename"]},
					{Kind: kit.MenuSeparator},
					{Key: "delete", Label: labels["delete"], Variant: kit.Destructive},
				},
			},
			readout(map[bool]string{true: tr("context.none", "Nothing is selected."), false: tr("context.selected", "Selected: {files}", "files", strings.Join(chosen, ", "))}[len(chosen) == 0]),
			If(picked != "", readout(picked))),
		demo("On a list row", "Each row is its own area, so the innermost menu wins.", col,
			Div(Class("flex w-72 flex-col overflow-hidden rounded-md border border-border text-sm"),
				fileRow(tr, "budget.xlsx", "1.2 MB"), fileRow(tr, "notes.md", "4 KB"), fileRow(tr, "photo.png", "3.8 MB"))),
	)
}

func fileRow(tr trFunc, name, size string) tela.Node {
	return kit.ContextMenu[string]{
		Props: Key(name), AriaLabel: name,
		Class:    "flex items-center justify-between border-b border-border px-3 py-2.5 last:border-b-0 hover:bg-accent/50",
		Children: Fragment(Span(Class("font-medium"), Text(name)), Span(Dir("ltr"), Class("text-muted-foreground"), Text(size))),
		Items: []kit.MenuItem[string]{
			{Key: "open", Label: tr("context.open", "Open")}, {Key: "rename", Label: tr("context.rename", "Rename")}, {Key: "duplicate", Label: tr("context.duplicate", "Duplicate")},
			{Kind: kit.MenuSeparator},
			{Key: "delete", Label: tr("context.delete", "Delete"), Variant: kit.Destructive},
		},
	}
}
