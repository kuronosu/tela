# ADR-265: Text drags and fallback-font baselines in fields

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-069, ADR-159 and ADR-259.

## Decision

- **Autoscroll:** a drag hovering within 20 px of a field's scrolling box (an Input's content box, a Textarea's border box without scrollbars) scrolls it from 0.2 s on, each frame bringing the point 20 px past the pointer into view (Chromium's drag-and-drop `AutoscrollController`). Input horizontal, Textarea both axes; the delay is a deadline; the drop caret follows.
- **Drag image:** the selected text alone in its own colors at 75% opacity, unclipped by the field (`DragImageForSelection`, `kDragImageAlpha`), painted on the UI thread from the field's layout.
- **Undo:** a move inside one field is one step: `edit.Command.JoinUndo` links the `insertFromDrop` to its `deleteByDrag` (`DragAndDropCommand`).
- **Baselines:** with `line-height: normal` an Input centers its laid-out line (taller when it falls back to Noto Sans Arabic) at its empty-line height; the placeholder sits on the value's baseline. Measured in Chromium 155 (`TestChromiumInputFallbackBaselines`).
- **Touch:** no long-press drag. No backend reports touch pointers (Win32 and X11 emulate a mouse, which drags as one; Wayland's `wl_touch` is unbound), `StartDrag` needs a held button, and there is no touch selection; it waits for touch input.

## Why

Chromium is the reference for fields; matching it fixes Arabic text sitting five pixels low.
