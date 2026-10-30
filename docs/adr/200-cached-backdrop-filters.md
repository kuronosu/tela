# ADR-200: Cached backdrop filters; damage under glass is not grown by content above it

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

Amends ADR-028 (blur results not cached) and ADR-078 (damage closure). **Damage:** core's backdrop closure counts, for a backdrop filter, only the damage of what is painted before it in the list (each damaged rectangle records where in the new frame its pixels start to differ; an old command takes the place of the next matched one): a change of a glass element's content, painted after its filter, no longer grows the damage by the filter's reach. **Cache:** `raster.BackdropCache` (`Options.Backdrops`) keeps, per backdrop filter that spreads pixels and lies outside every layer, its filtered output in device pixels, only the part computed from exact pixels, within two framebuffers' worth of memory. Core owns one per framebuffer: `frameDamage` carries to the next frame (`Retain`) the filters whose input did not change (matched with an equal command and bounds, outside shifted units, reading no damaged or shifted pixel painted before them), also when the matching gives up and the frame is full; forced full frames and scale or size changes reset it. A filter whose output inside the region is cached is drawn from it and does not grow the region (`BackdropCache.Region`, also the full-frame fallback's cost); one whose input was unchanged for a frame but whose cached part does not cover the region computes all of its output once. The rasterizer checks an entry's transform, device bounds, filters and list bounds; the input is the caller's word.

## Rationale

Output pixels of a filter do not depend on the region (ADR-078), so cached ones are bit-identical (damage-vs-full tests in raster, core and the dashboard). Measured on `examples/dashboard` (i5-10400, 12 threads, `TELA_DEV=0`): session p50/p95 at 1080p x1 15.0/34.4 → 4.2/10.7 ms (the damage fix alone 11.9/29.7), x2 48.5/111 → 9.5/32 ms; a card drag at 1416x846 x2 48 → 18.6 ms per move (1.14 → 0.43 Mpx); `BenchmarkScrollFrame` (a full frame: the table moves most of the window) 37 → 28 ms at x1, 108 → 83 ms at x2; `BenchmarkFullFrame` (forced, uncached) unchanged.
