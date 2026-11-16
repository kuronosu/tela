# ADR-286: GPU work not done

- **Date:** 2026-11-16
- **Status:** Accepted

## Decision

**Backdrop cache on the GPU.** Each window's surface keeps filtered backdrops in pool targets, within two targets' worth of pixels. Core's damage tells which backdrop filters kept their input (carried, as for `raster.BackdropCache`) and which changed only inside damage painted before them and not shifted (patched: every render of the frame writes the patch). An entry a render cannot write is dropped. Storing an entry advances `Frame.Res`, so a device that starts over never draws an empty one.

**The encoder skips work** with results independent of the Region: opaque fills, images and cached backdrops hide earlier commands unless something in between reads them; insides of fills, images and composites are drawn without shape or clip math, outer shadows skip their box; a one-rectangle opacity layer is drawn without a target; path coverage is cut into 16 px pieces, empty and full ones not rasterized, rows crossed only by vertical edges repeated when the first and last rows agree; clip masks are 64 px tiles.

**At rest** the renderer keeps what saves work and is bounded (coverage atlases, images on screen, cached backdrops) and releases what saves only an allocation (pool targets, staging and readback buffers, the shift scratch, images not on screen). Amends ADR-241 for the GPU.

## Rationale

Dashboard session at 2880x1800 x2 without a copy, frames over 16.7 ms of 704: RTX 3060 Ti 5 → 1–2, UHD 630 374 → 46–49 (p95 51 → 18 ms); the CPU path unchanged (99). The backdrop cache gives most of it (UHD 181 → 46 alone); a rest no longer costs the frame after it.

## Changes

ARCHITECTURE §13. `internal/gfx/gpu/backdropcache.go`, `cull.go`, `pieces.go`; core's damage reports patched filters. Amends ADR-200, ADR-282, ADR-284.
