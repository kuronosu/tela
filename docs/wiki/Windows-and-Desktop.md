# Windows and desktop services

An app is more than one window: a preferences window, a tool palette, a
message box, an icon in the tray, a title bar of its own, a file dialog, a
setting that survives a restart. Tela models the windows the way it models
everything else, as state in the component tree, and puts the services of the
operating system in a package of their own,
[`desktop`](API-Reference-desktop), that you import only when you need it.

For a complete app that uses all of this (a frameless main window with a title
bar, a Preferences window, a tray icon, open and save dialogs and a window that
reopens where it was), see
[`examples/notes`](https://github.com/kuronosu/tela/tree/main/examples/notes).

- [Several windows](#several-windows)
- [The window handle and its state](#the-window-handle-and-its-state)
- [Quitting, hiding and opening things](#quitting-hiding-and-opening-things)
- [Custom title bars](#custom-title-bars)
- [Dialogs](#dialogs)
- [Message boxes](#message-boxes)
- [Notifications](#notifications)
- [The tray](#the-tray)
- [Settings](#settings)
- [Remembering where windows were](#remembering-where-windows-were)
- [Folders of the app](#folders-of-the-app)
- [When a service is missing](#when-a-service-is-missing)
- [Platform notes](#platform-notes)
- [Testing](#testing)

## Several windows

A [`tela.Window`](API-Reference-tela#type-window) is a component. It opens a
window while it is mounted and closes it when it unmounts, so whether a window
exists is state, like whether a dialog is shown (this is the model of Flutter's
multi-view). It draws nothing where it is declared; its `Root` is the content
of the new window.

```go
func Notes() tela.Node {
	prefsOpen, setPrefsOpen := tela.UseState(false)
	return Div(Class("p-4"),
		kit.Button{Children: Text("Preferences"), OnClick: func() { setPrefsOpen.Set(true) }},
		If(prefsOpen, tela.Window{
			Title: "Preferences", Width: 420, Height: 320,
			Modal:   true,
			OnClose: func() { setPrefsOpen.Set(false) },
			Root:    Preferences{},
		}),
	)
}
```

All windows are **one tree**. `Root` is built by the component that declares the
`Window`, so it sees that component's state and props, the contexts above it
reach the new window (`UseContext`, the theme, the locale), and a `Value` read
there rebuilds it. Each window has its own layout, focus, hover, overlays
(`h.Portal`, menus, tooltips), cursor and scale.

- **Closing.** When the user asks to close the window (its close button,
  Alt+F4, a window control, `Close()`), `OnClose` runs. It should update the
  state that renders the `Window`, which unmounts it. **A `Window` without
  `OnClose` ignores the request** and stays open (dev mode warns when it opens),
  because the state owns whether the window exists, like a controlled input.
- **Hiding.** To keep a window and its state while it is out of sight, set
  `Hidden: true` instead of unmounting it.
- **Modal.** `Modal: true` makes the window modal to the window it is declared
  in: that one takes no input while this one is visible, a press on it brings
  the modal window forward, and its close button is refused.
- **Applied on change:** `Title`, `Width` and `Height`, the size limits,
  `AlwaysOnTop`, `Hidden`, `Icon`, `TitleBar`, `Backdrop` and `RootClass`. **Read when the window
  opens:** `FixedSize`, `Frameless`, `Modal` and `Placement`; give the `Window`
  a new key to open it again with others.
- **The main window** is the `tela.App`'s. It has the same window fields
  (`MinWidth`...`MaxHeight`, `FixedSize`, `Frameless`, `AlwaysOnTop`, `Hidden`,
  `Icon`, `TitleBar`, `Backdrop`, `Placement`, `AppID`) and an
  `OnClose func(tela.WindowHandle)`.

## Windows 11 look

On Windows 11 a window can take the colors of the system's own chrome. Both
fields exist on `tela.App` and on `tela.Window`, and Windows 10, Linux and the
headless backend ignore them.

- `TitleBar: tela.TitleBarColors{Background, Text, Border}` colors the native
  title bar and the window border (Windows 11, build 22000 and later). A zero
  `Color` keeps the system's. The dark or light title bar already follows the
  app's color scheme; these colors go on top of it. The border also applies to
  a `Frameless` window.
- `Backdrop` is `BackdropMica`, `BackdropMicaAlt`, `BackdropAcrylic`,
  `BackdropNone` or the zero `BackdropDefault` (Windows 11 22H2, build 22621).
  The material always shows in the native title bar. It shows in the content
  only where the app draws nothing: leave the root without a background
  (`RootClass: "text-foreground"`, not `bg-background`) or give it some
  transparency (`bg-background/60`), and the surfaces you draw over it cover it.
  Where the backdrop is missing (Windows 10, Linux) a transparent root is a
  white window, so keep a fallback background such as `bg-background/60`.

```go
tela.Run(tela.App{
    Title:     "Notes",
    TitleBar:  tela.TitleBarColors{Background: tela.Hex("#1e1b4b"), Text: tela.Hex("#e0e7ff")},
    Backdrop:  tela.BackdropMica,
    RootClass: "bg-background/60 text-foreground",
    Root:      Notes{},
})
```

A `Frameless` window with a client-area backdrop extends the window frame over
its content, and Windows 11 may then stop showing the snap layouts flyout over
the app's maximize button. Keep a frameless window without a backdrop if you
need the flyout. Under `tela dev` the title
bar colors and the backdrop of the title bar apply, but the content is opaque.

## The window handle and its state

[`tela.UseWindow()`](API-Reference-tela#func-usewindow) returns a
[`tela.WindowHandle`](API-Reference-tela#type-windowhandle) for the window the
component renders in (the main window, or the `tela.Window` it is inside of). It
is a small comparable value, the same for the window's whole life.

| Method | Does |
|---|---|
| `Minimize`, `Maximize`, `Unmaximize`, `ToggleMaximize`, `SetFullscreen(bool)` | Ask the OS to change the state |
| `Hide`, `Show` | Hide the window (it stays open, with its state) and show it again, activated |
| `Focus` | Raise the window and give it the keyboard focus, bringing it back from minimized |
| `Close` | Ask to close it, as its close button does: `OnClose` runs |
| `State()` | `Maximized`, `Fullscreen`, `Minimized`, `Focused`, `Hidden` and `ClientSideDecorations`; reading it in a component rebuilds it when the state changes |
| `Placement()` | Where the window is: size, position, maximized, fullscreen, a restore token |

```go
func MaximizeButton() tela.Node {
	win := tela.UseWindow()
	label := "Maximize"
	if win.State().Maximized { // the component rebuilds when it changes
		label = "Restore"
	}
	return Button(OnClick(win.ToggleMaximize), Text(label))
}
```

The requests are made on the UI thread in order with state updates and are safe
from any goroutine. They are requests: a tiling window manager does not
maximize, Wayland cannot report a minimized window. `State()` says what
happened. The handle of a closed window does nothing, and so does the handle in
`tela snapshot` (there is no window).

## Quitting, hiding and opening things

- `tela.Quit()` ends the app from any goroutine: `tela.Run` returns after
  unmounting every window, which runs the cleanups of the effects.
- When the main window is asked to close, `App.OnClose` decides. Nil quits;
  hiding the window keeps the app running (close to tray):

  ```go
  tela.App{
  	Title: "Notes", Root: Notes(),
  	OnClose: func(w tela.WindowHandle) { w.Hide() }, // keep running; the tray brings it back
  }
  ```

  Only do this when something can show the window again, a
  [tray icon](#the-tray) or a notification. See
  [When a service is missing](#when-a-service-is-missing).
- `tela.OpenURL("https://example.com")` opens a URL in the default application
  for its scheme (a web page, a `mailto:` address) and `tela.OpenPath(path)`
  opens a file in its default application or a folder in the file manager. Both
  return an error, and run on the UI thread (from a goroutine, wrap them in
  `tela.Post`).

## Custom title bars

Set `Frameless: true` on the `tela.App` or the `tela.Window` and the OS draws
no title bar or borders; the window is still resizable at its edges (unless
`FixedSize`). The content draws its own bar. On a Wayland desktop whose
compositor draws no title bars (GNOME), every window is in this situation and
`State().ClientSideDecorations` is true.

**With `kit`:** [`kit.TitleBar`](Kit#title-bars) is the whole bar, with
[`kit.WindowControls`](Kit#title-bars) (minimize, maximize or restore, close) in
the look of the OS:

```go
func Notes() tela.Node {
	return Div(Class("flex h-screen flex-col"),
		kit.TitleBar{
			Title:    Text("Notes"),
			Leading:  kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: lucide.Menu, AriaLabel: "Menu"},
			Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: lucide.Settings, AriaLabel: "Settings"},
		},
		Div(Class("min-h-0 grow"), content),
	)
}
```

The bar is a drag area: the OS moves the window when it is dragged, maximizes
it on a double click and opens the window menu on a right click. Buttons,
fields and other interactive elements inside it stay clickable.

**Without `kit`:** build the bar from three `h` props. The kit components do
exactly this, so use them unless you want another design.

| Prop | Does |
|---|---|
| `h.WindowDrag()` | Marks a drag area. Interactive descendants (buttons, links, fields, anything with a press handler) are holes in it by themselves |
| `h.WindowNoDrag()` | Leaves a plain area out of the drag area around it |
| `h.WindowControl(tela.WindowMinimize)` (also `WindowMaximize`, `WindowClose`) | Makes a button a window control: a click acts with no handler, and Windows 11 shows its snap layouts over the maximize button |

```go
Div(Class("flex h-10 items-center"), WindowDrag(),
	Span(Class("grow px-3 text-sm"), Text("Notes")),
	Button(WindowControl(tela.WindowMinimize), AriaLabel("Minimize"), Text("–")),
	Button(WindowControl(tela.WindowMaximize), AriaLabel("Maximize"), Text("□")),
	Button(WindowControl(tela.WindowClose), AriaLabel("Close"), Text("×")),
)
```

Give the buttons an `AriaLabel` (and a real `Button`, so the keyboard works).
The OS answers these regions natively: `WM_NCHITTEST` on Windows,
`xdg_toplevel.move`, `resize` and `show_window_menu` on Wayland,
`_NET_WM_MOVERESIZE` on X11.

To show the bar only where the OS draws none, test the state:
`If(tela.UseWindow().State().ClientSideDecorations, kit.TitleBar{...})`. A
fullscreen window usually has no bar either.

## Dialogs

`desktop.OpenFile`, `OpenFiles`, `SaveFile` and `PickFolder` show the OS's
dialogs. They are asynchronous like a browser's APIs: the call returns at once
and `done` runs once, later, **on the UI thread**, so it may set state.

```go
open := func() {
	err := desktop.OpenFile(func(path string, err error) {
		if err == nil && path != "" { // "" is cancel: not an error
			setPath.Set(path)
		}
	}, desktop.Title("Open a note"), desktop.Filter("Notes", "*.md", "*.txt"))
	if errors.Is(err, desktop.ErrUnsupported) {
		setTyping.Set(true) // no portal in this session: let the user type the path
	}
}
```

| Option | Does |
|---|---|
| `desktop.Title(s)` | The dialog's title |
| `desktop.Directory(dir)` | The folder it starts in |
| `desktop.DefaultName(name)` | The suggested name of a save dialog |
| `desktop.Filter(name, patterns...)`, `desktop.Filters(...)` | File types, as `"*.md"` patterns |
| `desktop.Parent(win)` | The window the dialog is modal to. By default it is the window the call comes from (the one whose handler or effect runs), else the focused one, else the main window |

The returned error says the dialog could not be shown (and then `done` never
runs): no running app, or a desktop without the service. Cancel is not an error:
`done` gets `""` (or nil paths) and a nil error. `SaveFile` returns a path and
the OS has asked about overwriting; the app writes the file. Call these from
handlers and effects; from a goroutine, wrap the call in `tela.Post`.

## Message boxes

`desktop.ShowMessage` shows a message box with a fixed set of buttons and calls
back with the one the user chose.

```go
desktop.ShowMessage(desktop.Message{
	Kind:    desktop.MessageQuestion, // Info, Warning, Error, Question
	Title:   "Unsaved changes",
	Text:    "Save the changes before closing?",
	Buttons: desktop.ButtonsYesNoCancel, // OK, OKCancel, YesNo, YesNoCancel
}, func(a desktop.Answer) {
	if a != desktop.AnswerCancel {
		win.Close()
	}
})
```

Answers are `AnswerOK`, `AnswerCancel`, `AnswerYes` and `AnswerNo`. Escape or the
close button answers `Buttons.DismissAnswer()` (Cancel when the set has it, No for
Yes/No, OK for OK) and Enter chooses `Buttons.DefaultAnswer()`.

On Windows the box is the OS's. Linux has no portal for message boxes, so Tela
draws it: a modal window of its own with the app's theme, in the look of shadcn's
alert dialog (the GNOME order of the buttons). The drawn box's button labels are in
English only. `desktop.MessagePreview` renders the drawn box in a test or a
gallery. For a message in the middle of your UI, use `kit.AlertDialog`; a message
box is for the moments the OS's own would be used.

## Notifications

```go
win := tela.UseWindow()
// ...
_, err := desktop.Notify(desktop.Notification{
	Title:   "Export finished", // required
	Body:    "notes.pdf was saved in Documents.",
	OnClick: func() { win.Show() },
	Actions: []desktop.NotificationAction{
		{Label: "Open", OnClick: func() { tela.OpenPath(pdf) }},
	},
	OnClose: func() { /* dismissed or expired */ },
})
if err != nil {
	// no notification service: show the news in the window instead
}
```

The callbacks run on the UI thread. `Notify` returns the `ShownNotification`
(`Close()` withdraws it; its callbacks are not called any more). **Windows shows
no action buttons** (its notifications are balloons for now), so keep every action
reachable from the app too; `OnClick` and `OnClose` work.

## The tray

`desktop.Tray` is a component: the icon is in the tray while it is mounted,
follows its fields and goes away when it unmounts. Put it anywhere in the tree.

```go
func (n Notes) Build() tela.Node {
	win := tela.UseWindow()
	muted, setMuted := tela.UseState(false)
	return Div(
		desktop.Tray{
			Icon:    appIcon, // []image.Image: the same picture at 16, 24 and 32 px
			Tooltip: "Notes",
			OnClick: func() { win.Show() },
			OnError: func(err error) { /* the tray is unavailable */ },
			Menu: []desktop.MenuItem{
				{Label: "Show Notes", OnSelect: func() { win.Show() }},
				{Label: "Mute reminders", Checkbox: true, Checked: muted,
					OnSelect: func() { setMuted.Set(!muted) }}, // the backend never toggles it
				{Separator: true},
				{Label: "Quit", OnSelect: tela.Quit},
			},
		},
		// ...the window's content
	)
}
```

`Icon` is required (dev mode panics without one). `MenuItem` has `Label`,
`Separator`, `Checkbox`/`Checked`, `Disabled`, `Submenu` and `OnSelect`. Items
have no IDs: tests name them by label. Some Linux panels show the menu on any click
and never report `OnClick`. With `App.OnClose` hiding the main window, the app
lives in the tray until `tela.Quit`.

## Settings

`desktop.UseSetting(key, default)` is `tela.UseState` whose value lives in the
app's settings file. It returns the value and a real `tela.Setter`; the setting
is saved when you set it and changes made elsewhere (another component with the
same key, `desktop.Settings().Set`) update every reader.

```go
func ThemeToggle() tela.Node {
	dark, setDark := desktop.UseSetting("dark", false)
	return kit.Switch{Checked: &dark, OnCheckedChange: setDark.Set, AriaLabel: "Dark mode"}
}
```

`T` must encode as JSON. A stored value that no longer decodes (the setting
changed type between versions of your app) reads as the default; the key must stay
the same for the life of the component. Writes are coalesced and atomic, and a
corrupt file is kept aside rather than overwritten.

Behind it is a `desktop.Store`: a small JSON key-value file with `Get(key, &v)`,
`Set`, `Has`, `Keys`, `Delete`, `Watch(key, f)` and `Flush`. `desktop.Settings()` is
the app's (`settings.json` in `ConfigDir`); `desktop.OpenStore(path)` opens another,
and `desktop.UseStoreSetting(store, key, default)` is `UseSetting` for it. Stores do
not see changes other programs make to their files.

## Remembering where windows were

`desktop.PersistApp` wraps your `tela.App` so the main window reopens where the user
left it: size, maximized or fullscreen state and, where the OS allows, position.
`desktop.SavedWindow` does the same for a `tela.Window`, under a key:

```go
var App = desktop.PersistApp(tela.App{
	AppID: "com.example.notes", Title: "Notes", Width: 900, Height: 640, Root: Notes(),
})

// in a component:
If(inspectorOpen, tela.Component(desktop.SavedWindow{
	Key: "inspector", // unique in the app
	Window: tela.Window{
		Title: "Inspector", Width: 360, Height: 600,
		OnClose: func() { setInspectorOpen.Set(false) },
		Root:    Inspector{},
	},
}))
```

The placement is saved when it changes (debounced by half a second) and when the window
closes, in `windows.json` in `StateDir`. The parts are public if you want your own:
`desktop.LoadPlacement(key)`, `SavePlacement(key, p)` and the hook
`desktop.UseSavePlacement(key)`; a `tela.Window` takes the result as `Placement`.
**Wayland clients cannot place windows:** there only the size, maximized and fullscreen
state come back, plus the position on compositors that restore sessions
(`xdg-session-management-v1`). X11 and Windows restore the position too. A saved
position is not clamped to the monitors that are present at the next start.

## Folders of the app

`desktop.ConfigDir()`, `DataDir()`, `CacheDir()`, `StateDir()` and `LogDir()` return the
folders the app should use, following each OS's conventions (XDG on Linux, `%APPDATA%`
and `%LOCALAPPDATA%` on Windows), created on demand with mode 0700. They are named after
the app ID: `desktop.SetAppID("com.example.notes")`, else `tela.App.AppID`, else the
executable's name. Call `SetAppID` at the start of `main`, before anything reads a
folder or a setting (`PersistApp` does it for you). The `TELA_DESKTOP_DIR` environment
variable (`desktop.DirEnv`) moves every folder under one directory, for tests and
portable installs.

## When a service is missing

Not every desktop has every service. A minimal Linux session has no XDG Desktop Portal
(no file dialogs) or no notification daemon, and GNOME shows no tray icons without the
AppIndicator extension. The calls then return an error wrapping
**`desktop.ErrUnsupported`** (it also satisfies `errors.Is(err, errors.ErrUnsupported)`);
`Tray` reports it to `OnError`. Write the fallback:

| Service | Fallback |
|---|---|
| File dialogs | A text field for the path |
| Notifications | A message in the window |
| Tray | Do not hide the window on close, so the app can still be reached |
| `tela.OpenURL`, `tela.OpenPath` | Show the URL or path to copy |

Calling a service when no app runs (in `tela snapshot`, or before `tela.Run`) returns an
error too. Tests make a service unsupported on purpose with `app.Unsupported(...)`.

## Platform notes

What is supported where (Linux is Wayland and X11 over the XDG Desktop Portal and the
freedesktop D-Bus services; macOS comes in a later phase):

| | Linux | Windows |
|---|---|---|
| File dialogs | Portal `FileChooser`; `ErrUnsupported` without a portal. On Wayland the dialog is not parented to the window (no exported handle yet) | Common item dialogs |
| Message boxes | Drawn by Tela (modal window) | The OS's (`MessageBoxW`) |
| Notifications | `org.freedesktop.Notifications`, with actions | Balloons: no action buttons; each one adds a notification-area icon while it lives |
| Tray | StatusNotifierItem; `ErrUnsupported` on GNOME without the AppIndicator extension | `Shell_NotifyIconW`; icons are not redrawn on DPI changes |
| Window position | Wayland: cannot be set or restored without session restore. X11: yes | Yes |
| Always on top | Not on Wayland | Yes |
| Minimized state | Wayland cannot report it | Yes |
| Frameless windows | Resize edges by the backend; the move, the window menu and the double click go through the compositor | Aero Snap and Windows 11 snap layouts on the maximize button |

Smaller things worth knowing: an app icon for the title bar and taskbar is
`Icon: []image.Image` at several sizes (on Linux the default is the one of the
`.desktop` file of the `AppID`); a Wayland desktop with server-side decorations
draws its own title bar and `ClientSideDecorations` is false; some of these paths
(window maximize and menu on every compositor, X11 on every window manager) have been
checked live on a subset of desktops only.

**Under `tela dev`** the windows and services are the host's: window options, controls,
hit regions and icons are forwarded, a `tela.Window` that is open keeps its real window
across a reload (matched by its place in the tree), trays and notifications survive, and
a window closed to the tray stays hidden. A window that a new build opens only later (from an
effect) closes and reopens on reload, and a dialog opened at startup shows again.
**`tela snapshot` and `tela inspect`** render the main window's content only: they open no
windows and no services. `tela mcp` sees only the main window.

## Testing

`telatest` plays a fake window manager and a fake desktop, so none of this opens a window
or touches the real system. See [Testing](Testing#several-windows) for the windows
(`app.Windows()`, `WaitForWindow`, `ExpectWindowState`, `ExpectQuit`, `HitRegions`) and
[the desktop services](Testing#desktop-services) (`TempDesktopDirs`, `AnswerFileDialog`,
`AnswerMessage`, `Notifications`, `ClickTrayItem`, `Unsupported`):

```go
func TestOpen(t *testing.T) {
	telatest.TempDesktopDirs(t) // settings and placements go to a temporary folder
	app := telatest.MountApp(t, App)

	app.Click(telatest.ByRole("button", "Open…"))
	app.AnswerFileDialog("/home/ana/ideas.md")
	app.ExpectText("ideas.md")
}
```
