# ADR-217: The app's ID for the desktop, and RegisterApp

- **Date:** 2026-11-01
- **Status:** Accepted. Amends ADR-210, ADR-213 and ADR-214.

## Decision

- **`platform.Desktop.SetAppID`**, called by package desktop when `tela.Run` starts with the ID `desktop.AppID` resolves. Linux uses it for the shortcut portal's registration, notifications and trays. Under `tela dev` it goes to the host's backend (protocol 5 amended).
- **A refused ID is an error.** The first `RegisterShortcut` registers the ID on the UI thread; a refusal is `ErrAppNotRegistered`, naming `<ID>.desktop` and `desktop.RegisterApp`, with no status after it. X11 then grabs keys; Wayland has no fallback.
- **`RegisterApp`, `UnregisterApp`, `AppStatus`**, explicit only. Linux: the app's own entry (`NoDisplay`, no `MimeType`, `X-Tela-App`), shared with associations, kept until `UnregisterApp`, which keeps what associations need; another's entry only with `Replace`. Windows writes nothing.
- **`ShortcutBound` with no `Trigger`**: registered, no keys assigned. Hyprland: `bind = MODS, KEY, global, <ID>:<shortcut>`.

## Rationale

A Hyprland test with xdg-desktop-portal 1.22: the executable's name has no entry, so every shortcut failed. ADR-209's conditions apply.
