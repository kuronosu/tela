package desktop_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// An AppMenuBar sets the app's bar and app items on a platform that has
// one, follows its fields, runs the chosen item and clears at unmount.
func TestAppMenuBar(t *testing.T) {
	var errs []error
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		wrap, setWrap := tela.UseState(false)
		note, setNote := tela.UseState("none")
		return Div(
			Textf("wrap: %v, note: %s", wrap, note),
			Button(OnClick(func() { setOn.Set(!on) }), Text("Toggle")),
			If(on, tela.Component(desktop.AppMenuBar{
				AppItems: []desktop.MenuItem{{Label: "Settings…", Accelerator: "CmdOrCtrl+,", OnSelect: func() { setNote.Set("settings") }}},
				Menus: []desktop.MenuItem{
					{Label: "File", Submenu: []desktop.MenuItem{{Label: "New", Accelerator: "CmdOrCtrl+N", OnSelect: func() { setNote.Set("new") }}}},
					{Label: "View", Submenu: []desktop.MenuItem{
						{Label: "Word wrap", Checkbox: true, Checked: wrap, OnSelect: func() { setWrap.Set(!wrap) }},
					}},
				},
				OnError: func(err error) { errs = append(errs, err) },
			})),
		)
	})
	bar, items := app.AppMenuBar(), app.AppMenuItems()
	if len(bar) != 2 || bar[0].Label != "File" || bar[0].Submenu[0].Accelerator == "" || bar[1].Submenu[0].Checked {
		t.Fatalf("menu bar %+v", bar)
	}
	if len(items) != 1 || items[0].Label != "Settings…" {
		t.Fatalf("app items %+v", items)
	}
	// The app bar is not a window's: the window has no menu of its own.
	if wb := app.MenuBar(); wb != nil {
		t.Errorf("the window has a menu bar %+v where the platform has an app bar", wb)
	}
	app.ChooseAppMenuItem("File", "New")
	app.ExpectText("note: new")
	app.ChooseAppMenuItem("Settings…")
	app.ExpectText("note: settings")
	app.ChooseAppMenuItem("View", "Word wrap")
	app.ExpectText("wrap: true")
	if bar := app.AppMenuBar(); !bar[1].Submenu[0].Checked {
		t.Errorf("the bar did not follow the state: %+v", bar)
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	if bar, items := app.AppMenuBar(), app.AppMenuItems(); bar != nil || items != nil {
		t.Errorf("bar after unmount: %+v, %+v", bar, items)
	}
	if len(errs) != 0 {
		t.Errorf("errors %v", errs)
	}
}

// With no app menu bar but window menu bars (Windows), the AppMenuBar is
// the bar of every framed window without a MenuBar of its own, later
// windows included; a window's own MenuBar wins on it and gives it back.
func TestAppMenuBarOnEveryWindow(t *testing.T) {
	var errs []error
	chosen := ""
	app := telatest.MountFunc(t, func() tela.Node {
		second, setSecond := tela.UseState(false)
		own, setOwn := tela.UseState(false)
		bar := tela.Component(desktop.AppMenuBar{
			Menus: []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{
				{Label: "New", OnSelect: func() { chosen = "app new" }},
			}}},
			OnError: func(err error) { errs = append(errs, err) },
		})
		return Div(
			bar,
			Button(OnClick(func() { setSecond.Set(true) }), Text("Open")),
			Button(OnClick(func() { setOwn.Set(!own) }), Text("Own")),
			If(own, tela.Component(desktop.MenuBar{Menus: []desktop.MenuItem{{Label: "Mine", Submenu: []desktop.MenuItem{
				{Label: "Do", OnSelect: func() { chosen = "own do" }},
			}}}})),
			If(second, tela.Component(tela.Window{
				Title: "Second", Width: 200, Height: 100, OnClose: func() { setSecond.Set(false) },
				Root: tela.C(func() tela.Node { return Text("second") }),
			})),
		)
	}, telatest.WithUnsupported(telatest.AppMenu), telatest.Title("Main"))

	if len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	if bar := app.MenuBar(); len(bar) != 1 || bar[0].Label != "File" {
		t.Fatalf("the main window's bar %+v", bar)
	}
	if bar := app.AppMenuBar(); len(bar) != 1 || bar[0].Label != "File" {
		t.Errorf("AppMenuBar %+v", bar)
	}
	app.ChooseAppMenuItem("File", "New")
	if chosen != "app new" {
		t.Errorf("chosen %q", chosen)
	}

	// A window that opens later has the bar too.
	second := app.WaitForWindow(func() { app.Click(telatest.ByRole("button", "Open")) })
	if bar := second.MenuBar(); len(bar) != 1 || bar[0].Label != "File" {
		t.Fatalf("the second window's bar %+v", bar)
	}
	chosen = ""
	second.ChooseMenuItem("File", "New")
	if chosen != "app new" {
		t.Errorf("the second window's item did not run the app's: %q", chosen)
	}

	// The main window's own MenuBar wins on it, only.
	app.Click(telatest.ByRole("button", "Own"))
	if bar := app.MenuBar(); len(bar) != 1 || bar[0].Label != "Mine" {
		t.Fatalf("the main window's bar %+v, want its own", bar)
	}
	chosen = ""
	app.ChooseMenuItem("Mine", "Do")
	if chosen != "own do" {
		t.Errorf("chosen %q: the app's bar ran too, or the own one did not", chosen)
	}
	if bar := second.MenuBar(); len(bar) != 1 || bar[0].Label != "File" {
		t.Errorf("the second window lost the app's bar: %+v", bar)
	}
	// The window gets the app's bar back when its own unmounts.
	app.Click(telatest.ByRole("button", "Own"))
	if bar := app.MenuBar(); len(bar) != 1 || bar[0].Label != "File" {
		t.Fatalf("the main window's bar after its own left: %+v", bar)
	}
	chosen = ""
	app.ChooseMenuItem("File", "New")
	if chosen != "app new" {
		t.Errorf("chosen %q after getting the app's bar back", chosen)
	}
	if len(errs) != 0 {
		t.Errorf("errors %v", errs)
	}
}

// The bar is removed from every window when the AppMenuBar unmounts.
func TestAppMenuBarOnEveryWindowUnmounts(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		return Div(
			Button(OnClick(func() { setOn.Set(false) }), Text("Off")),
			If(on, tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{{Label: "New"}}}}})),
		)
	}, telatest.WithUnsupported(telatest.AppMenu))
	if bar := app.MenuBar(); len(bar) != 1 {
		t.Fatalf("bar %+v", bar)
	}
	app.Click(telatest.ByRole("button", "Off"))
	if bar := app.MenuBar(); bar != nil {
		t.Errorf("bar after unmount: %+v", bar)
	}
}

// A NativeOnly bar is the app's menu bar where the platform has one
// (macOS), and nothing on a platform with window menu bars only (Windows):
// the framed windows keep no bar, and OnError gets ErrUnsupported
// (ADR-253).
func TestAppMenuBarNativeOnly(t *testing.T) {
	menus := []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{{Label: "New"}}}}
	var errs []error
	root := func() tela.Node {
		return tela.Component(desktop.AppMenuBar{Menus: menus, NativeOnly: true, OnError: func(err error) { errs = append(errs, err) }})
	}
	mac := telatest.MountFunc(t, root)
	if bar := mac.AppMenuBar(); len(bar) != 1 || bar[0].Label != "File" || len(errs) != 0 {
		t.Errorf("with an app menu bar: bar %+v, errors %v", bar, errs)
	}
	win := telatest.MountFunc(t, root, telatest.WithUnsupported(telatest.AppMenu))
	if bar := win.MenuBar(); bar != nil {
		t.Errorf("with window menu bars only, the window got the bar %+v", bar)
	}
	if len(errs) != 1 || !errors.Is(errs[0], desktop.ErrUnsupported) {
		t.Errorf("errors %v, want one ErrUnsupported", errs)
	}
}

// Without an app bar and without window bars (Linux), a frameless main
// window, or a tree with no window, OnError gets ErrUnsupported.
func TestAppMenuBarUnsupported(t *testing.T) {
	mount := func(opts ...telatest.Option) error {
		var got error
		telatest.MountFunc(t, func() tela.Node {
			return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{{Label: "New"}}}}, OnError: func(err error) { got = err }})
		}, opts...)
		return got
	}
	for name, opts := range map[string][]telatest.Option{
		"linux":     {telatest.WithUnsupported(telatest.AppMenu, telatest.NativeMenus)},
		"frameless": {telatest.WithUnsupported(telatest.AppMenu), telatest.Frameless()},
	} {
		if err := mount(opts...); !errors.Is(err, desktop.ErrUnsupported) {
			t.Errorf("%s: OnError got %v, want ErrUnsupported", name, err)
		}
	}
	if err := mount(); err != nil {
		t.Errorf("a platform with an app bar reported %v", err)
	}

	var ferr error
	_, err := telatest.Render(tela.C(func() tela.Node {
		return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "File"}}, OnError: func(err error) { ferr = err }})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ferr, desktop.ErrUnsupported) {
		t.Errorf("no window: OnError got %v, want ErrUnsupported", ferr)
	}
}

// Two mounted AppMenuBars: the later one wins, and the earlier one's
// items do not run.
func TestAppMenuBarTwice(t *testing.T) {
	ran := ""
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(
			tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "First", Submenu: []desktop.MenuItem{{Label: "Go", OnSelect: func() { ran = "first" }}}}}}),
			tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "Second", Submenu: []desktop.MenuItem{{Label: "Go", OnSelect: func() { ran = "second" }}}}}}),
		)
	})
	if bar := app.AppMenuBar(); len(bar) != 1 || bar[0].Label != "Second" {
		t.Fatalf("bar %+v, want the later one's", bar)
	}
	app.ChooseAppMenuItem("Second", "Go")
	if ran != "second" {
		t.Errorf("ran %q", ran)
	}
}

// A role item with no label gets the role's label; a menu role with no
// submenu gets the standard items; the platform sees the roles.
func TestMenuRolesDefaults(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{
			{Label: "File", Submenu: []desktop.MenuItem{{Role: desktop.RoleClose}, {Role: desktop.RoleMinimize, Label: "Minimizar"}}},
			{Role: desktop.RoleEditMenu},
			{Role: desktop.RoleWindowMenu},
			{Role: desktop.RoleHelpMenu, Submenu: []desktop.MenuItem{{Label: "Docs"}}},
		}})
	})
	bar := app.AppMenuBar()
	labels := func(items []telatest.MenuItem) []string {
		var out []string
		for _, it := range items {
			if it.Separator {
				out = append(out, "-")
			} else {
				out = append(out, it.Label)
			}
		}
		return out
	}
	if got := labels(bar); strings.Join(got, ",") != "File,Edit,Window,Help" {
		t.Fatalf("top-level labels %v", got)
	}
	if got := labels(bar[0].Submenu); strings.Join(got, ",") != "Close,Minimizar" {
		t.Errorf("File items %v", got)
	}
	if got := labels(bar[1].Submenu); strings.Join(got, ",") != "Undo,Redo,-,Cut,Copy,Paste,Select All" {
		t.Errorf("Edit items %v", got)
	}
	if got := labels(bar[2].Submenu); strings.Join(got, ",") != "Minimize,Zoom,Close" {
		t.Errorf("Window items %v", got)
	}
	if bar[1].Role != desktop.RoleEditMenu || bar[2].Role != desktop.RoleWindowMenu || bar[3].Role != desktop.RoleHelpMenu {
		t.Errorf("menu roles %v %v %v", bar[1].Role, bar[2].Role, bar[3].Role)
	}
	if bar[1].Submenu[4].Role != desktop.RoleCopy || bar[1].Submenu[4].Accelerator == "" {
		t.Errorf("Copy %+v: want the role and the platform's shortcut as text", bar[1].Submenu[4])
	}
	if bar[0].Submenu[0].Role != desktop.RoleClose {
		t.Errorf("Close %+v", bar[0].Submenu[0])
	}
}

// The roles mirror the platform's, with Electron's names.
func TestRoleNames(t *testing.T) {
	for _, c := range []struct {
		r     desktop.Role
		p     platform.MenuRole
		name  string
		label string
	}{
		{desktop.RoleUndo, platform.RoleUndo, "undo", "Undo"},
		{desktop.RoleRedo, platform.RoleRedo, "redo", "Redo"},
		{desktop.RoleCut, platform.RoleCut, "cut", "Cut"},
		{desktop.RoleCopy, platform.RoleCopy, "copy", "Copy"},
		{desktop.RolePaste, platform.RolePaste, "paste", "Paste"},
		{desktop.RoleSelectAll, platform.RoleSelectAll, "selectAll", "Select All"},
		{desktop.RoleMinimize, platform.RoleMinimize, "minimize", "Minimize"},
		{desktop.RoleZoom, platform.RoleZoom, "zoom", "Zoom"},
		{desktop.RoleClose, platform.RoleClose, "close", "Close"},
		{desktop.RoleEditMenu, platform.RoleEditMenu, "editMenu", "Edit"},
		{desktop.RoleWindowMenu, platform.RoleWindowMenu, "windowMenu", "Window"},
		{desktop.RoleHelpMenu, platform.RoleHelpMenu, "help", "Help"},
	} {
		if platform.MenuRole(c.r) != c.p || c.r.String() != c.name || c.r.Label() != c.label {
			t.Errorf("%v: platform %v, name %q, label %q; want %v, %q, %q", c.r, platform.MenuRole(c.r), c.r.String(), c.r.Label(), c.p, c.name, c.label)
		}
	}
	if desktop.RoleNone.String() != "" || desktop.RoleNone.Label() != "" {
		t.Error("RoleNone has a name or a label")
	}
}

// Mistakes in roles panic with the fix.
func TestMenuRolePanics(t *testing.T) {
	for name, tc := range map[string]struct {
		item desktop.MenuItem
		want string
	}{
		"help":     {desktop.MenuItem{Role: desktop.RoleHelpMenu}, "no standard items"},
		"submenu":  {desktop.MenuItem{Role: desktop.RoleCopy, Submenu: []desktop.MenuItem{{Label: "x"}}}, "role of an item"},
		"invalid":  {desktop.MenuItem{Label: "x", Role: desktop.Role(99)}, "invalid role"},
		"twoMenus": {desktop.MenuItem{Label: "Both", Submenu: []desktop.MenuItem{{Role: desktop.RoleEditMenu}, {Role: desktop.RoleEditMenu}}}, "one menu of each role"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(strings.ToLower(toString(p)), tc.want) {
					t.Errorf("panic %v, want one mentioning %q", p, tc.want)
				}
			}()
			telatest.MountFunc(t, func() tela.Node {
				return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{tc.item}})
			})
		})
	}
}

func toString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case error:
		return v.Error()
	}
	return ""
}

// An item's own OnSelect replaces the role: the platform is not told of
// the role, so it cannot perform it natively either.
func TestRoleWithOnSelect(t *testing.T) {
	ran := false
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{
			{Label: "Edit", Submenu: []desktop.MenuItem{{Role: desktop.RoleCopy, OnSelect: func() { ran = true }}}},
		}})
	})
	if it := app.AppMenuBar()[0].Submenu[0]; it.Label != "Copy" || it.Role != desktop.RoleNone {
		t.Errorf("item %+v: want the role's label and no platform role", it)
	}
	app.ChooseAppMenuItem("Edit", "Copy")
	if !ran {
		t.Error("OnSelect did not run")
	}
}

// Where the platform reports role items, package desktop performs them:
// minimize, zoom and close on the window, copy on the focused field.
func TestRolesPerformed(t *testing.T) {
	closed := 0
	app := telatest.MountApp(t, tela.App{
		Title: "Roles",
		OnClose: func(tela.WindowHandle) {
			closed++
		},
		Root: tela.C(func() tela.Node {
			text, setText := tela.UseState("")
			return Div(Class("p-2"),
				Input(AriaLabel("Note"), Value(text), OnChange(func(e tela.ChangeEvent) { setText.Set(e.Value) })),
				tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{
					{Role: desktop.RoleEditMenu},
					{Role: desktop.RoleWindowMenu},
				}}),
			)
		}),
	})
	app.Fill(telatest.ByRole("textbox", "Note"), "hello world")
	app.Press("ControlOrMeta+a")
	app.ChooseAppMenuItem("Edit", "Copy")
	if got := app.Clipboard().Text(); got != "hello world" {
		t.Fatalf("clipboard %q after Copy", got)
	}
	app.ChooseAppMenuItem("Edit", "Cut")
	if got := app.Clipboard().Text(); got != "hello world" {
		t.Errorf("clipboard %q after Cut", got)
	}
	app.Expect(telatest.ByRole("textbox", "Note"), telatest.HasValue(""))
	app.ChooseAppMenuItem("Edit", "Undo")
	app.Expect(telatest.ByRole("textbox", "Note"), telatest.HasValue("hello world"))
	app.ChooseAppMenuItem("Edit", "Redo")
	app.Expect(telatest.ByRole("textbox", "Note"), telatest.HasValue(""))
	app.ChooseAppMenuItem("Edit", "Paste")
	app.Expect(telatest.ByRole("textbox", "Note"), telatest.HasValue("hello world"))
	app.ChooseAppMenuItem("Edit", "Select All")

	app.ChooseAppMenuItem("Window", "Zoom")
	app.ExpectWindowState(telatest.Maximized(true))
	app.ChooseAppMenuItem("Window", "Zoom")
	app.ExpectWindowState(telatest.Maximized(false))
	app.ChooseAppMenuItem("Window", "Minimize")
	app.ExpectWindowState(telatest.Minimized(true))
	app.Restore()
	app.ChooseAppMenuItem("Window", "Close")
	if closed != 1 {
		t.Errorf("Close asked the window to close %d times, want 1 (OnClose)", closed)
	}
}

// Under the Mac key flavor the edit roles send Command+key.
func TestRolesPerformedMacFlavor(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		text, setText := tela.UseState("")
		return Div(Class("p-2"),
			Input(AriaLabel("Note"), Value(text), OnChange(func(e tela.ChangeEvent) { setText.Set(e.Value) })),
			tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Role: desktop.RoleEditMenu}}}),
		)
	}, telatest.PlatformKeys("darwin"))
	app.Fill(telatest.ByRole("textbox", "Note"), "mac text")
	app.Press("ControlOrMeta+a")
	app.ChooseAppMenuItem("Edit", "Copy")
	if got := app.Clipboard().Text(); got != "mac text" {
		t.Errorf("clipboard %q after Copy under the Mac flavor", got)
	}
	if it := app.AppMenuBar()[0].Submenu[4]; !strings.Contains(it.Accelerator, "Super") && !strings.Contains(it.Accelerator, "Cmd") && !strings.Contains(it.Accelerator, "⌘") {
		t.Logf("Copy shows %q", it.Accelerator)
	}
}

// Roles work in a window's own MenuBar and a popup menu too, on the
// window they belong to.
func TestRolesInWindowMenus(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		text, setText := tela.UseState("")
		return Div(Class("p-2"),
			Input(AriaLabel("Note"), Value(text), OnChange(func(e tela.ChangeEvent) { setText.Set(e.Value) })),
			tela.Component(desktop.MenuBar{Menus: []desktop.MenuItem{{Role: desktop.RoleEditMenu}}}),
		)
	})
	app.Fill(telatest.ByRole("textbox", "Note"), "window bar")
	app.Press("ControlOrMeta+a")
	app.ChooseMenuItem("Edit", "Copy")
	if got := app.Clipboard().Text(); got != "window bar" {
		t.Errorf("clipboard %q", got)
	}
}
