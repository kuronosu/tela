# Recipe: canvas

Questions: how to draw a chart, a gauge, a signature or sketch pad, a particle simulation or any shape from code; when the draw function runs; how to follow the pointer without rebuilding; how to animate and still cost nothing at rest; the theme's colors and fonts in a drawing; clicking on a shape; erasing; canvas or texture; testing a drawing.

`Canvas(draw func(c *tela.Canvas), props...)` is an element that a Go function draws with the 2D context of a browser's canvas: the same method names over `float64` (`BeginPath`, `Arc`, `FillRect`, `FillText`, `CreateLinearGradient`, `Save`, `Restore`, `GlobalCompositeOperation`...), its properties as fields (`c.FillStyle`, `c.LineWidth`, `c.Font`). It is a replaced element, 300x150 unless sized (`size-32`, `w-full h-48`), clipped to its content box. `c.Width` and `c.Height` are that box in logical pixels; the drawing is vectors, sharp at any display scale. It has no children. Name it (`AriaLabel`, role `img`) and put what it shows in text somewhere (an `sr-only` paragraph).

```go
package main

import (
	"math"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Gauge(progress *tela.Value[float64]) tela.Node {
	return Canvas(func(c *tela.Canvas) {
		r := min(c.Width, c.Height)/2 - 8
		c.LineWidth, c.LineCap = 8, tela.CapRound
		c.BeginPath()
		c.Arc(c.Width/2, c.Height/2, r, -math.Pi/2, -math.Pi/2+2*math.Pi*progress.Get())
		c.Stroke()
	}, Class("size-32 stroke-primary"), AriaLabel("Progress"))
}
```

**When the function runs.** While a frame is painted, after layout: at the mount, at every build of the component that made it (it closes over that build's variables), when its size, scale, style, theme or color scheme change, when a `tela.Value` it read with `Get` changes (the canvas draws again; nothing builds), and after `c.RequestFrame()`. Every draw starts from an empty canvas: a stroke, a particle or a bar must be in your state, and the function draws all of them. A draw equal to the last one paints nothing. Hooks, `Set`, `Update`, `tela.Animate` and `tela.Post` inside it panic in dev mode: keep what it carries in a `tela.UseRef`, or in a `Value` that a handler sets.

**State that changes many times a second is a `Value`.** A handler sets it, the draw reads it with `Get`, and only the canvas paints: a cursor, a stroke being drawn, a spring (`tela.Animate(v, 1, ...)` moves a `Value[float32]` that the draw interpolates by). `e.LocalX` and `e.LocalY` of a pointer event are in the canvas's coordinates.

```go
func Probe(series []float64) tela.Node {
	cursor := tela.UseValue(float32(-1))
	return Canvas(func(c *tela.Canvas) {
		c.FillStyle = c.Color("muted")
		c.FillRect(0, 0, c.Width, c.Height)
		if x := cursor.Get(); x >= 0 {
			c.FillStyle = c.Color("primary")
			c.FillRect(float64(x), 0, 1, c.Height)
		}
	},
		Class("h-48 w-full"), AriaLabel("Visitors by month"),
		OnPointerMove(func(e tela.PointerEvent) { cursor.Set(e.LocalX) }),
		OnPointerLeave(func() { cursor.Set(-1) }))
}
```

**Animation and 0% CPU.** A draw that calls `c.RequestFrame()` is followed by another at the window's next frame; one that does not, rests, and a window with nothing else moving asks for no frames. `c.Time` and `c.Delta` (`time.Duration`) are the canvas's clock; they stop while it rests or waits (off screen), and `Delta` is 0 on the first draw: move things by `Delta`, not by frames. A canvas that calls `RequestFrame` in every draw asks for a frame at every vsync, like `animate-spin`: call it only `if running`. Reduced motion changes nothing by itself: read `tela.UseReducedMotion()` and start paused.

```go
package main

import (
	"math"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Field(running bool) tela.Node {
	x := tela.UseRef(0.0)
	return Canvas(func(c *tela.Canvas) {
		if running {
			x.Current = math.Mod(x.Current+90*c.Delta.Seconds(), c.Width)
			c.RequestFrame()
		}
		c.BeginPath()
		c.Arc(x.Current, c.Height/2, 6, 0, 2*math.Pi)
		c.Fill()
	}, Class("h-24 w-full fill-primary"), AriaLabel("A dot that crosses"))
}
```

**Colors and fonts come from the element.** `FillStyle` and `StrokeStyle` start as its `fill-*` and `stroke-*` (its text color when it has neither: not black), `c.Font` as its `font-*`/`text-*` (a zero field of `tela.CanvasFont{Family, Size, Weight, Italic}` is the element's), so `dark:`, `hover:` and `transition` work on a canvas as on an icon. `c.Color("primary")` or `c.Color("red-500")` reads the theme in the element's scheme; `tela.Hex("#4f46e5")` is fixed; `c.CreateLinearGradient(...)` takes `AddColorStop(0, color.WithAlpha(0.3))` and interpolates in sRGB. `c.MeasureText(s).Width` measures with the current font: size axes and tooltips by it. Store a token name (`"chart-1"`) in your data, not a `Color`, so it follows the scheme.

**Clicking a shape.** Keep it as a `tela.Path2D` (`new(tela.Path2D)` then `MoveTo`, `Arc`...; `tela.ParsePath("M8 5v14l11-7z")` reads SVG path data), draw it with `c.FillPath(p)` and test `p.Contains(float64(e.LocalX), float64(e.LocalY))` in the handler, with no draw. A position is measured from the element's border box and the drawing starts at its content box: keep borders and padding on a `Div` round the canvas. The canvas is not focusable and a drawing has no parts a keyboard reaches: put the same actions in real buttons, and `TabIndex(0)` on it only if keys act on it.

**Compositing.** `c.GlobalCompositeOperation` takes the browser's 26 (`tela.CompositeSourceOver`...). An eraser: `c.GlobalCompositeOperation = tela.CompositeDestinationOut` and stroke with any opaque color; it takes away what the canvas drew and shows what is behind the element. A draw that uses another operation, or a partial `ClearRect`, makes the canvas a layer of its own.

**What differs from the browser.** Every draw starts empty; there is no `getContext`, `requestAnimationFrame`, `filter`, `getImageData` or `OffscreenCanvas`; `fill("evenodd")` is `c.FillRule = tela.EvenOdd` then `c.Fill()`; images are `image.Image` (`c.DrawImage`).

**Canvas or something else.** Shapes that code can describe, at any scale, themed: `Canvas`. Pixels that something else renders (3D, video, a map): `Texture` with `tela.UseTexture`. A file: `Img`. A glyph that follows the text: `Icon`.

**Testing.** `telatest.ByRole("img", "Progress")` or `ByTestID` find it; `app.PixelAt(x, y)`, screenshots and goldens show the drawing. A canvas that calls `RequestFrame` is named by `PendingCanvas` in `app.Pending()`; `app.Advance(d)` moves it, a draw per frame; `app.ExpectIdle()` fails until it rests, so test a pause with it. Moving the pointer builds nothing: `app.Measure` reports `Stats.Builds == 0`.

```go
func TestCursorBuildsNothing(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		cursor := tela.UseValue(float32(-1))
		return Canvas(func(c *tela.Canvas) { c.FillRect(float64(cursor.Get()), 0, 2, c.Height) },
			Class("h-24 w-full fill-primary"), AriaLabel("Probe"),
			OnPointerMove(func(e tela.PointerEvent) { cursor.Set(e.LocalX) }))
	}, telatest.Size(300, 100))
	probe := telatest.ByRole("img", "Probe")
	stats := app.Measure(func() { app.Hover(probe) })
	if stats.Builds != 0 {
		t.Errorf("the pointer built %d components", stats.Builds)
	}
	app.ExpectIdle() // a canvas that does not call RequestFrame rests
	app.Golden("probe.png")
}
```
