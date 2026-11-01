# ADR-215: Native menus in kit

- **Date:** 2026-11-01
- **Status:** Accepted. Implements the native-menus bullet of ADR-209 on ADR-213's `desktop.PopupMenu` and `desktop.MenuBar`.

## Decision

- **`tela.App.NativeMenus bool`** asks for the OS's menus for the whole app (child `Window`s follow it). Components read it with `tela.UseNativeMenus()`: a build-time lookup of the tree's `core.Config.NativeMenus`, with no hook slot, since it never changes.
- **`kit.ContextMenu` and `kit.Menubar` have `Native kit.NativeMode`**, a named type with constants like kit's variants: `NativeAuto` (the zero value: follow the app), `NativeOn`, `NativeOff`. A tri-state avoids `*bool` and keeps the zero value meaning "follow", so the menu's own setting wins in both directions. `DropdownMenu` stays drawn: it hangs off a button and has no OS counterpart.
- **Context menu:** `aria.ContextMenuTriggerOptions.Intercept(x, y) bool` is asked before a menu opens (pointer, long press, Shift+F10 or Menu key at the area's corner). kit shows `desktop.PopupMenu` there; on any error (`ErrUnsupported` on Linux) it returns false and the drawn menu opens in the same event and frame. A native menu leaves the drawn state, `Open` and `OnOpenChange` untouched.
- **Menu bar:** a native `Menubar` renders `desktop.MenuBar` and nothing drawn. When its `OnError` fires (Linux, a frameless Windows window, no window of a running app: `desktop.MenuBar` now reports that too), the drawn bar replaces it one frame later.
- **Conversion:** label, shortcut as accelerator (canonical via `desktop.ParseKeys` when it parses, else as written), disabled, separators; checkbox items and radio items become check items; a label becomes a disabled item; a group is flattened. Dropped: icons, the destructive variant, inset, `KeepOpen`, classes, check/radio icons. Choosing runs the same code as the drawn menu.
- `telatest.PreferNativeMenus()` is the option; `FromApp` applies the field.

## Rationale

The owner chose opt-in per menu or for the app, with the menu's own setting winning. The fallback needs no platform checks in kit: the desktop's errors say what exists.
