# ADR-118: A text field reveals its caret after layout

- **Date:** 2026-10-22
- **Status:** Accepted

## Decision

A text field reveals its caret after layout (amends ADR-069). The caret reveal (an Input's own horizontal scroll, a Textarea's scroll container) needs the field's laid-out content box, so it waits for a layout whenever the layout is not current: in a frame from its build to the end of its scroll phase (the focus an AutoFocus field takes after the style phase, `ElementRef.Focus` in a layout effect, a caret moved by a controlled `Value` in the build) and outside a frame while a frame's work is pending (a key that resizes a `field-sizing-content` Input). The fields queue once (`Tree.frev`) and are revealed after the scroll phase, before paint, so the frame shows the result; a reveal with a current layout still runs at once. Before, an AutoFocus Input was revealed with a content width of zero: one whose value fits stayed scrolled by the caret's 1 px (its text 1 px to the left and the caret at the start clipped away, as in the signup, aria and dashboard example goldens, regenerated), a caret moved to the end before the first layout showed none of the text, and a Textarea's reveal outside a frame with pending work was dropped

## Rationale

A width read before the layout that sets it is zero or stale; deferring to the layout of the same frame costs nothing visible (paint follows it) and keeps reveals with a current layout immediate; every golden of `internal/core` is unchanged
