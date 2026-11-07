# ADR-236: `TitleBarOverlay` keeps DWM's caption buttons on Windows 11

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-225 ("Frameless on Wayland, X11, Win32": Win32 on Windows 11 keeps its buttons).

## Decision

On Windows 11 (build 22000) a `TitleBarOverlay` window is a decorated window whose caption is client area, with DWM's minimize, maximize and close buttons over the app's title bar (details in `internal/platform/win32`'s doc, "Title bar overlay").

- `WM_NCCALCSIZE` keeps DefWindowProc's side and bottom frame and moves the top back to the window's top (Windows Terminal). `DwmExtendFrameIntoClientArea` extends the frame by a caption's height (the sheet of glass under Mica or Acrylic); `DwmDefWindowProc` sees every message first and answers the buttons' hit test (snap layouts), hover and presses.
- DWM draws the buttons in the extended frame *under* the client pixels, on a fill of `DWMWA_CAPTION_COLOR`: they show only where the presented pixels are transparent. The window presents the buttons' rectangle transparent and sets the caption color to the app's `TitleBar.Background`, else to its bar's color sampled from the presented frame just left of the buttons (sent to DWM only when it changes). The glyphs follow `DWMWA_USE_IMMERSIVE_DARK_MODE` from that color, else the scheme. A Mica or Acrylic window keeps the app's alpha instead.
- Maximized, the client area's top stays at the window's top, above the work area by the frame (with it clipped, DWM neither drew nor answered its buttons). The backend hides that band: the app's size, pixels and every position (pointer, hit test, `TitleBarControls`, IME, drag and drop) are the work area's.
- `WindowState.TitleBarControls` is `DWMWA_CAPTION_BUTTON_BOUNDS` in the app's logical coordinates, read at every state report and at the next Presents; empty in fullscreen. The pointer there is the OS's. No menu bar, as `Frameless`.
- Windows 10, no composition or no DWM procedures: `Frameless` as before, `TitleBarControls` empty.

## Why

DWM's own buttons are the native ones (snap layouts, accessibility, the system's look) with no drawing in Tela; the hole, the caption color and the hidden band are what DWM needs to draw and answer them (found on build 26200). Windows 10 is not verified, so it keeps the drawn buttons.
