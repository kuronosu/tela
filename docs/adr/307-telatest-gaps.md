# ADR-307: telatest's gaps

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-086, ADR-221, ADR-237, ADR-259, ADR-291, ADR-298.

## Decision

- **Modifiers:** `telatest.Modifiers("Shift", "ControlOrMeta"...)` is a `ClickOption` (`Click`, `ClickAt`, `DoubleClick`) and a `DragOption` (it already was one for drag and drop; `Drag`, `DragBy` and `DragFromTo` take it too), like Playwright's `click({modifiers})`. Every pointer event of the action carries them (`Shift`, `Ctrl`, `Alt`, `Meta`, `CmdOrCtrl`); no key events are sent, so a lone Alt does not arm a menu bar. `ClickOption` and `DragOption` become interfaces.
- **Flavor of menus:** the keys a menu role shows (Copy's `Ctrl+C` or `Super+C`) and the `CmdOrCtrl` of a `GlobalShortcut` follow the window's flavor (`PlatformKeys`), not the host's, through `platform.ParseAcceleratorFor`. `ParseAccelerator` and `desktop.ParseKeys` keep the host's.
