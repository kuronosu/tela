package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// editItems mixes the standard edit items with one of the app's.
func editItems(log *[]string) []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Key: "undo", Role: desktop.RoleUndo},
		{Kind: kit.MenuSeparator},
		{Key: "cut", Role: desktop.RoleCut},
		{Key: "copy", Role: desktop.RoleCopy},
		{Key: "paste", Role: desktop.RolePaste},
		{Key: "all", Role: desktop.RoleSelectAll},
		{Kind: kit.MenuSeparator},
		{Key: "find", Label: "Find…", Shortcut: "CmdOrCtrl+F", OnSelect: func() { *log = append(*log, "find") }},
	}
}

// A role item of a context menu on a text field acts on the field: the
// focus is back in it when the role is carried out.
func TestContextMenuRoleItems(t *testing.T) {
	var log []string
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ContextMenu[string]{
			AriaLabel: "Edit", Items: editItems(&log), OnSelect: func(k string) { log = append(log, "select:"+k) },
			Children: kit.Input{AriaLabel: "Note"},
		})
	}
	app := mountContext(t, page, 480, 360)
	note := telatest.ByRole("textbox", "Note")
	app.Click(note)
	app.Type("hello")
	app.Press("ControlOrMeta+a")
	app.Click(note, telatest.Right())
	// The role's label.
	app.ExpectCount(cmItem("Copy"), 1)
	app.Click(cmItem("Copy"))
	app.ExpectCount(telatest.ByRole("menu", "Edit"), 0)
	if got := app.Clipboard().Text(); got != "hello" {
		t.Errorf("clipboard %q after Copy", got)
	}
	app.ExpectFocused(note)
	// Cut, then Paste twice.
	app.Click(note, telatest.Right())
	app.Click(cmItem("Cut"))
	app.ExpectValue(note, "")
	for range 2 {
		app.Click(note, telatest.Right())
		app.Click(cmItem("Paste"))
	}
	app.ExpectValue(note, "hellohello")
	// A role item reports nothing; the app's own item does.
	app.Click(note, telatest.Right())
	app.Click(cmItem("Find…"))
	if got := fmt.Sprint(log); got != "[find select:find]" {
		t.Errorf("log %s", got)
	}
}

// A drawn menu bar's Edit menu acts on the field the user was in before
// pressing the bar: the bar's button took the focus, the role gives it
// back.
func TestMenubarRoleItemsActOnTheField(t *testing.T) {
	var log []string
	page := func() tela.Node {
		return h.Div(h.Class("flex flex-col gap-4 p-6"),
			kit.Menubar[string]{AriaLabel: "Application", Menus: []kit.MenubarMenu[string]{
				{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu, Items: editItems(&log)},
			}},
			kit.Input{AriaLabel: "Note"})
	}
	app := telatest.MountFunc(t, page, telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	note := telatest.ByRole("textbox", "Note")
	app.Click(note)
	app.Type("menu bar")
	app.Click(barItem("Edit"))
	app.Click(barItem("Select All"))
	app.ExpectFocused(note)
	app.Click(barItem("Edit"))
	app.Click(barItem("Copy"))
	if got := app.Clipboard().Text(); got != "menu bar" {
		t.Errorf("clipboard %q after Edit > Copy", got)
	}
	app.ExpectFocused(note)
}

// The roles go to a native menu bar as the desktop's roles, with no
// OnSelect, so the OS (or package desktop) carries them out; the app's own
// items keep theirs.
func TestNativeMenubarRoleItems(t *testing.T) {
	var log []string
	page := func() tela.Node {
		return h.Div(kit.Menubar[string]{Native: kit.NativeOn, Menus: []kit.MenubarMenu[string]{
			{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu, Items: editItems(&log)},
		}}, kit.Input{AriaLabel: "Note"})
	}
	app := telatest.MountFunc(t, page, telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	bar := app.MenuBar()
	if len(bar) != 1 || len(bar[0].Submenu) != 8 {
		t.Fatalf("menu bar %+v", bar)
	}
	items := bar[0].Submenu
	if items[3].Role != desktop.RoleCopy || items[3].Label != "Copy" || items[7].Role != desktop.RoleNone {
		t.Errorf("items %+v", items)
	}
	app.Click(telatest.ByRole("textbox", "Note"))
	app.Type("native")
	app.ChooseMenuItem("Edit", "Select All")
	app.ChooseMenuItem("Edit", "Copy")
	if got := app.Clipboard().Text(); got != "native" {
		t.Errorf("clipboard %q after the native Edit > Copy", got)
	}
	app.ChooseMenuItem("Edit", "Find…")
	if got := fmt.Sprint(log); got != "[find]" {
		t.Errorf("log %s", got)
	}
}

func TestMenuItemRoleOfAMenuPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("an item with RoleEditMenu did not panic")
		}
	}()
	items := []kit.MenuItem[string]{{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu}}
	app := telatest.MountFunc(t, func() tela.Node {
		return kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items}
	}, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
}
