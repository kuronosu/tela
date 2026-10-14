# Scrolling and VirtualList

## Scroll containers

An element scrolls when its classes say so: `overflow-auto` or
`overflow-scroll`, or per axis `overflow-x-*` and `overflow-y-*`. Give it a
size (`h-96`, `flex-1` in a column, `h-screen`), as with CSS.

```go
func Inbox(rows tela.Node) tela.Node {
	return Div(Class("h-96 overflow-y-auto rounded-lg border"), rows)
}
```

A scroll container scrolls with the wheel, the touchpad (with a kinetic fling
when the fingers lift), touch, the keyboard (arrows, Page Up and Down, Home and
End, Space) and its scrollbars. `overflow-hidden` scrolls only from code.
Wheel scrolling chains to the scrolling ancestors at an edge unless the element
has `overscroll-contain`.

The **window** scrolls like a browser page when the content is taller or wider
than it. `h-screen` and `h-full` on the outermost element still mean the
window's height. Set `RootClass: "overflow-hidden"` on the [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App) to keep the
window from scrolling and scroll an inner element instead.

### Scrollbars and animation

By default scrollbars are thin overlay bars that appear while scrolling.
[`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App) has two switches, and each has a class for a single element:

| [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App) field | Class | Effect |
|---|---|---|
| `ClassicScrollbars: true` | `scrollbar-classic` / `scrollbar-overlay` | Always-visible classic bars that take space (a Windows look) |
| `InstantScroll: true` | `scroll-instant` / `scroll-smooth` | Scrolling jumps instead of animating, and touch and touchpad scrolling do not fling |

The system's reduced-motion setting always turns the animation off.

## Scrolling from code

Attach an [`tela.ElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#ElementRef) with [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) (from [`tela.UseElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#UseElementRef)) and call its
methods from handlers and effects:

- [`ElementRef.ScrollTo`](API-Reference-tela#func-elementref-scrollto)(x, y) and [`ElementRef.ScrollBy`](API-Reference-tela#func-elementref-scrollby)(dx, dy) scroll
  to or by offsets, clamped to the content; [`ElementRef.ScrollOffset`](API-Reference-tela#func-elementref-scrolloffset) returns
  the offsets (the DOM's `scrollLeft` and `scrollTop`). They jump, or animate
  when the element has `scroll-smooth`.
- [`ElementRef.ScrollIntoView`](API-Reference-tela#func-elementref-scrollintoview) scrolls every scrolling ancestor as little as
  possible so the element is visible.
- [`ElementRef.Bounds`](API-Reference-tela#func-elementref-bounds) returns the element's border box in window coordinates
  as a [`tela.Rect`](https://pkg.go.dev/github.com/kuronosu/tela#Rect), moved by scrolling and transforms (`getBoundingClientRect`).

```go
func Page(rows tela.Node) tela.Node {
	list := tela.UseElementRef()
	return Div(Class("relative"),
		Div(Ref(list), Class("h-96 overflow-y-auto scroll-smooth"), rows),
		Button(OnClick(func(tela.MouseEvent) { list.ScrollTo(0, 0) }), Text("Top")),
	)
}
```

## Observing scrolling

[`OnScroll`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnScroll) runs after the element's offsets changed, by the user or by code,
at most once per frame, on the scrolling element only (it does not bubble). The
[`tela.ScrollEvent`](https://pkg.go.dev/github.com/kuronosu/tela#ScrollEvent) has the DOM's properties in logical pixels: `ScrollLeft`,
`ScrollTop`, `ScrollWidth`, `ScrollHeight`, `ClientWidth` and `ClientHeight`.

```go
func Feed(rows tela.Node, loadMore func()) tela.Node {
	return Div(Class("h-96 overflow-y-auto"),
		OnScroll(func(e tela.ScrollEvent) {
			if e.ScrollTop+e.ClientHeight >= e.ScrollHeight-200 {
				loadMore()
			}
		}),
		rows,
	)
}
```

To tie a **style** to the scroll position without rebuilding anything, use
[`tela.UseScroll`](https://pkg.go.dev/github.com/kuronosu/tela#UseScroll), which gives `Value`s of the offsets that a deferred style
closure can read; see [Hooks](Hooks#scroll-offsets-and-bounds). [`OnWheel`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnWheel)
observes the wheel and can cancel its scrolling; see [Events](Events#wheel).

## VirtualList

[`VirtualList`](https://pkg.go.dev/github.com/kuronosu/tela/h#VirtualList) is a vertical scrolling list of `count` items that builds, lays
out and paints only the items in view plus a few more on each side, so a list
of 100,000 rows costs about as much as one screenful of them. The node **is**
the scrolling element: props style it like a `Div` (`Class`, `Style`, `Ref`,
handlers, `Key`, `Role` and the `Aria` props), and it is `overflow-y-auto`
unless a class sets another overflow.

```go
type Person struct{ Name string }

var people = make([]Person, 100_000)

type RowProps struct{ Index int }

func Row(p RowProps) tela.Node {
	return Div(Role("listitem"), Class("flex h-16 items-center border-b px-4"),
		Textf("%d. %s", p.Index+1, people[p.Index].Name))
}

func People() tela.Node {
	return VirtualList(len(people),
		func(int) float32 { return 64 }, // estimated height of item i
		func(i int) tela.Node { return tela.CP(Row, RowProps{Index: i}) },
		Class("h-96 rounded-lg border"),
	)
}
```

- `estimateSize(i)` estimates the height of item `i` before it is built; the
  item is then measured once laid out, and the measurement replaces the
  estimate. Items may have any height, and the scrollbar reflects the whole
  list. When a measurement changes an item above the viewport, the offset moves
  by the difference in the same frame, so the content on screen does not jump.
- `renderItem(i)` returns the item. It runs inside the list's build, so it must
  not call hooks: return a component node ([`tela.C`](https://pkg.go.dev/github.com/kuronosu/tela#C), [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP)) for items with
  state. Prefer `CP`, so a list rebuilt by its parent skips the items whose
  props did not change.
- Items are keyed by index: an item component's state follows its index. The
  focused item stays mounted while it is scrolled away, so Tab goes on from it.
- Scrolling inside the built items only repaints; scrolling past them builds
  the entering items and unmounts the leaving ones.
- [`Overscan`](https://pkg.go.dev/github.com/kuronosu/tela/h#Overscan) sets how many items are built beyond each side of the visible
  ones (5 by default). More items cost more layout and paint per frame; 0
  rebuilds the list on every scroll that shows a new item.
- Lists are vertical; horizontal lists and grids are not virtualized yet.

### Scrolling to an item

[`ElementRef.ScrollToIndex`](API-Reference-tela#func-elementref-scrolltoindex)(i, align) scrolls to item `i` and keeps it
aligned while the items around it are measured. The alignments are
[`tela.AlignAuto`](https://pkg.go.dev/github.com/kuronosu/tela#AlignAuto) (as little as possible), [`tela.AlignStart`](https://pkg.go.dev/github.com/kuronosu/tela#AlignStart), [`tela.AlignCenter`](https://pkg.go.dev/github.com/kuronosu/tela#AlignCenter)
and [`tela.AlignEnd`](https://pkg.go.dev/github.com/kuronosu/tela#AlignEnd) (TanStack Virtual's `align`). It jumps, or animates with
`scroll-smooth`.

```go
func Jump() tela.Node {
	list := tela.UseElementRef()
	return Div(Class("flex h-screen flex-col"),
		Button(OnClick(func(tela.MouseEvent) { list.ScrollToIndex(5000, tela.AlignCenter) }),
			Text("Go to 5000")),
		VirtualList(100_000,
			func(int) float32 { return 40 },
			func(i int) tela.Node { return Div(Class("h-10"), Textf("Row %d", i)) },
			Ref(list), Class("flex-1 rounded-lg border")),
	)
}
```

The `examples/virtuallist` screen shows 100,000 people with variable row heights
and `ScrollToIndex`. On the CPU rasterizer, scrolling it has a p95 of 2.3 ms per
frame.

## Testing scrolling

[`App.Scroll`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Scroll) scrolls the wheel over an element like a touchpad, and the
auto-wait scrolls an element into view before clicking it; see
[Testing](Testing).
