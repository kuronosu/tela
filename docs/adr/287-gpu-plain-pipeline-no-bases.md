# ADR-287: A plain pipeline, and no bases on the GPU

- **Date:** 2026-11-16
- **Status:** Accepted

## Decision

ADR-282's fixed shaders gain a *plain* pipeline and its *opaque* variant without blending: the insides of solid fills, and of layers and cached backdrops composited source-over without a mask (instances already marked `FlagFull`), are drawn by a small shader instead of the quad shader. Opaque fills, opaque cached backdrops at full opacity and pixel copies use the variant. Pixels are bit-identical to the quad pipeline's (`TestPlain`).

ADR-275's bases do not carry over to the GPU: no snapshots and no framebuffer base. A damaged rectangle is always rendered from the start of the list.

## Rationale

On the UHD 630 the quad shader's cost per pixel bounds the insides: backdrop paint went from 0.42 to 0.78 Gpx/s and solid insides from 1.6 to 2.1 Gpx/s. The dashboard at 2880x1800 x2 went from 47 to 25–26 frames over 16.7 ms of 704; the RTX 3060 Ti stayed at 1–2.

Bases were built and measured with frames bit-identical to full ones, then removed. With snapshots, frames got worse (30–31 over) and peak memory rose from 93 to 199 MiB: a snapshot costs about one fill per pixel there, and most were never used. A framebuffer base alone made opening frames cheap but moved the backdrop work to the next frame (no change).

## Changes

ARCHITECTURE §13 (the shader list). `internal/gfx/gpu/shaders/plain.frag`. Amends ADR-282 and ADR-275 (CPU only).
