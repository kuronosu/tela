# ADR-225: Platform contract v4 (macOS desktop integration)

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-210 and ADR-214.

## Decision

Details of ADR-224 in `internal/platform` (contract in `doc.go`).

- **Quit requests:** the app-scoped `QuitRequestedEvent` (Cocoa's `applicationShouldTerminate:`, which answers cancel). Core runs `Options.OnQuit` (`tela.App.OnQuit`); nil sends every window a close request.
- **`WindowOptions.TitleBarOverlay`** (excludes `Frameless`: `Resolve` and `tela` panic) and **`WindowState.TitleBarControls`**, the buttons' rectangle, which takes presses over the hit regions. Wayland, X11 and Win32 make it `Frameless` (`OverlayAsFrameless`).
- **App menu bar:** `Desktop.SetAppMenu(menu, appItems)`, choices as the app-scoped `AppMenuEvent` (not `MenuEvent` with no window: events are window- or app-scoped by type). `ErrUnsupported` except in headless and remote.
- **`MenuItem.Role`** with Electron's names. Role items keep their IDs: a backend without the native action reports them. Menu roles mark one submenu each. In the macOS app menu bar an accelerator is a key equivalent.
- Headless fakes all of it; remote protocol 6 carries it.

## Rationale

Plain app-scoped events cross the remote protocol. IDs on role items let every backend report them.
