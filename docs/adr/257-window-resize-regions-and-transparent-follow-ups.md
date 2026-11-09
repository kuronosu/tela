# ADR-257: Window resize handles and transparent window follow-ups

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-249, ADR-252, ADR-178/179 and ADR-187 (contract v10, protocol 14).

## Decision

- **`h.WindowResize(opts...)`** (owner's choice: a prop on the card, like `WindowDrag`) marks a band inside the element's border box, 5 px (`tela.ResizeBand`) on the sides of `tela.ResizeEdges` (all by default), corners 16 px along each side, appended after the element's subtree as `platform.HitResizeTop`…`HitResizeBottomRight` regions. Backends start the OS resize where the window can be resized (not FixedSize, maximized, fullscreen): `xdg_toplevel.resize`, `_NET_WM_MOVERESIZE`, `HTLEFT`… (a transparent window gets `WS_THICKFRAME`), Cocoa its own loop of drags (no public window-server resize).
- **Caption clicks everywhere:** X11 waits for the drag threshold; Win32 compares rectangle, maximize and cursor around the move loop; Cocoa follows `performWindowDragWithEvent:` through `pressedMouseButtons`.
- **Pass-through:** Cocoa takes the pointer back from a global mouse monitor (slow safety poll); Win32 keeps its 16 ms poll (a `WH_MOUSE_LL` hook puts every desktop move through the UI thread and is dropped silently when slow). Both let a resting cursor through when Present changes the mask (no swallowed press).
- **X11:** XFixes 2.0 `SelectSelectionInput` on `_NET_WM_CM_S<n>`.
- **Backdrop:** macOS draws vibrancy behind the mask's shape (`maskImage`); Windows cannot (DWM materials need a DWM frame, a layered window has none).
- **`tela dev`:** the overlay composites with alpha (was opaque black over transparent pixels).

## Why

The band lies on the card's pixels, so the click-through mask keeps it; reusing hit regions gives every backend its native resize.
