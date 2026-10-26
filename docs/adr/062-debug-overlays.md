# ADR-062: Debug overlays

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Debug overlays (§17). Chosen by `TELA_DEBUG` (read by `core.NewHost`; unknown flags print one line with a suggestion) or `Config.Debug`; `telatest` ignores the environment unless `telatest.Debug` asks. Overlays are a display list of their own, rasterized over a private copy of the application's framebuffer (only where the app or the overlay changed), so they never take part in layers, damage, layout, hit-testing or semantics, and cost nothing when off. `paint-flash` fades the app's damage rects over 300 ms; `fps` requests a frame per vsync; `layout`, `layers` and `semantics` never request frames (a hover change asks for one). `telatest` does not wait for overlay-only frame requests

## Rationale

The overlays must not change what they observe: drawing them over a copy keeps the app's damage and pixels exact (checked by a property test with overlays on)
