# Canvas

[`h.Canvas`](https://pkg.go.dev/github.com/kuronosu/tela/h#Canvas) is an element
that a Go function draws: a chart, a gauge, a signature pad, a simulation. The
function gets a [`*tela.Canvas`](https://pkg.go.dev/github.com/kuronosu/tela#Canvas),
the 2D context of a browser's canvas with the same method names over `float64`
and its properties as fields, so what you know from `getContext("2d")` carries
over. Tela keeps what the function draws as vectors and paints it at any
display scale; the function runs again only when something it draws with
changes, and an app with a canvas that does not move asks for no frames.

```go
Canvas(func(c *tela.Canvas) {
	r := min(c.Width, c.Height)/2 - 8
	c.LineWidth, c.LineCap = 8, tela.CapRound
	c.BeginPath()
	c.Arc(c.Width/2, c.Height/2, r, 0, 2*math.Pi*progress.Get())
	c.Stroke()
}, Class("size-32 stroke-primary"), AriaLabel("Progress"))
```

[`examples/canvas`](https://github.com/kuronosu/tela/tree/main/examples/canvas)
is a chart with a cursor, a particle field and a sketch pad, with the tests that
go with them.

## The element

- It is a replaced element, 300x150 by default as an image or a
  [texture](Hooks#external-textures-usetexture) is; `size-*`, `w-*`, `h-*`,
  `aspect-*` and the flex and grid classes size it, and with one size (`w-full`)
  the other keeps the ratio. It takes no children: put an overlay beside it in a
  `relative` box.
- `c.Width` and `c.Height` are the size of its **content box** in logical
  pixels, whatever the display scale (the drawing is vectors: it is sharp at 2x
  with the same code). Borders, padding, rounded corners (which clip the
  drawing), shadows, opacity, filters, transforms, blend modes and masks are the
  element's, as for any other.
- It has the role `img` when it has a name (`AriaLabel` or `AriaLabelledBy`), and
  says nothing to assistive technologies otherwise. Name it, and say in text what
  it draws: the values of a chart, in a line next to it or in an `sr-only`
  paragraph. The drawing has no content a screen reader could read.

## When the function runs

The function runs on the UI thread, after the layout, while a frame is painted.
It runs when something it draws with changed:

- at the mount, and at every build of the component that made the element
  (the function is a closure over that build's variables);
- when its size, the display scale, its style (the text color, the `fill-*` and
  `stroke-*` classes, its font), the theme or the color scheme change;
- when a [`tela.Value`](Hooks#observable-values-usevalue) it read with `Get`
  changes: only the canvas draws again, and no component builds;
- when a draw asked for it with `c.RequestFrame()`.

A canvas that nothing shows (off screen in a scroll container, in a window that
gets no frames) waits until something does. Every draw starts from an empty
canvas: nothing you drew before is there. Two drawings equal by content paint
nothing, so redrawing the same picture costs no damage.

Do not call hooks or set state inside the function (a `Setter`, a `Value`,
`tela.Animate`, `tela.Post`): in dev mode they panic with a message that says
so, and `tela lint` reports hooks in it. What a simulation carries from one draw
to the next lives in a [`UseRef`](Hooks#memoization-usememo-useonce-usecallback-useref),
or in a `Value` that an event handler sets.

### Values: a cursor with no rebuild

State that changes many times a second, such as a pointer position, belongs in a
`Value`. The handler sets it and the draw reads it:

```go
cursor := tela.UseValue(float32(-1))
return Canvas(func(c *tela.Canvas) {
	drawChart(c, series, cursor.Get()) // Get here subscribes this canvas
},
	Class("h-48 w-full"), AriaLabel("Visitors by month"),
	OnPointerMove(func(e tela.PointerEvent) { cursor.Set(e.LocalX) }),
	OnPointerLeave(func() { cursor.Set(-1) }))
```

Moving the pointer paints the canvas and builds nothing: a test can assert it
with `app.Measure` (`Stats.Builds == 0`). A spring does the same:
`tela.UseSpring` and `tela.Animate` move a `Value[float32]` that the draw reads,
so a chart can morph between two series (`examples/canvas/chart.go`).

### Frames: animation and 0% CPU

A draw that calls `c.RequestFrame()` is followed by another one at the window's
next frame. `c.Time` and `c.Delta` (`time.Duration`) are the canvas's own clock:
they stand still while it rests or waits, and `Delta` is 0 on the first draw and
on a second one in the same frame. Move things by `Delta`, not by the number of
frames:

```go
sim := tela.UseRef(&field{})
return Canvas(func(c *tela.Canvas) {
	sim.Current.Step(c.Delta.Seconds(), c.Width, c.Height)
	sim.Current.Draw(c)
	if running {
		c.RequestFrame() // without this call the canvas is idle
	}
}, Class("h-44 w-full"), AriaLabel("Particles"))
```

When `running` goes false, the next draw (a rebuild gives it) does not ask for
a frame, and the window sleeps: no frames, no CPU. A canvas that asks for a frame
in every draw asks for one at every vsync, like `animate-spin`. Reduced motion
changes nothing by itself: read `tela.UseReducedMotion()` and start your
animation paused or as a still, as `examples/canvas` does.

## Colors, fonts and the theme

The initial `FillStyle` and `StrokeStyle` are the element's: `fill-*` and
`stroke-*` classes, or its text color when it has none (not black), so a canvas
follows the theme as an [icon](Icons) does, and `dark:` variants and
`transition` work on it. `c.Color` reads any other color of the theme in the
element's scheme:

```go
c.FillStyle = c.Color("chart-1")   // a theme token
c.StrokeStyle = c.Color("red-500") // a palette color
c.FillStyle = tela.Hex("#4f46e5")  // a fixed one
```

A theme or scheme change draws the canvas again, so every color keeps up. Keep a
token name in your data (as the sketch pad does) rather than a `Color`, when the
data should change with the scheme.

`c.Font` starts as the element's font (`font-*`, `text-*`, `italic`,
`tracking-*`, the theme's and the desktop's): set the fields you want to change
and leave the others at zero (`c.Font.Size, c.Font.Weight = 12, 600`), or give
a family list as in CSS (`tela.CanvasFont{Family: "ui-monospace, monospace"}`).
`c.MeasureText` returns the size of a text as the canvas writes it, so axes,
tooltips and badges fit their labels. Direction and language are the element's
(`h.Dir`, `h.Lang`), as are font features such as `tabular-nums`.

## Drawing

The methods are the browser's: paths (`MoveTo`, `LineTo`, `Arc`, `ArcTo`,
`BezierCurveTo`, `QuadraticCurveTo`, `RoundRect`, `Ellipse`, `Rect`), `Fill`,
`Stroke` and `Clip` (with `FillRule` for `evenodd`), `FillRect`, `StrokeRect`
and `ClearRect`, the transforms (`Translate`, `Rotate`, `Scale`, `Transform`,
`SetTransform`) with `Save` and `Restore`, line styles (`LineWidth`, `LineCap`,
`LineJoin`, `MiterLimit`, `SetLineDash`), gradients
(`CreateLinearGradient`, `CreateRadialGradient`, `CreateConicGradient`), patterns
(`CreatePattern` of an `image.Image`), images (`DrawImage`), text (`FillText`,
`StrokeText`) and shadows (`ShadowColor`, `ShadowBlur`, `ShadowOffsetX`,
`ShadowOffsetY`). What is open when the function returns is closed for you.
[`tela.Path2D`](https://pkg.go.dev/github.com/kuronosu/tela#Path2D) is a path
you keep and draw with any canvas (`FillPath`, `StrokePath`, `ClipPath`), and
`tela.ParsePath("M8 5v14l11-7z")` reads SVG path data.

Gradients interpolate in sRGB, as a browser's canvas does, not in Oklab as the
`bg-linear-*` classes do.

### Compositing

`GlobalCompositeOperation` takes the 26 operations of Canvas (`CompositeSourceOver`
to `CompositeLuminosity`). An eraser is `CompositeDestinationOut`:

```go
c.GlobalCompositeOperation = tela.CompositeDestinationOut
c.LineWidth, c.LineCap = 20, tela.CapRound
c.StrokeStyle = tela.Black // any opaque color: it is the shape that erases
c.BeginPath(); c.MoveTo(x0, y0); c.LineTo(x1, y1); c.Stroke()
```

It erases what the canvas drew and shows what is behind the canvas (its own
background, the card under it). A draw that uses an operation other than
`CompositeSourceOver`, or clears a part with `ClearRect`, makes the canvas a
layer of its own so that what is behind it is not erased; the others cost no
layer.

## Events and hit testing

Pointer events work on a canvas as on any element (`OnClick`, `OnPointerMove`,
`OnPointerDown`, `TabIndex`, `OnKeyDown`). In the handlers of a canvas
`e.LocalX` and `e.LocalY` are in the coordinates it draws in: from its content
box, whatever border and padding it has and however it is transformed, where
other elements measure from their border box. (`aria.PressEvent`'s are from the
border box on every element.)

`Path2D.Contains` tells whether a point is inside a path, in the path's
coordinates and without a draw, which is how a click finds what it hit:

```go
slice := new(tela.Path2D)
slice.MoveTo(50, 50)
slice.Arc(50, 50, 40, 0, math.Pi/2)
slice.ClosePath()
Canvas(func(c *tela.Canvas) { c.FillPath(slice) }, Class("size-25"), AriaLabel("Sales"),
	OnClick(func(e tela.MouseEvent) {
		if slice.Contains(float64(e.LocalX), float64(e.LocalY)) { open() }
	}))
```

A canvas is not focusable by itself, and a drawing has no parts a keyboard can
reach: give the interaction a second way in. Put the controls that do the same
(undo, the choice of a series, play and pause) in real buttons next to it, and
`TabIndex(0)` on the canvas if keys act on it.

## What differs from the browser

| Browser | Tela |
|---|---|
| `getContext`, `width`, `height` | the draw function's argument, `Width`, `Height` (logical pixels, no pixel ratio to handle) |
| `requestAnimationFrame(draw)` | `c.RequestFrame()`, `Time` and `Delta`; there is nothing to cancel |
| the bitmap stays between draws | every draw starts empty: keep the strokes or the particles in state, draw them all |
| `fillStyle = "#4f46e5"` | `FillStyle = tela.Hex("#4f46e5")`, or `c.Color("primary")` |
| `fill("evenodd")` | `FillRule = tela.EvenOdd`, then `Fill()` |
| `new Path2D("M0 0L9 9")` | `tela.ParsePath("M0 0L9 9")` |
| `font = "600 12px Inter"` | `Font = tela.CanvasFont{Family: "Inter", Size: 12, Weight: 600}` |
| `fillStyle` and `font` start as black and 10px sans-serif | they start as the element's |
| `filter`, `getImageData`, `putImageData`, `OffscreenCanvas` | none: for pixels use [`UseTexture`](Hooks#external-textures-usetexture) |
| `drawImage` of an element or a bitmap | `DrawImage` of an `image.Image` |

Invalid values never stop a draw: what the browser ignores is ignored, and what
it answers with an error (a negative radius, a gradient stop above 1) does
nothing. `tela.Canvas`'s own documentation has the whole table.

## Canvas, Texture, images and icons

| You have | Use |
|---|---|
| Shapes, charts, gauges, a signature, a small simulation that Go code can describe as drawing commands, at any scale | `h.Canvas`: vectors, themed, no pixels to manage |
| Pixels that something else renders (a 3D view, a decoded video, a map, a game) | [`h.Texture`](Hooks#external-textures-usetexture) with `tela.UseTexture`: you push frames, Tela composes them |
| A fixed picture, or an SVG file | [`h.Img`](Elements-and-Props) |
| A glyph that follows the text color and size | [`h.Icon`](Icons) |

A canvas is cheap to keep and to keep still; a drawing of a few thousand
commands every frame is what the CPU rasterizer is for, and a scene that is
really pixels (per-pixel shading, video) is a texture.

## Testing

A canvas is found like any other element: `telatest.ByRole("img", "Progress")`,
or `ByTestID`. Its drawing is in `app.PixelAt`, screenshots and goldens, which
are the same on every platform.

```go
app := telatest.Mount(t, tela.C(Gauge))
app.Click(telatest.ByRole("button", "More"))
if got := app.PixelAt(8, 64); got != (color.RGBA{79, 70, 229, 255}) {
	t.Errorf("the ring is %v where it should reach", got)
}

app.Advance(500 * time.Millisecond) // a running canvas: a draw per frame, by the app's clock
app.ExpectIdle()                    // fails while a canvas still calls RequestFrame
```

A canvas that animates never stops asking for frames, as an endless animation:
settling does not wait for it, `app.Pending()` names it (`PendingCanvas`), and
`ExpectIdle` fails until it rests. Test the pause by asserting `ExpectIdle`
after it, as `examples/canvas` does. `tela mcp` and the idle budgets treat a
running canvas as quiet work too.

## See also

- [Hooks](Hooks): `UseValue`, `UseRef`, `UseSpring` and `UseTexture`.
- [Animation](Animation): transitions of `fill`, `stroke` and `stroke-width`.
- The API: [`h.Canvas`](https://pkg.go.dev/github.com/kuronosu/tela/h#Canvas) and
  [`tela.Canvas`](https://pkg.go.dev/github.com/kuronosu/tela#Canvas), with every
  method and example.
