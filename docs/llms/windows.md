# Recipe: windows and desktop services

Questions: how to open a second window, close or hide one, draw a custom title bar, make a transparent window or use a backdrop; file dialogs, message boxes, notifications, the tray, settings kept on disk, app folders; how to test them.

A window is state, like a dialog: a `tela.Window{...}` in the tree opens a window while it is mounted and closes it when it unmounts. All windows share one component tree, so a child window sees the state and contexts of the component that declares it (its `Root` is built by that component). Each window has its own layout, focus, hover and overlays. OS services (file dialogs, message boxes, notifications, tray, app folders, settings) are in package `github.com/kuronosu/tela/desktop`.

- `tela.Window{Title, Width, Height, MinWidth..MaxHeight, FixedSize, Frameless, AlwaysOnTop, Modal, Hidden, Icon, RootClass, Placement, OnClose, Root}`. Keep "is it open" in `UseState` and render `If(open, tela.Window{...})`. `OnClose` runs when the user asks to close (close button, Alt+F4, `Close()`): set the state to false there. **A nil `OnClose` ignores the request**: the window cannot be closed (dev mode warns). `FixedSize`, `Frameless`, `Modal` and `Placement` are read when the window opens; `Hidden` keeps it open, with its state. `tela.App{OnQuit}` runs on Cmd+Q, the Dock or logout (nil: every window gets a close request; a tray app sets `OnQuit: tela.Quit`). A hidden or minimized window draws nothing but still builds and runs its effects, so a tray menu in it can follow `State().Hidden` ("Show"/"Hide"). On `tela.App` and `tela.Window`: `TitleBar: tela.TitleBarColors{Background, Text, Border}` colors Windows 11's title bar and border (zero = the system's); `Backdrop: tela.BackdropMica` (`BackdropMicaAlt`, `BackdropAcrylic`, `BackdropNone`) is the system material (Windows 11, macOS), shown where the root has no background (`RootClass` without `bg-background`, or `bg-background/60`); on a transparent window it is also a blur behind the card on Linux (below).
- `tela.App` has the same fields for the main window, plus `AppID` and `OnClose func(tela.WindowHandle)`: nil quits when the main window closes; `func(w tela.WindowHandle) { w.Hide() }` keeps the app running (with a `desktop.Tray` as the way back). `tela.Quit()` ends the app from any goroutine; `tela.OpenURL(url)` and `tela.OpenPath(path)` open the default application.
- `tela.UseWindow()` returns the `tela.WindowHandle` of the window the component renders in: `Minimize`, `Maximize`, `Unmaximize`, `ToggleMaximize`, `SetFullscreen(bool)`, `Hide`, `Show`, `Focus`, `Close`, `State()` (`Maximized`, `Fullscreen`, `Minimized`, `Focused`, `Hidden`, `ClientSideDecorations`; reading it rebuilds the component when it changes) and `Placement()`. Requests are safe from any goroutine; the OS may refuse one (a tiling window manager does not maximize). In `tela snapshot` there is no window and the handle does nothing.
- **Custom title bar:** `Frameless: true`, and `kit.TitleBar{Title: Text("Notes"), Leading, Children, Trailing}` at the top of a flex column: it is the drag area and draws minimize, maximize and close (`kit.WindowControls`) in the OS's look; buttons and fields in its slots stay clickable. Without `kit`, mark the bar `h.WindowDrag()`, a plain area inside it that must not drag `h.WindowNoDrag()`, and real buttons `h.WindowControl(tela.WindowMinimize|WindowMaximize|WindowClose)` (with an `AriaLabel`); the OS does the move, the double click and the window menu. Without server-side decorations (GNOME on Wayland: `State().ClientSideDecorations`) Tela draws a fallback bar above the content; any `h.WindowDrag` area replaces it. `TitleBarOverlay: true` (App or Window) keeps the OS's buttons over the bar instead (macOS traffic lights; elsewhere it is `Frameless`): `kit.TitleBar` keeps room for `State().TitleBarControls` and draws no controls.

- **Transparent windows:** `Transparent: true` with `Frameless: true` (on `tela.App` and `tela.Window`; `Run` panics without `Frameless`) shows the desktop through every pixel nothing paints: the root has no background (no `bg-background` in `RootClass`), so its first element floats with its own `rounded-*` and `shadow-*`. Pad the root so the shadow fits (`RootClass: "flex items-center justify-center p-12"`), size the window with `Width`/`Height`, and let the user resize it with `h.WindowResize` on the card (the transparent margin is not a handle). The OS draws no frame, shadow or corners, and a click on a pixel that is nearly transparent goes to the window below. Where the OS cannot show it (X11 without a compositing manager) the window is opaque and `tela.UseWindow().State().Transparent` is false: give the root a background then. `Backdrop` on a transparent window is a material behind the card's shape: vibrancy on macOS and a blur on Linux compositors that offer one (KWin, Wayland compositors with `ext-background-effect-v1`); nothing elsewhere, Windows included. In tests `telatest.Transparent()` keeps the alpha in goldens and snapshots, and `telatest.NoCompositing()` plays the opaque fallback.

```go
package float

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

// App is a card that floats on the desktop, with a blurred backdrop on macOS and some Linux compositors.
var App = tela.App{
	AppID: "com.example.float", Title: "Float", Width: 560, Height: 400,
	Frameless: true, Transparent: true, Backdrop: tela.BackdropAcrylic,
	Theme: &theme, RootClass: "flex items-center justify-center p-12 text-foreground",
	Root: Card{},
}

type Card struct{}

func (Card) Build() tela.Node {
	win := tela.UseWindow()
	bg := "bg-background"
	if win.State().Transparent {
		bg = "bg-background/70" // let the backdrop show through the card
	}
	// WindowResize on the card makes its edges the window's resize handles.
	return Div(WindowResize(), Class("flex size-full flex-col gap-3 rounded-2xl border border-border p-6 shadow-2xl", bg),
		Div(Class("h-8"), WindowDrag(), Text("Drag me")),
		kit.Button{Children: Text("Close"), OnClick: win.Close})
}
```

```go
package notes

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

// App: PersistApp reopens the main window where the user left it.
var App = desktop.PersistApp(tela.App{
	AppID: "com.example.notes", Title: "Notes", Width: 900, Height: 640,
	Frameless: true, Theme: &theme, RootClass: "bg-background text-foreground",
	Root: Notes{},
})

type Notes struct{}

func (Notes) Build() tela.Node {
	prefs, setPrefs := tela.UseState(false)
	return Div(Class("flex h-screen flex-col"),
		kit.TitleBar{Title: Text("Notes")},
		Div(Class("min-h-0 grow p-4"),
			kit.Button{Variant: kit.Outline, Children: Text("Preferences"),
				OnClick: func() { setPrefs.Set(true) }},
			If(prefs, tela.Window{
				Title: "Preferences", Width: 420, Height: 240, Modal: true,
				OnClose: func() { setPrefs.Set(false) },
				Root:    Preferences{},
			}),
		))
}

// Preferences is the content of the second window.
type Preferences struct{}

func (Preferences) Build() tela.Node {
	win := tela.UseWindow()
	dark, setDark := desktop.UseSetting("dark", false) // kept in the app's settings file
	return Div(Class("flex flex-col gap-4 p-6"),
		kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Dark mode",
			Children: kit.Switch{Checked: &dark, OnCheckedChange: setDark.Set}},
		kit.Button{Children: Text("Done"), OnClick: win.Close})
}
```

**Desktop services** (`desktop`). Dialogs and notifications are asynchronous like a browser's: the call returns at once and the callback runs later **on the UI thread**, so it may set state. Call them from handlers and effects (from a goroutine, wrap the call in `tela.Post`; dev mode panics otherwise). Each returns an error when nothing was shown; it wraps `desktop.ErrUnsupported` when the desktop lacks the service (no XDG portal, no notification daemon, no tray host), and the app should fall back. In `tela snapshot` (no running app) they return an error.

- File dialogs: `desktop.OpenFile`, `OpenFiles`, `SaveFile`, `PickFolder(done, opts...)` with options `Title`, `Directory`, `DefaultName`, `Filter("Notes", "*.md", "*.txt")`, `Parent(win)`. `done(path, err)` gets `""` and a nil error when the user cancels. The dialog is modal to the window the call comes from.
- `desktop.ShowMessage(desktop.Message{Kind, Title, Text, Buttons, Parent}, func(desktop.Answer))`: kinds `MessageInfo/Warning/Error/Question`, buttons `ButtonsOK/OKCancel/YesNo/YesNoCancel`, answers `AnswerOK/Cancel/Yes/No`. Native on Windows; on Linux Tela draws it as a modal window. Escape or the close button answers Cancel (No for Yes/No).
- `desktop.Notify(desktop.Notification{Title, Body, Icon, Actions, OnClick, OnClose})` returns the shown notification (`Close()`). Windows shows no action buttons: keep every action reachable in the app. `Title` is required.
- `desktop.Tray{Icon, Tooltip, Menu, OnClick, OnError}` is a component: the icon is in the tray while it is mounted (`Icon` is one picture at several sizes, 16, 24 and 32 px; `Menu` is `[]desktop.MenuItem{Label, Separator, Checkbox, Checked, Disabled, Submenu, OnSelect}`; a checkbox item does not toggle itself). On GNOME without the AppIndicator extension `OnError` gets an `ErrUnsupported`: then do not hide the window on close.
- `desktop.UseSetting(key, def)` is `UseState` whose value lives in the app's JSON settings file (`desktop.Settings()` is the `*desktop.Store`: `Get`, `Set`, `Watch`). Values must encode as JSON; `def` is used only while the key has no stored value (or it no longer decodes); a stored value replaces it, except that a struct field the stored value lacks keeps `def`'s, while slice, array and map elements always start empty (an `omitempty` field an element omits is zero, not `def`'s).
- `desktop.PersistApp(app)` and `desktop.SavedWindow{Key, Window}` save and restore a window's size, maximized and fullscreen state and position. Wayland cannot place windows: only the size and state come back, and the position where the compositor restores sessions.
- Folders: `desktop.ConfigDir()`, `DataDir()`, `CacheDir()`, `StateDir()`, `LogDir()` (created on demand, named by `App.AppID` or `desktop.SetAppID("com.example.notes")`, called before the first read).

```go
package notes

import (
	"errors"
	"image"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

var trayIcon = []image.Image{image.NewRGBA(image.Rect(0, 0, 32, 32))} // the app's icon at 16, 24 and 32 px (decode embedded PNGs)

// Editor opens a file, asks before closing and lives in the tray.
func Editor() tela.Node {
	return tela.C(func() tela.Node {
		win := tela.UseWindow()
		path, setPath := tela.UseState("")
		typing, setTyping := tela.UseState(false) // fallback: a field for the path
		open := func() {
			err := desktop.OpenFile(func(p string, err error) {
				if err == nil && p != "" { // "" is cancel
					setPath.Set(p)
				}
			}, desktop.Title("Open a note"), desktop.Filter("Notes", "*.md", "*.txt"))
			if errors.Is(err, desktop.ErrUnsupported) {
				setTyping.Set(true) // no portal in this session
			}
		}
		closeWindow := func() {
			desktop.ShowMessage(desktop.Message{
				Kind: desktop.MessageQuestion, Title: "Unsaved changes",
				Text: "Save the changes before closing?", Buttons: desktop.ButtonsYesNoCancel,
			}, func(a desktop.Answer) {
				if a != desktop.AnswerCancel {
					win.Hide() // the tray brings it back
				}
			})
		}
		return Div(
			desktop.Tray{
				Icon: trayIcon, Tooltip: "Notes", OnClick: win.Show,
				Menu: []desktop.MenuItem{
					{Label: "Show Notes", OnSelect: win.Show},
					{Separator: true},
					{Label: "Quit", OnSelect: tela.Quit},
				},
			},
			Button(OnClick(open), Text("Open…")),
			Button(OnClick(closeWindow), Text("Close")),
			If(typing, Input(Bind(path, setPath))),
			Text(path),
		)
	})
}
```

**Windows in tests.** `app.Windows()`, `app.Window("Settings")` and `app.WaitForWindow(func() { app.Click(open) })` give an `*App` scoped to one window, with the whole `telatest` API; also `RequestClose()` (the close button), `Maximize()`, `Minimize()`, `ExpectWindowState(telatest.Maximized(true))` (`Fullscreen`, `Minimized`, `Hidden`, `Active`, `ClientDecorations`), `ExpectTitle`, `ExpectWindowCount(n)`, `ExpectClosed()`, `ExpectQuit()` and `ExpectRunning()` (an `OnClose` that hides the window). `telatest.MountApp` applies the window fields of the `tela.App` (`Frameless`, `OnClose`, size limits...); for a custom title bar, `HitRegions()`, `HitAt(x, y)` and `CaptionDrags()` show what the OS would do.

**Desktop services in tests.** The headless desktop shows nothing: it records dialogs, notifications and tray icons, and the test answers them, as the user would. Call `telatest.TempDesktopDirs(t)` first in a test whose app uses `desktop.UseSetting`, `PersistApp` or the app folders, or it reads and writes the real ones. Nothing answers by itself.

```go
package notes

import (
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

func TestEditorServices(t *testing.T) {
	telatest.TempDesktopDirs(t) // settings and window placements go to a temporary folder
	app := telatest.Mount(t, Editor())

	app.Click(telatest.ByRole("button", "Open…"))
	app.AnswerFileDialog("/home/ana/ideas.md") // no paths: the user cancels
	app.ExpectText("/home/ana/ideas.md")

	app.Click(telatest.ByRole("button", "Close"))
	app.AnswerMessage(desktop.AnswerYes) // native or drawn box: the same call
	app.ExpectWindowState(telatest.Hidden(true))
	app.ExpectRunning()

	app.ClickTrayItem("Show Notes") // also ClickTray(), Tray(), Trays()
	app.ExpectWindowState(telatest.Hidden(false))

	app.Unsupported(telatest.FileDialogs) // from now on OpenFile returns ErrUnsupported
	app.Click(telatest.ByRole("button", "Open…"))
	app.ExpectRole("textbox", "") // the fallback path field
}
```

Also `app.Notifications()`, `ClickNotification(title)`, `ClickNotificationAction(title, label)`, `DismissNotification(title)` and `MessageBoxes()` / `FileDialogs()` to check what was asked. `Unsupported` takes `telatest.FileDialogs`, `MessageBoxes` (the box is drawn then), `Notifications`, `TrayIcons` and `OpenURLs`; with no arguments everything is supported again.
