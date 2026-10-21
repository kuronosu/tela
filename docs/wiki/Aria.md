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
| One "press" for mouse, touch, pen and keyboard | [`aria.UsePress`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UsePress) |
| A button, on any element | [`aria.UseButton`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseButton) |
| A button that is on or off | [`aria.UseToggleButton`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseToggleButton) |
| A checkbox or a switch | [`aria.UseCheckbox`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseCheckbox), [`aria.UseSwitch`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseSwitch) |
| To know that a mouse or a pen is over an element | [`aria.UseHover`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseHover) |
| A focus ring that shows after the keyboard, not after a click | [`aria.UseFocusRing`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseFocusRing) |
| A dialog that keeps the focus, or a toolbar with arrow keys | [`aria.UseFocusScope`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseFocusScope) |
| A popover, tooltip, menu or select list next to its button | [`aria.UseFloating`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseFloating) |

This page grows with the package; the sections below cover what exists.

## The pattern: options in, `Props` out

A hook takes an options struct and returns a struct. Its `Props` field is one
`tela.Prop` with everything the hook needs on the element (handlers, ref,
role, aria states); the other fields are state to style from:

```go
press := aria.UsePress(aria.PressOptions{
	OnPress:  func(e aria.PressEvent) { save() },
	Disabled: busy,
})
return Button(press.Props, Class("rounded-md px-3 py-1.5"),
	If(press.Pressed, Class("scale-95")), Text("Save"))
```

`Props` merges several props: the handlers of one event all run, in the order
given, so hook props combine with your own and with each other:

```go
ref := tela.UseElementRef()
btn := aria.UseButton(aria.ButtonOptions{Ref: ref, OnPress: func(aria.PressEvent) { save() }})
ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
hover := aria.UseHover(aria.HoverOptions{})
return Button(btn.Props, ring.Props, hover.Props,
	OnPointerDown(func() { track("down") }),
	Class("rounded-md px-3 py-1.5 transition"),
	If(hover.Hovered, Class("bg-accent")),
	If(ring.FocusVisible, Class("ring-2 ring-primary")), Text("Save"))
```

**One ref per element.** The hooks that need the element itself (press, focus
ring, focus scope, and the controls built on press) attach a ref to it, and an
element has one. When several hooks serve one element, make the ref once with
`tela.UseElementRef` and give it to each as `Ref`, as above; without it each
hook attaches its own and only the last stays attached (a press then never
sees the pointer leave). Do not add `h.Ref(other)` to an element that has hook
props: use the `Ref` field of a hook's result.

**Controlled or not.** A hook that holds state keeps it itself (uncontrolled,
starting at `DefaultSelected`), and lets you keep it (controlled) when you pass
`OnChange`: it then shows `Selected` and only asks `OnChange` for the next
value, like an `Input` with `Value` and `OnChange`. A change you do not store
is refused.

```go
box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})          // uncontrolled
on, setOn := tela.UseState(false)
sw := aria.UseSwitch(aria.ToggleOptions{Selected: on, OnChange: setOn.Set}) // controlled
```

## Press: `UsePress`

`UsePress` is React Aria's `usePress`: the same press for every input.

| Input | The press |
|---|---|
| Mouse, touch, pen | Starts when the primary button, a finger or the pen goes down on the element. A release over the element completes it; leaving the element ends it (`OnPressEnd`, no `OnPress`) and coming back while held starts it again, unless `ShouldCancelOnPointerExit`. The system taking the touch, a touch that becomes a scroll, or the element unmounting cancel it. |
| Enter | On the focused element, presses at once on the key down. Key repeats are ignored. |
| Space | On the focused element, starts on the key down and completes on the key up. Repeats are ignored, and a blur in the middle cancels it. |
| Virtual | A click that no pointer or key made: an agent's "click" action, a Label forwarding its click to the control, assistive technology. `PointerType` is `aria.PointerVirtual`. |

The callbacks are `OnPressStart`, `OnPressUp`, `OnPressEnd`, `OnPressChange`
and `OnPress`; `PressEvent` has the `PointerType`, the modifier keys and the
position of the event (`X`, `Y` in the window, `LocalX`, `LocalY` in the
element; the center for the keyboard and virtual presses). The result's
`Pressed` is true while pressed: use it for the look, since the `active:`
variant follows the pointer only.

A press **stops the propagation** of the events that make it, so a pressable
card inside a pressable row presses only the card. Call
`e.ContinuePropagation()` in a callback to let them bubble. It also prevents
the default action of the click: a `Button` with press props does not submit
its `Form` (call the handler from `OnPress`), and a `Label` does not forward
the click. A disabled press (`Disabled: true`) ignores everything. The hook
adds no role and no tab stop: for a button use `UseButton`.

## Buttons and toggles

`UseButton` is a press plus `Role("button")`, a tab stop and `Disabled`: it
makes a `Div` or an `A` a button, and a `Button` a correct one.

`UseToggleButton`, `UseCheckbox` and `UseSwitch` take `aria.ToggleOptions`
(`Selected`, `DefaultSelected`, `OnChange`, `Disabled`, `ReadOnly`, `ID`,
`Ref`) and return `Selected`, `Pressed`, `ID` and `Props`:

| Hook | Role and state | Keys |
|---|---|---|
| `UseToggleButton` | `button`, `aria-pressed` | Enter and Space |
| `UseCheckbox` | `checkbox`, `aria-checked` | Space (not Enter, as in HTML) |
| `UseSwitch` | `switch`, `aria-checked` | Space |

```go
func Notifications() tela.Node {
	box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
	return Div(Class("flex items-center gap-2"),
		Button(box.Props, Class("size-4 rounded border checked:bg-primary")),
		Label(For(box.ID), Text("Notifications")))
}
```

- Render a checkbox or switch with a **`Button`**: it is the element a `Label`
  can label, so `Label(For(box.ID), ...)` names the control and a click on the
  label toggles it.
- A selected checkbox or switch matches **`checked:`** (and `peer-checked:`,
  `group-checked:`), and `aria-checked:`; a toggle button matches
  `aria-pressed:`. `checked:` matches an element whose `h.AriaChecked` is
  true, because Tela has no native checkbox.
- `ReadOnly` keeps the control focusable and readable but not changeable
  (`aria-readonly`). There is no indeterminate state yet.

The aria snapshot shows them as Playwright does:

```yaml
- checkbox "Notifications" [checked]
- switch "Wi-Fi" [checked]
- switch "Bluetooth" [disabled]
- button "Bold" [pressed]
```

## Hover: `UseHover`

```go
hover := aria.UseHover(aria.HoverOptions{OnHoverStart: func(aria.HoverEvent) { preload() }})
return Div(hover.Props, If(hover.Hovered, Class("ring-2")), Text("Card"))
```

`Hovered` is true while a mouse or a pen is over the element or its
descendants. A touch never hovers (the enter that a touch press makes is
ignored). The `hover:` variant covers styling; use the hook when Go needs to
know.

## Focus ring: `UseFocusRing`

`UseFocusRing` returns `Focused` and `FocusVisible`: the ring shows after the
keyboard moves the focus (Tab, arrows) and not after a click, as Chromium's
`:focus-visible` does. With `Within: true` it follows the focus anywhere
inside the element (a group that rings all its parts).

```go
ring := aria.UseFocusRing(aria.FocusRingOptions{})
return Button(ring.Props, If(ring.FocusVisible, Class("ring-2 ring-primary")), Text("Save"))
```

## Focus scope: `UseFocusScope`

`UseFocusScope(aria.FocusScopeOptions{Contain, Restore, AutoFocus})` makes an
element a focus scope and returns `Props` and a `Manager`:

- `Contain` keeps Tab and Shift+Tab inside the scope (a dialog).
- `Restore` gives the focus back, when the scope unmounts, to the element that
  had it when the scope mounted (the button that opened the dialog).
- `AutoFocus` focuses the first tabbable element after the frame that mounts
  the scope, unless the focus is already inside.

```go
scope := aria.UseFocusScope(aria.FocusScopeOptions{Contain: true, Restore: true, AutoFocus: true})
return Div(scope.Props, Role("dialog"), AriaLabel("Settings"), ...)
```

The `Manager` moves the focus among the focusable elements of the scope from
your handlers, in tree order; each method reports whether it found one:

```go
scope := aria.UseFocusScope(aria.FocusScopeOptions{})
return Div(scope.Props, Role("toolbar"), OnKeyDown(func(e tela.KeyboardEvent) {
	switch e.Key {
	case tela.KeyArrowRight:
		e.PreventDefault()
		scope.Manager.FocusNext(aria.FocusOptions{Wrap: true})
	case tela.KeyArrowLeft:
		e.PreventDefault()
		scope.Manager.FocusPrevious(aria.FocusOptions{Wrap: true})
	}
}), buttons)
```

`FocusFirst`, `FocusLast`, `FocusNext` and `FocusPrevious` take
`aria.FocusOptions`: `Wrap` goes around at the ends, `Tabbable` leaves out the
elements that only a script can focus (`TabIndex(-1)`), `From` starts at
another element of the scope.

## Testing

Everything above runs in `telatest`: `app.Click`, `app.Press("Enter")`,
`app.Press("Space")`, `app.Focus`, `app.Hover`, `app.Expect(loc,
telatest.Checked(true))`, `app.AriaSnapshot()`. Touch and pen have no
`telatest` action yet; the package's own tests inject them as platform events.

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
