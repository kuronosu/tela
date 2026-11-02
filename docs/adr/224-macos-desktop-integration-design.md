# ADR-224: macOS desktop integration design (phase 15)

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-178, ADR-209 and ADR-213.

## Decision

The owner chose this shape for phases 12 and 13 on macOS (details in later ADRs and doc comments).

- **Development bundle.** A new `tela build [-o dir] [pkg]` builds the program. On macOS it wraps the program in `<Name>.app`, with an `Info.plist` that names `App.AppID`. `tela run [pkg] [args]` builds that bundle in the user cache and runs it. The packaging phase extends `tela build`. Plain `go run` and `go build` stay unbundled, with no re-exec. Services that need a bundle (notifications, Launch Services registration and launches) return `ErrUnsupported` and name `tela run`.
- **Title bar.** `Frameless` keeps its meaning: no OS title bar and no OS buttons. A new `TitleBarOverlay` on `tela.App` and `tela.Window` puts the content under a transparent title bar while the OS keeps its window buttons over it. On macOS those are the traffic lights, centered in the app's caption height. The window reports the buttons' rectangle, so `kit.TitleBar` leaves room for them and draws no controls there. Where the OS cannot do this, `TitleBarOverlay` acts as `Frameless` and the rectangle is empty.
- **One app menu bar, declared once** (Electron's `Menu.setApplicationMenu`). It is a new `desktop.AppMenuBar` component, and `kit.Menubar` can be it.
  - macOS: it is the menu bar of every window. Tela always puts the app menu first (About, the app's items such as Settings…, Services, Hide, Hide Others, Show All, Quit). Without an `AppMenuBar` the bar shows the app, Edit and Window menus.
  - Windows: it is the bar of every framed window that has no `desktop.MenuBar` of its own.
  - Elsewhere it reports `ErrUnsupported` and kit draws it.
  - A window's own `desktop.MenuBar` is unsupported on macOS.
  - Standard items have roles with Electron's names: undo, redo, cut, copy, paste, select all, minimize, zoom, close, and the Edit and Window menus. On macOS these are AppKit's own items, acting on the focused field and the key window.
  - On macOS an accelerator is the item's key equivalent: the key chooses the item and does not reach the app as a key event.
- **Quit.** `tela.App.OnQuit` runs when the user or the OS asks the app to quit (Cmd+Q, the app menu, the Dock, logout). A nil `OnQuit` sends a close request to every window, as today. An app-scoped platform event carries the request.
- **Windows.** Child and modal windows stay separate windows kept above their parent; they are not sheets. File dialogs and message boxes with a parent are sheets on it.
- **Backdrops.** The existing `Backdrop` values map onto the closest `NSVisualEffectView` material behind the window, and no new values are added.
- **Services:**
  - NSWorkspace, NSOpenPanel and NSSavePanel, NSAlert, and NSStatusItem.
  - UNUserNotificationCenter, in a bundle only.
  - Carbon `RegisterEventHotKey` for global shortcuts: no permission prompt, and releases are reported.
  - Single instance through a lock file and a Unix socket in the per-user temporary folder. The URLs and files that Launch Services sends (`application:openURLs:`) are launches.
  - Registration through the bundle's `Info.plist` and Launch Services, in `internal/platform/assoc`.
  - The Keychain (SecItem generic passwords; service: the app ID, account: the name).
  - `NSFileManager trashItemAtURL:`, which never deletes.

## Rationale

These are the native models: the menu bar belongs to the app and stays the same for every window, Cmd+Q quits even an app that runs in the tray, and Apple apps put the traffic lights over their own title bars. The owner kept `Frameless` borderless for apps that draw everything, and chose an explicit bundle step over a hidden re-exec.
