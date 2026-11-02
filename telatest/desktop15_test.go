package telatest_test

import (
	"math"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// RequestQuit runs OnQuit, from MountApp's App or the option; the app keeps
// running unless OnQuit quits.
func TestRequestQuitRunsOnQuit(t *testing.T) {
	asked := 0
	app := telatest.MountApp(t, tela.App{
		OnQuit: func() { asked++ },
		Root:   tela.C(func() tela.Node { return Text("editor") }),
	})
	app.RequestQuit()
	if asked != 1 {
		t.Fatalf("OnQuit ran %d times, want 1", asked)
	}
	app.ExpectRunning()
	app.RequestQuit()
	if asked != 2 {
		t.Errorf("OnQuit ran %d times, want 2", asked)
	}

	viaOption := 0
	opt := telatest.MountFunc(t, func() tela.Node { return Text("editor") }, telatest.OnQuit(func() { viaOption++ }))
	opt.RequestQuit()
	if viaOption != 1 {
		t.Errorf("telatest.OnQuit ran %d times, want 1", viaOption)
	}
}

// OnQuit may quit the app for real.
func TestRequestQuitQuits(t *testing.T) {
	app := telatest.MountApp(t, tela.App{OnQuit: tela.Quit, Root: tela.C(func() tela.Node { return Text("editor") })})
	app.RequestQuit()
	app.ExpectQuit()
}

// Without OnQuit every window gets a close request: the main window closes
// and the app quits; with an OnClose on the main window, the app decides.
func TestRequestQuitDefaultClosesWindows(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node { return Text("editor") })
	app.RequestQuit()
	app.ExpectQuit()

	closes := 0
	kept := telatest.MountApp(t, tela.App{
		OnClose: func(tela.WindowHandle) { closes++ },
		Root:    tela.C(func() tela.Node { return Text("tray app") }),
	})
	kept.RequestQuit()
	if closes != 1 {
		t.Errorf("OnClose ran %d times for a quit request, want 1", closes)
	}
	kept.ExpectRunning()
}

// TitleBarOverlay and OnQuit of an App are applied by MountApp; the
// rectangle of the OS's buttons is the test's: none (the window acts as
// Frameless), the Mac's under PlatformKeys("darwin"), or its own.
func TestMountAppTitleBarOverlay(t *testing.T) {
	rect := func(a *telatest.App) tela.Rect { return a.WindowState().TitleBarControls }
	root := tela.C(func() tela.Node { return Text("x") })

	app := telatest.MountApp(t, tela.App{TitleBarOverlay: true, Root: root})
	app.ExpectWindowState(telatest.ClientDecorations(true))
	if r := rect(app); r != (tela.Rect{}) {
		t.Errorf("an overlay window on Linux keys reports %v, want no buttons", r)
	}

	mac := telatest.MountApp(t, tela.App{TitleBarOverlay: true, Root: root}, telatest.PlatformKeys("darwin"))
	if r := rect(mac); r != telatest.MacTitleBarControls || r.Width <= 0 || r.Height <= 0 {
		t.Errorf("an overlay window under PlatformKeys(darwin) reports %v, want %v", r, telatest.MacTitleBarControls)
	}

	own := tela.Rect{X: 10, Y: 4, Width: 70, Height: 16}
	custom := telatest.MountApp(t, tela.App{TitleBarOverlay: true, Root: root}, telatest.PlatformKeys("darwin"), telatest.TitleBarControls(own))
	if r := rect(custom); r != own {
		t.Errorf("TitleBarControls: %v, want %v", r, own)
	}

	plain := telatest.MountApp(t, tela.App{Root: root}, telatest.PlatformKeys("darwin"))
	if r := rect(plain); r != (tela.Rect{}) {
		t.Errorf("a window without TitleBarOverlay reports %v", r)
	}
	plain.ExpectWindowState(telatest.ClientDecorations(false))
}

// SetTitleBarControls moves the buttons of an overlay window and the state
// follows; a window that is not one fails the test.
func TestSetTitleBarControls(t *testing.T) {
	changes := 0
	app := telatest.MountFunc(t, func() tela.Node {
		changes++
		return Textf("controls %v", tela.UseWindow().State().TitleBarControls.Width)
	}, telatest.TitleBarOverlay())
	app.ExpectText("controls 0")
	app.SetTitleBarControls(tela.Rect{X: 8, Y: 10, Width: 60, Height: 14})
	app.ExpectText("controls 60")
	app.SetTitleBarControls(tela.Rect{})
	app.ExpectText("controls 0")

	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, func() tela.Node { return Text("x") })
		a.SetTitleBarControls(tela.Rect{X: 8, Y: 10, Width: 60, Height: 14})
	}, "SetTitleBarControls", "TitleBarOverlay")
}

func TestTitleBarControlsValidation(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, func() tela.Node { return Text("x") }, telatest.TitleBarControls(tela.Rect{Width: -1}))
	}, "TitleBarControls")
	// An infinite rectangle is the option's error, not a panic of the fake
	// window manager.
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, func() tela.Node { return Text("x") }, telatest.TitleBarControls(tela.Rect{Width: float32(math.Inf(1)), Height: 14}))
	}, "TitleBarControls", "finite")
}

// The app menu bar helpers read the bar and choose by label path, and fail
// with the way out.
func TestAppMenuHelpers(t *testing.T) {
	chose := ""
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.Component(desktop.AppMenuBar{
			AppItems: []desktop.MenuItem{{Label: "Settings…", OnSelect: func() { chose = "settings" }}, {Label: "Locked", Disabled: true}},
			Menus: []desktop.MenuItem{
				{Label: "File", Submenu: []desktop.MenuItem{{Label: "New", OnSelect: func() { chose = "new" }}, {Label: "Off", Disabled: true}}},
				{Role: desktop.RoleEditMenu},
			},
		})
	})
	if bar := app.AppMenuBar(); len(bar) != 2 || bar[0].Label != "File" || bar[1].Role != desktop.RoleEditMenu {
		t.Fatalf("AppMenuBar %+v", bar)
	}
	if items := app.AppMenuItems(); len(items) != 2 || items[0].Label != "Settings…" {
		t.Fatalf("AppMenuItems %+v", items)
	}
	app.ChooseAppMenuItem("File", "New")
	if chose != "new" {
		t.Errorf("chose %q", chose)
	}
	app.ChooseAppMenuItem("Settings…")
	if chose != "settings" {
		t.Errorf("chose %q", chose)
	}

	for _, tc := range []struct {
		path []string
		want []string
	}{
		{[]string{"File", "Missing"}, []string{"no item", "Missing", "New"}},
		{[]string{"File", "Off"}, []string{"disabled"}},
		{[]string{"File"}, []string{"opens a submenu"}},
		{[]string{"Nope"}, []string{"no item", "Nope"}},
		{[]string{"Locked"}, []string{"disabled"}},
		{nil, []string{"give the label"}},
	} {
		path := tc.path
		mustFail(t, func(tb testing.TB) {
			a := telatest.MountFunc(tb, func() tela.Node {
				return tela.Component(desktop.AppMenuBar{
					AppItems: []desktop.MenuItem{{Label: "Locked", Disabled: true}},
					Menus:    []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{{Label: "New"}, {Label: "Off", Disabled: true}}}},
				})
			})
			a.ChooseAppMenuItem(path...)
		}, tc.want...)
	}

	// Without an app menu bar anywhere the way out is named.
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, func() tela.Node { return Text("x") }).ChooseAppMenuItem("File", "New")
	}, "no native menu bar")
	if bar := telatest.MountFunc(t, func() tela.Node { return Text("x") }).AppMenuBar(); bar != nil {
		t.Errorf("AppMenuBar of an app with none: %+v", bar)
	}
}

// The app bar reaches every Unsupported group the test plays: the platform
// bar (macOS), window bars (Windows) and none (Linux).
func TestAppMenuUnsupportedGroups(t *testing.T) {
	bar := func() tela.Node {
		return tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{{Label: "File", Submenu: []desktop.MenuItem{{Label: "New"}}}}})
	}
	mac := telatest.MountFunc(t, bar)
	if len(mac.AppMenuBar()) != 1 || mac.MenuBar() != nil {
		t.Errorf("platform bar: app %+v, window %+v", mac.AppMenuBar(), mac.MenuBar())
	}
	win := telatest.MountFunc(t, bar, telatest.WithUnsupported(telatest.AppMenu))
	if len(win.MenuBar()) != 1 {
		t.Errorf("window bars: %+v", win.MenuBar())
	}
	linux := telatest.MountFunc(t, bar, telatest.WithUnsupported(telatest.AppMenu, telatest.NativeMenus))
	if linux.MenuBar() != nil || linux.AppMenuBar() != nil {
		t.Errorf("none: window %+v, app %+v", linux.MenuBar(), linux.AppMenuBar())
	}
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, bar, telatest.WithUnsupported(telatest.DesktopService(99)))
	}, "WithUnsupported", "not a DesktopService")
}
