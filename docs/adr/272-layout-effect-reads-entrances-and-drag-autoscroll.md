# ADR-272: Scroll reads in layout effects, one entrance per showing, drag autoscroll of containers

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-176, ADR-265, ADR-164 and ADR-081.

## Decision

- **`ScrollOffset` in a layout effect** reads the frame's extents and runs the focus reveal and the scrolls queued before it, in the scroll phase's order; later scrolls of the frame run at once, and a later layout only clamps. Running them beats predicting the offset: what is read is what is painted.
- **One entrance per showing:** ADR-164's second pass restarts the `starting:` entrance of a box whose own `display: none` ended in the frame, as of one mounted in it; a box with a layout animation that appeared in the first pass appears at its final place.
- **Drag autoscroll of containers:** the autoscrollable is the field under the drag if its text overflows, else the innermost scroll container up the box tree that the user can scroll and that overflows (none past a box fixed to the window). Its band is 20 px inside its border box without scrollbars; from 0.2 s each frame reveals the 1 px square 20 px past the pointer, at once, in it and up its containing-block chain.
- **Drag image:** a selection's text goes through the field's transforms as one matrix: a rotated field drags rotated text.

## Why

Chromium's source: `scrollTop` forces layout; `@starting-style` applies when rendering starts; `AutoscrollController`, `LayoutBox::FindAutoscrollable`, `CalculateAutoscrollDirection` and `Autoscroll`; `DragImageForSelection` paints with every transform.
