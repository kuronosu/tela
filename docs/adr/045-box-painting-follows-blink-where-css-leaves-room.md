# ADR-045: Box painting follows Blink where CSS leaves room

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Box painting follows Blink where CSS leaves room: border boxes and border/outline widths snap to device pixels (`floor(edge*scale+0.5)/scale`; widths floored, at least 1 device px), radii and text do not; outer shadows under an opaque border-box background are clipped 1 device px inside the box and backgrounds under opaque rounded borders shrink 1 device px (bleed avoidance); dashes follow Blink's gap fitting. Stacking contexts follow CSS (z-index applies to positioned boxes and flex items; opacity, transform, filter, backdrop filter, blend mode and `isolate` create one) with the simplified painting order of `internal/core/paint.go`. `BackdropFilter` reads only the pixels inside its shape's bounds and mirrors them at the edges, like Chromium; a list that uses it rasterizes on one goroutine

## Rationale

Measured against Chromium on a 56-cell gallery at scales 1 and 2; §13's one-line stacking rule was looser than CSS
