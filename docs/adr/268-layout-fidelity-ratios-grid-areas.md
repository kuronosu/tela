# ADR-268: Layout fidelity: aspect ratios, grid min sizes and template areas

- **Date:** 2026-11-11
- **Status:** Accepted

## Decision

Phase 23 layout fixes, each measured in Chromium 155 (amends ADR-073, ADR-157, §9, §10). **Aspect ratios:** min and max cross sizes transferred through a flex item's ratio clamp only a base size measured from content and the content size suggestion, never a definite size or the hypothetical or flexed one (two `w-full aspect-square max-h-[20px]` in 280 px: 140×20, were 20×20); a block, flex or grid container with a ratio and a known width takes its height from it, and taller content grows it while min-height is auto and it is not a scroll container (css-sizing-4's automatic minimum; a `flex aspect-square` box in a block had its content's height); a column item with a ratio and no definite width is measured at its fit-content width (30×30 in a `w-fit` column, was 5×5). **Grid:** a percentage min size of an indefinite area counts as 0, not auto, as for flex items. **Areas:** `[grid-template-areas:'a_b'_'c_d']` (single or double quotes) and `[grid-area:…]` join §10's arbitrary properties; `col-[name]`/`row-[name]` already place by area. No typed prop: templates are static.

## Rationale

Chromium is the reference (ADR-043); each rule has a `testdata/cases` fixture that failed before. Tailwind v4 has no utility for areas, so the arbitrary property is what an LLM writes in React + Tailwind (§3).
