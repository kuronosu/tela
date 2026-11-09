# ADR-260: Keys and placement of drawn menus

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-133, ADR-247, ADR-255 and ADR-259.

## Decision

- **Lone Alt** (owner): as today on Windows and Linux; besides another key, a press, a wheel and a focus change, the pointer leaving the window and a resize cancel it (a window manager's Alt+drag). Never under the Mac flavor.
- **Access keys** (owner; not Mac): `MenuItem.AccessKey`, `MenubarMenu.AccessKey`; a window's bar gives every level keys by the Win32 bar's rule (`platform.AutoAccessKeys`, `aria.AccessKeys`), other menus only their own (Windows' popups). Underlined while Alt is held or the keyboard is in the bar; a key appended to a CJK label always shows. Alt+key opens a menu, the key alone on a trigger or in a menu chooses, before typeahead; shared keys cycle the focus. Physical letters back non-Latin layouts.
- **Typeahead** on any bar's triggers (APG, Radix's cycling).
- **Window's bar:** `AppMenu`, a drawn fallback of a native bar, or `Menubar.ActivationKeys`; other bars stay Radix's.
- `FocusedBefore` clears on a focus from nowhere (`core.FocusFromNowhere`).
- No focus: a keyboard context menu opens at (1, 1) (Chromium); one opened by the app at `Position`, else as the keys.
- A submenu with no room on either side shifts across into the window (Floating UI's shift crossAxis).
- A loading `Button` in a `TriggerFunc` tells its overlay by context: `LoadingTrigger` only for other elements.

## Why

Native menus and Chromium, with guards for Linux's Alt.
