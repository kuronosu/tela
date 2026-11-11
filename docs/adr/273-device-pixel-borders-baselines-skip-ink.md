# ADR-273: Border widths snapped in layout, half-pixel rounding, skip-ink

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-045, ADR-029 and ADR-159.

## Decision

- **Border widths:** `border-*-width`, `outline-width` and `outline-offset` are snapped "as a border width" (CSS Values 4) at the display scale, in layout as in paint: a width of at least one device pixel is floored to whole device pixels, a smaller non-zero one is one device pixel; an offset keeps its sign. `core` snaps when it maps a style to layout and when it lays out the edges of inline elements; `SetScale` maps bordered boxes again and assembles again the paragraphs of bordered inline elements. Styles stay in unsnapped logical px.
- **Half device pixels:** box edges, glyph baselines and underlines round to whole device pixels with halves up through `gfx.RoundDevice`, which counts a value within `DeviceTolerance` (1/1024 px, raster's grid tolerance) below a half as the half.

## Why

Blink snaps border widths when it computes the style, at the device scale factor (`ConvertBorderWidth`): at x1.5 a 1px border is 0.667 px wide, so the boxes inside and after it move. ADR-045 snapped only the painted widths: a 1px border moved content by 0.2 to 0.33 px at x1.25 and x1.5. `TestChromiumBorderScale` (Chromium 155, x1.25 and x1.5): every edge within 0.04 px of Chromium's (was 3.97).

Blink's layout is fixed point, so a position n + 1/3 px at x1.5 is an exact half device pixel, and Skia rounds it up; Tela's float32 sum can be a hair below it and was drawn one row higher (ADR-159's known case, a third of the half-pixel baselines at 1.25, 1.5 and 1.75). `TestChromiumHalfPixelBaselines` (`internal/text`): every cell on Chromium's row (19 of 120 cells were one row higher).

## What it changes

Layout at scales other than 1 and 2, sub-pixel widths (`border-[0.5px]` is 1 px at x1), edges and baselines on half device pixels; goldens at 1.25 and 1.5, and a few edges at x1.
