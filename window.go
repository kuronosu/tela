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
// size limits, AlwaysOnTop, Hidden, Icon or RootClass update the open
// window. FixedSize, Frameless, Modal and Placement are read when it opens
// (give the Window a new key to open it again with others).
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
	// AlwaysOnTop keeps the window above other windows (not on Wayland,
	// which has no way to ask for it).
	AlwaysOnTop bool
	// Modal makes the window modal to the window it is declared in: that
	// window takes no input while this one is visible (a press there
	// brings this one forward).
	Modal bool
	// Hidden hides the window: it stays open, with its state, until
	// Hidden is false again.
	Hidden bool
	// Icon is the window's icon (title bar, task bar, alt-tab), the same
	// picture at several sizes (16 to 256 pixels; the OS picks). Empty is
	// the app's default icon.
	Icon []image.Image
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
	core.UseWindowSpec(core.WindowSpec{
		Title: w.Title, Width: w.Width, Height: w.Height,
		MinWidth: w.MinWidth, MinHeight: w.MinHeight, MaxWidth: w.MaxWidth, MaxHeight: w.MaxHeight,
		FixedSize: w.FixedSize, Frameless: w.Frameless, AlwaysOnTop: w.AlwaysOnTop,
		Modal: w.Modal, Hidden: w.Hidden, Icon: w.Icon, RootClass: w.RootClass,
		Placement: w.Placement, OnClose: w.OnClose,
		Root: core.NodeOf(w.Root),
	})
	return Node{}
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
//     the tray.
//   - Focus: raise the window and give it the keyboard focus, bringing it
//     back from minimized.
//   - Close: ask to close the window as its close button does (its
//     OnClose runs; for the main window App.OnClose, or the app quits).
//   - State() WindowState: maximized, fullscreen, minimized, focused,
//     hidden and whether the app draws the title bar
//     (ClientSideDecorations). Read in a component's body, it rebuilds the
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
// (the app hid it) and ClientSideDecorations (the OS draws no title bar:
// the app draws one, as a frameless window does, and so on a Wayland
// desktop without server-side decorations).
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
