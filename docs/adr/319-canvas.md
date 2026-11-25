# ADR-319: Canvas

- **Date:** 2026-11-24
- **Status:** Accepted (draw function, `RequestFrame`, `float64`, fields, a font struct and the names: the owner's decisions)

## Decision

`h.Canvas(draw func(c *tela.Canvas), children ...)` is a replaced element (300x150 by default, as `h.Texture`; clipped to its content box; role `img` when it has a name). `draw` is a paint-phase function that records a display list: vector at every scale, always from an empty canvas.

- **When it runs:** on a mount, a build of its owner, a change of its size, scale, style or theme, of a Value it read, or on `c.RequestFrame()`. Lists are compared by content: an equal one damages nothing. Hooks, `Set` and `Update` in it panic in dev mode.
- **Frames:** `RequestFrame` asks for one more draw, at the window's next frame. Without it the canvas is idle. Off screen, or in a window without frames, it waits. `c.Time` and `c.Delta` (`time.Duration`) are the canvas's clock, which stops while it waits; `Delta` is 0 on the first draw and on a second one in the same frame.
- **State:** the browser's method names over `float64`; its properties are fields (`FillStyle`, `LineWidth`, `GlobalAlpha`...), plus `FillRule` for the argument of `fill` and `clip`. Save and Restore cover them; what is still open when `draw` returns is closed. `FillStyle` and `StrokeStyle` take a `Color`, a `*CanvasGradient` or a `*CanvasPattern`; gradients interpolate in sRGB, as Canvas does.
- **Style:** the initial fill, stroke and font are the element's (`fill-*`, `stroke-*`, `text-*`, `font-*`); without a fill or stroke class, its text color, not black. `c.Color(token)` reads the theme. `tela.CanvasFont{Family, Size, Weight, Italic}`: a zero field is the element's.
- **Events:** in a canvas's handlers `LocalX` and `LocalY` are from its content box, where its drawing starts (other elements: the border box).
- **Compositing:** the 26 operations of Canvas, so `gfx` gets the Porter-Duff ones and a tiled image paint (SVG patterns use it too), on the CPU and the GPU. A canvas is isolated only when an operation or a partial `ClearRect` needs it.

Not covered: pixel access (`UseTexture`), a persistent bitmap (keep the strokes in state), `ctx.filter`, images by asset path, offscreen canvases.

## Why

A ref, an effect, a pixel ratio, a resize observer and a cancelled frame callback are what React needs around a retained bitmap. A deferred draw has the component's lifecycle already.

## Changes

`internal/canvas` (the drawing API, over `gfx`, `svg` and `text`; `core` runs it). ARCHITECTURE §5, §10, §13. Roadmap: SVG and PDF are a phase of their own.
