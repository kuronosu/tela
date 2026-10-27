# ADR-180: Multiple windows in core and the public API

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Implements ADR-178's declarative windows on ADR-179's contract (amends §6, §14, §15).

- **One Tree per window, linked into one element tree.** A child window's `Tree` has its own box tree, viewport, scale, layers and damage, framebuffer, focus, hover and pointer state, gesture arena, portals and overlays, cursor, IME and semantics. Its root hangs below the `tela.Window` element for context lookups (`Tree.outer`), Value reads are tracked app-wide (`Tree.trk`, the main tree's), each reader is marked in its own tree, and theme, class aliases and color scheme propagate to child trees. Windows open, change and close after the build lock is released (`flushWindows`). A tree without a window (`tela snapshot`, `inspect`) opens none; telatest opens them but queries the main window only.
- **API:** `tela.Window{Title, Width, Height, Min/Max…, FixedSize, Frameless, AlwaysOnTop, Modal, Hidden, Icon, RootClass, Placement, OnClose, Root}`, applied on change (FixedSize, Frameless, Modal, Placement only at open). A nil `OnClose` makes the close request do nothing, with a dev warning when the window opens (state owns existence, like React's controlled inputs); a nil `Root` panics in dev mode. `App` gains the same fields plus `OnClose func(WindowHandle)` (nil quits). `tela.UseWindow() WindowHandle`: `Minimize`, `Maximize`, `Unmaximize`, `ToggleMaximize`, `SetFullscreen`, `Hide`, `Show`, `Focus`, `Close`, reactive `State() WindowState`, `Placement() WindowPlacement`; requests are queued with the window tree's updates (any goroutine). `tela.Quit()` (any goroutine, routed like `Post`), `tela.OpenURL`/`OpenPath` (UI thread; dev mode panics elsewhere).
- **Title bars:** `h.WindowDrag()`, `h.WindowNoDrag()`, `h.WindowControl(tela.WindowMinimize|WindowMaximize|WindowClose)`. Interactive descendants of a drag region are holes; regions are computed after frames that ran the pipeline, sent only on change; controls act on click.
- **Modality:** core drops a blocked window's input (after cancelling its pointers); a press there activates the modal window; a close request is refused.

## Rationale

A Tree per window keeps the single-window pipeline unchanged (no per-frame cost, goldens bit-identical) while giving Flutter's one-element-tree semantics through three small links.
