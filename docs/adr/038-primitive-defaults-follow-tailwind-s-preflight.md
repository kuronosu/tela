# ADR-038: Primitive defaults follow Tailwind's Preflight

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Primitive defaults follow Tailwind's Preflight: no margins or padding, `border: 0 solid` in the current color, unstyled headings and lists (they inherit size and weight), Inter at 16px with `line-height: 1.5` as the root text style, block images. `Button` keeps Preflight's inherited font and transparent background and, being inline-block in browsers, takes its fit-content width in block flow

## Rationale

The same markup looks like a Tailwind page in a browser, which is what users and LLMs expect and what the phase-3 Chromium comparison checks
