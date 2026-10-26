# ADR-041: Fusion safety

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Fusion safety (ADR-027) covers every layer whose results reach pixels: `gfx`, `raster`, `text`, `style` and `layout`. The deterministic elementary functions move to `internal/fmath` so all of them can use it; `math.Sincos`, `math.Tan` and `math.Hypot` are not used on paths that reach pixels (`gfx.Rotate`, `Skew`, `HueRotate`, `Point.Len` and COLR transforms switched)

## Rationale

Measured: `math.Sincos` and `math.Hypot` give different bits on arm64 than on amd64, even after rounding to float32; gradient geometry, transforms and layout feed the rasterizer
