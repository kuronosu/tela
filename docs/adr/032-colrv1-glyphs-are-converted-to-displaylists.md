# ADR-032: COLRv1 glyphs are converted to DisplayLists

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

COLRv1 glyphs are converted to DisplayLists. Until `raster` has blend modes (phase 6), blend composites are approximated with source-over (`exactBlend` switch in `internal/text`); `SrcIn` with non-solid masks clips to the mask's outlines; two-point (focal) radial gradients are emulated with banded circles

## Rationale

Noto Color Emoji uses only SrcIn (296 glyphs) and SoftLight (262 flags) composites and 17 focal gradients; the approximations stay within mean 4 levels of Chromium. A focal radial gradient and a mask layer in `gfx` would make them exact
