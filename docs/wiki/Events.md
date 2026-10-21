# Events

Events follow the DOM and React. A handler is a function that receives the
event **by value**; the event's methods control what happens next. Handlers
are props of package `h`: `OnClick`, `OnKeyDown`, `OnDrag`...

```go
func Toggle() tela.Node {
	open, setOpen := tela.UseState(false)

	return Div(Class("rounded-xl border p-4"),
		OnClick(func(e tela.MouseEvent) {
			e.StopPropagation() // an ancestor's OnClick does not run
			setOpen.Set(!open)
		}),
		IfElse(open, Text("Open"), Text("Closed")),
	)
}
```

Every handler prop panics when its handler is `nil`, with a message that shows
the right call.

## How events travel

- A pointer event's **target** is the topmost element under the pointer as
  painted, inside the clips and transforms of its ancestors, skipping
  `pointer-events-none` and invisible elements.
- Events **bubble** from the target to the root, calling each ancestor's
  handler. `StopPropagation()` stops that. `OnMouseEnter`, `OnMouseLeave`,
  `OnPointerEnter`, `OnPointerLeave`, `OnLongPress`, the drag handlers and
  `OnScroll` do not bubble.
- A [`Disabled`](https://pkg.go.dev/github.com/kuronosu/tela/h#Disabled) element and its descendants receive no pointer events.
- `PreventDefault()` cancels Tela's default action for the event (the focus
  move of Tab, the scroll of a wheel notch, the text a key types). Which
  default each event has is listed on its handler.
- After every discrete event (presses, releases, clicks, drag start and end,
  long press), pending state updates are applied and components rebuilt before
  the next event, so two clicks before a frame see each other's updates.
  Continuous events (moves, drag moves) are batched: state set there is applied
  once per frame.

## Handlers and their events

| Area | Handlers | Event |
|---|---|---|
| Mouse | [`OnClick`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnClick) [`OnDoubleClick`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnDoubleClick) [`OnContextMenu`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnContextMenu) [`OnMouseDown`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnMouseDown) [`OnMouseUp`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnMouseUp) [`OnMouseMove`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnMouseMove) [`OnMouseEnter`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnMouseEnter) [`OnMouseLeave`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnMouseLeave) | [`tela.MouseEvent`](https://pkg.go.dev/github.com/kuronosu/tela#MouseEvent) |
| Pointer (mouse, touch, pen) | [`OnPointerDown`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerDown) [`OnPointerMove`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerMove) [`OnPointerUp`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerUp) [`OnPointerCancel`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerCancel) [`OnPointerEnter`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerEnter) [`OnPointerLeave`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnPointerLeave) [`OnLongPress`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnLongPress) | [`tela.PointerEvent`](https://pkg.go.dev/github.com/kuronosu/tela#PointerEvent) |
| Drag | [`OnDragStart`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnDragStart) [`OnDrag`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnDrag) [`OnDragEnd`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnDragEnd) | [`tela.DragEvent`](https://pkg.go.dev/github.com/kuronosu/tela#DragEvent) |
| Keyboard | [`OnKeyDown`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnKeyDown) [`OnKeyUp`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnKeyUp) | [`tela.KeyboardEvent`](https://pkg.go.dev/github.com/kuronosu/tela#KeyboardEvent) |
| Focus | [`OnFocus`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnFocus) [`OnBlur`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnBlur) | [`tela.FocusEvent`](https://pkg.go.dev/github.com/kuronosu/tela#FocusEvent) |
| Wheel and scroll | [`OnWheel`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnWheel) [`OnScroll`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnScroll) | [`tela.WheelEvent`](https://pkg.go.dev/github.com/kuronosu/tela#WheelEvent), [`tela.ScrollEvent`](https://pkg.go.dev/github.com/kuronosu/tela#ScrollEvent) |
| Text fields and forms | [`OnChange`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnChange) [`OnBeforeInput`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnBeforeInput) [`OnCompositionStart`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnCompositionStart) [`OnCompositionUpdate`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnCompositionUpdate) [`OnCompositionEnd`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnCompositionEnd) [`OnSelect`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnSelect) [`OnSubmit`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnSubmit) | see [Forms and Text Input](Forms-and-Text-Input) |

The fields and methods of the event types are listed in
[API-Reference-tela](API-Reference-tela#types); the common ones are below.

## Mouse and pointer

Pointer events fire for every pointer: mouse, touch and pen. Mouse events are
the compatibility view of the primary pointer, in the DOM order: pointerdown,
mousedown, pointerup, mouseup, click.

| Field | Meaning |
|---|---|
| `X`, `Y` | Position in the window, in logical pixels |
| `LocalX`, `LocalY` | Position relative to the border box of the element whose handler runs |
| `Button`, `Buttons` | The button that changed ([`tela.MouseButtonLeft`](https://pkg.go.dev/github.com/kuronosu/tela#MouseButtonLeft), [`tela.MouseButtonRight`](https://pkg.go.dev/github.com/kuronosu/tela#MouseButtonRight), [`tela.MouseButtonMiddle`](https://pkg.go.dev/github.com/kuronosu/tela#MouseButtonMiddle)...) and the set held; `e.Buttons.Has(tela.MouseButtonLeft)` |
| `Shift`, `Ctrl`, `Alt`, `Meta` | Modifier keys (`Meta` is the Windows, Command or Super key) |
| `ClickCount` | 1 for a click, 2 for the second click of a double click |
| `Kind`, `PointerID`, `Pressure`, `IsPrimary` | On a `PointerEvent`: [`tela.PointerMouse`](https://pkg.go.dev/github.com/kuronosu/tela#PointerMouse), [`tela.PointerTouch`](https://pkg.go.dev/github.com/kuronosu/tela#PointerTouch) or [`tela.PointerPen`](https://pkg.go.dev/github.com/kuronosu/tela#PointerPen), and the pointer's identity |

```go
func Tab(title string, closeTab func()) tela.Node {
	return Div(Class("rounded-t-lg px-4 py-2 hover:bg-slate-100"),
		OnMouseDown(func(e tela.MouseEvent) {
			if e.Button == tela.MouseButtonMiddle {
				closeTab()
			}
		}),
		OnContextMenu(func(e tela.MouseEvent) { openMenu(e.X, e.Y) }),
		Text(title),
	)
}

func openMenu(x, y float32) {}
```

`OnDoubleClick` runs on the second click of a double click (two clicks within
500 ms and 5 px; 300 ms and 100 px for touch); the first click is not delayed,
so `OnClick` runs for both.

## Keyboard

Key events go to the focused element and bubble; with nothing focused they go
to the outermost element of the window's content. A [`tela.KeyboardEvent`](https://pkg.go.dev/github.com/kuronosu/tela#KeyboardEvent) has:

- `Key`, the **logical** key: what it means with the active layout and
  modifiers. A [`tela.KeyName`](https://pkg.go.dev/github.com/kuronosu/tela#KeyName) is either a named key ([`tela.KeyEnter`](https://pkg.go.dev/github.com/kuronosu/tela#KeyEnter),
  [`tela.KeyEscape`](https://pkg.go.dev/github.com/kuronosu/tela#KeyEscape), [`tela.KeyArrowDown`](https://pkg.go.dev/github.com/kuronosu/tela#KeyArrowDown)...) or the character the key types
  (`"a"`, `"A"`, `" "`). `Ctrl` does not change it: Ctrl+S has Key `"s"`.
- `Code`, the **physical** key (a [`tela.KeyCode`](https://pkg.go.dev/github.com/kuronosu/tela#KeyCode)), whatever the layout: [`tela.CodeKeyS`](https://pkg.go.dev/github.com/kuronosu/tela#CodeKeyS),
  [`tela.CodeKeyW`](https://pkg.go.dev/github.com/kuronosu/tela#CodeKeyW)... Use it for positions (WASD) and `Key` for meanings
  (Ctrl+Z).
- `Location` ([`tela.KeyLocationLeft`](https://pkg.go.dev/github.com/kuronosu/tela#KeyLocationLeft), [`tela.KeyLocationRight`](https://pkg.go.dev/github.com/kuronosu/tela#KeyLocationRight),
  [`tela.KeyLocationNumpad`](https://pkg.go.dev/github.com/kuronosu/tela#KeyLocationNumpad)), `Repeat`, and `Shift`, `Ctrl`, `Alt`, `Meta`.

```go
func Editor(save, closeEditor func()) tela.Node {
	return Div(TabIndex(0), Class("p-4 outline-none"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			switch {
			case e.Key == tela.KeyEscape:
				closeEditor()
			case e.Ctrl && e.Code == tela.CodeKeyS:
				e.PreventDefault()
				save()
			}
		}),
		Text("Press Ctrl+S to save, Esc to close"),
	)
}
```

After the handlers, Tela runs the default actions unless one called
`PreventDefault`: Tab and Shift+Tab move the focus through the focusable
elements; Enter on a focused `Button` clicks it on keydown, Space on keyup.
Typed text reaches text fields, which report it with `OnChange`.

## Focus

A [`Button`](https://pkg.go.dev/github.com/kuronosu/tela/h#Button) is focusable. Any other element becomes focusable with [`TabIndex`](https://pkg.go.dev/github.com/kuronosu/tela/h#TabIndex):
`TabIndex(0)` puts it in the Tab order in tree order, `TabIndex(-1)` makes it
focusable by click and by code only. Disabled and hidden elements are not
focusable. [`AutoFocus`](https://pkg.go.dev/github.com/kuronosu/tela/h#AutoFocus) focuses an element when it first appears. A press of the
primary button focuses the nearest focusable element under the pointer, and
blurs the focused one otherwise.

[`OnFocus`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnFocus) and [`OnBlur`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnBlur) **bubble**, like React's: a container's `OnFocus` also
runs when a descendant gains the focus, which makes "focus within" logic easy.
The `focus:`, `focus-visible:` and `focus-within:` [variants](Styling#variants)
style the focused element.

```go
func Combo() tela.Node {
	editing, setEditing := tela.UseState(false)
	return Div(
		OnFocus(func(tela.FocusEvent) { setEditing.Set(true) }),
		OnBlur(func(tela.FocusEvent) { setEditing.Set(false) }),
		Class("rounded-lg border p-2 focus-within:ring-2 focus-within:ring-indigo-500"),
		Input(AriaLabel("Name")),
		If(editing, Span(Class("text-xs text-slate-500"), Text("editing"))),
	)
}
```

To move the focus from code, attach an [`tela.ElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#ElementRef) with [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) and call
`Focus()`; see [Hooks](Hooks#handles-on-elements-useelementref).

## Drag

A press that moves farther than a few pixels (4 for a mouse, 18 for touch and
pen) before a long press becomes a drag, and does not click. Drag events go to
the innermost element with drag handlers. A [`tela.DragEvent`](https://pkg.go.dev/github.com/kuronosu/tela#DragEvent) carries `DeltaX` and
`DeltaY` (since the previous event), `OffsetX` and `OffsetY` (since the press,
so an element moved by them follows the pointer exactly) and `VelocityX`,
`VelocityY` in pixels per second (meaningful at the end, for flings).

Moving an element by repainting only is the job of [`tela.TranslateFn`](https://pkg.go.dev/github.com/kuronosu/tela#TranslateFn) with a
[`UseValue`](Hooks#observable-values-usevalue); no component builds while the
card follows the pointer:

```go
type Offset struct{ X, Y float32 }

func DraggableCard() tela.Node {
	offset := tela.UseValue(Offset{})
	start := tela.UseRef(Offset{})

	return Div(Class("w-48 rounded-xl bg-white p-6 shadow-lg"),
		Style(tela.TranslateFn(func() (x, y float32) {
			p := offset.Get()
			return p.X, p.Y
		})),
		OnDragStart(func(tela.DragEvent) { start.Current = offset.Latest() }),
		OnDrag(func(e tela.DragEvent) {
			offset.Set(Offset{start.Current.X + e.OffsetX, start.Current.Y + e.OffsetY})
		}),
		OnDragEnd(func(e tela.DragEvent) {
			if e.VelocityX > 800 {
				// a fling to the right
			}
		}),
		Text("Drag me"),
	)
}
```

The `examples/board` kanban board uses this to move cards between columns.

## Long press

[`OnLongPress`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnLongPress) runs when the primary button (or a touch) stays pressed for
500 ms without moving more than a few pixels (4 for a mouse, 18 for touch and
pen). The press then does not click or drag. It goes to the innermost element
with the handler and does not bubble.

```go
func Item(name string) tela.Node {
	return Div(OnLongPress(func(e tela.PointerEvent) { showMenu(e.X, e.Y) }), Text(name))
}

func showMenu(x, y float32) {}
```

In a test, hold the button with `telatest.Delay` or advance the manual clock;
see [Testing](Testing#the-clock).

## Wheel

[`OnWheel`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnWheel) observes a wheel notch or a touchpad scroll (DOM signs: positive
`DeltaY` scrolls down; a notch is 53 pixels, and Shift turns a vertical notch
horizontal). Unless a handler calls `PreventDefault()`, the nearest scrolling
element scrolls:

```go
func Canvas(zoom *tela.Value[float32]) tela.Node {
	return Div(OnWheel(func(e tela.WheelEvent) {
		if e.Ctrl { // Ctrl+wheel zooms instead of scrolling the page
			e.PreventDefault()
			zoom.Update(func(z float32) float32 { return z * (1 - e.DeltaY/500) })
		}
	}))
}
```

[`OnScroll`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnScroll) is covered in [Scrolling and VirtualList](Scrolling-and-VirtualList).

## See also

- [Accessibility](Accessibility): the same elements, seen by assistive
  technologies and tests.
- `telatest` simulates all of these with real events: [`App.Click`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Click),
  [`App.Hover`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Hover), [`App.Drag`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Drag), [`App.Press`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Press).
