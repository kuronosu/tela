# ADR-023: Each gradient carries its interpolation color space

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Each gradient carries its interpolation color space (sRGB, Oklab; Oklch later), Oklab by default. Compositing stays in sRGB (ADR-007)

## Rationale

Tailwind v4 gradients interpolate `in oklab`; matching it keeps gradients identical to the web
