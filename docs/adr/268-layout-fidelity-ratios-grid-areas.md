# ADR-268: Layout fidelity: aspect ratios, grid, inline out-of-flow boxes, template areas

- **Date:** 2026-11-11
- **Status:** Accepted

## Decision

Phase 23 layout fixes, each measured in Chromium 155 (amends ADR-073, ADR-157, ADR-176, §9, §10). **Ratios:** limits transferred through a flex item's ratio clamp only content-based sizes (two `w-full aspect-square max-h-[20px]` in 280 px: 140×20, were 20×20); a block, flex or grid container with a ratio and a known width takes its height from it, grown by taller content while min-height is auto (css-sizing-4's automatic minimum); a column item with a ratio and no definite width is measured at its fit-content width (30×30 in a `w-fit` column, was 5×5). **Grid:** a percentage min size of an indefinite area counts as 0, not auto. **Inline (ADR-176's gaps):** an out-of-flow box in a paragraph has its static position on its line (a block-level one at the next line's start), and a `relative` inline element contains its absolute children, from its first fragment's left to its last's right (`layout.InlineAnchored`, `InlinePlacer`). **Textarea:** a growing one's min-content width is its longest word. **Areas:** `[grid-template-areas:'a_b'_'c_d']` and `[grid-area:…]` join §10's arbitrary properties; no typed prop, templates are static.

## Rationale

Chromium is the reference (ADR-043); each rule has a layout case or a Chromium gallery cell that failed before. Tailwind v4 has no utility for areas, so the arbitrary property is what an LLM writes (§3).
