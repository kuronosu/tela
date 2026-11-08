# ADR-241: The rasterizer's scratch memory follows recent frames

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Amends ADR-078 (the barrier windows of a phased render) and ADR-200 (the cache's spare buffers).

- **Aging.** `raster.Renderer.EndFrame()` releases the scratch of a Renderer and of the Renderers it owns (free lists, blur planes, shadow tables, the grown region's copy) that no Render used in the last 300 frames (5 s of continuous frames: it bounds the memory of a window that never rests, such as one with an endless animation). `BackdropCache.EndFrame()` does the same for spare buffers. Core's `RasterizeFrame` calls both after every frame.
- **Deferred trim.** `Renderer.Trim()` and `BackdropCache.Trim()` release all of it; the cache's entries stay. The Host calls them through `Tree.TrimRaster` `RasterSettle` (1 s) after a frame that left nothing pending, if there is memory to release, with the same deadline mechanism as the framebuffer trim (ADR-242). Any frame moves or cancels it. That is one timer wake-up after the last frame, and none while frames come or when nothing would be released.
- A free buffer serves a request only if it wastes at most the request's size (beyond 64 KiB).
- A phased render returns each barrier's filtered window once every band has drawn it: one window at a time instead of one per filter.

## Why

Every free list kept its high-water buffers: after the dashboard's session at 3840x2160 the heap stayed at 388 MiB (raster scratch 164 MiB, backdrop cache 58 MiB) with the page at rest. Releasing on the idle frame itself got it down, but the end of every hover or transition is an idle frame, so the next interaction allocated everything again (session allocations 1066 → 1543 MiB). Deferring the release to a rest of one second keeps the low memory without that churn (the session at x2 allocates 1284 MiB, against 1047 before this change; its heap at rest is 163 MiB either way). A short age adds churn of its own: at 15 frames it is 1423 MiB, because buffers an interaction needs every few hundred milliseconds are released between uses. Pixels do not change: buffers are zeroed or fully written before they are read.
