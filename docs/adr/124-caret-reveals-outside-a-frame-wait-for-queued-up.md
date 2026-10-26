# ADR-124: Caret reveals outside a frame wait for queued updates, and a frame with nothing to lay

- **Date:** 2026-10-22
- **Status:** Accepted

## Decision

Caret reveals outside a frame wait for queued updates, and a frame with nothing to lay out runs its layout effects before its reveals (amends ADR-118 and ADR-122). **Queued updates**: outside a frame a caret reveal runs at once only when no frame work and no state or Value update is queued (`revealReady`, the test ADR-122 gave the reveals of the passive effects); otherwise it waits for the frame that applies them, as for pending frame work. Before, a handler or a posted function that set the state narrowing a field and then called `ElementRef.Focus` or `SetSelectionRange` revealed the caret with the old width, and the frame that narrowed the field did not reveal again: the caret at the end was clipped away. **Idle frames**: a frame with nothing to build or lay out runs the layout effects of builds that changed nothing (a `Mount` of the same content, such as a tela dev reload of the same code) first, then the reveals held at its start or asked for by those effects; it stays idle unless a reveal scrolls a field or the effects left work, which it does itself: a paint pass, or the whole pipeline, whose layout the held reveals then wait for. Before, it ran the reveals before those effects, so a reveal they asked for waited for an extra frame, and a focus they moved to an element that is not a text field left the tree dirty with no frame requested. `FlushEffects` alone reads whether its caller holds the reveals, and passes it to `runPassive`

## Rationale

Review findings of phase 9. A queued update is frame work the layout has not seen, whoever asks for the reveal; the frame that applies it comes anyway, so the click, key and posted cases still take one frame and one paint. The layout effects of a frame belong to it: the idle frame does the work they leave at once (one frame and one paint, where it took an idle frame and a requested one) and never strands it. Every golden is unchanged
