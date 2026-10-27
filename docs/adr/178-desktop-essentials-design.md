# ADR-178: Desktop essentials design (phase 12)

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Owner-approved shape of phase 12 (amends §2, §5, §15).

- **Windows are declarative.** A `tela.Window{...}` struct component in the tree opens a window while it is mounted and closes it when it unmounts (Flutter's multi-view model): one Element tree (state and context flow into child windows), one Box tree, layer tree, focus, hover and overlay stack per window. The main window is `App`'s. `Modal` makes it modal to the window it is declared in; `OnClose` runs when the user asks to close it. `tela.UseWindow()` returns a comparable handle of the window the component renders in (minimize, maximize, fullscreen, hide/show, close, state). `tela.Quit()` ends the app from any goroutine; `tela.OpenURL`/`tela.OpenPath` open the default application.
- **Custom title bars:** a frameless window marks drag regions and window-control buttons with `h` props; the backend answers them natively (Win32 `WM_NCHITTEST` with snap layouts, `xdg_toplevel.move`/`resize`/`show_window_menu`, `_NET_WM_MOVERESIZE`). It is also the Wayland fallback when the compositor has no server-side decorations.
- **OS services live in a new public package `desktop`**, between `tela`/`h` and `i18n` in the layer order: file and folder dialogs, message boxes (native on Windows, drawn on Linux, where the portal has none), notifications, a declarative tray icon with a menu, per-app config/data/cache paths, a small settings store with change events, persisted window state. Asynchronous calls answer on the UI thread.
- **Linux services use D-Bus through `github.com/godbus/dbus/v5`** (already approved for AT-SPI; now used from phase 12): XDG Desktop Portal (FileChooser, OpenURI), `org.freedesktop.Notifications`, StatusNotifierItem with `com.canonical.dbusmenu`. Shared by the Wayland and X11 backends.
- **Windows adds `shell32`, `ole32` and `comdlg32`** to the allowed system DLLs (common item dialogs over COM, `Shell_NotifyIconW`, `ShellExecuteW`).
- **Window position on Wayland:** clients cannot place windows; `xdg-session-management-v1` restores geometry where the compositor supports it, otherwise only size, maximized and fullscreen are restored. X11 and Windows restore the position too.

## Rationale

Declarative windows keep the React model (a window is UI that exists while state says so) and let child windows share state and context without plumbing; Flutter's multi-view proves the one-tree, many-views split. A separate `desktop` package keeps `tela` small; godbus is pure Go and was already vetted.
