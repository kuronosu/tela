# ADR-084: A box's opacity fades its backdrop filter

- **Date:** 2026-10-19
- **Status:** Accepted

## Decision

A box's opacity fades its backdrop filter (phase-6 review; supersedes the part of ADR-045 under which the backdrop was not faded by the box's own opacity). `gfx.BackdropFilter` gains `Opacity`: the filtered backdrop is painted with source-over at that opacity (`Builder.BackdropFilterOpacity`; `Builder.BackdropFilter` records 1, and a call at opacity 0 is dropped, reading nothing). Core draws a stacking context's backdrop filter at its opacity before the layer that carries that opacity (the layer is a backdrop root, so the filter cannot be inside it), and not at all at opacity 0; the content is then composited in the layer at the same opacity. This is Chromium's composition (the filtered backdrop at opacity a, then the content at opacity a), not the exact group composite of the spec, and the two differ when translucent content covers the backdrop: measured, Chromium matches the former. The raster scales the shaded span (`fadedShader`), leaving the buffer that the bands of a phased render share untouched; opacity 1 keeps the previous path bit for bit. The box's own `filter` and `mix-blend-mode` still do not apply to its backdrop

## Rationale

`Presence` fades glass dialogs (`starting:opacity-0`, `Exit("opacity-0")`) and tooltips rest at `opacity-0`: their backdrop blurred at full strength while invisible. Measured against Chromium 152 (`internal/core` `TestChromiumBackdropOpacity`, 7 panels over stripes at scales 1 and 2): `backdrop-blur-md` alone 0.000/0.002 mean levels per channel (max 0/1); with `opacity-50` 23.19 mean, max 77 before and 0.081/0.090, max 1 after; `opacity-50 bg-white/20` 20.99 → 0.369/0.388 (max 2); `opacity-0` 46.45 (max 155) → 0 exactly; `opacity-25 rounded-xl bg-sky-500/50` 30.13 → 0.260/0.259 (max 8/9 on the corners); `opacity-50` and `opacity-0` with `backdrop-grayscale` 17.72 and 35.57 → 0. The exact group composite would put the `opacity-50 bg-white/20` panel 5-7 levels away from Chromium on the stripes. Incremental frames stay bit-identical to full ones while panels fade (an opacity transition and an `OpacityFn` layer) at scales 1, 1.5 and 2, and phased renders equal sequential ones with faded filters
