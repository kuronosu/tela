// Package desktop gives a Tela app the services of the desktop it runs on
// (ADR-178, ADR-181): file and folder dialogs, message boxes,
// notifications, a tray icon, the app's own folders, a settings store and
// windows that open where the user left them. It works the same on Linux
// (Wayland and X11, through the XDG Desktop Portal and the freedesktop
// D-Bus services) and Windows, and in the headless backend of telatest and
// tela snapshot.
//
// # Dialogs
//
// [OpenFile], [OpenFiles], [SaveFile] and [PickFolder] show the OS's
// dialogs; [Title], [Directory], [DefaultName], [Filter] and [Parent]
// configure them. [ShowMessage] shows a message box with a fixed set of
// buttons: the OS's on Windows, drawn by Tela with the app's theme on
// Linux, whose portal has none. They are asynchronous like a browser's
// APIs: the call returns at once, and the callback runs later on the UI
// thread, so it may set state. Cancel is not an error: a dialog answers
// "" (or nil paths) and a nil error.
//
//	Button(OnClick(func() {
//	    desktop.OpenFile(func(path string, err error) {
//	        if err == nil && path != "" {
//	            setPath.Set(path)
//	        }
//	    }, desktop.Filter("Notes", "*.md", "*.txt"))
//	}), Text("Open…"))
//
// The calls run on the UI thread: from event handlers and effects, or
// from a goroutine through tela.Post. The dialog is modal to the window
// the call comes from ([Parent] chooses another).
//
// # Notifications and the tray
//
// [Notify] shows a notification whose OnClick, action and OnClose
// callbacks run on the UI thread. [Tray] is a component: the icon is in
// the tray while it is mounted and follows its fields, like any other
// element; its [MenuItem] values run OnSelect. With App.OnClose hiding the
// main window, the app lives in the tray until tela.Quit.
//
// # Folders, settings and window placement
//
// [ConfigDir], [DataDir], [CacheDir], [StateDir] and [LogDir] return the
// app's folders, following each OS's conventions (XDG on Linux,
// %APPDATA% and %LOCALAPPDATA% on Windows, ~/Library on macOS), named
// after the app ID ([SetAppID], else App.AppID, else the executable) and
// created on demand. [Store] is a small JSON key-value file written
// atomically; [Settings] is the app's, and [UseSetting] is a useState
// whose value lives there:
//
//	theme, setTheme := desktop.UseSetting("theme", "system")
//
// [PersistApp] makes the main window open where it was closed;
// [SavedWindow] does the same for a tela.Window, and [LoadPlacement] with
// [UseSavePlacement] are the parts they are made of.
//
// # When a service is missing
//
// Not every desktop has every service: GNOME shows no tray icons without
// an extension, a minimal Linux session may have no portal (no file
// dialogs) and no notification daemon. The calls then return an error
// wrapping [ErrUnsupported] (errors.Is), and the app falls back: a path
// field instead of a dialog, a message in the window instead of a
// notification, a window that does not hide itself on close without a
// tray.
//
// # Tests
//
// telatest answers dialogs (App.AnswerFileDialog, App.AnswerMessage),
// reads and clicks notifications and the tray (App.Notifications,
// App.ClickNotification, App.Tray, App.ClickTrayItem), and points the
// folders at a temporary directory (telatest.TempDesktopDirs, which sets
// the [DirEnv] variable).
package desktop
