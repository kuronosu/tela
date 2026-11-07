# ADR-236: `TitleBarOverlay` keeps DWM's caption buttons on Windows 11

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-225 ("Frameless on Wayland, X11, Win32": Win32 on Windows 11 keeps its buttons).

## Decision

On Windows 11 (build 22000) a `TitleBarOverlay` window is a decorated window whose caption is client area, with DWM's minimize, maximize and close buttons over the app's title bar (details in `internal/platform/win32`'s doc, "Title bar overlay").

- `WM_NCCALCSIZE` keeps DefWindowProc's side and bottom frame and moves the top back to the window's top (Windows Terminal); maximized, the client area is the work area. `DwmExtendFrameIntoClientArea` extends the frame by a caption's height (the sheet of glass under Mica or Acrylic); `DwmDefWindowProc` sees every message first and answers the buttons' hit test (snap layouts), hover and presses.
- `WindowState.TitleBarControls` is `DWMWA_CAPTION_BUTTON_BOUNDS` in logical client coordinates, read at every state report and at the next Presents; empty in fullscreen. The pointer there is the OS's.
- The glyphs follow `DWMWA_USE_IMMERSIVE_DARK_MODE`, set from the app's `TitleBar.Background` when given, else from the scheme. No menu bar, as `Frameless`.
- Windows 10, no composition or no DWM procedures: `Frameless` as before, `TitleBarControls` empty.

## Why

DWM's own buttons are the native ones (snap layouts, accessibility, the system's look) with no drawing in Tela. Windows 10 is not verified, so it keeps the drawn buttons.
