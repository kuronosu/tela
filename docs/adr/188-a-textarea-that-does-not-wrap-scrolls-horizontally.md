# ADR-188: A Textarea whose lines do not wrap scrolls horizontally

- **Date:** 2026-10-28
- **Status:** Accepted

## Decision

Amends ADR-069 and ADR-118 (details in `internal/core/field.go`, `fieldExtents` and `revealNow`).

- A Textarea whose white-space does not wrap (`text-nowrap`, `whitespace-pre`, `whitespace-nowrap`) has a horizontal extent: its widest line (or placeholder) past the content width, plus room for the 1 px caret after it when a left-to-right field has no end padding (Blink's end padding for an editable scroll container). Overflowing lines are start-aligned, so a right-to-left field overflows to the left. Wrapping fields keep no horizontal extent.
- The caret is revealed on both axes ("nearest"); horizontally it is kept inside the content box (the padding counted on its sides) as far as the extents allow, so Home scrolls back to 0. A selection drag autoscrolls horizontally too.
- With classic scrollbars, an edit that makes an `overflow: auto` axis start or stop overflowing lays the field out once to add or drop its gutter (both axes). Overlay scrollbars (the default) still never lay out on a keystroke.
- A pointer press that focuses a field no longer reveals the caret the field had: the press places the caret where the text is shown and reveals that one (a label's press still reveals its control's caret).

## Rationale

Chromium scrolls such a textarea horizontally; before, its extent was always 0 and long lines were clipped (`examples/notes` with word wrap off). Revealing the old caret on a pointer focus scrolled a scrolled field back to its start under the pointer.
