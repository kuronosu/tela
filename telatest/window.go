package telatest

import (
	"fmt"
	"image"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Windows (ADR-183). An app has one window per tela.Window that is mounted,
// besides its main window, and the headless backend is a deterministic fake
// window manager (ADR-179): it grants every request at once, gives hidden
// and minimized windows no frames, and records the hit regions, the caption
// presses, the parents and the modality of the windows. The host commits
// the builds and effects of a window without frames itself (ADR-185), and
// settling waits for them (core.Host.PausedWork).
//
// An *App is scoped to one window: the one Mount returns to the main window,
// and App.Windows and the other selectors return one per window, the same
// *App for the same window for as long as it is open. Everything that
// queries, acts, asserts or compares pixels (Click, Fill, Press, Expect…,
// AriaSnapshot, Golden, Bounds, PixelAt…) works on the window the App is
// scoped to; the clock, settling and the end of the app are the whole
// app's. Methods named for the window (Title, WindowState, RequestClose,
// Minimize…) work on the App's own window too, the main window's in the
// App Mount returns.

// view returns the App scoped to window w, made on first use.
func (a *App) view(w platform.Window) *App {
	hw := w.(*headless.Window)
	if v, ok := a.sh.views[hw]; ok {
		return v
	}
	v := &App{
		tb: a.tb, cfg: a.cfg, backend: a.backend, host: a.host,
		win: hw, tree: a.host.TreeOf(w), sh: a.sh, dev: a.dev,
	}
	a.sh.views[hw] = v
	return v
}

// openWindows returns the App of every open window, in order of opening.
func (a *App) openWindows() []*App {
	ws := a.host.Windows()
	out := make([]*App, len(ws))
	for i, w := range ws {
		out[i] = a.view(w)
	}
	return out
}

// windowList describes the open windows for failure messages.
func (a *App) windowList() string {
	ws := a.openWindows()
	if len(ws) == 0 {
		return "the app has no open windows"
	}
	var b strings.Builder
	b.WriteString("open windows, in order of opening:")
	for i, w := range ws {
		fmt.Fprintf(&b, "\n  %d: %q", i, w.win.Title())
		if i == 0 {
			b.WriteString(" (main)")
		}
	}
	return b.String()
}

// Windows returns the app's open windows in the order they opened, the main
// window first: an [App] scoped to each, with the same queries, actions,
// expectations and goldens as the one Mount returns. A window that is
// hidden (the app's Hidden, a hidden Window) or minimized is still open and
// listed. The app is settled first, so a window a handler just asked for is
// listed; use [App.WaitForWindow] or [App.ExpectWindowCount] for one that
// opens later (a goroutine, a timer). Like Playwright's context.pages().
//
//	for _, w := range app.Windows() {
//	    t.Logf("%q is %v", w.Title(), w.WindowState())
//	}
func (a *App) Windows() []*App {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	a.mustRun()
	return a.openWindows()
}

// MainWindow returns the App scoped to the app's main window, the one
// [Mount] returned.
func (a *App) MainWindow() *App {
	a.tb.Helper()
	a.mustRun()
	return a.view(a.host.Window())
}

// Window returns the open window whose title is title (exactly), waiting
// until there is one, as Playwright waits for a locator. It fails the test
// at the timeout, listing the titles of the open windows, and at once when
// several windows have the title (select one with [App.WindowAt]).
//
//	settings := app.Window("Settings")
//	settings.Click(telatest.ByRole("button", "Close"))
func (a *App) Window(title string) *App {
	a.tb.Helper()
	a.mustRun()
	var found *App
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		a.mustRun()
		var matches []*App
		for _, w := range a.openWindows() {
			if w.win.Title() == title {
				matches = append(matches, w)
			}
		}
		switch len(matches) {
		case 0:
			return false, fmt.Sprintf("no open window has the title %q\n%s", title, a.windowList())
		case 1:
			found = matches[0]
			return true, ""
		}
		a.tb.Fatalf("telatest: %d open windows have the title %q; select one with WindowAt(index)\n%s", len(matches), title, a.windowList())
		return false, ""
	})
	return found
}

// WindowAt returns the open window at index i in the order the windows
// opened (0 is the main window), waiting until there are that many.
func (a *App) WindowAt(i int) *App {
	a.tb.Helper()
	a.mustRun()
	if i < 0 {
		a.tb.Fatalf("telatest: WindowAt(%d): the index counts from 0, the main window, in the order the windows opened", i)
	}
	var found *App
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		a.mustRun()
		ws := a.openWindows()
		if i < len(ws) {
			found = ws[i]
			return true, ""
		}
		return false, fmt.Sprintf("WindowAt(%d): the app has %d open window(s)\n%s", i, len(ws), a.windowList())
	})
	return found
}

// LastWindow returns the newest open window: the one opened last, or the
// main window when it is the only one.
func (a *App) LastWindow() *App {
	a.tb.Helper()
	ws := a.Windows()
	return ws[len(ws)-1]
}

// WaitForWindow runs open, which should make the app open a window (a
// click, a key press, a goroutine started by one), and returns the App of
// the window that opened as a result, waiting for it: Playwright's
// page.waitForEvent('popup'). When several windows opened it returns the
// first of them. It fails the test at the timeout.
//
//	settings := app.WaitForWindow(func() {
//	    app.Click(telatest.ByRole("button", "Settings"))
//	})
//	settings.ExpectText("Preferences")
func (a *App) WaitForWindow(open func()) *App {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	known := map[*headless.Window]bool{}
	for _, w := range a.openWindows() {
		known[w.win] = true
	}
	open()
	var found *App
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		a.mustRun()
		for _, w := range a.openWindows() {
			if !known[w.win] {
				found = w
				return true, ""
			}
		}
		return false, "no new window opened\n" + a.windowList()
	})
	return found
}

// ExpectWindowCount waits until the app has n open windows (the main
// window counts), and fails the test at the timeout.
func (a *App) ExpectWindowCount(n int) {
	a.tb.Helper()
	a.mustRun()
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		a.mustRun()
		if got := len(a.host.Windows()); got != n {
			return false, fmt.Sprintf("expected %d open window(s), the app has %d\n%s", n, got, a.windowList())
		}
		return true, ""
	})
}

// ExpectNoWindow waits until no open window has the title, and fails the
// test at the timeout.
func (a *App) ExpectNoWindow(title string) {
	a.tb.Helper()
	a.mustRun()
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		a.mustRun()
		for _, w := range a.openWindows() {
			if w.win.Title() == title {
				return false, fmt.Sprintf("the window %q is still open\n%s", title, a.windowList())
			}
		}
		return true, ""
	})
}

// ---------------------------------------------------------------------
// Reading a window.

// Title returns the window's title, as the OS shows it: tela.App.Title or
// tela.Window.Title, as of the latest build.
func (a *App) Title() string {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	return a.win.Title()
}

// ExpectTitle waits until the window's title is title.
func (a *App) ExpectTitle(title string) {
	a.tb.Helper()
	a.mustBeOpen()
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		if got := a.win.Title(); got != title {
			return false, fmt.Sprintf("the window title is %q, expected %q", got, title)
		}
		return true, ""
	})
}

// WindowState returns the window's state as the app sees it
// (tela.WindowHandle.State): maximized, fullscreen and minimized as the
// fake window manager reported them, whether the window is the active one,
// whether the app hid it, and whether the app draws its title bar.
func (a *App) WindowState() tela.WindowState {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	return a.host.Handle(a.win).State()
}

// TitleBar returns the title bar colors the window has asked the OS for
// (tela.App.TitleBar, tela.Window.TitleBar), at the 8-bit precision the OS
// takes; a color that is the system's is the zero Color.
func (a *App) TitleBar() tela.TitleBarColors {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	c := a.win.TitleBar()
	col := func(w platform.WindowColor) tela.Color {
		if !w.Set {
			return tela.Color{}
		}
		return tela.RGB(w.R, w.G, w.B)
	}
	return tela.TitleBarColors{Background: col(c.Background), Text: col(c.Text), Border: col(c.Border)}
}

// Backdrop returns the system backdrop the window has asked the OS for
// (tela.App.Backdrop, tela.Window.Backdrop).
func (a *App) Backdrop() tela.Backdrop {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	return a.win.Backdrop()
}

// WindowSize returns the size of the window's content area in logical
// pixels, as the app was told (a maximized window has the fake screen's
// size, see [Screen]).
func (a *App) WindowSize() (width, height float32) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	s := a.win.Metrics().Size
	return s.W, s.H
}

// Placement returns where the window is (tela.WindowHandle.Placement): its
// size outside maximized and fullscreen, its position once the app or
// [App.Move] set one, the state to open in. It still answers after the
// window closed.
func (a *App) Placement() tela.WindowPlacement {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	return a.host.Handle(a.win).Placement()
}

// Closed reports whether the window is closed: it unmounted (its
// tela.Window left the tree, or the user's close request was granted) or
// the app ended.
func (a *App) Closed() bool {
	a.tb.Helper()
	if !a.sh.closed {
		a.settleQuery()
	}
	return a.sh.closed || a.win.Closed()
}

// ExpectClosed waits until the window is closed (see [App.Closed]) and
// fails the test at the timeout. A close request the app refuses (a
// tela.Window whose OnClose does not update the state that renders it)
// leaves the window open.
func (a *App) ExpectClosed() {
	a.tb.Helper()
	a.mustRun()
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		if a.sh.closed || a.win.Closed() {
			return true, ""
		}
		return false, fmt.Sprintf("the window %q is still open", a.win.Title())
	})
}

// Modal reports whether the window is modal to the window it was declared
// in (tela.Window.Modal).
func (a *App) Modal() bool {
	a.tb.Helper()
	a.mustRun()
	return a.win.Options().Modal
}

// Parent returns the window this one was declared in (the App of the
// window whose component rendered the tela.Window), or nil for the main
// window.
func (a *App) Parent() *App {
	a.tb.Helper()
	a.mustRun()
	if p := a.win.Parent(); p != nil {
		return a.view(p)
	}
	return nil
}

// Blocks returns the window a modal window blocks while it is visible: its
// parent. It is nil for a window that is not modal, or hidden.
func (a *App) Blocks() *App {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	if a.win.Options().Modal && a.win.Visible() {
		return a.Parent()
	}
	return nil
}

// BlockedBy returns the visible modal window that blocks this one, or nil.
// A blocked window takes no input: core drops it (after cancelling the
// pointers) and a press in it activates the modal window, and a request to
// close it is refused.
func (a *App) BlockedBy() *App {
	a.tb.Helper()
	a.mustRun()
	a.settleQuery()
	for _, c := range a.win.Children() {
		if c.Options().Modal && c.Visible() {
			return a.view(c)
		}
	}
	return nil
}

// Blocked reports whether a visible modal window blocks this one; see
// [App.BlockedBy].
func (a *App) Blocked() bool {
	a.tb.Helper()
	return a.BlockedBy() != nil
}

// WindowCond is a condition on a window's state for [App.ExpectWindowState].
type WindowCond struct {
	name  string
	check func(tela.WindowState) bool
}

// Maximized is met when the window is (or is not) maximized.
func Maximized(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("Maximized(%t)", on), func(s tela.WindowState) bool { return s.Maximized == on }}
}

// Fullscreen is met when the window is (or is not) fullscreen.
func Fullscreen(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("Fullscreen(%t)", on), func(s tela.WindowState) bool { return s.Fullscreen == on }}
}

// Minimized is met when the window is (or is not) minimized.
func Minimized(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("Minimized(%t)", on), func(s tela.WindowState) bool { return s.Minimized == on }}
}

// Hidden is met when the app has hidden the window (tela.Window.Hidden,
// WindowHandle.Hide), or has not.
func Hidden(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("Hidden(%t)", on), func(s tela.WindowState) bool { return s.Hidden == on }}
}

// Active is met when the window is the active one, the one with the keyboard
// focus (WindowState.Focused), or is not.
func Active(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("Active(%t)", on), func(s tela.WindowState) bool { return s.Focused == on }}
}

// ClientDecorations is met when the app draws the window's title bar
// (WindowState.ClientSideDecorations: a Frameless window), or does not.
func ClientDecorations(on bool) WindowCond {
	return WindowCond{fmt.Sprintf("ClientDecorations(%t)", on), func(s tela.WindowState) bool { return s.ClientSideDecorations == on }}
}

// ExpectWindowState waits until the window's state meets every condition,
// and fails the test at the timeout, showing the unmet conditions and the
// state it has.
//
//	win.Maximize()
//	win.ExpectWindowState(telatest.Maximized(true), telatest.Minimized(false))
func (a *App) ExpectWindowState(conds ...WindowCond) {
	a.tb.Helper()
	a.mustBeOpen()
	if len(conds) == 0 {
		a.tb.Fatalf("telatest: ExpectWindowState needs a condition, such as telatest.Maximized(true); read the whole state with WindowState()")
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		st := a.host.Handle(a.win).State()
		var unmet []string
		for _, c := range conds {
			if !c.check(st) {
				unmet = append(unmet, c.name)
			}
		}
		if len(unmet) == 0 {
			return true, ""
		}
		return false, fmt.Sprintf("the window %q does not meet %s; its state is %s", a.win.Title(), strings.Join(unmet, ", "), describeWindowState(st))
	})
}

// describeWindowState lists the flags a state has set.
func describeWindowState(s tela.WindowState) string {
	var flags []string
	for _, f := range [...]struct {
		on   bool
		name string
	}{{s.Maximized, "maximized"}, {s.Fullscreen, "fullscreen"}, {s.Minimized, "minimized"},
		{s.Focused, "active"}, {s.Hidden, "hidden"}, {s.ClientSideDecorations, "client-side decorations"}} {
		if f.on {
			flags = append(flags, f.name)
		}
	}
	if len(flags) == 0 {
		return "{}"
	}
	return "{" + strings.Join(flags, ", ") + "}"
}

// ---------------------------------------------------------------------
// Window actions: what the user does with the window manager.

// RequestClose asks to close the window as the user does with the close
// button of its title bar or Alt+F4: the window's OnClose runs (for the
// main window tela.App.OnClose, or the app quits). Whether the window
// closes is up to the app: use [App.ExpectClosed], [App.ExpectQuit] or
// [App.ExpectWindowCount] to assert it. A blocked window refuses the
// request and the modal window above it is activated. To press the close
// button of a custom title bar, click it ([App.Click]).
func (a *App) RequestClose() {
	a.tb.Helper()
	a.mustBeOpen()
	a.win.RequestClose()
	a.settleAfter()
}

// wm changes the window's state as the user does with the window manager
// (its own title bar, the task bar, a shortcut), not through the app: the
// app sees the new state as events.
func (a *App) wm(change func(*platform.WindowState)) {
	a.tb.Helper()
	a.mustBeOpen()
	st := a.win.OSState()
	change(&st)
	a.win.SetState(st)
	a.settleAfter()
}

// Minimize minimizes the window, as the user does with the OS: it loses the
// focus and gets no frames until it is restored or activated.
func (a *App) Minimize() {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) { s.Minimized = true })
}

// Maximize maximizes the window to the fake screen ([Screen]) as the
// user does with the OS; it also brings it back from minimized.
func (a *App) Maximize() {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) { s.Maximized, s.Minimized = true, false })
}

// Unmaximize restores a maximized window to its size.
func (a *App) Unmaximize() {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) { s.Maximized = false })
}

// SetFullscreen makes the window fullscreen on the fake screen, or leaves
// fullscreen; entering also brings it back from minimized.
func (a *App) SetFullscreen(on bool) {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) {
		s.Fullscreen = on
		if on {
			s.Minimized = false
		}
	})
}

// Restore brings the window back to its normal state: not minimized, not
// maximized and not fullscreen.
func (a *App) Restore() {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) { s.Minimized, s.Maximized, s.Fullscreen = false, false, false })
}

// Move moves the window so that its content area's top-left corner is at
// (x, y) physical pixels of the fake desktop, as the user dragging it. The
// position shows in [App.Placement] from then on.
func (a *App) Move(x, y int) {
	a.tb.Helper()
	a.wm(func(s *platform.WindowState) { s.HasPosition, s.Position = true, image.Pt(x, y) })
}

// BringToFront activates the window as the user does by clicking its task
// bar entry: it comes back from minimized and, when it is visible, takes the
// keyboard focus from the other windows. The pointer and key actions do it
// by themselves, as clicking or typing in a window does. Playwright's
// page.bringToFront().
func (a *App) BringToFront() {
	a.tb.Helper()
	a.mustBeOpen()
	a.win.Activate()
	a.settleAfter()
}

// focused reports whether the window has the keyboard focus, as the app
// knows it.
func (a *App) focused() bool { return a.host.Handle(a.win).State().Focused }

// activate gives the window the keyboard focus when it has not, as the
// user clicking in it does (the main window of a single-window app has it
// from Mount). A hidden window cannot take it.
func (a *App) activate() {
	a.tb.Helper()
	if a.focused() {
		return
	}
	a.win.Activate()
	a.deliver()
}

// ---------------------------------------------------------------------
// Custom title bars.

// HitKind is what a region of a window's custom title bar is to the OS.
type HitKind = platform.HitKind

const (
	// HitClient is ordinary content: what lies outside every region, and
	// what a hole in a drag region is.
	HitClient = platform.HitClient
	// HitCaption is a drag area (h.WindowDrag): the OS handles presses in
	// it (move, double click to maximize, window menu).
	HitCaption = platform.HitCaption
	// HitMinimize, HitMaximize and HitClose are the window-control buttons
	// (h.WindowControl): events over them reach the app as usual.
	HitMinimize = platform.HitMinimize
	HitMaximize = platform.HitMaximize
	HitClose    = platform.HitClose
)

// HitRegion is one rectangle of a window's custom title bar, in logical
// pixels from the window's top-left corner.
type HitRegion struct {
	Kind HitKind
	Rect tela.Rect
}

func (r HitRegion) String() string {
	return fmt.Sprintf("%v (%g,%g %gx%g)", r.Kind, r.Rect.X, r.Rect.Y, r.Rect.Width, r.Rect.Height)
}

// HitRegions returns the hit regions the window sent to the OS, in order
// (later regions are on top): the drag areas, the holes in them and the
// window-control buttons that h.WindowDrag, h.WindowNoDrag and
// h.WindowControl mark. It is nil for a window without them.
func (a *App) HitRegions() []HitRegion {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	rs := a.win.HitRegions()
	out := make([]HitRegion, len(rs))
	for i, r := range rs {
		out[i] = HitRegion{Kind: r.Kind, Rect: tela.Rect{X: r.Rect.X, Y: r.Rect.Y, Width: r.Rect.W, Height: r.Rect.H}}
	}
	return out
}

// HitAt returns what the OS would find at the point (x, y), in logical
// pixels from the window's top-left corner: the kind of the last hit region
// that contains it, or [HitClient].
func (a *App) HitAt(x, y float32) HitKind {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	return platform.HitTest(a.win.HitRegions(), platform.Point{X: x, Y: y})
}

// CaptionDrags returns how many presses in a caption region started a move
// of the window by the fake window manager: a primary press that is not
// the second of a double click. A press in a caption region is the OS's: the
// app does not see it or the rest of its pointer sequence. Drive one with
// [App.Click] on an element inside an h.WindowDrag region, or
// [App.ClickAt].
func (a *App) CaptionDrags() int {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	return a.win.CaptionDrags()
}

// WindowMenus returns how many secondary presses in a caption region opened
// the window menu of the fake window manager.
func (a *App) WindowMenus() int {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	return a.win.WindowMenus()
}

// ---------------------------------------------------------------------
// The end of the app.

// HasQuit reports whether the app quit: tela.Quit ran, or the main window
// was asked to close without a tela.App.OnClose. A quit app is over, as
// when tela.Main returns: every window was unmounted, so the cleanups of
// the effects ran, and using the app fails the test. The app is settled
// first.
func (a *App) HasQuit() bool {
	a.tb.Helper()
	if !a.sh.closed {
		a.settleQuery()
	}
	return a.sh.quit
}

// ExpectQuit waits until the app quits (see [App.HasQuit]) and fails the
// test at the timeout. Once the app quit, its windows are unmounted and the
// cleanups of their effects have run, so the test can assert them.
//
//	app.Click(telatest.ByRole("button", "Quit"))   // OnClick(tela.Quit)
//	app.ExpectQuit()
//	if !cleaned { t.Error("the effect did not clean up") }
func (a *App) ExpectQuit() {
	a.tb.Helper()
	a.waitIn(false, a.settleQuery, func() (bool, string) {
		a.tb.Helper()
		switch {
		case a.sh.quit:
			return true, ""
		case a.sh.closed:
			a.tb.Fatalf("telatest: ExpectQuit: the app was closed by App.Close, it did not quit")
		}
		return false, "the app is still running: neither tela.Quit was called nor was the main window closed without an OnClose\n" + a.windowList()
	})
}

// ExpectRunning fails the test when the app quit (see [App.HasQuit]): the
// check of an OnClose that hides the window to keep the app in the tray.
func (a *App) ExpectRunning() {
	a.tb.Helper()
	if !a.sh.closed {
		a.settleQuery()
	}
	if a.sh.quit {
		a.tb.Fatalf("telatest: the app quit, but it was expected to keep running")
	}
	a.mustRun()
}
