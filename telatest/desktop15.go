package telatest

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
)

// The macOS desktop integration of phase 15 in tests (ADR-224, ADR-226):
// quit requests, the OS's window buttons over a TitleBarOverlay window and
// the app's menu bar, through the headless backend's fakes.

// RequestQuit plays the user or the OS asking the app to quit, as macOS
// does for Cmd+Q, the app menu's Quit, the Dock's Quit and a logout
// (tela.App.OnQuit): the app's OnQuit runs ([OnQuit]), or, without one,
// every window gets a close request as from its close button, so an app
// whose main window has no OnClose quits. The app settles.
//
//	app := telatest.MountApp(t, tela.App{Root: Editor{}, OnQuit: func() { askToSave() }})
//	app.RequestQuit()
//	app.ExpectText("Save changes?")
func (a *App) RequestQuit() {
	a.tb.Helper()
	a.mustRun()
	a.backend.RequestQuit()
	a.settleAfter()
}

// SetTitleBarControls plays the OS moving or resizing the window buttons
// it keeps over a [TitleBarOverlay] window (macOS places the traffic lights
// in the height of the app's caption; fullscreen hides them): r is the
// rectangle in logical window coordinates, and the empty one removes them.
// The window reports it as [tela.WindowState].TitleBarControls, and the
// app settles. It fails the test for a window that is not a TitleBarOverlay
// one.
//
//	app.SetTitleBarControls(tela.Rect{X: 8, Y: 20, Width: 58, Height: 14})
func (a *App) SetTitleBarControls(r tela.Rect) {
	a.tb.Helper()
	a.mustBeOpen()
	func() {
		defer func() {
			if p := recover(); p != nil {
				a.fatalf("SetTitleBarControls(%v): %v (mount the app with telatest.TitleBarOverlay(), or App.TitleBarOverlay in MountApp)", r, p)
			}
		}()
		a.win.SetTitleBarControls(platform.Rect{X: r.X, Y: r.Y, W: r.Width, H: r.Height})
	}()
	a.settleAfter()
}

// AppMenuBar returns the app's menu bar (a mounted desktop.AppMenuBar) as
// the platform has it, nil without one, after settling the app. Where the
// platform has no app bar but windows have menu bars (Windows;
// [WithUnsupported](AppMenu)), it is the bar of this App's window, which
// App.MenuBar returns too. The app items are [App.AppMenuItems].
func (a *App) AppMenuBar() []MenuItem {
	a.tb.Helper()
	a.settleAfter()
	menu, _ := a.backend.AppMenu()
	if len(menu) == 0 {
		return a.MenuBar()
	}
	return menuItems(menu)
}

// AppMenuItems returns the items the app gave macOS's app menu
// (desktop.AppMenuBar.AppItems), after settling the app. They are empty
// where the platform ignores them.
func (a *App) AppMenuItems() []MenuItem {
	a.tb.Helper()
	a.settleAfter()
	_, items := a.backend.AppMenu()
	return menuItems(items)
}

// ChooseAppMenuItem chooses an item of the app's menu bar, named by the
// labels from the top-level entry down, as the user does with the mouse:
// its OnSelect runs and the app settles. An item of macOS's app menu is
// named by its label alone ([App.AppMenuItems]). Role items are reported
// like any item, as on a platform without the role's native action, so
// the app carries them out (Copy acts on the focused field). Where the
// platform has no app bar but windows have menu bars (Windows;
// [WithUnsupported](AppMenu)), it chooses from this App's window's bar. It
// fails the test when the app has no menu bar, or the item is missing,
// disabled or opens a submenu.
//
//	app.ChooseAppMenuItem("File", "New Note")
//	app.ChooseAppMenuItem("Settings…")
func (a *App) ChooseAppMenuItem(path ...string) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleAfter()
	menu, items := a.backend.AppMenu()
	if len(menu) == 0 && len(items) == 0 {
		a.ChooseMenuItem(path...)
		return
	}
	pool := menu
	if len(path) == 1 {
		// A top-level label of the bar wins; else the app menu's item.
		if _, ok := findTopLevel(menu, path[0]); !ok {
			pool = items
		}
	}
	it := a.findMenuItem("ChooseAppMenuItem", pool, path)
	a.backend.ChooseAppMenu(it.ID)
	a.settleAfter()
}

// findTopLevel reports whether menu has an item with the label.
func findTopLevel(menu []platform.MenuItem, label string) (platform.MenuItem, bool) {
	for _, it := range menu {
		if !it.Separator && it.Label == label {
			return it, true
		}
	}
	return platform.MenuItem{}, false
}
