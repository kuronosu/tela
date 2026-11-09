package tela

import (
	"image"

	"github.com/kuronosu/tela/internal/core"
)

// Window is a component that opens a window while it is mounted and closes
// it when it unmounts: whether a window is open is state, like whether a
// dialog is shown. It renders nothing where it is declared; its Root is
// the content of the new window.
//
//	func Notes() tela.Node {
//	    settingsOpen, setSettingsOpen := tela.UseState(false)
//	    return Div(
//	        Button(OnClick(func() { setSettingsOpen.Set(true) }), Text("Settings")),
//	        If(settingsOpen, tela.Component(tela.Window{
//	            Title:   "Settings",
//	            Width:   480, Height: 360,
//	            Modal:   true,
//	            OnClose: func() { setSettingsOpen.Set(false) },
//	            Root:    SettingsPanel{},
//	        })),
//	    )
//	}
//
// The window belongs to the window it is declared in: it stays above it,
// and with Modal that window takes no input while this one is visible.
// The app has one tree of components across its windows: Root is built by
// the component that declares the Window, so it sees that component's state
// and props, and the contexts above it reach the new window (UseContext);
// a Value read in the new window rebuilds it when it changes. Each window
// has its own layout, focus, hover, overlays (h.Portal), cursor and
// scale; the theme, the class aliases and the color scheme are the app's.
//
// When the user asks to close the window (its close button, Alt+F4, a
// WindowClose control, [WindowHandle].Close), OnClose runs: it should
// update the state that renders the Window, which unmounts it and closes
// the window. Without OnClose the request does nothing and the window
// stays open (dev mode warns once). To keep the window and its state
// while it is out of sight, set Hidden instead of unmounting it.
//
// The fields are applied when they change: a new Title, Width and Height,
// size limits, AlwaysOnTop, Hidden, Icon, TitleBar, Backdrop or RootClass
// update the open window. FixedSize, Frameless, TitleBarOverlay,
// Transparent, Modal and Placement are read when it opens (give the
// Window a new key to open it again with others).
//
// On a desktop whose compositor draws no title bars (GNOME on Wayland),
// a framed window gets Tela's fallback title bar unless its content has a
// title bar of its own; see [App].
//
// tela snapshot, tela inspect and tela mcp show the main window only;
// in tests, telatest's App.Windows and App.Window reach the others. Under
// tela dev the other windows keep their parent, modality and window
// control, and an open window survives a reload (it is keyed by its place
// in the tree).
type Window struct {
	// Title is the window title.
	Title string
	// Width and Height are the size of the content area in logical
	// pixels. Zero means the platform's default size (800×600); set both
	// or neither.
	Width, Height float32
	// MinWidth, MinHeight, MaxWidth and MaxHeight limit the size the
	// user can give the window; zero is no limit on that side.
	MinWidth, MinHeight, MaxWidth, MaxHeight float32
	// FixedSize keeps the user from resizing the window.
	FixedSize bool
	// Frameless removes the OS title bar and borders: the content draws
	// its own title bar, marked with h.WindowDrag and h.WindowControl.
	// The window stays resizable at its edges (unless FixedSize).
	Frameless bool
	// TitleBarOverlay extends the content under a transparent title bar
	// while the OS keeps its window buttons over it: on macOS the traffic
	// lights, centered in the height of the app's drag area along the
	// top (the top-most h.WindowDrag area touching it, as in
	// kit.TitleBar), or where AppKit puts them without one; in full
	// screen they are in the OS's auto-hiding title bar and the rectangle
	// is empty. The content draws the rest of the title bar (kit.TitleBar
	// leaves room for the buttons: [WindowState].TitleBarControls is the
	// rectangle they cover). On Windows 11 DWM keeps its minimize,
	// maximize and close buttons (with snap layouts) at the end of the
	// bar, at the system's caption height, on the bar's own color
	// (TitleBar.Background when set, else the color the bar has just
	// left of them), and TitleBarControls is their rectangle. Elsewhere
	// it behaves as Frameless, and the
	// rectangle is empty. Set one of Frameless and TitleBarOverlay: the
	// window panics with both.
	TitleBarOverlay bool
	// Transparent makes the window's unpainted pixels show the desktop:
	// the root has no background (unless RootClass gives it one), so the
	// root's content floats over what is below the window with its own
	// background, radius and shadow-*, and clicks on the pixels the frame
	// leaves transparent (or nearly: alpha under a quarter, which the
	// shadows of shadow-* stay under) go to what is below. Make the
	// window larger than the content so its shadow fits, with padding on
	// the root (RootClass: "p-12 flex items-center justify-center").
	// It needs Frameless (the window panics without it); the OS draws no shadow,
	// border or rounded corners for it, and its edges are not resize
	// handles (the margin is transparent), so size it with Width and
	// Height. Where the OS cannot show transparency (X11 without a
	// compositing manager) the window is an opaque Frameless one, which
	// [WindowState].Transparent reports, read through [UseWindow]: an app
	// can give its root a background there. tela dev shows it as well.
	Transparent bool
	// AlwaysOnTop keeps the window above other windows (not on Wayland,
	// which has no way to ask for it).
	AlwaysOnTop bool
	// Modal makes the window modal to the window it is declared in: that
	// window takes no input while this one is visible (a press there
	// brings this one forward).
	Modal bool
	// Hidden hides the window: it stays open, with its state, until
	// Hidden is false again. A hidden window draws nothing, but its
	// components build and its effects run as in a shown one.
	Hidden bool
	// Icon is the window's icon (title bar, task bar, alt-tab), the same
	// picture at several sizes (16 to 256 pixels; the OS picks). Empty is
	// the app's default icon.
	Icon []image.Image
	// TitleBar sets the colors of the native title bar and border on
	// Windows 11; see [TitleBarColors]. The zero value keeps the
	// system's. Other platforms ignore it.
	TitleBar TitleBarColors
	// Backdrop is the system material behind the window (Mica, Mica Alt,
	// Acrylic) on Windows 11 22H2; see [Backdrop]. The zero value is the
	// system's choice. Other platforms ignore it.
	Backdrop Backdrop
	// RootClass are Tailwind classes for the window's root, which fills
	// the window, as App.RootClass for the main window ("bg-background").
	RootClass string
	// Placement is where the window opens: a WindowPlacement saved from
	// [WindowHandle].Placement, so the window comes back where the user
	// left it. Its size, when set, wins over Width and Height. The zero
	// value lets the OS place the window.
	Placement WindowPlacement
	// OnClose runs when the user asks to close the window. Update the
	// state that renders this Window there.
	OnClose func()
	// Root is the content of the window: a struct component value
	// (Root: Settings{}) or a function component made with C.
	Root Component
}

// Build opens the window while the component is mounted (see [Window]).
func (w Window) Build() Node {
	core.UseWindowSpec(w.spec())
	return nil
}

// spec returns the core description of the window: every field of
// core.ChildWindowSpec (TestWindowFieldsReachCore checks none is left
// out).
func (w Window) spec() core.ChildWindowSpec {
	return core.ChildWindowSpec{
		WindowSpec: core.WindowSpec{
			Title: w.Title, Width: w.Width, Height: w.Height,
			MinWidth: w.MinWidth, MinHeight: w.MinHeight, MaxWidth: w.MaxWidth, MaxHeight: w.MaxHeight,
			FixedSize: w.FixedSize, Frameless: w.Frameless, TitleBarOverlay: w.TitleBarOverlay, Transparent: w.Transparent, AlwaysOnTop: w.AlwaysOnTop,
			Hidden: w.Hidden, Icon: w.Icon,
			TitleBar: w.TitleBar, Backdrop: w.Backdrop,
			Placement: w.Placement,
		},
		Modal: w.Modal, RootClass: w.RootClass, OnClose: w.OnClose,
		Root: core.NodeOf(w.Root),
	}
}

// WindowHandle is a window of the app: the one a component renders in
// ([UseWindow]), or the main window given to App.OnClose. It is a small
// comparable value, the same for the window's whole life, with these
// methods:
//
//   - Minimize, Maximize, Unmaximize, ToggleMaximize, SetFullscreen(bool):
//     ask the OS to change the window's state.
//   - Hide and Show: hide the window (it stays open, with its state) and
//     show it again (activated). The app keeps running while its windows
//     are hidden: App.OnClose can hide the main window to keep the app in
//     the tray. A hidden or minimized window draws nothing, but its
//     components build, its effects run and its Windows open as usual (a
//     tray menu declared in it follows its state); it draws the current
//     state when it shows.
//   - Focus: raise the window and give it the keyboard focus, bringing it
//     back from minimized.
//   - Close: ask to close the window as its close button does (its
//     OnClose runs; for the main window App.OnClose, or the app quits).
//   - State() WindowState: maximized, fullscreen, minimized, focused,
//     hidden, whether the app draws the title bar
//     (ClientSideDecorations) and where the OS's window buttons are over
//     it (TitleBarControls). Read in a component's body, it rebuilds the
//     component when the state changes.
//   - Placement() WindowPlacement: where the window is, to save and give
//     back as App.Placement or Window.Placement next time.
//   - IsZero() bool: the zero handle (no window).
//
// The requests are safe from any goroutine: they are made on the UI
// thread, in order with the state updates. The OS may refuse one (a tiling
// window manager does not maximize); State says what happened. The handle
// of a closed window, or one in tela snapshot (no window), does nothing.
type WindowHandle = core.WindowHandle

// WindowState is the state of a window ([WindowHandle].State):
// Maximized, Fullscreen and Minimized as the OS reports them (Wayland
// cannot report minimized windows), Focused (the active window), Hidden
// (the app hid it), ClientSideDecorations (the OS draws no title bar:
// the app draws one, as a frameless or TitleBarOverlay window does, and
// so on a Wayland desktop without server-side decorations, such as GNOME;
// there Tela draws a plain one for a framed window that has none of its
// own, see [App]) and TitleBarControls (the [Rect] the OS's window buttons
// cover in a TitleBarOverlay window, in the window's logical pixels:
// macOS's traffic lights, which the app's title bar leaves free; the zero
// Rect elsewhere) and Transparent (a Transparent window whose OS shows its
// transparent pixels: false where it cannot, as on X11 without a
// compositing manager, and the window is then opaque).
type WindowState = core.WindowState

// WindowPlacement is where a window is: Width and Height (its logical
// size outside maximized and fullscreen), X and Y (the position in
// physical pixels of the desktop, when HasPosition: never on Wayland),
// Maximized, Fullscreen and RestoreToken (the OS's session restore token;
// on Wayland it lets the compositor put the window back where supported).
// Save it ([WindowHandle].Placement) and give it back as App.Placement or
// Window.Placement: the window opens where the user left it. The zero
// value means nothing is known.
type WindowPlacement = core.WindowPlacement

// WindowControl is what a window-control button of a custom title bar
// does, given to h.WindowControl.
type WindowControl = core.WindowControl

const (
	// WindowMinimize minimizes the window.
	WindowMinimize = core.WindowMinimize
	// WindowMaximize maximizes the window, or restores it when it is
	// maximized.
	WindowMaximize = core.WindowMaximize
	// WindowClose asks to close the window, as its close button does.
	WindowClose = core.WindowClose
)

// UseWindow returns the handle of the window the component renders in:
// the main window, or the [Window] it is inside of.
//
//	func titleBar() tela.Node {
//	    win := tela.UseWindow()
//	    maximize := "Maximize"
//	    if win.State().Maximized { // the component rebuilds when it changes
//	        maximize = "Restore"
//	    }
//	    return Div(Class("flex h-9 items-center"), WindowDrag(),
//	        Span(Class("grow px-3 text-sm"), Text("Notes")),
//	        Button(WindowControl(tela.WindowMinimize), AriaLabel("Minimize"), Text("–")),
//	        Button(WindowControl(tela.WindowMaximize), AriaLabel(maximize), Text("□")),
//	        Button(WindowControl(tela.WindowClose), AriaLabel("Close"), Text("×")),
//	    )
//	}
func UseWindow() WindowHandle { return core.UseWindow() }

// CloseRequestEvent is the user asking to close a window, given to
// [UseCloseRequest]: Window is the window, PreventDefault keeps its
// default (App.OnClose or quitting for the main window, the Window's
// OnClose for another) from running, DefaultPrevented tells whether a
// handler called it.
type CloseRequestEvent = core.CloseRequestEvent

// QuitRequestEvent is the user or the OS asking the app to quit, given to
// [UseQuitRequest]: PreventDefault keeps the default (App.OnQuit, else a
// close request to every window) from running.
type QuitRequestEvent = core.QuitRequestEvent

// UseCloseRequest calls f each time the user asks to close the window the
// component renders in: its close button, Alt+F4, a WindowClose control,
// [WindowHandle].Close (Tauri's onCloseRequested, Electron's close event).
// It is App.OnClose inside the component tree, where the state that
// decides lives: call e.PreventDefault and the window's default does not
// run (for the main window App.OnClose, else the app quits; for a
// [Window] its OnClose). Every mounted handler of the window runs, in the
// order they mounted.
//
//	tela.UseCloseRequest(func(e tela.CloseRequestEvent) {
//	    if keepInTray {
//	        e.PreventDefault()
//	        e.Window.Hide()
//	    } else if unsaved > 0 {
//	        e.PreventDefault()
//	        askThenQuit() // tela.Quit() once the user answers
//	    }
//	})
//
// To end after asking, call [Quit] (main window) or drop the Window from
// the state (another window): [WindowHandle].Close would ask again.
func UseCloseRequest(f func(CloseRequestEvent)) { core.UseCloseRequest(f) }

// UseQuitRequest calls f each time the user or the OS asks the app to
// quit: on macOS Cmd+Q, the app menu's and the Dock's Quit, a logout
// (Electron's before-quit). It is App.OnQuit inside the component tree:
// call e.PreventDefault and the default (App.OnQuit, else a close request
// to every window) does not run; the handler quits later with [Quit].
// Every mounted handler of the app runs, in the order they mounted.
//
//	tela.UseQuitRequest(func(e tela.QuitRequestEvent) {
//	    e.PreventDefault()
//	    askThenQuit() // quits for real, also with "keep running in the tray" on
//	})
func UseQuitRequest(f func(QuitRequestEvent)) { core.UseQuitRequest(f) }

// Quit quits the app: Run returns (and Main with it) after unmounting
// every window, which runs the cleanups of their effects. It is safe from
// any goroutine and is applied in order with the state updates queued
// before it.
func Quit() { core.Quit() }

// OpenURL opens url in the user's default application for its scheme: a
// web page in the browser, a mailto: address in the mail client. The URL
// must have a scheme ("https://example.com"); open local files and
// folders with [OpenPath]. The error reports a URL that cannot be opened
// or a desktop without the service (headless). Call it on the UI thread
// (a handler, an effect); from another goroutine, wrap it in [Post].
func OpenURL(url string) error { return core.OpenURL(url) }

// OpenPath opens a local file in its default application, or a folder in
// the file manager. Call it on the UI thread (a handler, an effect); from
// another goroutine, wrap it in [Post].
func OpenPath(path string) error { return core.OpenPath(path) }
