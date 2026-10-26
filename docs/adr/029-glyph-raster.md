# ADR-029: Glyph raster

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Glyph raster: `gfx` defines a `Font` interface (em-space outlines, bounds, COLR glyphs as DisplayLists) that `text` implements; raster caches A8 masks and color sprites keyed by font instance, device em size and pen x quantized to 1/4 device px, with the baseline snapped to whole device px. Rotation, skew, flips and device em sizes above 256 px fill outlines uncached and unsnapped. Under a gradient paint, color glyphs get a transparent foreground; color glyphs nest at most 3 levels

## Rationale

`gfx` stays independent of go-text; quarter-pixel x positioning is what Skia and browsers use, whole-pixel baselines keep text crisp; masks are bit-identical to filling the snapped outline, so cache state never changes pixels. Text whose transform switches between the two paths (an animated rotation) can jump by up to 1/8 px in x and 1/2 px in y at the switch
