# ADR-221: Platform flavor and `CmdOrCtrl`

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-065, ADR-069, ADR-104 and ADR-086.

## Decision

The key conventions of a platform (text field bindings, word rules, the primary modifier of shortcuts) are a three-state `platform.Flavor` (`FlavorLinux`, `FlavorWindows`, `FlavorMac`), separate from the backend. The zero value, `FlavorHost`, resolves through `platform.HostFlavor()`, the only reader of `runtime.GOOS` for this.

- **core:** `Config.Flavor`, resolved once per Tree and inherited by child windows' Trees; text fields read it.
- **Public API:** `CmdOrCtrl() bool` on `KeyboardEvent`, `MouseEvent`, `PointerEvent`, `WheelEvent` and `DragEvent`: Meta under the Mac flavor, Ctrl otherwise, from the dispatching Tree (a hand-built event follows the host). It tests only that modifier, like React Aria's isCtrlKeyPressed and Playwright's ControlOrMeta; the name is Electron's, as in `ParseAccelerator`.
- **aria:** collections use `CmdOrCtrl` (Cmd+click, Cmd+A on a Mac).
- **telatest:** `PlatformKeys(goos)` sets one app's flavor; `ControlOrMeta` follows it.
- **Pointer:** under the Mac flavor a primary press with Ctrl is a context click, as Blink does on the Mac: `OnContextMenu` fires at the release and no click does (phase-14 review).
- `ParseAccelerator` keeps the host's flavor.

## Rationale

Four copies read `runtime.GOOS` (review C11), so no Mac behavior could be tested off a Mac, and apps had no primary-modifier query (the examples checked Ctrl or Meta).

## Out of scope

Visual looks picked from `runtime.GOOS` (`kit`'s Default window controls, `looks/detect`, the notes example's preferences) do not follow the flavor.
