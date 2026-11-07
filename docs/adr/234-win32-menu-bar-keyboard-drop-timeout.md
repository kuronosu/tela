# ADR-234: Win32 menu bar keyboard and dark drawing, bounded drop reads, DPI and layout changes

- **Date:** 2026-11-07
- **Status:** Accepted (amends ADR-179, ADR-210, ADR-213)

## Decision

Phase 16, Win32 gaps (`internal/platform/win32` only; the contract is unchanged).

- **The menu bar opens from the keyboard.** The key translation still keeps the system keys from `DefWindowProc` (the app sees every key first, windows without a bar keep today's behavior). A window with a bar sends itself `SC_KEYMENU`: on the release of a lone Alt or F10 (any other key, mouse button, wheel or lost focus in between disarms it; AltGr, Alt with Ctrl, Shift+F10 and Alt+F10 are not it), and on Alt plus the access key of a top-level item (`WM_SYSCHAR`). The bar's labels are plain text, so the backend gives each menu level access keys (first free Latin letter or digit, an `&` in the native label); Windows underlines them per the "underline access keys" setting, and the window shows them for a keyboard opening when that setting hides them. An app Alt shortcut that shares a key with a menu still reaches the app and also opens the menu: the contract cannot say the key was consumed.
- **The bar is dark in dark mode** (system app theme, not high contrast) with user32 and gdi32 only: the undocumented `WM_UAHDRAWMENU` and `WM_UAHDRAWMENUITEM` of the themed bar are painted (menu font, `DrawTextW`, colors of the dark title bar), plus the line under the bar after `WM_NCPAINT` and `WM_NCACTIVATE`. Drop-down and popup menus stay light: darkening them needs uxtheme's undocumented ordinals, a DLL not in §2, and owner-drawing them loses access keys, screen reader text and system metrics.
- **Reads from a drag's source are bounded.** The UI thread enables COM call cancellation and a timer cancels the pending call (`CoCancelCall`) when no call (`EnumFormatEtc`, `GetData`, a stream chunk, the shell helper's) has completed for **5 s**. Formats that cannot be listed offer nothing; data that cannot be read completes the drop with no data (`integ.dropErr` keeps the error). No worker thread: a proxy marshaled to another apartment may route back through the blocked STA.
- **Tray and notification icons are drawn again** at the new side on `WM_DPICHANGED` (tray window), `WM_DISPLAYCHANGE` and `WM_SETTINGCHANGE`.
- **Global shortcuts declared by character are bound again** on `WM_INPUTLANGCHANGE` when their virtual key moves (failed ones are kept to retry; all moving keys are released before any is registered again), reporting a `ShortcutStatusEvent` when the status changes.

## Why

Native bars open with Alt and F10, and a light bar under a dark title bar looks broken. A cross-process call without a bound freezes the app for as long as the source does. Each fix uses the OS mechanism the native apps use.
