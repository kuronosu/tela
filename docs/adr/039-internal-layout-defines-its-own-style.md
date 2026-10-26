# ADR-039: `internal/layout` defines its own `Style`

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

`internal/layout` defines its own `Style` (a Taffy-like subset) and node interface; `core` maps `ComputedStyle` onto it. Leaves (text, images) are measured by a callback given the available space (definite, min-content or max-content). Conformance uses Taffy v0.14.0's fixtures (block, flex, blockflex, leaf) with expected boxes measured by headless Chromium, as Taffy's gentest does, committed as JSON; fixtures needing margin collapsing, `content-box` or features of later phases are excluded with a recorded reason

## Rationale

`layout` is built and tested without `style` or `core`; Chromium is the ground truth Taffy itself uses
