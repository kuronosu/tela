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
| An overlay that Escape and a press outside dismiss, modal or not | [`aria.UseOverlay`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseOverlay) |
| A dialog or an alert dialog (role, name, focus) | [`aria.UseDialog`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseDialog) |
| A popover positioned next to its trigger and dismissed | [`aria.UsePopover`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UsePopover) |
| The open state of an overlay and its trigger's props | [`aria.UseOverlayTrigger`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseOverlayTrigger) |
| A menu button (press, long press, arrow keys) | [`aria.UseMenuTrigger`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseMenuTrigger) |
| A tooltip with delays | [`aria.UseTooltipTrigger`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseTooltipTrigger) |

The menu, listbox, select, tabs and combobox collections come later.

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
`tela.UseElementRef` and give it to each as `Ref`, as above. Two different
refs on one element are a mistake, and **dev mode panics** naming both call
sites and the fix (outside dev mode the last one wins, and a press never sees
the pointer leave). That includes an `h.Ref(other)` next to the props of a
hook: use the `Ref` field of a hook's result, or pass the ref to the hook. The
same ref twice is fine.

**Controlled or not.** A hook that holds state keeps it itself (uncontrolled,
starting at `DefaultSelected` or `DefaultOpen`), and lets you keep it
(controlled) when you pass a **pointer** to your value (`Selected: &on`,
`Open: &open`): it then shows it and only asks `OnChange` or `OnOpenChange`
for the next value, like an `Input` with `Value` and `OnChange`. A change you
do not store is refused. `OnChange` is also called for an uncontrolled
control, to follow it.

```go
box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})          // uncontrolled
box = aria.UseCheckbox(aria.ToggleOptions{OnChange: func(on bool) { save(on) }}) // uncontrolled, notified
on, setOn := tela.UseState(false)
sw := aria.UseSwitch(aria.ToggleOptions{Selected: &on, OnChange: setOn.Set}) // controlled
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
its `Form` (use `UseButton`'s `Type`, or call the handler from `OnPress`), and
a `Label` does not forward the click. A disabled press (`Disabled: true`) ignores everything. The hook
adds no role and no tab stop: for a button use `UseButton`.

## Buttons and toggles

`UseButton` is a press plus `Role("button")`, a tab stop and `Disabled`: it
makes a `Div` or an `A` a button, and a `Button` a correct one.

Its `Type` is HTML's: `"button"` (the default) does nothing to the `Form` that
contains it, `"submit"` submits it and `"reset"` resets it, when the press
completes (after `OnPress`), whatever the element is:

```go
save := aria.UseButton(aria.ButtonOptions{Type: "submit"})
return Form(OnSubmit(func(tela.SubmitEvent) { send() }),
	Input(Name("title")),
	Div(save.Props, Class("rounded-md bg-primary px-3 py-1.5"), Text("Send")))
```

`ElementRef.Click()` is the DOM's `element.click()`: from a handler or an effect
it dispatches a click (a virtual press for the hooks) at the element and runs
its activation behavior, so a hook-made button with `Type: "submit"` submits
through it, a `Button` in a `Form` submits or resets by its `h.Type`, and a
`Label` forwards to its control. A disabled or unmounted element ignores it.

`UseToggleButton`, `UseCheckbox` and `UseSwitch` take `aria.ToggleOptions`
(`Selected`, `DefaultSelected`, `OnChange`, the press callbacks `OnPress`,
`OnPressStart`, `OnPressEnd`, `OnPressChange` and `OnPressUp`, `Disabled`,
`ReadOnly`, `ID`, `Ref`) and return `Selected`, `Pressed`, `ID` and `Props`:

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

## Overlays

The overlay hooks follow React Aria's `useOverlay`, `useModalOverlay`,
`useDialog`, `usePopover`, `useOverlayTrigger`, `useMenuTrigger` and
`useTooltipTrigger`, with the open state built in. You render the overlay
(in a `Portal`, while it is open); the hooks give the props.

```go
func Filters() tela.Node {
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
	pop := aria.UsePopover(aria.PopoverOptions{
		Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
		Placement: "bottom-start", Offset: 4,
	})
	dlg := aria.UseDialog(aria.DialogOptions{Ref: trig.OverlayRef, AriaLabel: "Filters"})
	return Fragment(
		Button(trig.TriggerProps, Class("rounded-md border px-3 py-1.5"), Text("Filters")),
		Portal(Presence(If(trig.Open,
			Div(pop.Props, dlg.Props, Exit("opacity-0"),
				Class("w-64 rounded-md border bg-popover p-4 shadow-md transition starting:opacity-0"),
				Text("Content"))))))
}
```

The trigger, the popover and the dialog all put props on the overlay's
element, so they share its ref: `trig.OverlayRef` is made by the trigger hook
and passed to the others as `Ref` (the dev panic of the previous section tells
you when one is missing).

### The overlay stack: `UseOverlay`

`UseOverlay(aria.OverlayOptions{Open, OnClose, Dismissable, KeyboardDismissDisabled, ShouldCloseOnBlur, ShouldCloseOnInteractOutside, Modal})`
returns `Props` that put the element on the **overlay stack** while it is
mounted and open:

- **Escape** calls `OnClose` of the top overlay only, and nothing below it
  sees the key. With `KeyboardDismissDisabled` the overlay still takes the key
  but ignores it.
- A **press outside** (the left button, a touch or a pen; at its release, with
  the press kept from what is under it and its click cancelled) calls
  `OnClose` of the top overlay when it is `Dismissable` and
  `ShouldCloseOnInteractOutside(e)` (default: yes) allows it. Inside means
  inside in the element tree: the content of a `Portal` rendered from inside
  an overlay, and the overlays opened from it, count, so a press in a
  popover opened from a dialog does not close the dialog.
- **`Modal`** makes everything outside the overlay (and outside the overlays
  above it) inert while it is open: it is not hit, cannot be focused, is out
  of the Tab order and is hidden from the aria snapshot, as React Aria's
  `ariaHideOutside`. This folds React Aria's `useModalOverlay` into one hook:
  there are no `underlayProps`, since a press outside already hits nothing
  below; draw a scrim as an ordinary element.
- `ShouldCloseOnBlur` calls `OnClose` when the focus moves to another element
  outside the overlay.

An alert dialog is an overlay that is not `Dismissable`.

### Dialogs: `UseDialog`

`UseDialog(aria.DialogOptions{Role, Modal, AriaLabel, Ref})` returns `Props`
and `TitleProps`: the `dialog` (or `alertdialog`) role, `aria-modal`, the name
(the element with `TitleProps`, or `AriaLabel`) and a focus scope that moves
the focus to the first tabbable element inside when the dialog opens (the
dialog itself when there is none), keeps Tab inside it when `Modal`, and gives
the focus back to the element that had it when the dialog closes.

```go
dlg := aria.UseDialog(aria.DialogOptions{Modal: true, Ref: trig.OverlayRef})
ov := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true})
Portal(If(trig.Open,
	Div(ov.Props, dlg.Props, Class("w-96 rounded-xl bg-background p-6 shadow-xl"),
		H2(dlg.TitleProps, Text("Delete project?")), ...)))
```

```yaml
- dialog "Delete project?":
  - heading "Delete project?" [level=2]
  - button "OK"
  - button "Cancel"
```

### The trigger: `UseOverlayTrigger`

Holds the open state (`Open *bool` to control it, `DefaultOpen`,
`OnOpenChange`) and returns `TriggerProps` (a press that toggles, the `button`
role, `aria-haspopup` from `Type` ("dialog", "menu", "listbox", "tree" or
"grid"), `aria-expanded` and, while open, `aria-controls` the overlay),
`Open`, `SetOpen`, `Toggle`, `Close`, and the refs `Ref` (the trigger's) and
`OverlayRef`.

### Popovers: `UsePopover`

`UsePopover(aria.PopoverOptions{Trigger, Open, OnClose, Placement, Offset, Floating, Modal, ...})`
positions the element with `UseFloating` (the placement and offset, `Flip`
and `Shift(12)`, then the `Floating` options you add, such as a `Size`, then
an arrow) and dismisses it with the overlay stack: Escape, a press outside, or
the focus going elsewhere close it, but not a press on the trigger, which
toggles it (a **modal** popover closes on a trigger press, since the trigger
is inert). `ArrowProps` goes on an arrow element inside. The popover is where
its trigger says in the first frame that shows it.

### Menu buttons: `UseMenuTrigger`

`UseMenuTrigger(aria.MenuTriggerOptions{Trigger, Type, Open, DefaultOpen, OnOpenChange, Disabled, OnPress})`
returns `TriggerProps`, `MenuProps`, `Open`, `FocusStrategy`, `SetOpen`,
`Toggle`, `Close`, `Ref` and `MenuRef`. A mouse or a pen opens the menu at the
press and a touch at the release (focusing the menu itself); Enter, Space and
ArrowDown open it with `FocusStrategy` `"first"`, ArrowUp with `"last"`. With
`Trigger: "longPress"` a press that lasts opens it (`"first"`), the quick
press is `OnPress`, and the keys need Alt. The menu collection (`UseMenu`,
coming) reads `FocusStrategy` to focus its first or last item and calls
`Close` when an item is chosen. `MenuProps` holds the ref, `aria-labelledby`
the trigger, and a focus scope that gives the focus back to the trigger.

### Tooltips: `UseTooltipTrigger`

```go
tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{})
fl := aria.UseFloating(tip.Ref, aria.Placement("top"), aria.Offset(6), aria.Flip(), aria.Shift(8))
return Fragment(
	Button(tip.TriggerProps, Text("Save")),
	Portal(Presence(If(tip.Open,
		Div(tip.TooltipProps, fl.Props, Exit("opacity-0"),
			Class("rounded bg-foreground px-2 py-1 text-xs text-background transition starting:opacity-0"),
			Text("Saves the file (Ctrl+S)"))))))
```

React Aria's rules: the tooltip opens `Delay` (1.5 s) after the pointer
rests on the trigger, or at once when the keyboard focuses it (a focus that a
mouse press gave does not count), and closes `CloseDelay` (0.5 s) after the
pointer leaves, when the trigger is pressed or a key is typed on it, when the
focus leaves, and with Escape (which closes it before anything under it).
Moving the pointer onto the tooltip keeps it open. The tooltips of a window
are a group: opening one closes the others, and while one is open, or for
`CloseDelay` after the last closed, the next one opens at once (the warm-up
and cool-down). `Trigger: "focus"` ignores the pointer. A zero delay is the
default; `aria.NoDelay` is none. The tooltip's `TooltipProps` give it the
`tooltip` role and make the trigger `aria-describedby` it (`h.AriaDescribedBy`).

The delays run on the frame clock, with no goroutine: `app.Advance` drives
them in `telatest`, and a pending or open tooltip asks for no frames.

```yaml
- button "Save"
- tooltip "Saves the file (Ctrl+S)"
```

### Exit animations

Render the overlay inside a `Presence` with `If(open, ...)` and give its
element `Exit` classes and a transition (and `starting:` for the entrance), as
in the examples. While it exits the overlay is **inert** (ADR-080): it is
out of the overlay stack, so the page under it works, a second Escape reaches
the next overlay, and it is gone from the aria snapshot; the exit ends when
its transitions do.

### How it differs from React Aria

- One hook for modal and non-modal overlays (`Modal`), and no underlay props;
  popovers are non-modal unless `Modal` is set (React Aria's are modal unless
  `isNonModal`).
- `ShouldCloseOnInteractOutside` receives the press (`tela.PointerEvent`,
  with its position), not the element; blur always closes.
- A popover's trigger is excluded from outside presses by comparing the
  press with the trigger's bounds.
- Tooltips: the Escape is a window-level default action (the tooltip is on the
  overlay stack as an Escape-only overlay, so it does not take presses), and
  there is no separate `UseTooltip`.
- `aria-haspopup="dialog"` is set for dialogs (React Aria omits it).
