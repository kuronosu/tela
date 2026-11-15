# ADR-282: GPU renderer

- **Date:** 2026-11-15
- **Status:** Accepted

## Decision

`internal/gfx/gpu` renders a `gfx.DisplayList` with the meaning `raster` gives it: the same options (scale, region, clear), premultiplied RGBA8 in sRGB, source-over on the encoded values (ADR-007), into a target that keeps its pixels between frames, so damage works as on the CPU.

**Two halves.** An *encoder* without build tags turns a list into a frame: passes over targets, batches of instances, and the textures they need. It knows no graphics API and is tested everywhere. A *device* (Vulkan now; Metal and D3D11 later) executes frames and is compiled only with `telagpu`.

**Fixed shaders**, no variants generated at run time:

- *Quad:* one instanced pipeline for rectangles and rounded rectangles (analytic coverage), rounded-rectangle shadows (the closed-form erf of ADR-017, inset too), images and coverage masks, with a solid or gradient paint. Gradients read ramps the CPU bakes with the rasterizer's own code, so stops and Oklab interpolation match.
- *Coverage atlas:* glyphs, filled paths, strokes and path clips are rasterized by the CPU rasterizer into an 8-bit atlas, cached, and drawn as quads. Glyphs use the rasterizer's cache keys, so text has the same coverage on both backends.
- *Blur* (separable, reduced resolution for large radii), *color matrix*, *blend* (W3C modes, reading a copy of the backdrop) and *composite* (opacity and masks) for layers and backdrop filters, on pooled offscreen targets.

Clips that are device-pixel rectangles are scissors; the others are coverage that multiplies what is drawn inside them.

**Tolerance.** GPU output is compared with the CPU rasterizer's for the same list, in the form of `telatest.Tolerance`: per scene, a largest channel difference for all pixels but a small share, and a hard limit for those. The numbers live in the comparison test (`internal/gfx/gpu`), start at 2 levels for 99.5% of the pixels and 24 for the rest, and only tighten. A scene whose difference is the rasterizer's own approximation (rounded corners under rotation, flattened at 0.1 px; shadows under a skew, blurred in device space) or a pixel center exactly on a gradient discontinuity has an override there, with its cause. Goldens stay CPU-only.

## Rationale

Paths on the GPU (stencil and cover, sparse strips) are the costly part of a GPU renderer, and UI paths are small and mostly static: icons, strokes, glyphs. Reusing the CPU coverage makes them exact and leaves the GPU what it is good at: fills, gradients, shadows, blurs, layers and compositing at high resolution. One quad pipeline keeps a frame to a few draw calls, as GPUI's quads do. An encoder apart from the device lets CI without a GPU test most of the logic, and a second device reuse it.

## Changes

ARCHITECTURE §13 (GPU backend). The CPU rasterizer exports what the encoder shares (gradient ramps, glyph and path coverage).
