# ADR-046: Colors are extended sRGB

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Colors are extended sRGB (CSS Color 4) from parsing to rasterization: OKLCH/Oklab convert without gamut mapping, solid paints clip per channel, and gradients (Oklab and sRGB interpolation) interpolate unclipped premultiplied stops and clip each pixel. Color conversions use `internal/fmath` and are pinned by a digest (ADR-041). Supersedes phase 1's CSS Color 4 gamut mapping

## Rationale

It is what Chromium 152 paints: the 286 Tailwind colors match within one rounding level and gradient interiors within 1 level, against up to 47 levels inside gradients with gamut-mapped stops; a third of Tailwind v4's palette is outside sRGB
