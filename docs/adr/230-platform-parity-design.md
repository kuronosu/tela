# ADR-230: Platform parity design (phase 16)

- **Date:** 2026-11-07
- **Status:** Accepted (amends §2)

## Decision

The owner's choices for phase 16 (Platform parity), on 2026-11-07:

- **`combase.dll` is allowed on Windows** (§2's DLL list): WinRT activation and strings (`RoInitialize`, `RoActivateInstance`, `RoGetActivationFactory`, `WindowsCreateString`, `WindowsDeleteString`, `WindowsGetStringRawBuffer`) for toast notifications with actions (`Windows.UI.Notifications`), called through COM vtables in pure Go like the existing `ole32` code. Balloons stay the fallback where toasts are unavailable.
- **A fallback title bar is automatic:** when a framed window has no server-side decorations (`ClientSideDecorations`, GNOME on Wayland), Tela draws a plain title bar (title, window buttons, drag, double-click, resize edges) unless the app draws its own; the opt-out and the look are settled by a later ADR after the owner picks a rendered option.
- The work branch `phase-16-platform-parity` is pushed for CI on `macos-latest` and `windows-latest` (ADR-220).
- Japanese through XIM is checked with fcitx5 and Mozc on the owner's Linux machine.

## Why

Toasts are the only Windows notifications with buttons that stay in the Action Center; WinRT has no entry point outside `combase`. The winit (`sctk-adwaita`) and Electron model of a drawn fallback frame keeps a window usable on GNOME without code in every app.
