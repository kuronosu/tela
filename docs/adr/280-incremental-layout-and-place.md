# ADR-280: Frames lay out and place what changed

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-043, ADR-061 and ADR-204.

## Decision

- A restyled box maps its own layout style and its children's, not its siblings'.
- `layout.Engine.MarkContent` (new text, image, field text) lays the leaf out again for its cached requests and stops at the first small ancestor whose results hold; a request's parent height is ignored for a node without percentage heights.
- The paint phase places the marked boxes (`internal/core/place.go`) and the way to them, a box's children when what they read of it changed; scroll containers, sticky, floating and layout-animated boxes every time. Boxes with over 64 children keep their sums, culls and participants by runs of 64.
- `TELA_CHECK_LAYOUT=1` (on in core's tests) checks layout styles, layouts and places against full ones.

## Why

A plain 2000-row `kit.Table` mapped, placed and walked all 24,000 boxes on every frame that was not a scroll: a hover 30.7 → 1.1 ms, a header's text 17.6 → 0.17 ms, a px column's cell 125 → 1.0 ms (medians).

## What it changes

`Stats` gains `LaidOut`, `Remapped`, `Placed`; a box is 56 bytes larger; same pixels and recordings.
