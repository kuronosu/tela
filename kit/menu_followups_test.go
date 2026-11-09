package kit_test

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The follow-ups of ADR-255: menus, menu bars and triggers.

// shareFile is the File menu with a Share submenu at its end.
func shareFile() kit.MenubarMenu[string] {
	file := fileMenu()
	file.Items = append(file.Items, kit.MenuItem[string]{Kind: kit.MenuSub, Key: "share", Label: "Share", Items: []kit.MenuItem[string]{
		{Key: "email", Label: "Email link"}, {Key: "messages", Label: "Messages"}}})
	return file
}

// A press on another button of the bar while a submenu is open switches to
// that menu, as it does with no submenu open (it only closed the menu: the
// submenu, the top overlay, took the press as one outside it). A touch, which
// does not hover: the mouse switches menus when it enters the button, before
// its press.
func TestMenubarPressSwitchesWithASubmenuOpen(t *testing.T) {
	var opened []string
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) {
		m.Menus[0] = shareFile()
		m.OnOpenChange = func(k string, open bool) { opened = append(opened, fmt.Sprint(k, open)) }
	}), telatest.Size(640, 480))
	app.Click(barItem("File"))
	app.Click(barItem("Share"))
	app.ExpectCount(barItem("Email link"), 1)
	app.Tap(barItem("Edit"))
	app.ExpectCount(menuNamed("Edit"), 1)
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectCount(barItem("Email link"), 0)
	// As without a submenu: the touch's focus closes File, its release
	// opens Edit.
	if got := fmt.Sprint(opened); got != "[filetrue filefalse edittrue]" {
		t.Errorf("OnOpenChange %s", got)
	}
	// A press outside still closes everything.
	app.Tap(barItem("File"))
	app.Click(barItem("Share"))
	app.ExpectCount(barItem("Email link"), 1)
	app.ClickAt(600, 450)
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectCount(barItem("Email link"), 0)
}

// editBarPage is a menu bar with the standard Edit items and a field
// before it.
func editBarPage(log *[]string, appMenu bool) func() tela.Node {
	return func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"),
			kit.Input{AriaLabel: "Note"},
			kit.Menubar[string]{AriaLabel: "Application", AppMenu: appMenu, Menus: []kit.MenubarMenu[string]{
				fileMenu(),
				{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu, Items: editItems(log)},
			}},
			h.Button(h.Class("h-9"), h.Text("After")))
	}
}

// A drawn role item of a menu opened from the keyboard acts on the field
// the focus came from (it acted on the bar's button, where the focus was).
func TestMenubarRoleFromTheKeyboard(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, editBarPage(&log, false), telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	note := telatest.ByRole("textbox", "Note")
	app.Click(note)
	app.Type("keyboard")
	app.Press("ControlOrMeta+a")
	app.Press("Tab") // the bar's File
	app.ExpectFocused(barItem("File"))
	app.Press("ArrowRight")
	app.Press("ArrowDown") // Edit, on Undo
	app.ExpectFocused(barItem("Undo"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(barItem("Copy"))
	app.Press("Enter")
	if got := app.Clipboard().Text(); got != "keyboard" {
		t.Errorf("clipboard %q after Edit > Copy from the keyboard", got)
	}
	app.ExpectFocused(note)

	// The focus that left the bar for another element is no origin: a
	// later Tab from the After button back to the bar acts there.
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "After"))
	app.Press("Shift+Tab") // the bar's tab stop, the last button focused
	app.ExpectFocused(barItem("Edit"))
	app.Press("ArrowDown")
	app.Press("End") // Find…
	app.Press("ArrowUp")
	app.ExpectFocused(barItem("Select All"))
	app.Press("Enter")
	app.ExpectFocused(telatest.ByRole("button", "After"))
}

// The same for a dropdown menu: Tab from the field to its trigger, then
// the keys.
func TestDropdownRoleFromTheKeyboard(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex gap-4 p-6"),
			kit.Input{AriaLabel: "Note"},
			kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Edit")}, Items: editItems(&log)})
	}, telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	note := telatest.ByRole("textbox", "Note")
	app.Click(note)
	app.Type("dropdown")
	app.Press("ControlOrMeta+a")
	app.Press("Tab")
	app.Press("Enter")
	app.ExpectFocused(cmItem("Undo"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(cmItem("Copy"))
	app.Press("Enter")
	if got := app.Clipboard().Text(); got != "dropdown" {
		t.Errorf("clipboard %q after Copy from the keyboard", got)
	}
	app.ExpectFocused(note)
}

// The app's drawn menu bar takes F10 and a lone Alt from anywhere in the
// window (Windows and Linux): the first button gets the focus, and the
// same keys or Escape give it back.
func TestMenubarActivationKeys(t *testing.T) {
	for _, key := range []string{"F10", "Alt"} {
		t.Run(key, func(t *testing.T) {
			var log []string
			app := telatest.MountFunc(t, editBarPage(&log, true), telatest.Size(480, 360),
				telatest.Theme(kit.Theme()), telatest.PlatformKeys("linux"))
			note := telatest.ByRole("textbox", "Note")
			app.Click(note)
			app.Type("activated")
			app.Press("ControlOrMeta+a")
			app.Press(key)
			app.ExpectFocused(barItem("File"))
			app.ExpectCount(menuNamed("File"), 0) // highlighted, not open
			app.Press("Escape")
			app.ExpectFocused(note)
			// Again, open Edit, and the key leaves the menu and the bar.
			app.Press(key)
			app.Press("ArrowRight")
			app.Press("ArrowDown")
			app.ExpectCount(menuNamed("Edit"), 1)
			app.Press(key)
			app.ExpectCount(menuNamed("Edit"), 0)
			app.ExpectFocused(note)
			// Edit > Copy acts on the field.
			app.Press(key)
			app.Press("ArrowRight")
			app.Press("ArrowDown") // Undo
			app.Press("ArrowDown")
			app.Press("ArrowDown")
			app.ExpectFocused(barItem("Copy"))
			app.Press("Enter")
			if got := app.Clipboard().Text(); got != "activated" {
				t.Errorf("clipboard %q", got)
			}
			app.ExpectFocused(note)
		})
	}
}

// The keys need AppMenu (the window's bar), and the Mac has no such keys.
func TestMenubarActivationKeysScope(t *testing.T) {
	cases := []struct {
		name    string
		appMenu bool
		goos    string
	}{{"not the app's bar", false, "linux"}, {"mac", true, "darwin"}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var log []string
			app := telatest.MountFunc(t, editBarPage(&log, c.appMenu), telatest.Size(480, 360),
				telatest.Theme(kit.Theme()), telatest.PlatformKeys(c.goos))
			note := telatest.ByRole("textbox", "Note")
			app.Click(note)
			app.Press("F10")
			app.Press("Alt")
			app.ExpectFocused(note)
		})
	}
}

// A MenuSub's ContentClass styles its submenu's panel (Class is its
// trigger's); the menu's own Class and ContentClass stay on the root
// panel.
func TestMenuSubContentClass(t *testing.T) {
	items := []kit.MenuItem[string]{
		{Key: "team", Label: "Team"},
		{Kind: kit.MenuSub, Key: "invite", Label: "Invite users", Class: "italic", ContentClass: "min-w-56 ring-2", Items: []kit.MenuItem[string]{
			{Key: "email", Label: "Email"}, {Key: "message", Label: "Message"}}},
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items, Class: "ring-1"})
	}, telatest.Size(640, 480), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
	app.Click(cmItem("Invite users"))
	app.ExpectCount(telatest.ByClass("ring-2"), 1)
	app.ExpectCount(telatest.ByClass("ring-1"), 1)
	app.ExpectCount(telatest.ByClass("italic"), 1)
	if w := app.Bounds(telatest.ByClass("ring-2")).Width; w != 224 {
		t.Errorf("submenu width %v, want min-w-56 (224)", w)
	}
}

// The submenu with a ContentClass, and a shortcut under right to left
// (it reads "⌘T", not "T⌘").
func TestMenuSubContentClassGolden(t *testing.T) {
	items := []kit.MenuItem[string]{
		{Key: "team", Label: "Team", Shortcut: "⇧⌘T"},
		{Kind: kit.MenuSub, Key: "invite", Label: "Invite users", ContentClass: "w-56 bg-muted", Items: []kit.MenuItem[string]{
			{Key: "email", Label: "Email", Shortcut: "Ctrl+E"}, {Key: "message", Label: "Message"}}},
	}
	for _, dir := range []string{"ltr", "rtl"} {
		page := func() tela.Node {
			return h.Div(h.Class("flex justify-center p-6"), h.Dir(dir), kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")}, Items: items,
			})
		}
		openGolden(t, "menu_sub_class_"+dir, 720, 260, page, func(a *telatest.App) {
			a.Focus(telatest.ByRole("button", "Open"))
			a.Press("Enter")
			a.Press("ArrowDown")
			if dir == "rtl" {
				a.Press("ArrowLeft")
			} else {
				a.Press("ArrowRight")
			}
		})
	}
}

// A user's TriggerFunc around a loading Button opens nothing once it is a
// LoadingTrigger, from the pointer or the keyboard, and keeps the focus.
func TestLoadingTriggerFunc(t *testing.T) {
	trigger := func(loading bool) kit.Trigger {
		f := kit.TriggerFunc(func(p tela.Prop) tela.Node {
			return kit.Button{Loading: loading, Variant: kit.Outline, Props: p, Children: h.Text("Open")}
		})
		return kit.LoadingTrigger(f, loading)
	}
	items := []kit.MenuItem[string]{{Key: "a", Label: "Item A"}}
	for _, name := range []string{"dropdown", "popover", "tooltip over a menu"} {
		t.Run(name, func(t *testing.T) {
			loading := true
			var set tela.Setter[bool]
			app := telatest.MountFunc(t, func() tela.Node {
				l, s := tela.UseState(loading)
				set = s
				var ov tela.Component
				switch name {
				case "dropdown":
					ov = kit.DropdownMenu[string]{Trigger: trigger(l), Items: items}
				case "popover":
					ov = kit.Popover{Trigger: trigger(l), Children: h.Text("Body")}
				default:
					ov = kit.Tooltip{Content: h.Text("Tip"), Trigger: kit.DropdownMenu[string]{Trigger: trigger(l), Items: items}}
				}
				return h.Div(h.Class("p-6"), ov)
			}, telatest.Theme(kit.Theme()))
			open := telatest.ByRole("button", "Open")
			opened := func() int {
				return app.Count(telatest.ByText("Body")) + app.Count(cmItem("Item A"))
			}
			app.Focus(open)
			app.ExpectFocused(open)
			for _, k := range []string{"Enter", "Space", "ArrowDown"} {
				app.Press(k)
				if opened() != 0 {
					t.Fatalf("%s opened the overlay of a loading trigger", k)
				}
			}
			app.ExpectFocused(open)
			app.Expect(open, telatest.Disabled(false))
			// Done loading: it opens.
			set.Set(false)
			app.Click(open)
			if opened() != 1 {
				t.Errorf("the idle trigger opens nothing")
			}
		})
	}
}

// Drawn role items take their labels from the locale of the nearest
// i18n.Provider (they followed the process-wide locale).
func TestMenuRoleLabelsFollowTheProvider(t *testing.T) {
	err := i18n.Load(fstest.MapFS{
		"l/en.json":  {Data: []byte(`{"tela": {"menu": {"copy": "Copy", "paste": "Paste"}}}`)},
		"l/tlm.json": {Data: []byte(`{"tela": {"menu": {"copy": "Copiar", "paste": "Pegar"}}}`)},
	}, "l/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := i18n.SetFallback("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLocale("") })
	items := []kit.MenuItem[string]{{Key: "copy", Role: desktop.RoleCopy}, {Key: "paste", Role: desktop.RolePaste}}
	app := telatest.Mount(t, i18n.Provider("tlm", tela.C(func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items})
	})), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectCount(cmItem("Copiar"), 1)
	app.ExpectCount(cmItem("Pegar"), 1)
	// The typeahead reads the same labels.
	app.Press("p")
	app.ExpectFocused(cmItem("Pegar"))
}

// keep the aria import used when only a subset of tests is built.
var _ = aria.FocusFirst
