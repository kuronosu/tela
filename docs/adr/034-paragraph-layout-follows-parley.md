# ADR-034: Paragraph layout follows Parley

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Paragraph layout follows Parley: shape once per paragraph, break lines many times without reshaping (line ends keep their kerning); line boxes are quantized to logical px (ascent, descent, half-leading rounded down, like Blink at 1x); where CSS and convenience differ, CSS wins (the ellipsis uses the block's style, tab stops the block's font, no letter-spacing in cursive scripts); line breaking is UAX #14 without Blink's ASCII pair table

## Rationale

Cheap re-breaking is what flex/grid measuring needs; matches Chromium within 0.052 px on 34 of 38 reference paragraphs, the rest differ by at most 0.4 px at line ends or in URL break points
