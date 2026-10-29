# ADR-191: The plain outside-press listener is removed

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

Amends ADR-096 (codebase review 2026-10-29, C20). `core.InteractOutside`/`core.OutsideOptions` (a listener told of every press outside an element, whether or not it is an overlay) and `core.IsTopOverlay` are removed. Presses outside reach only the top overlay, through `core.Overlay`'s `OnInteractOutsideStart` and `OnInteractOutside`, with the same rules as before: what counts as a press (ADR-167), inside meaning the element tree, a drag from outside to inside ending nothing, capture-like StopPropagation and PreventDefault, and `KeepOutsidePress`.

## Rationale

Package `aria` uses only `core.Overlay`; the plain listener and `IsTopOverlay` had no caller outside `internal/core`'s tests, yet kept a second entry into the press path. A press now records at most one overlay.

## What it changes

No behavior change for `aria`, `kit` or apps. The press rules move into the top comment of `internal/core/overlay.go`, and their test is ported to an Overlay (`TestOverlayPressOutside`). ARCHITECTURE §14 no longer lists interact-outside apart from the overlay stack.
