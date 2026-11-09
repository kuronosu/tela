package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Access keys, activation keys and the trigger slot of ADR-260.

// underlined counts the underlined access keys on screen.
func underlined(app *telatest.App) int { return app.Count(telatest.ByClass("underline")) }

// The window's drawn bar has access keys on Windows and Linux: Alt and a
// menu's key open it from anywhere, the menus' and items' keys are
// underlined while the keyboard is in the bar, and an item's key chooses
// it (Edit's Copy takes O, after Cut's C, as a native Windows bar gives
// it).
func TestMenubarAccessKeys(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, editBarPage(&log, true), telatest.Size(480, 360),
		telatest.Theme(kit.Theme()), telatest.PlatformKeys("windows"))
	note := telatest.ByRole("textbox", "Note")
	app.Click(note)
	app.Type("keys")
	app.Press("ControlOrMeta+a")
	if n := underlined(app); n != 0 {
		t.Errorf("%d access keys underlined at rest", n)
	}
	app.Press("Alt+e")
	app.ExpectCount(menuNamed("Edit"), 1)
	app.ExpectFocused(barItem("Undo"))
	// File, Edit; Undo, Cut, Copy, Paste, Select All, Find.
	if n := underlined(app); n != 8 {
		t.Errorf("%d access keys underlined in the bar and the Edit menu, want 8", n)
	}
	app.Press("o")
	if got := app.Clipboard().Text(); got != "keys" {
		t.Errorf("clipboard %q after O (Copy)", got)
	}
	app.ExpectCount(menuNamed("Edit"), 0)
	app.ExpectFocused(note)
	if n := underlined(app); n != 0 {
		t.Errorf("%d access keys underlined once the keyboard left the bar", n)
	}
	// F10 shows them on the bar, moving nothing; the key alone opens the
	// menu there.
	file, edit := app.Bounds(barItem("File")), app.Bounds(barItem("Edit"))
	app.Press("F10")
	if n := underlined(app); n != 2 {
		t.Errorf("%d access keys underlined after F10, want 2", n)
	}
	if f, e := app.Bounds(barItem("File")), app.Bounds(barItem("Edit")); f != file || e != edit {
		t.Errorf("the cues moved the buttons: %+v %+v, were %+v %+v", f, e, file, edit)
	}
	app.Press("e")
	app.ExpectCount(menuNamed("Edit"), 1)
	app.Press("Escape")
	app.Press("Escape")
	app.ExpectFocused(note)
}

// The Mac's key conventions have no access keys: nothing is underlined,
// Alt and a key do nothing, and a key on a focused button is typeahead.
func TestMenubarAccessKeysMac(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, editBarPage(&log, true), telatest.Size(480, 360),
		telatest.Theme(kit.Theme()), telatest.PlatformKeys("darwin"))
	app.Click(telatest.ByRole("textbox", "Note"))
	app.Press("Alt+e")
	app.ExpectCount(menuNamed("Edit"), 0)
	app.Focus(barItem("File"))
	app.Press("e")
	app.ExpectFocused(barItem("Edit"))
	app.ExpectCount(menuNamed("Edit"), 0)
	if n := underlined(app); n != 0 {
		t.Errorf("%d access keys underlined under the Mac flavor", n)
	}
}

// A drawn bar is the window's bar for the keyboard when it is the app's
// (AppMenu), when it is drawn in place of the native bar it asked for,
// and with ActivationKeys; any other bar has typeahead only.
func TestMenubarWindowBar(t *testing.T) {
	cases := []struct {
		name string
		edit func(*kit.Menubar[string])
		opts []telatest.Option
		keys bool
	}{
		{"inline", nil, nil, false},
		{"ActivationKeys", func(m *kit.Menubar[string]) { m.ActivationKeys = true }, nil, true},
		{"native fallback", func(m *kit.Menubar[string]) { m.Native = kit.NativeOn },
			[]telatest.Option{telatest.PreferNativeMenus(), telatest.WithUnsupported(telatest.NativeMenus)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page := func() tela.Node {
				return h.Div(h.Class("flex flex-col items-start gap-4 p-6"), kit.Input{AriaLabel: "Note"}, appBar(c.edit))
			}
			app := telatest.MountFunc(t, page, append([]telatest.Option{telatest.Size(480, 360), telatest.Theme(kit.Theme()),
				telatest.PlatformKeys("linux")}, c.opts...)...)
			note := telatest.ByRole("textbox", "Note")
			app.Click(note)
			app.Press("F10")
			if c.keys {
				app.ExpectFocused(barItem("File"))
				app.Press("Escape")
			}
			app.ExpectFocused(note)
			app.Press("Alt+v")
			app.ExpectCount(menuNamed("View"), map[bool]int{true: 1, false: 0}[c.keys])
		})
	}
}

// An own AccessKey a label does not have is appended, always shown (the
// native menus of Chinese, Japanese and Korean), and a menu role's English
// letter goes to a label without letters of its own; on a Mac nothing is.
func TestMenubarAppendedAccessKeys(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			page := func() tela.Node {
				return h.Div(h.Class("p-6"), kit.Menubar[string]{AriaLabel: "アプリ", AppMenu: true, Menus: []kit.MenubarMenu[string]{
					{Key: "file", Label: "ファイル", AccessKey: 'F', Items: []kit.MenuItem[string]{{Key: "open", Label: "開く", AccessKey: 'O'}, {Key: "close", Label: "閉じる"}}},
					{Key: "edit", Label: "編集", Role: desktop.RoleEditMenu, Items: []kit.MenuItem[string]{{Key: "copy", Role: desktop.RoleCopy, Label: "コピー"}}},
				}})
			}
			app := telatest.MountFunc(t, page, telatest.Size(480, 360), telatest.Theme(kit.Theme()), telatest.PlatformKeys(goos))
			file, edit := "ファイル(F)", "編集(E)"
			if goos == "darwin" {
				file, edit = "ファイル", "編集"
			}
			app.ExpectCount(barItem(file), 1)
			app.ExpectCount(barItem(edit), 1)
			if goos == "darwin" {
				return
			}
			app.Press("Alt+f")
			// Open has its own O, Close the first free letter (A); Copy's role
			// letter C.
			app.ExpectFocused(barItem("開く(O)"))
			app.ExpectCount(barItem("閉じる(A)"), 1)
			app.Press("Alt+e")
			app.ExpectCount(barItem("コピー(C)"), 1)
		})
	}
}

// A context menu's or a dropdown's items have only their own access keys,
// underlined when the menu opens from the keyboard.
func TestMenuOwnAccessKeys(t *testing.T) {
	var log []string
	items := []kit.MenuItem[string]{
		{Key: "rename", Label: "Rename", AccessKey: 'm'},
		{Key: "delete", Label: "Delete"},
		{Key: "duplicate", Label: "Duplicate"},
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items,
			OnSelect: func(k string) { log = append(log, k) }})
	}, telatest.Theme(kit.Theme()), telatest.PlatformKeys("windows"))
	app.Click(telatest.ByRole("button", "Open"))
	if n := underlined(app); n != 0 {
		t.Errorf("%d access keys underlined in a menu opened with the mouse", n)
	}
	app.Press("Escape")
	app.Press("Enter")
	if n := underlined(app); n != 1 {
		t.Errorf("%d access keys underlined in a menu opened with the keyboard, want 1", n)
	}
	// D has no item of its own: typeahead.
	app.Press("d")
	app.ExpectFocused(cmItem("Delete"))
	app.Press("m")
	if len(log) != 1 || log[0] != "rename" {
		t.Errorf("M chose %v, want rename", log)
	}
}

// A loading Button that a TriggerFunc renders reaches the overlay without
// LoadingTrigger: nothing opens, from the pointer or the keyboard, and it
// keeps its tab stop and aria-busy; a loading Button inside an overlay's
// content does not make its trigger busy.
func TestLoadingButtonInTriggerFunc(t *testing.T) {
	items := []kit.MenuItem[string]{{Key: "a", Label: "Item A"}}
	trigger := func(loading bool) kit.Trigger {
		return kit.TriggerFunc(func(p tela.Prop) tela.Node {
			return kit.Button{Loading: loading, Variant: kit.Outline, Props: p, Children: h.Text("Open")}
		})
	}
	for _, name := range []string{"dropdown", "popover", "dialog", "tooltip over a menu"} {
		t.Run(name, func(t *testing.T) {
			var set tela.Setter[bool]
			app := telatest.MountFunc(t, func() tela.Node {
				l, s := tela.UseState(true)
				set = s
				var ov tela.Component
				switch name {
				case "dropdown":
					ov = kit.DropdownMenu[string]{Trigger: trigger(l), Items: items}
				case "popover":
					ov = kit.Popover{Trigger: trigger(l), Children: h.Text("Body")}
				case "dialog":
					ov = kit.Dialog{Trigger: trigger(l), Title: h.Text("Body")}
				default:
					ov = kit.Tooltip{Content: h.Text("Tip"), Trigger: kit.DropdownMenu[string]{Trigger: trigger(l), Items: items}}
				}
				return h.Div(h.Class("p-6"), ov)
			}, telatest.Theme(kit.Theme()))
			open := telatest.ByRole("button", "Open")
			opened := func() int { return app.Count(telatest.ByText("Body")) + app.Count(cmItem("Item A")) }
			app.Focus(open)
			for _, k := range []string{"Enter", "Space", "ArrowDown"} {
				app.Press(k)
				if opened() != 0 {
					t.Fatalf("%s opened the overlay of a loading trigger", k)
				}
			}
			app.ExpectFocused(open)
			app.Expect(open, telatest.Disabled(false))
			set.Set(false)
			app.Click(open)
			if opened() != 1 {
				t.Errorf("the idle trigger opens nothing")
			}
		})
	}
	// A loading Button in a popover's content leaves its trigger alone.
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Popover{Trigger: kit.Button{Children: h.Text("Open")},
			Children: kit.Button{Loading: true, Children: h.Text("Saving")}})
	}, telatest.Theme(kit.Theme()))
	open := telatest.ByRole("button", "Open")
	app.Click(open)
	app.ExpectCount(telatest.ByRole("button", "Saving"), 1)
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("button", "Saving"), 0)
	app.Click(open)
	app.ExpectCount(telatest.ByRole("button", "Saving"), 1)
}

// A submenu wider than the room on either side of its menu is moved into
// the window, over its parent, as native menus are (it overflowed).
func TestSubmenuShiftedIntoTheWindow(t *testing.T) {
	items := []kit.MenuItem[string]{
		{Key: "team", Label: "Team"},
		{Kind: kit.MenuSub, Key: "invite", Label: "Invite users", ContentClass: "w-[300px]", Items: []kit.MenuItem[string]{
			{Key: "email", Label: "Email"}, {Key: "message", Label: "Message"}}},
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex justify-center p-6"), kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items})
	}, telatest.Size(420, 300), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
	app.Click(cmItem("Invite users"))
	app.ExpectCount(cmItem("Email"), 1)
	sub := app.Bounds(telatest.ByRole("menu", "Invite users"))
	if sub.Width != 300 || sub.X < 0 || sub.X+sub.Width > 420 {
		t.Errorf("submenu %+v in a window 420 wide", sub)
	}
}

// The access keys underlined on the bar and in its open menu, after Alt
// and the Edit menu's key (the Windows flavor, so that every host renders
// the same), and a key appended to a label without letters ("…(T)"; the
// test fonts have no Japanese for "ツール(T)").
func TestMenubarAccessKeysGolden(t *testing.T) {
	var log []string
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Menubar[string]{AriaLabel: "Application", AppMenu: true, Menus: []kit.MenubarMenu[string]{
			fileMenu(),
			{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu, Items: editItems(&log)},
			{Key: "tools", Label: "…", AccessKey: 'T', Items: []kit.MenuItem[string]{{Key: "open", Label: "Open"}}},
		}})
	}
	for _, scheme := range []string{"light", "dark"} {
		opts := []telatest.Option{telatest.Size(420, 320), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"),
			telatest.PlatformKeys("windows")}
		if scheme == "dark" {
			opts = append(opts, telatest.Dark())
		}
		app := telatest.MountFunc(t, page, opts...)
		app.Press("Alt+e")
		app.ExpectFocused(barItem("Undo"))
		app.GoldenScales("menubar_access_keys_"+scheme, 1, 2)
	}
}
