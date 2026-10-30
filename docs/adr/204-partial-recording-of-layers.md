# ADR-204: Partial recording of layers

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

Amends ADR-061 ("frames that build, restyle or lay out re-place and re-record every layer") and the "Not changed" of ADR-161. A paint records again only the layers whose lists can have changed (`internal/core/record.go`). While a list is recorded, the painter notes the boxes it reads: their inputs (`dep`: paint style, geometry, children, paragraph, image, field), their placement (`odep`: display, participation and z-index, stacking, deferred layer, scroll space, children, overflow clip) and the outcome of each cull test (`cdep`). Each box keeps both as of its last place with the epoch of their last change; places compare them for the boxes some list reads, color transition steps and `BgFn` stamp the box directly. Dirty layers under clean ancestors are recorded again in place from what their opening needed (kind, lead box, hoisted box and its stacking context, space, interest rect, culling); a child opened again keeps its list when clean. An interest rect is kept while it holds the visible part. Full recordings stay for the first paint, a new scale, viewport or canvas, mounted `TextColorFn`s, and layers that stop or start being layers. `TELA_CHECK_LAYERS=1` compares every partial frame with a full recording, command for command.

## Rationale

A hover, a transition step or a text change recorded the whole page. Measured (i5-10400, `TELA_DEV=0`, pipeline to the composed frame): 120 positioned cards at x2, a color transition step 107 → 2 layers, 0.74 → 0.06 ms; a card's new text 107 → 2, 1.0 → 0.35 ms; a hover 107 → 3, 0.85 → 0.18 ms; the gallery's Calendar page, a hover 359 → 2 layers, 1.45 → 0.41 ms; the dashboard board's drag at 1416×846 x2 with `transition-colors` back on the cards, 13.8 → 0.6 layers per frame, a fade frame 0.75 → 0.14 ms (its raster, ~19 ms, is the dragged card's). Costs: 352 bytes per box; a frame that places 10,000 boxes +6%, a 10k-list scroll frame +3 µs. Same pixels: damage-vs-full tests, goldens and the list check (always on in core's tests) across core, kit and examples.
