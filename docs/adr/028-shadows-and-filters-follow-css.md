# ADR-028: Shadows and filters follow CSS

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Shadows and filters follow CSS: `DrawShadow` uses σ = blur/2 and the spread radius rule, and always clips with the box's coverage; the content of a filter layer is kept up to the filters' reach beyond the clip active at `PushLayer`, which cuts the filtered result; device σ is capped at 256 px and drop-shadow offsets at 512 px; blur results are not cached until layer caching arrives

## Rationale

Matches Chromium within ±3 levels away from anti-aliased edges on the Tailwind presets; an ancestor clip cuts a filter's output in CSS, not its input; the caps bound surface sizes. Blink skips the clip under an opaque background, so edge pixels can differ (up to 29 levels) until paint passes that fact on
