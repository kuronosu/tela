# ADR-241: The rasterizer's scratch memory follows recent frames

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Amends ADR-078 (the barrier windows of a phased render) and ADR-200 (the cache's spare buffers).

- **Aging.** `raster.Renderer.EndFrame()` releases the scratch of a Renderer and of the Renderers it owns (free lists, blur planes, shadow tables, the grown region's copy) that no Render used in the last 15 frames. `BackdropCache.EndFrame()` does the same for spare buffers. Core's `RasterizeFrame` calls both after every frame.
- **Deferred trim.** `Renderer.Trim()` and `BackdropCache.Trim()` release all of it; the cache's entries stay. The Host calls them through `Tree.TrimRaster` `RasterSettle` (1 s) after a frame that left nothing pending, if there is memory to release, with the same deadline mechanism as the framebuffer trim (ADR-242). Any frame moves or cancels it. That is one timer wake-up after the last frame, and none while frames come or when nothing would be released.
- A phased render returns each barrier's filtered window once every band has drawn it: one window at a time instead of one per filter.

## Why

Every free list kept its high-water buffers: after the dashboard's session at 3840x2160 the heap stayed at 353 MiB (raster scratch 164 MiB, backdrop cache 58 MiB) with the page at rest. Releasing on the idle frame itself brought it down, but the end of every hover or transition is an idle frame, so the next interaction allocated everything again (the session's allocations went from 1066 to 1543 MiB, before the fonts' lazy outlines; 1047 MiB is the same base with them). With the release deferred to a one-second rest the session allocates 1359 MiB, and its heap at rest is 163 MiB. The 15-frame aging accounts for most of the remaining churn, and keeps the heap during the session at most 211 MiB (339 MiB before). Longer ages allocate a little less but hold more than before during activity (60 frames: 1355 MiB, 391 MiB). Rejecting oversized free buffers for small requests was tried and dropped: it allocated more and saved nothing once aging runs. Pixels do not change: buffers are zeroed or fully written before they are read.
