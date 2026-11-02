# ADR-224: macOS desktop integration design (phase 15)

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-178, ADR-209 and ADR-213.

## Decision

The owner's shape for phases 12 and 13 on macOS (details in ADR-225 to 228 and doc comments).

- **Development bundle:** `tela build` wraps a darwin program in `<Name>.app` with the app's ID; `tela run` builds it in the user cache and runs it. No re-exec: under plain `go run`, bundle-only services (notifications, Launch Services) return `ErrUnsupported` naming `tela run`.
- **Title bar:** `Frameless` is unchanged. A new `TitleBarOverlay` puts the content under a transparent title bar with the OS's buttons over it (the traffic lights, centered in the app's caption) and reports their rectangle, which `kit.TitleBar` leaves free. Where the OS cannot, it is `Frameless`.
- **One app menu bar, declared once** (Electron's `setApplicationMenu`): `desktop.AppMenuBar`, or `kit.Menubar` as the app's. macOS shows it for every window after the app menu Tela adds (About, app items, Services, Hide, Quit); without one, the app, Edit and Window menus. Windows: every framed window's bar. Standard items have Electron's roles. On macOS an accelerator is a key equivalent.
- **Quit:** `tela.App.OnQuit` (Cmd+Q, the Dock, logout); nil sends every window a close request.
- **Windows:** child and modal windows are not sheets; dialogs and message boxes with a parent are.
- **Backdrops** map onto `NSVisualEffectView` materials, with no new values.
- **Services:** NSWorkspace, open and save panels, NSAlert, NSStatusItem, UNUserNotificationCenter (in a bundle), Carbon hot keys, single instance by a lock file and a Unix socket, Launch Services' URLs and files as launches, registration in the bundle's Info.plist, the Keychain, `trashItemAtURL:`.

## Rationale

The native models: one menu bar per app, Cmd+Q quits even a tray app, Apple's apps put the traffic lights over their own bars. The owner kept `Frameless` for apps that draw everything and chose an explicit bundle step over a hidden re-exec.
