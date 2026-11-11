# ADR-273: Border widths snapped in layout, half-pixel rounding, skip-ink

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-045.

## Decision

- **Border widths:** `border-*-width`, `outline-width` and `outline-offset` are snapped "as a border width" (CSS Values 4) at the display scale, in layout as in paint: a width of at least one device pixel is floored to whole device pixels, a smaller non-zero one is one device pixel; an offset keeps its sign. `core` snaps when it maps a style to layout and when it lays out the edges of inline elements; `SetScale` maps bordered boxes again and assembles again the paragraphs of bordered inline elements. Styles stay in unsnapped logical px.

## Why

Blink snaps them when it computes the style, at the device scale factor (`ConvertBorderWidth`): at x1.5 a 1px border is 0.667 px wide (getComputedStyle reports it), so the boxes inside and after it move. ADR-045 snapped only the painted widths, and at x1.25 and x1.5 a 1px border moved content by 0.2 to 0.33 px (up to 4 px after six boxes in a row). `TestChromiumBorderScale` (Chromium 155, x1.25 and x1.5): every edge within 0.04 px of Chromium's (was 3.97).

## What it changes

Layout at scales other than 1 and 2, and sub-pixel widths (`border-[0.5px]` is 1 px at x1); goldens at 1.25 and 1.5.
