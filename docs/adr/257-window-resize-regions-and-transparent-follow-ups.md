# ADR-257: Window resize handles and transparent window follow-ups

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-249, ADR-252, ADR-179 and ADR-187 (contract v10, protocol 14).

## Decision

- **`h.WindowResize(opts...)`** (the owner's prop on the card): a band inside the element's border box, 5 px (`tela.ResizeBand`) on the sides of `tela.ResizeEdges`, corners 16 px, sent after its subtree as `platform.HitResize…` regions. Where the window can be resized, backends start the OS resize: `xdg_toplevel.resize`, `_NET_WM_MOVERESIZE`, `HTLEFT`… (a transparent window gets `WS_THICKFRAME`), Cocoa its own drag loop (no public window-server resize).
- **Caption clicks everywhere:** X11 waits for the drag threshold; Win32 compares rectangle, maximize and cursor around the move loop; Cocoa follows `performWindowDragWithEvent:` through `pressedMouseButtons`.
- **Pass-through:** Cocoa uses a global mouse monitor (slow safety poll); Win32 keeps its poll (a `WH_MOUSE_LL` hook runs every desktop move through the UI thread). Both pass a resting cursor when Present changes the mask.
- **X11** follows `_NET_WM_CM_S<n>` with XFixes. **Backdrop:** macOS vibrancy masked to the card; none on Windows (layered windows have no DWM frame). **`tela dev`'s overlay** keeps the alpha.

## Why

The band lies on the card's pixels, so the click-through mask keeps it, and hit regions give each backend its native resize.
