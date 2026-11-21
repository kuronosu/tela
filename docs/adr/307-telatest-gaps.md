# ADR-307: telatest's gaps

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-086, ADR-221, ADR-237, ADR-259, ADR-291, ADR-298.

## Decision

- **Modifiers:** `telatest.Modifiers("Shift", "ControlOrMeta"...)` is a `ClickOption` (`Click`, `ClickAt`, `DoubleClick`) and a `DragOption` (it already was one for drag and drop; `Drag`, `DragBy` and `DragFromTo` take it too), like Playwright's `click({modifiers})`. Every pointer event of the action carries them (`Shift`, `Ctrl`, `Alt`, `Meta`, `CmdOrCtrl`); no key events are sent, so a lone Alt does not arm a menu bar. `ClickOption` and `DragOption` become interfaces.
- **Flavor of menus:** the keys a menu role shows (Copy's `Ctrl+C` or `Super+C`) and the `CmdOrCtrl` of a `GlobalShortcut` follow the window's flavor (`PlatformKeys`), not the host's, through `platform.ParseAcceleratorFor`. `ParseAccelerator` and `desktop.ParseKeys` keep the host's.
- **Tray:** `TrayIcon.Template` reports `desktop.Tray.Template`.
- **Start launches and defaults:** `telatest.StartLaunch(desktop.Launch{Args, Dir, AtLogin})` is an `Option` that gives the app its own start (`core.Options.StartLaunch`, read by `UseLaunch` before the process's), per app, so parallel tests do not interfere; `App.AskForFileDefaults(bool)` plays Windows (`MakeDefault` of a file type returns `DefaultAsked`) and `App.DefaultsAsked()` lists the requests.
- **Waits:** between two failed tries a wait advances the clock to the next deadline of the app (a `UseTimeout`, `UseInterval`, caret blink, fade) when nothing else is due, instead of one frame, so a wait for a long timer takes a few tries whatever the machine's speed.
