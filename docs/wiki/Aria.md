# Aria: behavior hooks

Package [`aria`](https://pkg.go.dev/github.com/kuronosu/tela/aria) holds the
unstyled *behavior* of interface components, in the model of React Aria and
Floating UI: hooks that implement interaction, positioning and accessibility
once, and return props to put on your own elements. The look is yours (or
the `kit` components').

```go
import "github.com/kuronosu/tela/aria"
```

| You want | Use |
|---|---|
| A popover, tooltip, menu or select list next to its button | [`aria.UseFloating`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseFloating) |

This page grows with the package; the sections below cover what exists.

## Floating elements: `UseFloating`

`UseFloating` places a floating element next to an anchor. It is a port of
[Floating UI](https://floating-ui.com)'s `computePosition` and its
middleware, built into Tela's frame pipeline so that the element is
**in the right place in the first frame that shows it** and follows its
anchor while an ancestor scrolls, with no frame of its own and no cost
while nothing changes.

```go
func Popover(open bool) tela.Node {
	anchor := tela.UseElementRef()
	arrow := tela.UseElementRef()
	fl := aria.UseFloating(anchor,
		aria.Placement("bottom-start"), aria.Offset(8),
		aria.Flip(), aria.Shift(8), aria.Arrow(arrow))

	return Fragment(
		Button(Ref(anchor), Text("Open")),
		If(open, Portal(
			Div(fl.Props, Class("w-64 rounded-md border bg-popover p-4 shadow-md"),
				Text("Content"),
				Div(Ref(arrow), fl.ArrowProps, Class("size-2 rotate-45 border bg-popover"))))),
	)
}
```

- `anchor` is the `tela.ElementRef` of the element the popover goes next to
  (attach it with `h.Ref`).
- `fl.Props` goes on the floating element. It makes it `fixed` at the
  window's top-left corner (a later class such as `absolute` wins) and
  places it. Put the element in a `Portal` so that no ancestor clips it.
- The options are **functional options applied in the order given**, like
  Floating UI's `middleware` array, and the order matters: `Offset` before
  `Flip` and `Shift`, `Arrow` last, `Size` after `Flip` and `Shift`.
- `fl.Placement` is the placement after `Flip` chose one (`"top-start"`).
  It reaches your build one frame after the position, so style with it
  (origin of an animation, the arrow's rotation), and use the values below
  for what must be right in the same frame.

### Placement

`aria.Placement("bottom-start")` takes one of Floating UI's 12 placements:
`top`, `right`, `bottom` (the default) and `left`, each with `-start` and
`-end`. In a right-to-left element (`h.Dir("rtl")` on the floating element)
`-start` and `-end` mirror, as Floating UI's `rtl` does for `top` and
`bottom`. A mistake panics in dev mode with the closest valid placement:

```text
aria: Placement("bottm-start"): unknown placement "bottm-start". Did you mean "bottom-start"? ...
```

### The middleware

| Option | What it does |
|---|---|
| `aria.Offset(8)` | The gap between the anchor and the element. `aria.OffsetOptions` adds `CrossAxis` (skid along the anchor's edge) and `AlignmentAxis`. |
| `aria.Flip()` | Goes to the opposite side when the placement would leave the window. `aria.FlipOptions`: `Padding`, `FallbackPlacements`, `FallbackStrategy`, `FallbackAxisSideDirection`, `NoFlipAlignment`, `NoMainAxis`, `NoCrossAxis`, `CrossAxisAlignment`. |
| `aria.Shift(8)` | Slides along the anchor's edge to stay 8 px inside the window. `aria.ShiftOptions`: `CrossAxis`, `NoMainAxis`, `Limiter` (`aria.LimitShift(...)` keeps it attached to the anchor). |
| `aria.Size(func(a aria.Available) { a.MaxHeight(a.Height) })` | Gives you the room the element has (`a.Width`, `a.Height`, `a.Rects`) and limits it with `a.MaxWidth(w)` and `a.MaxHeight(h)`. |
| `aria.Arrow(ref)` | Computes where an arrow element points at the anchor. |
| `aria.Hide()` | Tracks whether the anchor is scrolled out of view (`fl.ReferenceHidden`, `fl.Escaped`). |
| `aria.AutoPlacement()` | Picks the placement with the most room, instead of `Flip`. |
| `aria.AnchorRect(f)` | A virtual anchor (see below). |

Every options struct is a variadic argument and its zero value is Floating
UI's defaults, so `aria.Flip()` and `aria.Flip(aria.FlipOptions{Padding: 8})`
both work. Options that default to true in Floating UI are negated here
(`NoFlipAlignment`) so that the zero value stays the default.

### A list that fits the window: `Size`

`Size` limits the element's border box in the same frame, with no flash of
the unlimited element. Give the element `overflow-y-auto` (or put its content
in a scrolling child) and a long list scrolls inside the window:

```go
fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(4),
	aria.Flip(), aria.Shift(8),
	aria.Size(func(a aria.Available) {
		a.MaxHeight(a.Height)
	}, aria.SizeOptions{Padding: 8}))
Portal(Div(fl.Props, Class("flex w-64 flex-col overflow-y-auto rounded-md border bg-popover"), items))
```

A limit that `apply` does not ask for in a run is removed. Putting `Size`
after `Flip` and `Shift` gives the placement Floating UI gives; before them
the two can settle on a different placement than Floating UI's
(ADR-099).

### The arrow

Attach the ref given to `aria.Arrow` to the arrow element, with
`fl.ArrowProps` beside it. `ArrowProps` makes the arrow `absolute` in the
floating element and moves it, in the same frame, to the edge that faces
the anchor (half of it outside the border box, as in Floating UI's
recipe) and along it to point at the anchor's center, even when the
popover shifted. `fl.Side` and `fl.Align` tell the side for rotation. For
your own placement of the arrow, `fl.ArrowX` and `fl.ArrowY` are Floating
UI's arrow data as `tela.Value`s: read them inside a `tela.TranslateFn`.

### Context menus and other virtual anchors

For a menu at the pointer, give `UseFloating` the zero `tela.ElementRef`
and an `AnchorRect` that returns a rectangle in window coordinates:

```go
pointer := tela.UseValue(tela.Rect{})
fl := aria.UseFloating(tela.ElementRef{},
	aria.Placement("bottom-start"), aria.Flip(), aria.Shift(4),
	aria.AnchorRect(pointer.Get))
// in an OnContextMenu handler: pointer.Set(tela.Rect{X: e.X, Y: e.Y})
```

A `Value` that `AnchorRect` reads places the element again when it changes,
without building.

### Hiding a popover whose anchor scrolled away

```go
fl := aria.UseFloating(anchor, aria.Flip(), aria.Hide())
Div(fl.Props, Style(tela.OpacityFn(tela.MapValue(fl.ReferenceHidden, func(hidden bool) float32 {
	if hidden {
		return 0
	}
	return 1
}))))
```

### How it differs from Floating UI

- Positioning runs inside the frame, on the anchor *as painted* (scroll
  offsets, transforms and layout animations applied), not on rectangles that
  a `ResizeObserver` or scroll listener noticed a frame later. There is no
  `autoUpdate` to configure.
- The size middleware does not measure the element again inside the
  computation: Tela lays the tree out again within the frame (at most
  twice) and runs the position again with the new size.
- The clipping area is the window cut by the overflow clips of the
  element's containing-block chain: for an element in a `Portal`, the window.
  There is no `boundary` or `rootBoundary` option, and padding is one number.
- `offset` takes numbers only (no function of the placement), the arrow
  computes with the floating element's border box, and there is no `inline`
  middleware.
