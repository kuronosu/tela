package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The app's menu bar in kit (ADR-226): Menubar with AppMenu is
// desktop.AppMenuBar where native menus apply and the platform has an app
// bar, and the drawn bar where it is declared otherwise.

func appMenuPage(log *[]string, edit func(*kit.Menubar[string])) func() tela.Node {
	m := appBar(func(m *kit.Menubar[string]) {
		m.AppMenu = true
		m.OnSelect = func(menu, item string) { *log = append(*log, menu+"/"+item) }
		m.AppItems = []kit.MenuItem[string]{{Key: "settings", Label: "Settings…", Shortcut: "CmdOrCtrl+,", OnSelect: func() { *log = append(*log, "settings") }}}
		m.OnAppSelect = func(item string) { *log = append(*log, "app:"+item) }
		if edit != nil {
			edit(m)
		}
	})
	return func() tela.Node { return h.Div(h.Class("p-4"), m, h.Button(h.Text("Other"))) }
}

// Native, on a platform with an app bar: the bar and the app items are the
// OS's, nothing is drawn, and choosing runs what the drawn menu does.
func TestMenubarAppMenuNative(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, appMenuPage(&log, nil), telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	bar := app.AppMenuBar()
	if len(bar) != 3 || bar[0].Label != "File" || bar[2].Label != "View" {
		t.Fatalf("app menu bar %+v", bar)
	}
	if items := app.AppMenuItems(); len(items) != 1 || items[0].Label != "Settings…" || items[0].Accelerator == "" {
		t.Errorf("app items %+v", items)
	}
	if wb := app.MenuBar(); wb != nil {
		t.Errorf("the window has its own bar %+v", wb)
	}
	if n := app.Count(telatest.ByRole("menuitem", "File")); n != 0 {
		t.Errorf("a drawn bar beside the native one:\n%s", app.AriaSnapshot())
	}
	app.ChooseAppMenuItem("File", "New Window")
	app.ChooseAppMenuItem("Settings…")
	if got := fmt.Sprint(log); got != "[file/new-window settings app:settings]" {
		t.Errorf("log %s", got)
	}
}

// Drawn when the app does not ask for native menus (the default in
// telatest), when the platform has no app bar, and when Native says so;
// AppItems are not drawn.
func TestMenubarAppMenuDrawn(t *testing.T) {
	file := telatest.ByRole("menuitem", "File")
	for name, tc := range map[string]struct {
		edit func(*kit.Menubar[string])
		opts []telatest.Option
	}{
		"default":   {nil, nil},
		"nativeOff": {func(m *kit.Menubar[string]) { m.Native = kit.NativeOff }, []telatest.Option{telatest.PreferNativeMenus()}},
		// Neither an app bar nor window bars: Linux.
		"linux": {nil, []telatest.Option{telatest.PreferNativeMenus(), telatest.WithUnsupported(telatest.AppMenu, telatest.NativeMenus)}},
	} {
		t.Run(name, func(t *testing.T) {
			var log []string
			app := telatest.MountFunc(t, appMenuPage(&log, tc.edit),
				append([]telatest.Option{telatest.Size(480, 320), telatest.Theme(kit.Theme())}, tc.opts...)...)
			app.ExpectCount(file, 1)
			if bar := app.AppMenuBar(); bar != nil {
				t.Errorf("a native bar beside the drawn one: %+v", bar)
			}
			app.Click(file)
			app.Click(telatest.ByRole("menuitem", "New Window"))
			if got := fmt.Sprint(log); got != "[file/new-window]" {
				t.Errorf("log %s", got)
			}
			app.ExpectCount(telatest.ByText("Settings…"), 0)
		})
	}
}

// Where only windows have menu bars (Windows), the app's bar is the
// window's, native and not drawn.
func TestMenubarAppMenuOnWindows(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, appMenuPage(&log, nil), telatest.Size(480, 320), telatest.Theme(kit.Theme()),
		telatest.PreferNativeMenus(), telatest.WithUnsupported(telatest.AppMenu))
	if bar := app.MenuBar(); len(bar) != 3 {
		t.Fatalf("window bar %+v", bar)
	}
	app.ExpectCount(telatest.ByRole("menuitem", "File"), 0)
	app.ChooseAppMenuItem("File", "New Window")
	if got := fmt.Sprint(log); got != "[file/new-window]" {
		t.Errorf("log %s", got)
	}
}

// A per-window native Menubar on a platform whose windows have no bars (a
// Mac: the bar belongs to the app) is drawn.
func TestMenubarWindowBarDrawnWhereOnlyTheAppHasOne(t *testing.T) {
	page := nativeBar(func(m *kit.Menubar[string]) { m.Native = kit.NativeOn })
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()),
		telatest.WithUnsupported(telatest.NativeMenus))
	app.ExpectCount(telatest.ByRole("menuitem", "File"), 1)
}

// A menu with a role is the OS's standard menu in a native bar and is
// left out of the drawn one when it has no items.
func TestMenubarRoleMenus(t *testing.T) {
	menus := []kit.MenubarMenu[string]{
		fileMenu(),
		{Key: "edit", Label: "Edit", Role: desktop.RoleEditMenu},
		{Key: "window", Label: "Window", Role: desktop.RoleWindowMenu},
	}
	page := func() tela.Node {
		return h.Div(kit.Menubar[string]{AppMenu: true, Menus: menus})
	}
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()), telatest.PreferNativeMenus())
	bar := app.AppMenuBar()
	if len(bar) != 3 || bar[1].Role != desktop.RoleEditMenu || len(bar[1].Submenu) != 7 || bar[2].Submenu[1].Label != "Zoom" {
		t.Fatalf("app menu bar %+v", bar)
	}
	drawn := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	drawn.ExpectCount(telatest.ByRole("menuitem", "File"), 1)
	drawn.ExpectCount(telatest.ByRole("menuitem", "Edit"), 0)
	drawn.ExpectCount(telatest.ByRole("menuitem", "Window"), 0)
}

func TestMenubarMenuRoleMustBeAMenuRole(t *testing.T) {
	defer func() {
		r := recover()
		if s := fmt.Sprint(r); !strings.Contains(s, "RoleEditMenu") {
			t.Errorf("panic %v, want one that names the menu roles", r)
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		return kit.Menubar[string]{Menus: []kit.MenubarMenu[string]{{Key: "x", Label: "X", Role: desktop.RoleCopy}}}
	}, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
}
