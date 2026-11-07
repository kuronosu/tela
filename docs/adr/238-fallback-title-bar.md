# ADR-238: The fallback title bar

- **Date:** 2026-11-07
- **Status:** Accepted (completes ADR-230)

## Decision

The owner picked look **B, Slim**, from three rendered options (Adwaita, Slim, Seamless). A framed window whose OS draws no title bar (`ClientSideDecorations` while neither `Frameless` nor `TitleBarOverlay`: GNOME on Wayland) gets one drawn by Tela: 36 px, the app's `bg-background` with a `border-border` hairline under it, the window's title centered (13 px, medium, `text-muted-foreground`), flat 28 px window buttons at the end that tint on hover (close in the destructive color), GNOME-style 16 px glyphs, dimmed while the window is not focused. It is a drag area: the OS moves the window, maximizes on a double click and opens the window menu on a right click; resize edges are the backends' as today.

It hides by itself when the window's tree has an `h.WindowDrag` area of its own (the app draws its own bar, as `kit.TitleBar` does): no new public API for the common case. The buttons follow GNOME's `button-layout` (read through the settings portal) when the desktop has one.

## Why

The owner judged by the rendered looks; Slim takes little room and matches Electron's and winit's fallback frames. Detecting the app's own drag area keeps existing apps (`examples/notes`) unchanged.
