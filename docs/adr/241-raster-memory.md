# ADR-241: The rasterizer's scratch memory follows recent frames

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Amends ADR-078 (the barrier windows of a phased render) and ADR-200 (the cache's spare buffers). A `raster.Renderer` and its workers keep their scratch (free lists, blur planes, shadow tables, the grown region's copy) only while frames use it: `Renderer.EndFrame(idle)` releases what no Render used in the last 15 frames, and with `idle` what the ending frame did not use. A free buffer serves a request only if it wastes at most the request's size (beyond 64 KiB), so a window-sized buffer does not stay alive for small masks. A phased render returns each barrier's filtered window once every band has drawn it, so the next barrier reuses it: one window at a time instead of one per filter. `BackdropCache.EndFrame(idle)` frees the spare buffers no entry took (all of them when idle); entries stay, and the budget stays two frames. Core's `RasterizeFrame` calls both after each frame, idle when nothing animates, nothing is dirty and no frame was requested.

## Why

Every free list kept its high-water buffers forever: after the dashboard's session at 3840x2160 the live heap stayed at 374 MiB (raster scratch 164 MiB, backdrop cache 58 MiB) though the page was at rest. With this change it is 199 MiB (scratch 3 MiB, cache 30 MiB, the entries of the filters on screen). Pixels do not change: buffers are zeroed or fully written before they are read.
