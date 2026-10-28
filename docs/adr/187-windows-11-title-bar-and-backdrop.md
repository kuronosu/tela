# ADR-187: Windows 11 title bar colors and system backdrop

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

`tela.App` and `tela.Window` gain `TitleBar tela.TitleBarColors{Background, Text, Border}` (the zero `Color` is the system's) and `Backdrop tela.Backdrop` (`BackdropDefault`, `None`, `Mica`, `MicaAlt`, `Acrylic`). Both are applied on change. Amends ADR-179 (contract), ADR-180 (fields) and ADR-184 (protocol 4).

- **Platform:** `WindowOptions.TitleBar`/`Backdrop` and `Window.SetTitleBarColors`/`SetBackdrop` (`platform.TitleBarColors` of 8-bit colors with `Set`, `platform.Backdrop`); Wayland and X11 do nothing, headless records them, the remote protocol forwards them (options, two window-control ops).
- **Win32:** `DwmSetWindowAttribute` CAPTION_COLOR, TEXT_COLOR, BORDER_COLOR (build 22000) and SYSTEMBACKDROP_TYPE (build 22621); failures are ignored. The native title bar shows the backdrop with nothing else.
- **Client area:** Mica, Mica Alt and Acrylic also call `DwmExtendFrameIntoClientArea(-1)` and keep the premultiplied alpha in the retained DIB (`SetDIBitsToDevice` carries it; verified on Windows 11 26200). The window implements `platform.AlphaPresenter`; core then makes the tree's canvas transparent (`Tree.SetTransparentCanvas`): a root without a background clears to `{0,0,0,0}` instead of white, and a translucent one is composed once. Other windows and platforms render as before.
- **Unchanged on purpose:** a window without a client backdrop never extends the frame and keeps forcing opaque alpha (the snap layouts flyout, commit `da6d1f5`; transparent windows after an RDP reconnect). A frameless window with a backdrop extends the frame with a zero top margin and wide side margins (any non-zero top margin makes DWM draw its caption buttons over the app's title bar); the snap layouts flyout may still be lost there, documented. `WM_DWMCOMPOSITIONCHANGED` and `WM_THEMECHANGED` apply the attributes and margins again.

## Rationale

DWM draws the material and shows it where a glass window is transparent, so the only work is alpha through our GDI blit. `tela dev` renders opaque content (the child cannot know the host's window).
