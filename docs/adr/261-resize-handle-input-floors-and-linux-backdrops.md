# ADR-261: Resize handle input, resize floors and Linux backdrops

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-257, ADR-249 and ADR-187 (contract v12; remote protocol unchanged).

## Decision

- **Handles take input over their whole rectangle** while the window can be resized, transparent pixels included (`platform.HandleRects`): Wayland and X11 add them to the input region and shape; Win32 and Cocoa, whose window servers pass zero alpha through, present those pixels with alpha 1 (`platform.HandleCatch`).
- **Portal content wins:** core punches `HitClient` holes where a portal layer's content covers caption, button or resize regions.
- **Floors:** Win32's minimum track size of frameless and transparent windows and the Cocoa loop use `MinSize` per axis, else `platform.ResizeFloor`: two 16 px corners of handles, the margin kept, never above the current size.
- **macOS:** AppKit's two-way cursors over handles; diagonals from `frameResizeCursorFromPosition:inDirections:` (15) or the private `_windowResize…Cursor` (before), behind `respondsToSelector:` as in winit. No edge snapping, by design: AppKit cannot start the window server's resize from an event.
- **Linux backdrops** (owner, 2026-11-09): a material backdrop on a transparent window blurs behind its mask with `ext-background-effect-v1` (MIT) and `_KDE_NET_WM_BLUR_BEHIND_REGION`. `org_kde_kwin_blur` is skipped: its XML is LGPL-2.1-or-later.
- FixedSize transparent Win32 windows keep no `WS_THICKFRAME`, by design.

## Why

A corner that resizes only inside its curve, a popover that resizes the window, a card that loses its handles: each broke what the band promised.
