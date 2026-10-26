# ADR-060: Virtualization

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Virtualization (§9). `h.VirtualList(count, estimateSize, renderItem, props...)` is a vertical scroll container component whose props style the container; `h.Overscan` (default 5) and `ElementRef.ScrollToIndex(i, tela.ScrollAlign)` with `AlignAuto`/`Start`/`Center`/`End` (TanStack semantics, kept aligned while measurements move the item). Items are keyed by index in block-flow wrappers; sizes (estimates, then measured border-box heights) live in a Fenwick tree with float64 sums (no products, ADR-041). An offset change rebuilds a list only when its visible items ±1 leave the built range (hysteresis: most scroll frames are paint-only); the scroll phase measures, anchors with TanStack's rule (items starting above the viewport shift the offset) and rebuilds and re-lays out within the frame (at most 6 passes). Count changes keep sizes per index; only a new estimate function (code pointer) resets them. The focused item ±1 stays mounted

## Rationale

TanStack Virtual's model with synchronous windows: 100,000 rows mount in 2.6 ms with a 1.9 MB heap and keep p95 pipeline time at 1.1 ms per scroll frame. The API shape was chosen while the owner was away (to review)
