# Hooks

Hooks keep state and run code across builds, with the names and semantics of
React's. A **component** is a struct whose `Build` method returns a
[`tela.Node`](https://pkg.go.dev/github.com/kuronosu/tela#Node), or a function wrapped in `tela.C` or `tela.CP`; it may call hooks. The
sections below follow the order you usually need them.

## The rules

Hooks follow React's rules: call them at the top level of a component's
`Build` method or function, in the same order on every build, never inside
conditions or loops. A hook called outside a component build panics with an
explanation (a component called where a node is expected,
`telatest.Mount(t, Counter())`, is told to write `tela.C(Counter)`, and
`Card{}.Build()` to pass `Card{}` itself). In dev mode, on in every
development build and test (see [Getting Started](Getting-Started#dev-mode)), a change of the hook
order panics with both call sites, and so does a hook called from a function
that is neither the component being built (its `Build`, its function, or a
function literal inside them) nor a custom hook named `UseXxx` or `useXxx`.

Everything runs on the UI thread except the methods of [`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter),
[`tela.Dispatcher`](https://pkg.go.dev/github.com/kuronosu/tela#Dispatcher) and [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value), and [`tela.Post`](https://pkg.go.dev/github.com/kuronosu/tela#Post), which are safe from any
goroutine.

## Components

A **component** is a struct with a `Build` method (a value receiver) that
returns a node: a [`tela.Component`](https://pkg.go.dev/github.com/kuronosu/tela#Component). Its fields are its props, and its
children are an ordinary field of type `tela.Component` (several of them
through [`Fragment`](https://pkg.go.dev/github.com/kuronosu/tela/h#Fragment)):

```go
type Card struct {
	Title    string
	OnOpen   tela.Callback[func()]
	Children tela.Component
}

func (c Card) Build() tela.Node {
	open, setOpen := tela.UseState(false)
	return Div(Class("rounded-lg border p-4"),
		Button(OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text(c.Title)),
		If(open, c.Children))
}

func Screen() tela.Node {
	return Div(
		Card{Title: "Unread", Children: Fragment(Text("2 new"), Span(Text("from Ada")))},
		Card{Title: "Archive", Children: Text("empty")},
	)
}
```

- Every component value given to an element is a **component boundary**: an
  element with its own hook state that rebuilds on its own. Its identity
  among its siblings is its type and its key, so at one position a value of
  the same type keeps its state and a value of another type mounts anew.
  [`tela.WithKey`](https://pkg.go.dev/github.com/kuronosu/tela#WithKey)`(Card{...}, id)` keys it (and turns it into a `tela.Node`,
  for [`Map`](https://pkg.go.dev/github.com/kuronosu/tela/h#Map)).
- When the parent builds again with an **equal** value, and neither the
  component's own state nor a context it reads changed, its build is skipped
  (automatic skipping, as in Jetpack Compose). The comparison is `==`, field
  by field, and never panics: a field that cannot be compared (a slice, a
  map, a func, or an interface holding one) makes it build every time. A
  node in `Children` compares by identity, so a fresh `Div(...)` is a change;
  pass a [`tela.Callback`](https://pkg.go.dev/github.com/kuronosu/tela#Callback) rather than a func to keep it skippable.
- In dev mode, a pointer (`Div(&Card{})`) panics, and so does a prop in a
  field of type `tela.Component` (`Card{Children: Class("p-4")}`): Go cannot
  tell it from a component at compile time. A field meant to take props too
  is declared as [`tela.Child`](https://pkg.go.dev/github.com/kuronosu/tela#Child).
- [`If`](https://pkg.go.dev/github.com/kuronosu/tela/h#If) returns the zero value of its argument's type when the condition is
  false, and the zero value of a struct is a component that renders: write
  `If(loading, tela.Component(Spinner{}))` (dev mode panics on
  `If(loading, Spinner{})`).
- A function returning a node is a component when you wrap it:

```go
func Header() tela.Node { return H1(Text("Inbox")) }

type BadgeProps struct{ Count int }

func Badge(p BadgeProps) tela.Node {
	return Span(Class("rounded-full bg-indigo-500 px-2 text-white"), Textf("%d", p.Count))
}

func Toolbar() tela.Node {
	return Div(tela.C(Header), tela.CP(Badge, BadgeProps{Count: 3}))
}
```

- [`tela.C`](https://pkg.go.dev/github.com/kuronosu/tela#C) wraps a function without props. It builds whenever its parent builds.
- [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) wraps a function with **comparable** props. When the parent builds
  again with props `==` to the previous ones, and neither the component's own
  state nor a context it reads changed, the build is skipped (automatic
  skipping, as in Jetpack Compose). Pass a named function and put everything
  it uses in the props; dev mode reports anonymous functions.
- [`tela.Memo`](https://pkg.go.dev/github.com/kuronosu/tela#Memo) is [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) for props that are not comparable: you give the
  comparison function.
- `C`, `CP` and `Memo` take an optional last argument, the key `Key(id)`, for
  the items of a list: `tela.CP(Row, p, Key(p.ID))` is `tela.WithKey(tela.CP(Row, p), p.ID)`.
  Anything else but `Key` panics.
- Calling a component function directly, `Header()`, runs it inside the
  caller. That is fine for a function without hooks (a wrapper that styles
  its children and passes them on with [`Fragment`](https://pkg.go.dev/github.com/kuronosu/tela/h#Fragment)), but the hooks of
  a function called this way would take the caller's slots: dev mode panics
  and says to wrap it in `tela.C` or `tela.CP`. The same goes for
  `Card{}.Build()`: pass `Card{}` as a child instead (or `Fragment(Card{})`
  where a `tela.Node` is expected).
- A **custom hook** is a function named `UseXxx` or `useXxx` that calls hooks
  for the component that calls it. A struct of [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value) pointers is a good
  return value: stable, comparable, and each reader redoes only what it read.

```go
type Drag struct{ X, Y *tela.Value[float32] }

func UseDrag() Drag {
	return Drag{X: tela.UseValue(float32(0)), Y: tela.UseValue(float32(0))}
}
```

- [`tela.WithKey`](https://pkg.go.dev/github.com/kuronosu/tela#WithKey) keys a component or fragment, which cannot take the [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) prop.
  Both take a comparable key: a slice or a map does not compile.

## State: UseState

[`tela.UseState`](https://pkg.go.dev/github.com/kuronosu/tela#UseState) returns the current state and a [`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter). The first build
stores the initial value; later builds return the latest one.

```go
func Clicks() tela.Node {
	count, setCount := tela.UseState(0)

	return Button(
		OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
		Textf("Clicked %d times", count),
	)
}
```

`Set(v)` replaces the state. `Update(f)` computes it from the state left by the
previous queued update, which is what you want when several updates may be
queued before the next build (events in one frame, goroutines):

```go
func Counter() tela.Node {
	count, setCount := tela.UseState(0)
	add := func(tela.MouseEvent) {
		setCount.Update(func(n int) int { return n + 1 })
	}
	return Button(OnClick(add), Textf("%d", count))
}
```

For comparable types, a new state `==` to the current one does not rebuild. A
[`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter) is a small comparable value, the same on every build, so it can be
stored, passed to goroutines, and put in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component.

[`tela.UseStateSet`](https://pkg.go.dev/github.com/kuronosu/tela#UseStateSet) and [`tela.UseStateUpdate`](https://pkg.go.dev/github.com/kuronosu/tela#UseStateUpdate) return the setter's `Set` and
`Update` methods as plain functions when you only need one of them. They
exist for React's shape, a setter that is a function: to pass it where a
`func(T)` is expected without wrapping it, and to keep code ported from React
reading the same. Funcs are not comparable in Go, so in the props of a
`tela.CP` component pass the `Setter` instead:

```go
func NameField() tela.Node {
	name, setName := tela.UseStateSet("")
	return Input(Value(name), OnChange(func(e tela.ChangeEvent) { setName(e.Value) }))
}
```

[`tela.Toggle`](https://pkg.go.dev/github.com/kuronosu/tela#Toggle)`(setOpen)` returns a `func()` that flips a `bool` state with
`Update`: `OnClick(tela.Toggle(setOpen))`. Two clicks that land in one frame
both count, which `setOpen.Set(!open)` loses (both read the same `open`).

## State machines: UseReducer

[`tela.UseReducer`](https://pkg.go.dev/github.com/kuronosu/tela#UseReducer) takes a pure reducer and returns the state with a
[`tela.Dispatcher`](https://pkg.go.dev/github.com/kuronosu/tela#Dispatcher). Queued actions apply in order on the UI thread.

```go
type action struct{ kind string }

func reduce(n int, a action) int {
	switch a.kind {
	case "inc":
		return n + 1
	case "reset":
		return 0
	}
	return n
}

func Stepper() tela.Node {
	count, dispatch := tela.UseReducer(reduce, 0)
	return Div(
		Textf("%d", count),
		Button(OnClick(func(tela.MouseEvent) { dispatch.Dispatch(action{"inc"}) }), Text("+")),
		Button(OnClick(func(tela.MouseEvent) { dispatch.Dispatch(action{"reset"}) }), Text("Reset")),
	)
}
```

## Effects: UseEffect, UseMount and UseLayoutEffect

[`tela.UseEffect`](https://pkg.go.dev/github.com/kuronosu/tela#UseEffect) runs a function after the frame is presented, when its
dependencies changed since the previous build. The function may return a
cleanup, which runs before the effect runs again and when the component
unmounts. The dependencies follow React:

```go
tela.UseEffect(f)            // after every build
tela.UseEffect(f, id, n)     // when id or n changed (compared with ==)
tela.UseEffect(f, tela.Once) // only after the first build (React's [])
tela.UseMount(f)             // the same
```

[`tela.UseMount`](https://pkg.go.dev/github.com/kuronosu/tela#UseMount) is the idiom for subscriptions: the effect runs on mount and its
cleanup on unmount. [`tela.Once`](https://pkg.go.dev/github.com/kuronosu/tela#Once) is the "mount only" dependency of every hook
that takes dependencies (`UseLayoutEffect`, `UseGo`, `UseEffectDo`,
`UseMemo`). `UseEffect(f, nil)` still means the same, but it is deprecated
(React reads a null list as "after every render"): dev mode warns and
suggests `tela.Once`, and a later version will reject it.

```go
func Status(store *Store) tela.Node {
	online, setOnline := tela.UseState(store.Online())

	tela.UseMount(func() func() {
		return store.Subscribe(func(on bool) { setOnline.Set(on) }) // returns the unsubscribe
	})

	label := "Offline"
	if online {
		label = "Online"
	}
	return Span(Text(label))
}

type Store struct{}

func (*Store) Online() bool                      { return true }
func (*Store) Subscribe(func(bool)) (stop func()) { return func() {} }
```

Two variants save the boilerplate of common effects:
[`tela.UseEffectDo`](https://pkg.go.dev/github.com/kuronosu/tela#UseEffectDo) takes an effect without a cleanup, and
[`tela.UseUpdateEffect`](https://pkg.go.dev/github.com/kuronosu/tela#UseUpdateEffect) skips the first build, to react to changes only:

```go
func ThemeSaver(theme string, save func(string)) tela.Node {
	tela.UseEffectDo(func() { fmt.Println("the theme is", theme) }, theme)
	tela.UseUpdateEffect(func() func() {
		save(theme) // not on mount: only when the theme changes
		return nil
	}, theme)
	return Text(theme)
}
```

Dependencies that cannot be compared (functions, slices, maps) count as
changed on every build, and dev mode says so: pass an ID or a comparable value
instead. Within a frame, the effects of children run before their parents', and
effects of a component whose build was skipped do not run.

[`tela.UseLayoutEffect`](https://pkg.go.dev/github.com/kuronosu/tela#UseLayoutEffect) has the same shape but runs after layout and before
paint, for code that needs the geometry.

## Periodic and background work: UseInterval and UseGo

[`tela.UseInterval`](https://pkg.go.dev/github.com/kuronosu/tela#UseInterval) calls a function on the UI thread every period, until the
component unmounts; a period of 0 or less pauses it, and a new period restarts
it. The function is always the latest build's, so it sees the latest state.
It runs on the window's frame clock, with no goroutine, so `telatest`'s
`App.Advance` moves it:

```go
func Clock() tela.Node {
	now, setNow := tela.UseState(time.Now())
	tela.UseInterval(time.Second, func() { setNow.Set(time.Now()) })
	return Span(Class("tabular-nums"), Text(now.Format("15:04:05")))
}
```

[`tela.UseGo`](https://pkg.go.dev/github.com/kuronosu/tela#UseGo) runs a function on its own goroutine after the frame, like an effect
with dependencies, and cancels its `context.Context` when the dependencies
change or the component unmounts; the next run starts after that. The function
must return soon after the context is done, and change the UI only through
what is safe from any goroutine (a `Setter`, a `Value`, `tela.Post`):

```go
func Results(query string, search func(context.Context, string) []string) tela.Node {
	found, setFound := tela.UseState([]string(nil))
	tela.UseGo(func(ctx context.Context) {
		r := search(ctx, query)
		if ctx.Err() == nil { // a newer query did not cancel this one
			setFound.Set(r)
		}
	}, query)
	return Div(Map(found, func(s string) tela.Node { return P(Key(s), Text(s)) }))
}
```

With no dependencies, `UseGo` restarts its goroutine after every build, as
`UseEffect` runs after every build.

## Memoization: UseMemo, UseOnce, UseCallback, UseRef

[`tela.UseMemo`](https://pkg.go.dev/github.com/kuronosu/tela#UseMemo) caches a computation until its dependencies change, and
[`tela.UseOnce`](https://pkg.go.dev/github.com/kuronosu/tela#UseOnce) computes a value on the first build and keeps it (a client, a
cache, the state of a custom hook):

```go
type Todo struct {
	ID   int
	Text string
}

func Filtered(todos []Todo, query string) tela.Node {
	visible := tela.UseMemo(func() []Todo {
		var out []Todo
		for _, t := range todos {
			if strings.Contains(strings.ToLower(t.Text), strings.ToLower(query)) {
				out = append(out, t)
			}
		}
		return out
	}, len(todos), query)

	return Map(visible, func(t Todo) tela.Node { return Div(Key(t.ID), Text(t.Text)) })
}
```

[`tela.UseCallback`](https://pkg.go.dev/github.com/kuronosu/tela#UseCallback) returns a [`tela.Callback`](https://pkg.go.dev/github.com/kuronosu/tela#Callback): one stable function, the same on
every build, that calls the latest function you gave it. Because a Callback is
comparable, it can travel in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component without
defeating its skipping. Call `cb.Fn()` to get the function:

```go
type ToolbarProps struct {
	OnSave tela.Callback[func(tela.MouseEvent)]
}

func Toolbar(p ToolbarProps) tela.Node {
	return Button(OnClick(p.OnSave.Fn()), Text("Save"))
}

func Editor() tela.Node {
	draft, setDraft := tela.UseState("")
	onSave := tela.UseCallback(func(tela.MouseEvent) { save(draft) })
	return Div(
		Input(Bind(draft, setDraft)),
		tela.CP(Toolbar, ToolbarProps{OnSave: onSave}), // does not rebuild while typing
	)
}

func save(string) {}
```

[`tela.UseRef`](https://pkg.go.dev/github.com/kuronosu/tela#UseRef) returns a [`tela.Ref`](https://pkg.go.dev/github.com/kuronosu/tela#Ref), a box whose `Current` field persists across
builds. Changing it does not rebuild: use it for values that rendering does not
show, such as a timer or the previous value of a prop.

```go
func RenderCount() tela.Node {
	renders := tela.UseRef(0)
	renders.Current++
	return Textf("built %d times", renders.Current)
}
```

## Context

[`tela.CreateContext`](https://pkg.go.dev/github.com/kuronosu/tela#CreateContext) makes a context with a default value; create contexts once,
at package level. `Provider(value, children...)` gives the value to a subtree
and [`tela.UseContext`](https://pkg.go.dev/github.com/kuronosu/tela#UseContext) reads the nearest one. When the provided value changes,
only the components that read it rebuild, even if the components between skip
their build.

```go
var Theme = tela.CreateContext("light")

func App() tela.Node {
	theme, setTheme := tela.UseState("dark")
	return Theme.Provider(theme,
		tela.C(Page),
		Button(OnClick(func(tela.MouseEvent) { setTheme.Set("light") }), Text("Light")),
	)
}

func Page() tela.Node {
	theme := tela.UseContext(Theme)
	return P(Textf("The theme is %s", theme))
}
```

A component that reads a context with no Provider above it gets the default.
When the default would panic if used (it is nil, or holds a nil function,
pointer, map or channel, or a zero `Setter` or `Callback`), dev mode warns
once per context. Provide the context above every reader, give
`CreateContext` a default that works, or check with `IsZero`, which
[`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter) and [`tela.Callback`](https://pkg.go.dev/github.com/kuronosu/tela#Callback) have for this (and for optional callbacks in
props).

## Identity: UseID

[`tela.UseID`](https://pkg.go.dev/github.com/kuronosu/tela#UseID) returns a string that is unique in the process and the same on every
build (and the same for the same mount order, so snapshots are deterministic).
Use it to relate elements, such as a label and its field:

```go
func Field(label string) tela.Node {
	id := tela.UseID()
	return Div(
		Label(For(id), Text(label)),
		Input(ID(id)),
	)
}
```

## Handles on elements: UseElementRef

[`tela.UseElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#UseElementRef) returns an [`tela.ElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#ElementRef). Attach it with [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) and act
on the element from handlers and effects: focus it, scroll it, read its bounds,
read or select the text of a field.

```go
func Search() tela.Node {
	input := tela.UseElementRef()

	tela.UseMount(func() func() {
		input.Focus()
		return nil
	})

	return Input(Ref(input), Type("search"), Placeholder("Search"),
		Class("rounded-md border px-2 py-1"))
}
```

Its methods are: [`ElementRef.Focus`](API-Reference-tela#func-elementref-focus), [`ElementRef.Blur`](API-Reference-tela#func-elementref-blur),
[`ElementRef.Focused`](API-Reference-tela#func-elementref-focused), [`ElementRef.Bounds`](API-Reference-tela#func-elementref-bounds), the scroll methods
[`ElementRef.ScrollTo`](API-Reference-tela#func-elementref-scrollto), [`ElementRef.ScrollBy`](API-Reference-tela#func-elementref-scrollby),
[`ElementRef.ScrollOffset`](API-Reference-tela#func-elementref-scrolloffset), [`ElementRef.ScrollIntoView`](API-Reference-tela#func-elementref-scrollintoview) and
[`ElementRef.ScrollToIndex`](API-Reference-tela#func-elementref-scrolltoindex), and, for text fields, [`ElementRef.Value`](API-Reference-tela#func-elementref-value),
[`ElementRef.Select`](API-Reference-tela#func-elementref-select), [`ElementRef.SetSelectionRange`](API-Reference-tela#func-elementref-setselectionrange) and
[`ElementRef.SelectionRange`](API-Reference-tela#func-elementref-selectionrange). The scroll methods are explained in
[Scrolling and VirtualList](Scrolling-and-VirtualList) and the text ones in
[Forms and Text Input](Forms-and-Text-Input).

## Observable values: UseValue

Some state changes every frame: a drag offset, a scroll position, download
progress. Rebuilding a component 60 times a second for that is wasteful.
[`tela.UseValue`](https://pkg.go.dev/github.com/kuronosu/tela#UseValue) returns a [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value), an observable cell, and **where you read
it decides what a change redoes**:

| `v.Get()` called in... | A change of `v` redoes... |
|---|---|
| the component's body | the component's build, like `UseState` |
| a layout closure ([`tela.WidthFn`](https://pkg.go.dev/github.com/kuronosu/tela#WidthFn), [`tela.HeightFn`](https://pkg.go.dev/github.com/kuronosu/tela#HeightFn)) | that element's layout only |
| a paint closure ([`tela.TranslateFn`](https://pkg.go.dev/github.com/kuronosu/tela#TranslateFn), [`tela.OpacityFn`](https://pkg.go.dev/github.com/kuronosu/tela#OpacityFn), [`tela.BgFn`](https://pkg.go.dev/github.com/kuronosu/tela#BgFn), [`tela.TextColorFn`](https://pkg.go.dev/github.com/kuronosu/tela#TextColorFn)) | that element's paint only |
| an event handler, effect or goroutine | nothing; `Get` just returns the value |

[`tela.MapValue`](https://pkg.go.dev/github.com/kuronosu/tela#MapValue)`(v, f)` returns `func() U` that applies `f` to the value
when the closure runs, for the deferred props, and `h.Scale(v, 36)` is its
float32 shorthand: `Style(tela.HeightFn(Scale(open, 36)))`.

A progress bar fed by a goroutine, which lays out the bar again without
building `Download`:

```go
func Download(url string) tela.Node {
	progress := tela.UseValue(0.0) // from 0 to 1

	tela.UseGo(func(ctx context.Context) {
		fetch(ctx, func(done float64) { progress.Set(done) }) // any goroutine
	}, url)

	return Div(Class("h-2 w-64 overflow-hidden rounded-full bg-slate-200"),
		Div(Class("h-full bg-indigo-500"),
			Style(tela.WidthFn(func() float32 { return float32(256 * progress.Get()) }))))
}

func fetch(ctx context.Context, report func(float64)) {
	for i := 0; i <= 10 && ctx.Err() == nil; i++ {
		report(float64(i) / 10)
		time.Sleep(100 * time.Millisecond)
	}
}
```

`Set` and `Update` are safe from any goroutine and queue the change, in order
with state updates. `Get` returns the value the UI thread applied last; use
[`Value.Latest`](API-Reference-tela#func-value-latest) in a handler that has just called `Set` and wants the new
value. Dev mode warns, once per call site, about a `Get` outside a build or a
deferred closure that misses queued changes. A [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value) is comparable (the same pointer on every build), so it can
go in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component or through a context.

Text can be deferred the same way with [`TextFn`](https://pkg.go.dev/github.com/kuronosu/tela/h#TextFn), and a field can be bound to a
`Value[string]` with [`BindValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#BindValue).

### Scroll offsets and bounds

[`tela.UseScroll`](https://pkg.go.dev/github.com/kuronosu/tela#UseScroll) gives two Values that follow the scroll offsets of an element,
and [`tela.UseBounds`](https://pkg.go.dev/github.com/kuronosu/tela#UseBounds) a Value of its [`tela.Rect`](https://pkg.go.dev/github.com/kuronosu/tela#Rect) in the window (a ResizeObserver
that also sees position):

```go
func FadingHeader(rows tela.Node) tela.Node {
	list := tela.UseElementRef()
	_, y := tela.UseScroll(list)

	header := Div(Class("h-16 bg-white shadow"),
		Style(tela.OpacityFn(func() float32 {
			return 1 - min(y.Get()/200, 1) // repaints only
		})))
	return Div(Class("flex h-screen flex-col"), header,
		Div(Ref(list), Class("grow overflow-y-auto"), rows))
}
```

## Animation: UseSpring, UseTween, Animate and UsePresence

Animations are Values that move on the frame clock, so they cost what a
`UseValue` read costs: a `TranslateFn` read only repaints, a `WidthFn` read
only lays out. [`tela.UseSpring`](https://pkg.go.dev/github.com/kuronosu/tela#UseSpring) and
[`tela.UseTween`](https://pkg.go.dev/github.com/kuronosu/tela#UseTween) follow a target that can change on every
build (a new target restarts the motion from where it is; a spring keeps its
velocity), and [`tela.Animate`](https://pkg.go.dev/github.com/kuronosu/tela#Animate) moves any `Value` from a handler,
an effect or a goroutine. They start at their target unless the `Initial` option
gives a start value (`tela.UseSpring(target, tela.Initial(float32(0)))`).
Options such as `Stiffness`, `Duration`, `Ease`,
`Velocity` and `OnDone` tune them, [`tela.UsePresence`](https://pkg.go.dev/github.com/kuronosu/tela#UsePresence)
lets a component run its own exit inside an `h.Presence`, and
[`tela.UsePresenceTween`](https://pkg.go.dev/github.com/kuronosu/tela#UsePresenceTween) is the whole enter-and-exit
tween as one `Value` from 0 to 1 and back:

```go
func Drawer(open bool) tela.Node {
	target := float32(-320) // off screen to the left
	if open {
		target = 0
	}
	x := tela.UseSpring(target) // starts at the target, then follows open
	return Div(Class("fixed inset-y-0 left-0 w-80 bg-white shadow-xl"),
		Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })), // repaints only
		Text("Menu"))
}
```

The whole picture, with transitions, enter and exit and layout animations, is
in [Animation](Animation).

## Environment: UseColorScheme and UseReducedMotion

[`tela.UseColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#UseColorScheme) returns the effective color scheme, `tela.Light` or
`tela.Dark` (the app's choice or the system's preference), and
[`tela.UseReducedMotion`](https://pkg.go.dev/github.com/kuronosu/tela#UseReducedMotion) whether the system asks for reduced motion. The component
builds again when they change; nothing else does. Classes do not need them
(`dark:` and `motion-reduce:` follow the same settings); use them for what
classes cannot do, such as picking an image.
[`tela.SetColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#SetColorScheme) changes the app's scheme from any goroutine; see
[Styling](Styling#dark-mode).

## Background work: Post

[`tela.Post`](https://pkg.go.dev/github.com/kuronosu/tela#Post) runs a function on the UI thread, in order with state updates. It is
the way to touch UI-thread state, such as a [`tela.Ref`](https://pkg.go.dev/github.com/kuronosu/tela#Ref), from a goroutine:

```go
func load(cache *tela.Ref[[]byte]) {
	go func() {
		data := []byte("downloaded")
		tela.Post(func() { cache.Current = data })
	}()
}
```

Functions posted before [`tela.Run`](https://pkg.go.dev/github.com/kuronosu/tela#Run) starts run once the window is up. In tests,
[`telatest.Mount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Mount) delivers them.

## See also

- The API reference of every hook: [API-Reference-tela](API-Reference-tela#functions).
- [Testing](Testing) shows how to assert on state without a window.
