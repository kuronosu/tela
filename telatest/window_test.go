package telatest_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// winNotes is an app with a main window and a modal Settings window that
// a button opens; both record the cleanups of their effects.
type winNotes struct {
	clicks  *int
	cleaned *[]string
}

func (n winNotes) Build() tela.Node {
	open, setOpen := tela.UseState(false)
	tela.UseMount(func() func() { return func() { *n.cleaned = append(*n.cleaned, "main") } })
	return Div(Class("flex gap-2 p-4"),
		Button(OnClick(func() { *n.clicks++ }), Text("Count")),
		Button(OnClick(func() { setOpen.Set(true) }), Text("Settings")),
		If(open, tela.Component(tela.Window{
			Title: "Settings", Width: 300, Height: 200, Modal: true, RootClass: "bg-rose-500",
			OnClose: func() { setOpen.Set(false) },
			Root:    winSettings{cleaned: n.cleaned},
		})),
	)
}

type winSettings struct{ cleaned *[]string }

func (s winSettings) Build() tela.Node {
	n, setN := tela.UseState(0)
	tela.UseMount(func() func() { return func() { *s.cleaned = append(*s.cleaned, "settings") } })
	return Div(Class("p-4"),
		H1(Text("Preferences")),
		Button(OnClick(func() { setN.Set(n + 1) }), Textf("Applied %d", n)),
	)
}

func mountNotes(tb testing.TB, clicks *int, cleaned *[]string, opts ...telatest.Option) *telatest.App {
	tb.Helper()
	opts = append([]telatest.Option{telatest.Size(400, 300), telatest.Title("Notes"), telatest.RootClass("bg-slate-800"), telatest.Screen(1000, 700)}, opts...)
	return telatest.Mount(tb, winNotes{clicks: clicks, cleaned: cleaned}, opts...)
}

func openSettings(app *telatest.App) *telatest.App {
	return app.WaitForWindow(func() { app.Click(telatest.ByRole("button", "Settings")) })
}

// A two-window app with a modal: the windows are listed and selected, each
// has the queries, actions and pixels of its own, the modal blocks the main
// window, and closing both ends the app with the cleanups run.
func TestWindowsOfAnApp(t *testing.T) {
	clicks := 0
	var cleaned []string
	app := mountNotes(t, &clicks, &cleaned)

	if ws := app.Windows(); len(ws) != 1 || ws[0] != app || app.MainWindow() != app {
		t.Fatalf("before the window opens: %d windows, the first is the App Mount returned: %v", len(ws), len(ws) > 0 && ws[0] == app)
	}
	if app.Title() != "Notes" || app.Parent() != nil || app.Modal() || app.Blocked() {
		t.Errorf("main window: title %q, parent %v, modal %v, blocked %v", app.Title(), app.Parent(), app.Modal(), app.Blocked())
	}

	settings := openSettings(app)
	ws := app.Windows()
	if len(ws) != 2 || ws[0] != app || ws[1] != settings {
		t.Fatalf("Windows() has %d entries; want the main App and the new window", len(ws))
	}
	if app.Window("Settings") != settings || app.WindowAt(1) != settings || app.LastWindow() != settings || settings.MainWindow() != app || settings.Window("Notes") != app {
		t.Error("a window selected by title, index or as the newest is not the same App each time")
	}
	if settings.Title() != "Settings" || !settings.Modal() || settings.Parent() != app || settings.Blocks() != app {
		t.Errorf("Settings: title %q, modal %v, parent is main %v", settings.Title(), settings.Modal(), settings.Parent() == app)
	}
	if app.BlockedBy() != settings || !app.Blocked() || settings.Blocked() || app.Blocks() != nil {
		t.Errorf("modality: main blocked by Settings %v, blocked %v, Settings blocked %v", app.BlockedBy() == settings, app.Blocked(), settings.Blocked())
	}

	// Queries and pixels are scoped to the window.
	settings.ExpectText("Preferences")
	app.ExpectNoText("Preferences")
	if settings.Count(telatest.ByRole("button", "Applied 0")) != 1 || app.Count(telatest.ByRole("button", "Applied 0")) != 0 {
		t.Error("a locator matched across windows")
	}
	if b := settings.Screenshot().Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Errorf("the Settings window is %v, want 300x200", b)
	}
	if got := settings.PixelAt(299, 199); got.R < 0xf0 || got.G > 0x40 {
		t.Errorf("Settings corner %v, want rose-500 (its RootClass)", got)
	}
	if got := app.PixelAt(399, 299); got.R != 0x1d || got.G != 0x29 {
		t.Errorf("main corner %v, want slate-800", got)
	}
	if !strings.Contains(settings.AriaSnapshot(), "Preferences") || strings.Contains(app.AriaSnapshot(), "Preferences") {
		t.Errorf("aria snapshots: main\n%s\nsettings\n%s", app.AriaSnapshot(), settings.AriaSnapshot())
	}

	// Actions go to the window; clicking in it activates it.
	settings.Click(telatest.ByRole("button", "Applied 0"))
	settings.ExpectText("Applied 1")
	settings.ExpectWindowState(telatest.Active(true))
	app.ExpectWindowState(telatest.Active(false))

	// A blocked window drops the input, and a press in it activates the modal window.
	app.Click(telatest.ByRole("button", "Count"))
	if clicks != 0 {
		t.Errorf("a click in the blocked main window ran its handler (%d clicks)", clicks)
	}
	settings.ExpectWindowState(telatest.Active(true))
	app.ExpectWindowState(telatest.Active(false))
	app.RequestClose()
	app.ExpectRunning()
	app.ExpectWindowCount(2)

	// The user closes the modal window: its OnClose unmounts it.
	settings.RequestClose()
	settings.ExpectClosed()
	app.ExpectWindowCount(1)
	app.ExpectNoWindow("Settings")
	if app.Blocked() || !settings.Closed() || !slices.Contains(cleaned, "settings") || slices.Contains(cleaned, "main") {
		t.Errorf("after closing Settings: main blocked %v, Settings closed %v, cleanups %v", app.Blocked(), settings.Closed(), cleaned)
	}
	if settings.Title() != "Settings" {
		t.Errorf("the title of a closed window is %q", settings.Title())
	}
	app.Click(telatest.ByRole("button", "Count"))
	if clicks != 1 {
		t.Errorf("%d clicks after the modal closed, want 1", clicks)
	}

	// Closing the main window without OnClose quits the app.
	if app.HasQuit() {
		t.Fatal("the app quit before the main window was closed")
	}
	app.RequestClose()
	app.ExpectQuit()
	if !app.HasQuit() || !slices.Contains(cleaned, "main") {
		t.Errorf("after closing the main window: quit %v, cleanups %v", app.HasQuit(), cleaned)
	}
}

// Using a window that closed, or an app that quit, fails with the reason.
func TestWindowViewsAfterTheEnd(t *testing.T) {
	var clicks int
	var cleaned []string
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned)
		settings := openSettings(app)
		settings.RequestClose()
		settings.ExpectClosed()
		settings.AriaSnapshot()
	}, `the window "Settings" was closed`, "App.Windows")
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned)
		app.RequestClose()
		app.ExpectQuit()
		app.AriaSnapshot()
	}, "the app quit")
}

// What a missing window and an ambiguous title report.
func TestWindowSelectionFailures(t *testing.T) {
	var clicks int
	var cleaned []string
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned, telatest.Timeout(50*time.Millisecond))
		app.Window("Nope")
	}, `no open window has the title "Nope"`, `0: "Notes" (main)`)
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned, telatest.Timeout(50*time.Millisecond))
		app.WindowAt(3)
	}, "WindowAt(3): the app has 1 open window(s)")
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned, telatest.Timeout(50*time.Millisecond))
		app.WaitForWindow(func() {})
	}, "no new window opened")
	mustFail(t, func(tb testing.TB) {
		app := mountNotes(tb, &clicks, &cleaned, telatest.Timeout(50*time.Millisecond))
		app.ExpectWindowCount(2)
	}, "expected 2 open window(s), the app has 1")
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, func() tela.Node {
			return Div(
				tela.Component(tela.Window{Title: "Twin", Width: 100, Height: 80, OnClose: func() {}, Root: tela.C(winBody)}),
				tela.Component(tela.Window{Title: "Twin", Width: 100, Height: 80, OnClose: func() {}, Root: tela.C(winBody)}),
			)
		}, telatest.Size(200, 100))
		app.Window("Twin")
	}, `2 open windows have the title "Twin"`, "WindowAt(index)")
}

func winBody() tela.Node { return Div(Text("body")) }

// A window opened later (a timer) is waited for like a frame: selecting it
// settles the app, advancing its clock, until it exists.
func winLate() tela.Node {
	open, setOpen := tela.UseState(false)
	tela.UseTimeout(2*time.Second, func() { setOpen.Set(true) })
	return Div(Text("main"), If(open, tela.Component(tela.Window{
		Title: "Late", Width: 200, Height: 100,
		OnClose: func() { setOpen.Set(false) },
		Root:    tela.C(winBody),
	})))
}

func TestWindowAutoWait(t *testing.T) {
	app := telatest.MountFunc(t, winLate, telatest.Size(300, 200))
	if n := len(app.Windows()); n != 1 {
		t.Fatalf("%d windows before the timer fired, want 1", n)
	}
	late := app.Window("Late")
	late.ExpectText("body")
	app.ExpectWindowCount(2)
	late.RequestClose()
	late.ExpectClosed()
	app.ExpectWindowCount(1)
}

// The title of a window follows the app's state.
func TestWindowExpectTitle(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		n, setN := tela.UseState(0)
		title := "Draft"
		if n > 0 {
			title = "Draft (edited)"
		}
		return Div(
			Button(OnClick(func() { setN.Set(n + 1) }), Text("Edit")),
			tela.Component(tela.Window{Title: title, Width: 200, Height: 100, OnClose: func() {}, Root: tela.C(winBody)}),
		)
	}, telatest.Size(300, 200))
	draft := app.Window("Draft")
	app.Click(telatest.ByRole("button", "Edit"))
	draft.ExpectTitle("Draft (edited)")
	if app.Window("Draft (edited)") != draft {
		t.Error("the renamed window is not the same App")
	}
	app.ExpectNoWindow("Draft")
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, winLate, telatest.Size(300, 200), telatest.Timeout(50*time.Millisecond))
		a.ExpectTitle("Other")
	}, `the window title is "telatest", expected "Other"`)
}

// A window that refuses to close stays open.
func TestWindowCloseRefused(t *testing.T) {
	stuck := func() tela.Node {
		return Div(tela.Component(tela.Window{Title: "Stuck", Width: 100, Height: 80, Root: tela.C(winBody)}))
	}
	app := telatest.MountFunc(t, stuck, telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
	w := app.Window("Stuck")
	w.RequestClose()
	app.ExpectWindowCount(2)
	if w.Closed() {
		t.Error("a Window without OnClose closed")
	}
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, stuck, telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
		a.Window("Stuck").ExpectClosed()
	}, `the window "Stuck" is still open`)
}

// The window manager changes a window's state, as the user does with the
// OS: the app sees the state, the size and the placement.
func TestWindowStateThroughTheWindowManager(t *testing.T) {
	var clicks int
	var cleaned []string
	app := mountNotes(t, &clicks, &cleaned)
	settings := openSettings(app)
	size := func() [2]float32 { w, h := settings.WindowSize(); return [2]float32{w, h} }
	if size() != [2]float32{300, 200} {
		t.Fatalf("the window is %v, want 300x200", size())
	}

	settings.Maximize()
	settings.ExpectWindowState(telatest.Maximized(true), telatest.Fullscreen(false), telatest.Minimized(false))
	if size() != [2]float32{1000, 700} {
		t.Errorf("a maximized window is %v, want the fake screen's 1000x700", size())
	}
	settings.Unmaximize()
	settings.ExpectWindowState(telatest.Maximized(false))
	if size() != [2]float32{300, 200} {
		t.Errorf("an unmaximized window is %v, want 300x200", size())
	}

	settings.SetFullscreen(true)
	settings.ExpectWindowState(telatest.Fullscreen(true))
	if size() != [2]float32{1000, 700} {
		t.Errorf("a fullscreen window is %v, want 1000x700", size())
	}
	settings.SetFullscreen(false)
	settings.ExpectWindowState(telatest.Fullscreen(false))

	settings.Minimize()
	settings.ExpectWindowState(telatest.Minimized(true), telatest.Active(false))
	settings.Restore()
	settings.ExpectWindowState(telatest.Minimized(false))
	settings.BringToFront()
	settings.ExpectWindowState(telatest.Active(true))
	app.ExpectWindowState(telatest.Active(false))

	settings.Move(40, 50)
	if p := settings.Placement(); !p.HasPosition || p.X != 40 || p.Y != 50 {
		t.Errorf("placement %+v, want the position 40,50", p)
	}

	settings.Resize(250, 150)
	if size() != [2]float32{250, 150} {
		t.Errorf("a resized window is %v, want 250x150", size())
	}

	// A failed expectation shows what was unmet and the state it has.
	mustFail(t, func(tb testing.TB) {
		a := mountNotes(tb, &clicks, &cleaned, telatest.Timeout(50*time.Millisecond))
		a.ExpectWindowState(telatest.Maximized(true), telatest.Hidden(false))
	}, "does not meet Maximized(true)", "its state is {active}")
	mustFail(t, func(tb testing.TB) {
		a := mountNotes(tb, &clicks, &cleaned)
		a.ExpectWindowState()
	}, "needs a condition")
}

// A custom title bar: the hit regions it sends to the OS, a press on its
// drag area, a double click that maximizes, and its window-control buttons.
func winTitleBar() tela.Node {
	maximize := "Maximize"
	if tela.UseWindow().State().Maximized {
		maximize = "Restore"
	}
	return Div(Class("flex h-9 w-full items-center bg-slate-800 text-white"), WindowDrag(), TestID("titlebar"),
		Span(Class("grow px-3 text-sm"), Text("Notes")),
		Button(Class("size-9"), WindowControl(tela.WindowMinimize), AriaLabel("Minimize"), Text("-")),
		Button(Class("size-9"), WindowControl(tela.WindowMaximize), AriaLabel(maximize), Text("+")),
		Button(Class("size-9"), WindowControl(tela.WindowClose), AriaLabel("Close"), Text("x")),
	)
}

func winFramelessRoot() tela.Node {
	return Div(Class("flex flex-col"),
		tela.C(winTitleBar),
		Div(Class("p-4"), Text("content")),
	)
}

func TestWindowCustomTitleBar(t *testing.T) {
	app := telatest.MountApp(t, tela.App{
		Title: "Notes", Width: 400, Height: 300, Frameless: true,
		Root: tela.C(winFramelessRoot),
	}, telatest.Screen(1000, 700))
	app.ExpectWindowState(telatest.ClientDecorations(true))

	var kinds []string
	for _, r := range app.HitRegions() {
		kinds = append(kinds, r.Kind.String())
	}
	if got := strings.Join(kinds, " "); got != "caption minimize maximize close" {
		t.Fatalf("hit regions %q, want the caption and the three controls", got)
	}
	bar := app.Bounds(telatest.ByTestID("titlebar"))
	closeBtn := app.Bounds(telatest.ByRole("button", "Close"))
	if got := app.HitAt(bar.X+20, bar.Y+bar.Height/2); got != telatest.HitCaption {
		t.Errorf("the title text is %v, want caption", got)
	}
	if got := app.HitAt(closeBtn.X+closeBtn.Width/2, closeBtn.Y+closeBtn.Height/2); got != telatest.HitClose {
		t.Errorf("the close button is %v, want close", got)
	}
	if got := app.HitAt(200, 200); got != telatest.HitClient {
		t.Errorf("the content is %v, want client", got)
	}

	// A press on the drag area is the OS's: it starts a move.
	app.ClickAt(20, 18)
	if app.CaptionDrags() != 1 {
		t.Errorf("%d caption drags after a press on the drag area, want 1", app.CaptionDrags())
	}
	app.ExpectWindowState(telatest.Maximized(false))
	// A double click on it maximizes.
	app.DoubleClick(telatest.ByTestID("titlebar"))
	app.ExpectWindowState(telatest.Maximized(true))
	if w, h := app.WindowSize(); w != 1000 || h != 700 {
		t.Errorf("the maximized window is %gx%g, want 1000x700", w, h)
	}
	// The controls are the app's: the label follows the state.
	app.Click(telatest.ByRole("button", "Restore"))
	app.ExpectWindowState(telatest.Maximized(false))
	app.Click(telatest.ByRole("button", "Maximize"))
	app.ExpectWindowState(telatest.Maximized(true))
	app.Click(telatest.ByRole("button", "Restore"))
	app.Click(telatest.ByRole("button", "Minimize"))
	app.ExpectWindowState(telatest.Minimized(true), telatest.Active(false))
	app.Restore()
	app.BringToFront()
	app.ExpectWindowState(telatest.Minimized(false), telatest.Active(true))

	// The close control asks to close the main window, which quits the app.
	app.ExpectRunning()
	app.Click(telatest.ByRole("button", "Close"))
	app.ExpectQuit()
}

// tela.Quit ends the app from a handler, and unmounts every window.
func TestQuitRunsTheCleanups(t *testing.T) {
	var cleaned []string
	root := func() tela.Node {
		tela.UseMount(func() func() { return func() { cleaned = append(cleaned, "main") } })
		return Div(
			Button(OnClick(tela.Quit), Text("Quit")),
			tela.Component(tela.Window{Title: "Tools", Width: 100, Height: 80, OnClose: func() {}, Root: tela.C(func() tela.Node {
				tela.UseMount(func() func() { return func() { cleaned = append(cleaned, "tools") } })
				return Div(Text("tools"))
			})}),
		)
	}
	app := telatest.MountFunc(t, root, telatest.Size(200, 100))
	app.ExpectWindowCount(2)
	app.ExpectRunning()
	if len(cleaned) != 0 || app.HasQuit() {
		t.Fatalf("before quitting: cleanups %v, quit %v", cleaned, app.HasQuit())
	}
	app.Click(telatest.ByRole("button", "Quit"))
	app.ExpectQuit()
	slices.Sort(cleaned)
	if !app.HasQuit() || !slices.Equal(cleaned, []string{"main", "tools"}) {
		t.Errorf("after tela.Quit: quit %v, cleanups %v", app.HasQuit(), cleaned)
	}
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, root, telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
		a.ExpectQuit()
	}, "the app is still running")
}

// An OnClose that hides the window keeps the app running, in the tray; a
// hidden window draws nothing until it is shown; tela.Quit ends the app.
func TestAppOnCloseAndHidden(t *testing.T) {
	var handle tela.WindowHandle
	root := func() tela.Node {
		handle = tela.UseWindow()
		return Div(Text("hello"))
	}
	trayApp := tela.App{
		Title: "Tray", Width: 200, Height: 100, Hidden: true, Root: tela.C(root),
		OnClose: func(w tela.WindowHandle) { w.Hide() },
	}
	mustFail(t, func(tb testing.TB) {
		telatest.MountApp(tb, trayApp).Screenshot()
	}, "it is hidden")

	app := telatest.MountApp(t, trayApp)
	if app.Title() != "Tray" {
		t.Errorf("title %q, want the App's", app.Title())
	}
	app.ExpectWindowState(telatest.Hidden(true), telatest.Active(false))
	handle.Show()
	app.ExpectWindowState(telatest.Hidden(false), telatest.Active(true))
	app.ExpectText("hello")

	app.RequestClose()
	app.ExpectWindowState(telatest.Hidden(true))
	app.ExpectRunning()
	if app.HasQuit() || app.Closed() {
		t.Fatalf("closing the window quit the app (quit %v, closed %v)", app.HasQuit(), app.Closed())
	}
	tela.Quit()
	app.ExpectQuit()
}

// MountApp applies the OS-window fields of the App; FromApp, which tools
// use to render the screen, does not.
func TestMountAppWindowFields(t *testing.T) {
	app := telatest.MountApp(t, tela.App{
		Title: "Limits", Width: 400, Height: 300, MinWidth: 300, MinHeight: 200, MaxWidth: 500, MaxHeight: 400,
		Root: tela.C(winBody),
	})
	if app.Title() != "Limits" {
		t.Errorf("title %q, want Limits", app.Title())
	}
	app.Resize(100, 100)
	if w, h := app.WindowSize(); w != 300 || h != 200 {
		t.Errorf("a window resized below MinWidth/MinHeight is %gx%g, want 300x200", w, h)
	}
	app.Resize(900, 900)
	if w, h := app.WindowSize(); w != 500 || h != 400 {
		t.Errorf("a window resized above MaxWidth/MaxHeight is %gx%g, want 500x400", w, h)
	}

	placed := telatest.MountApp(t, tela.App{
		Width: 400, Height: 300, Root: tela.C(winBody),
		Placement: tela.WindowPlacement{Width: 500, Height: 350, X: 30, Y: 40, HasPosition: true},
	})
	if w, h := placed.WindowSize(); w != 500 || h != 350 {
		t.Errorf("a placed window is %gx%g, want the placement's 500x350", w, h)
	}
	if p := placed.Placement(); !p.HasPosition || p.X != 30 || p.Y != 40 {
		t.Errorf("placement %+v, want 30,40", p)
	}

	// FromApp: only what the content looks like.
	tool := telatest.Mount(t, tela.C(winBody), telatest.FromApp(tela.App{Title: "Tray", Hidden: true, Frameless: true}))
	if tool.Title() != "telatest" || tool.WindowState().Hidden || tool.WindowState().ClientSideDecorations {
		t.Errorf("FromApp applied the OS-window fields: title %q, state %+v", tool.Title(), tool.WindowState())
	}
	tool.ExpectText("body")
}

// The options set the OS-window fields by hand.
func TestWindowOptions(t *testing.T) {
	app := telatest.Mount(t, tela.C(winBody), telatest.Size(300, 200), telatest.Title("Hand"), telatest.Frameless(),
		telatest.MinSize(250, 150), telatest.Screen(800, 600))
	if app.Title() != "Hand" || !app.WindowState().ClientSideDecorations {
		t.Errorf("title %q, state %+v", app.Title(), app.WindowState())
	}
	app.Maximize()
	if w, h := app.WindowSize(); w != 800 || h != 600 {
		t.Errorf("a maximized window is %gx%g, want the 800x600 screen", w, h)
	}
	mustFail(t, func(tb testing.TB) { telatest.Mount(tb, tela.C(winBody), telatest.Screen(800, 0)) }, "telatest.Screen(800, 0)")
	mustFail(t, func(tb testing.TB) { telatest.Mount(tb, tela.C(winBody), telatest.MinSize(-1, 0)) }, "MinSize width is -1")
}

func ExampleApp_WaitForWindow() {
	var clicks int
	var cleaned []string
	app := mountNotes(t, &clicks, &cleaned)

	settings := app.WaitForWindow(func() {
		app.Click(telatest.ByRole("button", "Settings"))
	})
	settings.ExpectText("Preferences")
	settings.Click(telatest.ByRole("button", "Applied 0"))
	settings.ExpectText("Applied 1")
	fmt.Println(settings.Title(), settings.Modal(), app.Blocked(), len(app.Windows()))
	// Output: Settings true true 2
}
