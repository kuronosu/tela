# ADR-122: Caret reveals held by passive effects run when the effects end

- **Date:** 2026-10-22
- **Status:** Accepted

## Decision

Caret reveals held by passive effects run when the effects end (amends ADR-118 and ADR-120). `FlushEffects` still holds the reveals its effects ask for, but outside a frame it runs them right after the effects when the layout is still current (no frame work and no update queued), and only otherwise leaves them to the next frame, which it requests (a tree without a frame request reports it with `Dirty`); in a frame they still run after its layout. A held reveal no longer requests a repaint itself: a field is queued once, only a reveal that waits outside a frame for pending work requests that frame, a frame with nothing to build or lay out runs the reveals held at its start (passive effects, posted functions, scroll handlers) at its early-return check and stays idle unless one scrolls a field, and a frame or an effects run that ends early (a panic) with reveals held requests the next frame. Before, `ElementRef.Focus` in a `UseEffect` that changed nothing else revealed only in the next frame: until then the field was not scrolled, a host gave the IME a stale caret area (outside the window, then the right one), and a tree that ran no other frame kept it; and every held reveal forced a paint pass on its frame

## Rationale

Review findings of phase 9. ADR-120 only needs to wait for updates the layout has not seen; when the effects queued none, the layout is the one they ran with. Whoever holds reveals runs them, so a held reveal needs no repaint of its own. The frame and paint counts of the effect and frame-start cases are unchanged (the focus and the selection paint the field anyway), and every golden is unchanged
