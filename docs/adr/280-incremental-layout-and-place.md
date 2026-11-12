# ADR-280: Frames lay out and place what changed

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-043, ADR-061 ("frames that build, restyle or lay out re-place … every layer") and ADR-204.

## Decision

- **Layout styles:** a restyled box maps its own layout style and its children's, not its siblings' (a sibling's reads its parent; a moved last element changes the parent's children).
- **Content cut-off** (`layout.Engine.MarkContent`): a leaf whose text, image or field text changed is laid out again for every request its cache holds; if each result holds, its ancestors keep theirs, else its parent (≤ 32 children, no escaping absolute boxes, not a paragraph with atoms, a complete cache) is checked the same way, up to a budget; otherwise `MarkDirty` from there. Grid items are sorted through an index permutation.
- **Partial places** (`internal/core/place.go`): a frame that builds, restyles or lays out places the marked boxes (restyles, animation steps, new layouts through `layout.ChangeObserver`, children, paragraphs, images, layout styles) and the way to them; a box places all its children when its origin, paint style, children or content space changed, its whole subtree when its containment, position or content space changed. Scroll containers, sticky, floating and layout-animated boxes are placed on every place; a change of the sticky or floating set, or new children above one, places everything. A box with more than 64 children keeps its children's sums by runs.
- **Recording:** a list culls a run of 64 children whose inks miss its interest rect as one dependency; a stacking context collects through a box with many children only the children with participants (`colIn` counts changes of that set).
- `TELA_CHECK_LAYOUT=1` (on in core's tests) checks layout styles, layouts (a fresh `layout.CheckFull`) and places (a full place) after each frame.

## Why

A plain 2000-row `kit.Table` walked, mapped, placed and recorded all 24,000 boxes on every frame that was not a scroll (ROADMAP phase 24).

## What it changes

`Stats` gains `LaidOut`, `Remapped`, `Placed`; a box is ~120 bytes larger. Measurements are in the phase's report and `kit/table_perf_test.go`.
