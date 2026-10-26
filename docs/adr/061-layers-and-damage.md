# ADR-061: Layers and damage

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Layers and damage (refines §13 "Layers and caching" and "Damage tracking"; updates ADR-057's last sentence and ADR-045's region note). Layers are display lists, not raster surfaces: the root; each scroll container whose content can move (its content and the positioned boxes that scroll with it, painted at unscrolled positions in their scroll space and moved by the compositor, recorded within an interest rect of the visible part plus one scrollport per scrolling axis); each stacking context with paint-phase deferred props (the compositor applies their effects); and scrollbars. Frames that build, restyle or lay out re-place and re-record every layer; paint-only frames (scroll, `TranslateFn`/`OpacityFn`, scrollbar fades) re-record only scrollbars unless a layer is stale. Layers are composed into one frame list (`gfx.Marker`, `Builder.AppendList`). The persistent framebuffer is the only raster cache, because 8-bit source-over blending is not associative: compositing a pre-rasterized layer would not match painting in place. Damage comes from matching the old and new frame lists command by command, grown by filter reach, closed over backdrop-filter reads; scrolled content moved by whole device pixels is shifted in the framebuffer when that is provably exact (rectangular clips, integer translations, a uniform solid fill below), and content that is not (gradients, images, backdrop filters, uncached glyphs, non-integer transforms) is re-rasterized. Damage over 70 % of the frame falls back to a full rasterization; the host presents only the damage. `raster.Options.Region` grows a region by the read bounds of spreading backdrop filters (except inside filter layers). Property tests and `TELA_CHECK_DAMAGE=1` check the incremental framebuffer against full rasterizations

## Rationale

Measured: a 10k-element scroll frame 6.4 → 0.56 ms, the player's progress tick 12.8 → 0.14 ms (live: 613 → 4 CPU ticks per 5 s), p95 scroll frames 2.2 ms (100k virtual list) and 2.8 ms (plain 10k list) with raster at 800×600 x2; live windows after scrolling are pixel-identical to `tela snapshot` on Wayland and X11
