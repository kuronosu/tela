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
| A list of options to select from, with the keyboard | [`aria.UseListBox`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseListBox) |
| The menu that a menu button opens | [`aria.UseMenu`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseMenu) |
| Tabs and their panels | [`aria.UseTabs`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseTabs) |
| A select: a button that shows the chosen item and opens a list | [`aria.UseSelect`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseSelect) |
| A text field that filters a list of suggestions | [`aria.UseCombobox`](https://pkg.go.dev/github.com/kuronosu/tela/aria#UseCombobox) |
| What is selected in those | [`aria.Selection`](https://pkg.go.dev/github.com/kuronosu/tela/aria#Selection) |

The select and the combobox are built on the list box and the popover.

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
press is `OnPress`, and the keys need Alt. The menu collection ([`UseMenu`](#menus-usemenu))
reads `FocusStrategy` to focus its first or last item and calls
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

## Collections

A list box, a menu, a tab list, a select and a combobox are *collections*: keyed
lists of items with a focused key and a selection. You give the keys (any comparable type) on every
build and render one element per key with the props of `ItemProps(key)`; the
hook holds the state, moves the focus and selects, with React Aria's rules.

All of them take `Keys []K`, `DisabledKeys []K` and (all but tabs) `TextValue
func(K) string`, and return a result with `Props` for the collection's element
(for the select and the combobox, `ListBox.Props`) and these, shared by all
(`aria.Collection[K]`):

| | |
|---|---|
| `ItemProps(k)` | The props of the item with key `k`: role, `aria-selected` / `aria-checked` / `aria-disabled`, tab stop, ref and handlers. |
| `IsSelected(k)`, `IsFocused(k)`, `IsFocusVisible(k)`, `IsDisabled(k)` | The state, for styling in Go. |
| `Focused()` | The focused key and whether there is one. |
| `FocusKey(k)`, `Focus(strategy)`, `ClearFocus()` | Move the focus from a handler or an effect. |
| `ItemRef(k)` | The ref `ItemProps` attaches (one ref per element: do not attach another). |
| `Section(name)` | The props of a group of items and of its heading. |

Style items with the `aria-selected:`, `aria-checked:` and `aria-disabled:`
variants (the item's own props, so they match exactly when the item is
selected, checked or disabled), with `focus-visible:` (the element that has the
focus, a ring only after the keyboard) and with the booleans above. A disabled
item also gets `Disabled`, so `disabled:` matches too.

### The selection: `aria.Selection`

```go
sel := aria.SelectionOf("b", "c")
sel.Has("b")             // true
sel.With("a").Keys()     // [b c a]
sel.Without("b").Keys()  // [c]
sel.In(allKeys)          // the selected keys in the order of allKeys
aria.All[string]()       // React Aria's "all"
```

A `Selection[K]` is an immutable set: its methods return new values, so it is
safe in `tela.UseState` and shared freely. `All` is a selection of every item
that can be selected; it does not list the keys (`Len` is 0), so turn it into
keys with `Resolve(keys)` when you need them. `Equal` compares the keys
(`==` compares identity, so two equal sets are not `==`).

The **zero** `Selection` means "not given": a hook option left at it is
*uncontrolled*, the hook keeps the selection (starting at `DefaultSelected`).
A non-zero `Selected` is *controlled*: the collection shows it and only asks
`OnSelectionChange` for the next one, so a change you do not store is refused.
`aria.SelectionOf[string]()` (no keys) is a controlled, empty selection.
`OnSelectionChange` is also called for an uncontrolled collection.

```go
// Uncontrolled.
aria.ListBoxOptions[string]{Keys: fruit, SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("banana")}

// Controlled.
sel, setSel := tela.UseState(aria.SelectionOf("banana"))
aria.ListBoxOptions[string]{Keys: fruit, SelectionMode: "multiple", Selected: sel, OnSelectionChange: setSel.Set}
```

### List boxes: `UseListBox`

```go
var fruit = []string{"apple", "banana", "cherry"}

lb := aria.UseListBox(aria.ListBoxOptions[string]{
	Keys: fruit, TextValue: strings.ToUpper, SelectionMode: "single", AriaLabel: "Fruit",
})
return Div(lb.Props, Class("w-48 rounded-md border p-1"),
	Range(len(fruit), func(i int) tela.Node {
		k := fruit[i]
		return Div(lb.ItemProps(k), Key(k),
			Class("rounded px-2 py-1 aria-selected:bg-primary aria-selected:text-primary-foreground focus-visible:ring-2"),
			Text(k))
	}))
```

```yaml
- listbox "Fruit":
  - option "apple"
  - option "banana" [selected]
  - option "cherry"
```

Render the items as `Div`s or `Li`s, not Buttons (a Button activates on Enter
and Space by itself). The options:

| Option | Does |
|---|---|
| `SelectionMode` | `"none"` (default), `"single"`, `"multiple"`. |
| `SelectionBehavior` | `"toggle"` (default: a click toggles the item, so plain clicks build a multiple selection) or `"replace"` (a click selects only that item; Ctrl+click toggles, Shift+click selects a range; the arrow keys move the selection). |
| `Selected`, `DefaultSelected`, `OnSelectionChange` | See above. |
| `DisallowEmptySelection`, `DisallowSelectAll` | Keep one item selected; turn off Ctrl+A. |
| `OnAction` | Called when an item is activated: Enter, a tap, or a click when nothing is selected (`"toggle"`) or the list does not select. With `"replace"` and a selection, a click selects and a double click or Enter runs the action. |
| `Orientation`, `Direction` | `"horizontal"` lists use the left and right arrows (swapped by `"rtl"`). |
| `ShouldFocusWrap` | ArrowDown on the last item focuses the first. |
| `ShouldFocusOnHover` | The mouse focuses the item it moves over. |
| `AutoFocus`, `FocusStrategy` | Focus on mount: the first selected item, else `aria.FocusFirst` / `aria.FocusLast`, else the list. |
| `AriaLabel`, `AriaLabelledBy`, `Ref` | Name the list; share its ref with other hooks. |

**Keyboard.** Tab enters the list (the first selected item, else the first, takes
the focus; only the focused item is a tab stop). ArrowUp and ArrowDown, Home and
End move the focus and skip disabled items; PageUp and PageDown move a
viewport of items; typing text moves to the next item that starts with it
(case-insensitive; the typed string expires after one second on the frame clock);
Space and Enter select or act; with a multiple selection Shift+ArrowDown extends
the range, Ctrl+A (Cmd+A on a Mac) selects everything, Ctrl+ArrowDown moves
without selecting (`"replace"`) and Escape clears the selection.

**Sections.** `lb.Section("fruit")` returns `Props` for a group of items (role
`group`, named by its heading) and `HeadingProps` for the heading; the keys stay
one flat list, so the arrow keys cross sections.

```yaml
- listbox "Food":
  - group "Fruit":
    - text: Fruit
    - option "Apple"
```

#### Long lists

Give the items to a [`h.VirtualList`](Scrolling-and-VirtualList) with `lb.Props`
on the list itself (it carries the ref the hook scrolls through):

```go
h.VirtualList(len(keys), func(int) float32 { return 32 }, func(i int) tela.Node {
	return Div(lb.ItemProps(keys[i]), Class("h-8"), Text(keys[i]))
}, lb.Props, Class("h-64"))
```

Keys of items that are not rendered can be focused: End, typeahead and the page
keys scroll the list to the item, which takes the focus when it mounts. Ctrl+A
selects the whole list.

#### Virtual focus

With `ShouldUseVirtualFocus` the list keeps the focus and *highlights* an item
(`aria-activedescendant`); the items are not focusable and `IsFocused(k)` says
which one is highlighted. `ExternalFocus` is the same for a list that never has
the focus because another element does, a combobox's input: that element calls
`lb.HandleKeyDown` from its key handler and puts `lb.ActiveDescendant()` on
itself; a press on an option leaves the focus where it is.

```go
Input(AriaLabel("Search"), OnKeyDown(lb.HandleKeyDown), lb.ActiveDescendant(), OnBlur(lb.ClearFocus))
```

The input keeps its own Home, End, Space, Ctrl+A and Escape, and there is no
typeahead; the arrows, PageUp, PageDown and Enter go to the list.

### Menus: `UseMenu`

`UseMenu` is the collection of a [menu button](#menu-buttons-usemenutrigger). Call
it in the component that is rendered while the menu is open (it focuses an item
when it mounts), give it the trigger's `FocusStrategy`, `Close` and `MenuRef`,
and put the trigger's `MenuProps` on the same element: they hold the focus
scope that gives the focus back to the trigger, so `UseMenu` adds none.

```go
func ActionsButton() tela.Node {
	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
	pop := aria.UsePopover(aria.PopoverOptions{
		Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
		Placement: "bottom-start", Modal: true,
	})
	return Fragment(
		Button(mt.TriggerProps, Text("Actions")),
		Portal(If(mt.Open, ActionsMenu{Trigger: mt, Popover: pop})))
}

type ActionsMenu struct {
	Trigger aria.MenuTriggerResult
	Popover aria.PopoverResult
}

func (m ActionsMenu) Build() tela.Node {
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: actions, TextValue: strings.ToUpper,
		OnAction: run, OnClose: m.Trigger.Close,
		AutoFocus: true, FocusStrategy: m.Trigger.FocusStrategy, Ref: m.Trigger.MenuRef,
	})
	return Div(m.Popover.Props, m.Trigger.MenuProps, menu.Props,
		Class("w-48 rounded-md border bg-popover p-1 shadow-md"),
		Range(len(actions), func(i int) tela.Node {
			return Div(menu.ItemProps(actions[i]), Key(actions[i]),
				Class("rounded px-2 py-1.5 focus-visible:bg-accent aria-disabled:opacity-50"), Text(actions[i]))
		}))
}
```

```yaml
- menu "Actions":
  - menuitem "Edit"
  - menuitem "Duplicate"
  - menuitem "Archive" [disabled]
```

ArrowDown on the trigger opens the menu with the first item focused, ArrowUp
the last; a mouse opens it with the menu itself focused. Enter, Space and
a click choose an item: it is selected (with a selection mode), `OnAction(key)`
runs, `OnClose` closes the menu and the focus goes back to the trigger. Escape
closes it. The mouse focuses the item it moves over; typeahead and disabled
items work as in a list box.

`SelectionMode: "single"` makes the items radio items (`menuitemradio`) and
`"multiple"` checkbox items (`menuitemcheckbox`), with `aria-checked`:

```yaml
- menu "View":
  - menuitemcheckbox "Ruler" [checked]
  - menuitemcheckbox "Grid"
```

Choosing an item closes the menu unless the selection is multiple (several
checkbox items are toggled in a row); `CloseOnSelect` (a `*bool`) overrides it.
A single selection never unchecks by choosing the checked item. There are no
submenus yet.

### Tabs: `UseTabs`

```go
tabs := aria.UseTabs(aria.TabsOptions[string]{Keys: names, AriaLabel: "Settings"})
sel, _ := tabs.Selected.First()
return Div(
	Div(tabs.Props, Class("flex gap-1 border-b"),
		Range(len(names), func(i int) tela.Node {
			k := names[i]
			return Div(tabs.TabProps(k), Key(k),
				Class("px-3 py-2 aria-selected:border-b-2 aria-selected:border-primary focus-visible:ring-2"), Text(k))
		})),
	Div(tabs.PanelProps(sel), Class("p-4"), panel(sel)))
```

```yaml
- tablist "Settings":
  - tab "account" [selected]
  - tab "password"
- tabpanel "account": account settings
```

`UseTabs` is React Aria's `useTabList`, `useTab` and `useTabPanel` in one hook.
The selected tab is the only tab stop; the arrow keys (up and down for
`Orientation: "vertical"`; swapped by `Direction: "rtl"`) move between tabs and
wrap, Home and End jump, and disabled tabs are skipped. By default the focused
tab is selected (`KeyboardActivation: "automatic"`); with `"manual"` the arrows
only move the focus and Enter or Space selects. A tab list always has one
selected, enabled tab: the first when none is given. The selection is a
`Selection` of one key, uncontrolled or controlled like any other
(`tabs.Selected.First()` is the key). A tab has `aria-controls` the panel while
it is selected and `PanelProps(k)` names the panel by its tab; render the panel
of the selected tab only. `PanelProps` makes the panel a tab stop (as React Aria
does when it has no focusable content): add `TabIndex(-1)` after it when the
panel has controls of its own.

### Selects: `UseSelect`

A select is a button that shows the chosen item and opens a list of options
next to it. `UseSelect` is the menu button (`UseMenuTrigger` with
`aria-haspopup="listbox"`), a list box and a popover in one hook. It returns the
props of five elements: the label, the trigger, the element inside it that shows
the value, the popover and the list.

```go
sel := aria.UseSelect(aria.SelectOptions[string]{
	Keys: fruit, TextValue: capital, Placeholder: "Pick a fruit", MatchWidth: true,
})
return Fragment(
	Span(sel.LabelProps, Text("Favorite fruit")),
	Button(sel.TriggerProps, Class("flex w-48 justify-between rounded-md border px-3 py-2"),
		Span(sel.ValueProps, Text(sel.Value)), Icon(chevronDown)),
	Portal(If(sel.Open,
		Div(sel.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
			Div(sel.ListBox.Props, Class("max-h-60 overflow-auto"),
				Range(len(fruit), func(i int) tela.Node {
					k := fruit[i]
					return Div(sel.ListBox.ItemProps(k), Key(k),
						Class("rounded px-2 py-1.5 focus-visible:bg-accent aria-selected:font-medium"),
						Text(capital(k)))
				}))))))
```

The aria snapshot, closed and open, reads like React Aria's select in Chromium
(checked against Playwright): a `button` named by its value and its label, and
the `listbox` next to it.

```yaml
- text: Favorite fruit
- button "Banana Favorite fruit" [expanded]: Banana
- listbox "Favorite fruit":
  - option "Apple"
  - option "Banana" [selected]
  - option "Cherry" [disabled]
```

The options are those of a [list box](#list-boxes-uselistbox) for `Keys`,
`TextValue` (the text shown for the selected item, `fmt.Sprint(key)` when
omitted), `DisabledKeys` and the single `Selected`, `DefaultSelected` and
`OnSelectionChange`; those of a [menu button](#menu-buttons-usemenutrigger) for
`Open`, `DefaultOpen` and `OnOpenChange`; and these:

| Option | Does |
|---|---|
| `Placeholder` | What `sel.Value` says while nothing is selected. |
| `Disabled` | The trigger is inert and disabled. |
| `Placement`, `Offset`, `Floating` | Where the popover goes (`"bottom-start"` by default), as in [`UsePopover`](#popovers-usepopover). |
| `MatchWidth` | The popover is as wide as the trigger. |
| `Modal` | The rest of the window is inert while the list is open (React Aria's select is modal; the default here is a non-modal popover). |
| `DisallowTypeAhead` | Turns off typeahead on the trigger and in the list. |
| `AriaLabel` | Names the select when there is no label element. |
| `Ref` | The trigger's ref, for hooks used together on one element. |

**Keyboard and pointer.** A press, Enter, Space and ArrowDown open the list
(ArrowUp too, with the last item as the strategy); the selected item has the
focus, else the first (the last for ArrowUp), else the list itself. Choosing an
item (a click, Enter or Space) selects it, closes the list and gives the focus
back to the trigger; choosing the selected item closes it without a change.
Escape and a press outside close it, Tab closes it and moves on from the
trigger, the mouse focuses the item it moves over, and typeahead and disabled
items work as in a list box. **On the closed trigger** typing text selects the
next item that starts with it (a one-second buffer; a space inside a search is a
character) and ArrowLeft and ArrowRight select the previous and next item, with
the list closed, as React Aria does.

**Name.** The trigger is named by the value followed by the label, the way React
Aria sets `aria-labelledby="value label"`: `LabelProps` marks the label (a click
on it focuses the trigger) and `ValueProps` the element that shows the value;
`AriaLabel` adds a name when there is no label. The list is named by the label
too.

The list lives in the component that calls the hook, and its items are rendered
by you only while `sel.Open`. There is no hidden native `<select>`: a `Form`
does not submit the value, read `sel.Selected` (`sel.Selected.First()` is the
key) in `OnSubmit`.

### Comboboxes: `UseCombobox`

A combobox is a text field with a list of suggestions. `UseCombobox` holds the
text, the filter, the selection and the open state, and returns the props of the
input, an optional toggle button, the popover and the list. The input **keeps
the focus** at all times: the list items are highlighted (virtual focus,
`aria-activedescendant`), never focused.

```go
cb := aria.UseCombobox(aria.ComboboxOptions[string]{
	Items: fruit, TextValue: capital, AriaLabel: "Fruit", MatchWidth: true,
})
return Fragment(
	Div(Class("flex"),
		Input(cb.InputProps, Class("w-48 rounded-l-md border px-3 py-2")),
		Button(cb.ButtonProps, Class("rounded-r-md border px-2"), Icon(chevronDown))),
	Portal(If(cb.Open,
		Div(cb.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
			Div(cb.ListBox.Props, Class("max-h-60 overflow-auto"),
				Range(len(cb.Keys), func(i int) tela.Node {
					k := cb.Keys[i]
					return Div(cb.ListBox.ItemProps(k), Key(k), Class("rounded px-2 py-1.5"),
						If(cb.ListBox.IsFocused(k), Class("bg-accent")), Text(capital(k)))
				}))))))
```

```yaml
- combobox "Fruit" [expanded]: blu
- button "Show suggestions" [expanded]: v
- listbox "Fruit":
  - option "Blueberry"
```

**The items.** Give `Items` (all of them) and the hook shows the ones that match
the text with `Filter` (`aria.Contains` by default, or `aria.StartsWith`,
`aria.EndsWith` or your own `func(text, query string) bool`); render
`cb.Keys`. Or give `Keys`, already filtered by you (a server search, a fuzzy
match), and the hook shows them as they are. `aria.Contains` and its siblings
ignore case and diacritics (`"zur"` finds "Zürich": React Aria's collator with
base sensitivity). After the list was opened by hand (the button, the arrows)
every item is shown until the text changes; `cb.ShowAll` says so, for a caller
who filters `Keys`.

**The text.** `InputValue` (a `*string`), `DefaultInputValue` and
`OnInputChange` work like the selection of a list box: uncontrolled by default,
controlled when `InputValue` is non-nil. The text starts as the text of the
selected item, and choosing an item sets it to that item's `TextValue`.

| Option | Does |
|---|---|
| `MenuTrigger` | When the list opens: `aria.MenuTriggerInput` (typing; the default), `aria.MenuTriggerFocus` (also when the input gets the focus, with every item) or `aria.MenuTriggerManual` (only the arrows and the button). |
| `AllowsCustomValue` | The text may stay as typed when the input loses the focus; the selection is cleared. Without it the text goes back to that of the selected item (empty when none is). |
| `AllowsEmptyCollection` | The list stays open when nothing matches, to show a "no results" message. |
| `Disabled`, `ReadOnly` | The input and the button are inert; a read-only input never opens the list. |
| `ShouldFocusWrap` | The arrows go around the ends. |
| `AnchorRef`, `MatchWidth`, `Placement`, `Offset`, `Floating` | The popover is placed next to `AnchorRef` (the input by default; give the wrapper of the input and the button), as wide as it with `MatchWidth`. |
| `Selected`, `DefaultSelected`, `OnSelectionChange`, `Open`, `DefaultOpen`, `OnOpenChange`, `TextValue`, `DisabledKeys`, `AriaLabel`, `Ref` | As above. |

**Keyboard** (React Aria's `useComboBox`). Typing filters the list and, in the
default mode, opens it. ArrowDown and ArrowUp open it on the first or last item
(the selected item when there is one) and then highlight the next and previous;
PageUp and PageDown move a page. Enter chooses the highlighted item (sets the
text, closes the list: selection and text agree); with none highlighted, or a
closed list, it commits the text as Tab does. Escape closes the list and puts the
text back to that of the selected item (cleared when none is); it reaches an
enclosing dialog only when there was nothing to put back. Home, End, Space,
ArrowLeft and ArrowRight stay with the text field (the arrows drop the
highlight). **While an IME composes text no key navigates**, and the list follows
the preedit text without disturbing the composition.

**Pointer.** Pressing an option chooses it and the input keeps the focus; the
mouse highlights the option it moves over. A press outside moves the focus away,
which commits the text and closes the list (the press reaches what it hits). The
toggle button opens the list with every item or closes it, and keeps the focus on
the input; it is not a tab stop and is labelled "Show suggestions" (an
`AriaLabel` after `ButtonProps` replaces the label).

The label: `cb.LabelProps` on a label element names the input and a click on it
focuses the input; without one, give `AriaLabel`.

### How it differs from React Aria

- No collection object, `Item` or `Section` elements and no `Node` type: the
  keys are a Go slice and the items are ordinary elements. One state per hook and
  one ref per key replace the hook that React Aria calls in every item.
- The selection is an immutable `Selection` with an `All` value, controlled
  when non-zero, instead of `selectedKeys` as a `Set` or `"all"`.
- A menu's radio items cannot be unchecked by choosing them again; there are no
  submenus, per-item `closeOnSelect`, `disabledBehavior`, links or press, drag and
  release across menu items.
- Typeahead is a case-insensitive prefix match (no collator).
- Select: no hidden native select (a `Form` does not submit it), the popover is
  non-modal by default, and an item is chosen on the click (React Aria selects on
  the press's release, so a press that starts on the trigger can be dragged onto
  an item); the trigger's `aria-labelledby` points at the value and label elements
  you mark with `ValueProps` and `LabelProps`; no `isRequired`, validation,
  description or error message.
- Combobox: `Items` with `Filter`, or `Keys` filtered by you, replace
  `defaultItems`/`items` with `defaultFilter`; `Contains` and `StartsWith` fold
  case and diacritics (NFD without combining marks) instead of using a collator;
  a press outside the popover does not dismiss it by itself: the input blurs and
  that commits the text; blur always commits (no `shouldCloseOnBlur`);
  `aria-autocomplete` is always `list`; the button's label is the English "Show
  suggestions" (React Aria localizes it); `OnOpenChange` does not tell what opened
  the list; no `name` or `formValue`.
- Shift+Tab into a list focuses its first item, not the last one: Tela's focus
  events do not carry the element the focus came from.
