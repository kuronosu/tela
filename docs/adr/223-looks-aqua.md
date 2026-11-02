# ADR-223: `looks/aqua`, two macOS designs mixed per part

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-197.

## Decision

`looks/aqua` exports `Look(opts ...Option) looks.Look` with two designs, each an `Option` that sets every part: `aqua.Tahoe` (macOS 26: capsule controls, the toolbar's groups and the sidebar as floating glass, opaque content) and `aqua.Sequoia` (macOS 11–15: rounded rectangles, a flat toolbar unified with the title bar, a plain sidebar). `Look()` is Tahoe. Per-part options mix them: `Controls(d)` (buttons, fields, toggles, selects, sliders), `Chrome(d)` (toolbar and sidebar), and `GlassContent()` adds glass to the content's cards. `detect.For("darwin", "")` is `aqua.Look()`; `detect.Look()` on a Mac picks Tahoe from macOS 26 and Sequoia before it. The gallery shows both designs and a mix.

## Rationale

The owner chose to ship both and let the app decide (2026-11-02), after prototypes rendered with Tela. Tahoe is what a current Mac shows; Sequoia is what many apps and older systems look like, and costs no backdrop blur on the CPU. Glass is a blur, a fill and a highlight: Tela does not refract.

## What it changes

ADR-197's single aqua look becomes two designs and a mix. Apple's fonts and symbols are not used (Inter and Lucide stand in).
