# ADR-188: A Textarea whose lines do not wrap scrolls horizontally

- **Date:** 2026-10-28
- **Status:** Accepted

## Decision

Amends ADR-069 and ADR-118 (details in `internal/core/field.go`, `fieldExtents` and `revealNow`).

- A Textarea whose white-space does not wrap (`text-nowrap`, `whitespace-pre`, `whitespace-nowrap`) has a horizontal extent: its widest line (or placeholder) past the content width, plus room for the 1 px caret after it when a left-to-right field has no end padding (Blink's end padding for an editable scroll container). Overflowing lines are start-aligned, so a right-to-left field overflows to the left. Wrapping fields keep no horizontal extent.
- The caret's rect is revealed on both axes ("nearest", Blink); a caret at an end of the horizontal overflow also brings the padding there (Home scrolls to 0, End on the longest line to the extent), unless the pointer placed it: a click never scrolls. A selection drag scrolls a Textarea toward the pointer (Blink's `LayoutBox::Autoscroll`), so it reaches the extents past a short line or the bottom padding.
- With classic scrollbars, an edit that makes an `overflow: auto` axis start or stop overflowing lays the field out once to add or drop its gutter (both axes). Overlay scrollbars (the default) still never lay out on a keystroke.
- A press on a field that focuses it does not reveal the caret the field had, since it places one under the pointer (`focusState.pressCaret`); any other focus, a label's press included, does.

## Rationale

Chromium scrolls such a textarea horizontally; before, its extent was always 0 and long lines were clipped (`examples/notes` with word wrap off). Revealing the old caret on a pointer focus scrolled a scrolled field back to its start under the pointer.
