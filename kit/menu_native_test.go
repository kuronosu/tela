package kit_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Native menus (ADR-215): the app's NativeMenus, a menu's own Native
// setting, the conversion of the items and the fallback to the drawn menu.

// nativeItems is a menu with every kind of entry.
func nativeItems(log *[]string) []kit.MenuItem[string] {
	say := func(s string) func() { return func() { *log = append(*log, s) } }
	return []kit.MenuItem[string]{
		{Kind: kit.MenuLabel, Label: "Edit"},
		{Key: "copy", Label: "Copy", Shortcut: "ctrl+c", OnSelect: say("copy")},
		{Key: "paste", Label: "Paste", Shortcut: "⌘V", Disabled: true},
		{Key: "delete", Label: "Delete", Variant: kit.Destructive, Icon: tela.Icon{}, Inset: true, KeepOpen: true},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuCheckbox, Key: "bars", Label: "Status bar", DefaultChecked: true, OnCheckedChange: func(on bool) { *log = append(*log, fmt.Sprint("bars=", on)) }},
		{Kind: kit.MenuRadioGroup, Key: "theme", DefaultValue: new("light"), OnValueChange: func(k string) { *log = append(*log, "theme="+k) }, Items: []kit.MenuItem[string]{
			{Key: "light", Label: "Light"}, {Key: "dark", Label: "Dark"}}},
		{Kind: kit.MenuGroup, Label: "More", Disabled: true, Items: []kit.MenuItem[string]{{Key: "extra", Label: "Extra"}}},
	}
}

func nativeMenuPage(log *[]string, edit func(*kit.ContextMenu[string])) func() tela.Node {
	m := kit.ContextMenu[string]{
		Items:    nativeItems(log),
		OnSelect: func(k string) { *log = append(*log, "select:"+k) },
	}
	if edit != nil {
		edit(&m)
	}
	return contextPage(m)
}

func TestNativeContextMenuFromApp(t *testing.T) {
	var log []string
	app := mountContext(t, nativeMenuPage(&log, nil), 480, 320)
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 0 || app.Count(cmMenu) != 1 {
		t.Fatalf("the default is a drawn menu: popups %v\n%s", app.PopupMenus(), app.AriaSnapshot())
	}

	// NativeMenus on the app: the OS's menu, nothing drawn.
	app = telatest.MountFunc(t, nativeMenuPage(&log, nil), telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	app.ClickAt(100, 80, telatest.Right())
	ms := app.PopupMenus()
	if len(ms) != 1 || ms[0].At != (tela.Point{X: 100, Y: 80}) {
		t.Fatalf("popup menus %+v, want one at (100, 80)", ms)
	}
	if n := app.Count(cmMenu); n != 0 {
		t.Errorf("a drawn menu is open beside the native one:\n%s", app.AriaSnapshot())
	}
	want := []telatest.MenuItem{
		{Label: "Edit", Disabled: true},
		{Label: "Copy", Accelerator: "Ctrl+C"},
		{Label: "Paste", Accelerator: "⌘V", Disabled: true},
		{Label: "Delete"},
		{Separator: true},
		{Label: "Status bar", Checkbox: true, Checked: true},
		{Label: "Light", Checkbox: true, Checked: true},
		{Label: "Dark", Checkbox: true},
		{Label: "Extra", Disabled: true},
	}
	if !reflect.DeepEqual(ms[0].Items, want) {
		t.Errorf("items\n got %+v\nwant %+v", ms[0].Items, want)
	}

	// Choosing runs the item and the menu's OnSelect, and the menu closes.
	app.ChoosePopupItem("Copy")
	if got := fmt.Sprint(log); got != "[copy select:copy]" {
		t.Errorf("log %s", got)
	}
	if len(app.PopupMenus()) != 0 {
		t.Errorf("the native menu is still open")
	}
	// A dismissed menu chooses nothing.
	app.ClickAt(100, 80, telatest.Right())
	app.DismissPopupMenu()
	if got := fmt.Sprint(log); got != "[copy select:copy]" {
		t.Errorf("a dismissed menu chose: %s", got)
	}
	if n := app.Count(cmMenu); n != 0 {
		t.Errorf("a drawn menu opened after the native one was dismissed")
	}

	// A checkbox and a radio item keep their state in the kit's store: the
	// next menu shows it.
	log = nil
	app.ClickAt(100, 80, telatest.Right())
	app.ChoosePopupItem("Status bar")
	app.ClickAt(100, 80, telatest.Right())
	app.ChoosePopupItem("Dark")
	app.ClickAt(100, 80, telatest.Right())
	items := app.PopupMenus()[0].Items
	if items[5].Checked || items[6].Checked || !items[7].Checked {
		t.Errorf("state after choosing: %+v", items[5:8])
	}
	if got := fmt.Sprint(log); got != "[bars=false theme=dark]" {
		t.Errorf("log %s", got)
	}
	app.DismissPopupMenu()
}

func TestNativeContextMenuOverrides(t *testing.T) {
	var log []string
	// The app asks for native menus; this menu stays drawn.
	app := telatest.MountFunc(t, nativeMenuPage(&log, func(m *kit.ContextMenu[string]) { m.Native = kit.NativeOff }),
		telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 0 || app.Count(cmMenu) != 1 {
		t.Errorf("NativeOff under NativeMenus: popups %v\n%s", app.PopupMenus(), app.AriaSnapshot())
	}
	// The app does not ask; this menu does.
	app = telatest.MountFunc(t, nativeMenuPage(&log, func(m *kit.ContextMenu[string]) { m.Native = kit.NativeOn }),
		telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 1 || app.Count(cmMenu) != 0 {
		t.Errorf("NativeOn without NativeMenus: popups %v\n%s", app.PopupMenus(), app.AriaSnapshot())
	}
	// The field of tela.App reaches the tree through MountApp too.
	app = telatest.MountApp(t, tela.App{NativeMenus: true, Width: 480, Height: 320, Root: tela.C(nativeMenuPage(&log, nil))})
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 1 {
		t.Errorf("tela.App.NativeMenus: popups %v", app.PopupMenus())
	}
}

// Where the platform has no native menus the drawn menu opens at once, at
// the same point, and works as always.
func TestNativeContextMenuFallsBack(t *testing.T) {
	var log []string
	app := mountContext(t, nativeMenuPage(&log, func(m *kit.ContextMenu[string]) { m.Native = kit.NativeOn }), 480, 320)
	app.Unsupported(telatest.NativeMenus)
	app.ClickAt(100, 80, telatest.Right())
	app.ExpectCount(cmMenu, 1)
	if m := app.Bounds(cmMenu); m.X != 100 || m.Y != 80 {
		t.Errorf("drawn menu at (%v, %v), want (100, 80)", m.X, m.Y)
	}
	app.Click(cmItem("Copy"))
	if got := fmt.Sprint(log); got != "[copy select:copy]" {
		t.Errorf("log %s", got)
	}
	if len(app.PopupMenus()) != 0 {
		t.Errorf("a native menu appeared too")
	}
	// Supported again: the next opening is native.
	app.Unsupported()
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 1 || app.Count(cmMenu) != 0 {
		t.Errorf("after Unsupported(): popups %v\n%s", app.PopupMenus(), app.AriaSnapshot())
	}
}

// Shift+F10 and the Menu key open the native menu below the focused
// element, as the drawn one.
func TestNativeContextMenuFromKeyboard(t *testing.T) {
	var log []string
	for _, key := range []string{"Shift+F10", "ContextMenu"} {
		app := telatest.MountFunc(t, nativeMenuPage(&log, func(m *kit.ContextMenu[string]) { m.Native = kit.NativeOn }),
			telatest.Size(480, 320), telatest.Theme(kit.Theme()))
		app.Press("Tab") // the button inside the area
		app.Press(key)
		ms := app.PopupMenus()
		b := app.Bounds(telatest.ByRole("button", "Inner"))
		if len(ms) != 1 || ms[0].At != (tela.Point{X: b.X, Y: b.Y + b.Height}) {
			t.Errorf("%s: popup menus %+v, want one below the focused button %v", key, ms, b)
		}
		if n := app.Count(cmMenu); n != 0 {
			t.Errorf("%s: a drawn menu is open beside the native one", key)
		}
		app.DismissPopupMenu()
	}
}

// A label is a disabled native item, so a menu of labels is still native.
func TestNativeContextMenuOnlyLabels(t *testing.T) {
	app := mountContext(t, contextPage(kit.ContextMenu[string]{
		Native: kit.NativeOn, Items: []kit.MenuItem[string]{{Kind: kit.MenuLabel, Label: "Nothing"}},
	}), 480, 320)
	app.ClickAt(100, 80, telatest.Right())
	if len(app.PopupMenus()) != 1 {
		t.Fatalf("popups %v", app.PopupMenus())
	}
	app.DismissPopupMenu()
}

func TestNativeModeInvalidPanicsInDev(t *testing.T) {
	defer func() {
		r := recover()
		if s := fmt.Sprint(r); !strings.Contains(s, "kit.NativeAuto") {
			t.Errorf("panic %v, want one that names the valid values", r)
		}
	}()
	telatest.MountFunc(t, contextPage(kit.ContextMenu[string]{Native: kit.NativeMode(9)}), telatest.Size(480, 320))
}

func nativeBar(edit func(*kit.Menubar[string])) func() tela.Node {
	m := appBar(edit)
	return func() tela.Node { return h.Div(h.Class("p-4"), m, h.Button(h.Text("Other"))) }
}

func TestNativeMenubar(t *testing.T) {
	var log []string
	page := nativeBar(func(m *kit.Menubar[string]) {
		m.OnSelect = func(menu, item string) { log = append(log, menu+"/"+item) }
	})

	// Drawn by default.
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	if app.Count(telatest.ByRole("menuitem", "File")) != 1 || app.MenuBar() != nil {
		t.Fatalf("the default bar is drawn:\n%s", app.AriaSnapshot())
	}

	// NativeMenus on the app: the window's menu bar and nothing drawn.
	app = telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	bar := app.MenuBar()
	if len(bar) != 3 || bar[0].Label != "File" || bar[2].Label != "View" {
		t.Fatalf("menu bar %+v", bar)
	}
	if n := app.Count(telatest.ByRole("menuitem", "File")); n != 0 {
		t.Errorf("a drawn bar beside the native one:\n%s", app.AriaSnapshot())
	}
	file := bar[0].Submenu
	if file[0].Label != "New Tab" || file[0].Accelerator != "⌘T" || !file[2].Disabled || !file[3].Separator {
		t.Errorf("File menu %+v", file)
	}
	view := bar[2].Submenu
	if !view[1].Checked || view[0].Checked || !view[0].Checkbox {
		t.Errorf("View checkboxes %+v", view[:2])
	}
	app.ChooseMenuItem("File", "New Window")
	app.ChooseMenuItem("View", "Always Show Bookmarks Bar")
	app.ChooseMenuItem("View", "Andy") // checkbox and radio items do not call OnSelect, as drawn
	if got := fmt.Sprint(log); got != "[file/new-window]" {
		t.Errorf("log %s", got)
	}
	// The state follows into the bar.
	view = app.MenuBar()[2].Submenu
	if !view[0].Checked {
		t.Errorf("the checkbox did not follow: %+v", view[:2])
	}
	for _, it := range view {
		if it.Label == "Andy" && !it.Checked || it.Label == "Benoit" && it.Checked {
			t.Errorf("radio items %+v", view)
		}
	}
}

func TestNativeMenubarOverrides(t *testing.T) {
	page := nativeBar(func(m *kit.Menubar[string]) { m.Native = kit.NativeOff })
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	if app.MenuBar() != nil || app.Count(telatest.ByRole("menuitem", "File")) != 1 {
		t.Errorf("NativeOff under NativeMenus:\n%s", app.AriaSnapshot())
	}
	page = nativeBar(func(m *kit.Menubar[string]) { m.Native = kit.NativeOn })
	app = telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	if app.MenuBar() == nil || app.Count(telatest.ByRole("menuitem", "File")) != 0 {
		t.Errorf("NativeOn without NativeMenus:\n%s", app.AriaSnapshot())
	}
}

// A window with no native menu bar gets the drawn one, a frame later.
func TestNativeMenubarFallsBack(t *testing.T) {
	page := nativeBar(func(m *kit.Menubar[string]) { m.Native = kit.NativeOn })
	file := telatest.ByRole("menuitem", "File")

	// A frameless window (kit.TitleBar draws its own).
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.Frameless())
	app.ExpectCount(file, 1)
	if app.MenuBar() != nil {
		t.Errorf("a frameless window has a native bar: %+v", app.MenuBar())
	}
	// The drawn bar works.
	app.Click(file)
	app.Expect(telatest.ByRole("menuitem", "New Tab"), telatest.Focused(false))
	app.ExpectCount(telatest.ByRole("menuitem", "New Tab"), 1)

	// A platform without native menus: the bar was native until it lost
	// them, then a new one is drawn (a window sets its bar once it mounts).
	app = telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		return h.Div(h.Button(h.OnClick(func() { setOn.Set(true) }), h.Text("Show")),
			h.If(on, kit.Menubar[string]{Native: kit.NativeOn, Menus: []kit.MenubarMenu[string]{fileMenu()}}))
	}, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	app.Unsupported(telatest.NativeMenus)
	app.Click(telatest.ByRole("button", "Show"))
	app.ExpectCount(file, 1)
	if app.MenuBar() != nil {
		t.Errorf("native bar after Unsupported: %+v", app.MenuBar())
	}
}

// The native bar leaves with the component, and the window keeps none.
func TestNativeMenubarUnmounts(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		return h.Div(h.Button(h.OnClick(func() { setOn.Set(!on) }), h.Text("Toggle")),
			h.If(on, kit.Menubar[string]{Native: kit.NativeOn, Menus: []kit.MenubarMenu[string]{fileMenu()}}))
	}, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	if len(app.MenuBar()) != 1 {
		t.Fatalf("menu bar %+v", app.MenuBar())
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	if app.MenuBar() != nil {
		t.Errorf("menu bar after unmount: %+v", app.MenuBar())
	}
}

// A child window follows the app's setting.
func TestNativeMenusChildWindowFollowsApp(t *testing.T) {
	var got []bool
	app := telatest.MountApp(t, tela.App{NativeMenus: true, Root: tela.C(func() tela.Node {
		return tela.Component(tela.Window{Title: "Child", Width: 200, Height: 100, OnClose: func() {}, Root: tela.C(func() tela.Node {
			got = append(got, tela.UseNativeMenus())
			return h.Text("child")
		})})
	})})
	app.Window("Child")
	if len(got) == 0 || !got[len(got)-1] {
		t.Errorf("child window read UseNativeMenus = %v, want true", got)
	}
}
