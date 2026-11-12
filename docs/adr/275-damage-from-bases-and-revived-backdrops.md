# ADR-275: Damaged rectangles rendered from a base; past backdrop outputs found again

- **Date:** 2026-11-11
- **Status:** Accepted

## Decision

Phase 24, CPU performance. Amends ADR-061 (the framebuffer is the only raster cache; the matcher gives up), ADR-200 (the backdrop cache) and ADR-241 (memory at rest).

- **Bases.** A damaged rectangle whose pixels differ only from list position p on is rendered from p (`raster.Options.From`) over a base holding what it reads as the commands before p leave it: the framebuffer, when the old frame's commands there are all matched before p (a scrim or dialog laid over the page; spoiled parts are split off), or a `raster.Snapshot` the previous frame took (`Options.Snap`), when the commands before both positions are matched one to one there (rows scrolling over a fixed glass pane, a dialog animating over the page). Core checks it (`damagebase.go`). Snapshots are taken where what lies under p is costly, with a 16 px margin, at most a framebuffer of them, dropped by `TrimRaster`.
- **Matching** no longer gives up: past `matchLookahead` it resumes at the next equal draw command; the fallback is the cost alone (a rectangle from a base counts a quarter).
- **Occlusion.** The damage of unmatched commands inside the fully covered pixels of a later matched opaque cover (an opaque fill or linear gradient, or an opaque image, under clips and translations only) is dropped (`occlude.go`): a root background that follows the scheme under a wallpaper that does not no longer makes every pane blur its unchanged backdrop again.
- **Revive.** `BackdropCache` keeps the outputs of filters whose input changed (at most 8; those of inputs that did not rest age like spares; Trim frees all); `Revive(canvas)` returns one when the commands under its reads are equal again (a theme switched back).

## Why

Dashboard at 1080p, base and new interleaved on a loaded i5-10400: `BenchmarkScrollFrame` 31-40 → 17-20 ms at x1 (full frame 31-61), 91-111 → 40-47 ms at x2 (full 99-142); the palette's scrim 18-32 → 6.2-6.5 ms, x2 63-91 → 24 ms; session CPU 14.5 → 10.8 s at x1, 49.6 → 33.7 s at x2, full frames 12 → 10. A theme switch stays a full frame (its wallpaper changes); switched back without a rest 34 → 21-27 ms at x1. A scheme switch under glass over a background the same in both schemes (`BenchmarkSchemeSwitchUnderGlass`, 600x400) 10-11 → 2.9-3.2 ms at x1 (forced full frame 8.2-9.3), 25.6 → 8.8-10.1 ms at x2 (full 21), no backdrop pixel filtered. Pixels are those of a full render (damage-vs-full tests, `TestRenderFrom`).
